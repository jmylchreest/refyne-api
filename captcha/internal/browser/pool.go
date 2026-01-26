// Package browser provides browser pool management for the captcha service.
package browser

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/oklog/ulid/v2"

	"github.com/jmylchreest/refyne-api/captcha/internal/config"
)

var (
	// ErrPoolExhausted is returned when all browsers are in use and pool is at max capacity.
	ErrPoolExhausted = errors.New("browser pool exhausted")
	// ErrPoolClosed is returned when trying to use a closed pool.
	ErrPoolClosed = errors.New("browser pool is closed")
	// ErrBrowserUnhealthy is returned when a browser fails health check.
	ErrBrowserUnhealthy = errors.New("browser is unhealthy")
)

// ManagedBrowser wraps a rod.Browser with management metadata.
type ManagedBrowser struct {
	ID           string
	Browser      *rod.Browser
	InUse        bool
	CreatedAt    time.Time
	LastUsedAt   time.Time
	RequestCount int
	Proxy        string
	UserAgent    string
}

// Pool manages a pool of browser instances.
type Pool struct {
	mu       sync.RWMutex
	browsers map[string]*ManagedBrowser
	waiting  []chan *ManagedBrowser
	cfg      *config.Config
	logger   *slog.Logger
	closed   bool

	// Launcher configuration
	chromePath string
	headless   bool

	// Ready state for async warmup
	ready     bool
	readyChan chan struct{}
}

// NewPool creates a new browser pool.
func NewPool(cfg *config.Config, logger *slog.Logger) *Pool {
	return &Pool{
		browsers:   make(map[string]*ManagedBrowser),
		waiting:    make([]chan *ManagedBrowser, 0),
		cfg:        cfg,
		logger:     logger,
		chromePath: cfg.ChromePath,
		headless:   true,
		ready:      false,
		readyChan:  make(chan struct{}),
	}
}

// Ready returns true if the pool has completed warmup.
func (p *Pool) Ready() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.ready
}

// WaitReady blocks until the pool is ready or context is cancelled.
func (p *Pool) WaitReady(ctx context.Context) error {
	select {
	case <-p.readyChan:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Warmup ensures Chromium is downloaded and optionally pre-creates browsers.
// This should be called during startup to avoid download delays on first request.
func (p *Pool) Warmup(ctx context.Context, preCreate int) error {
	p.logger.Info("warming up browser pool...")

	// If custom Chrome path is set, verify it exists
	if p.chromePath != "" {
		p.logger.Info("using custom Chrome path", "path", p.chromePath)
	} else {
		// Ensure Chromium is downloaded (rod auto-downloads if missing)
		p.logger.Info("ensuring Chromium is available...")
		browserPath, err := launcher.NewBrowser().Get()
		if err != nil {
			return err
		}
		p.logger.Info("Chromium ready", "path", browserPath)
	}

	// Pre-create browsers if requested
	if preCreate > 0 {
		if preCreate > p.cfg.BrowserPoolSize {
			preCreate = p.cfg.BrowserPoolSize
		}
		p.logger.Info("pre-creating browsers", "count", preCreate)

		for i := 0; i < preCreate; i++ {
			browser, err := p.createBrowser(ctx)
			if err != nil {
				p.logger.Error("failed to pre-create browser", "error", err)
				return err
			}
			browser.InUse = false
			p.mu.Lock()
			p.browsers[browser.ID] = browser
			p.mu.Unlock()
		}
		p.logger.Info("browser pool warmed up", "browsers", preCreate)
	}

	// Mark pool as ready
	p.mu.Lock()
	p.ready = true
	close(p.readyChan) // Signal all waiters
	p.mu.Unlock()

	return nil
}

// Acquire gets a browser from the pool, creating one if necessary.
// Blocks if pool is full and all browsers are in use.
func (p *Pool) Acquire(ctx context.Context) (*ManagedBrowser, error) {
	p.mu.Lock()

	if p.closed {
		p.mu.Unlock()
		return nil, ErrPoolClosed
	}

	// Try to find an available browser
	for _, b := range p.browsers {
		if !b.InUse && p.isHealthy(b) {
			b.InUse = true
			b.LastUsedAt = time.Now()
			p.mu.Unlock()
			return b, nil
		}
	}

	// Create new browser if pool has capacity
	if len(p.browsers) < p.cfg.BrowserPoolSize {
		browser, err := p.createBrowser(ctx)
		if err != nil {
			p.mu.Unlock()
			return nil, err
		}
		p.browsers[browser.ID] = browser
		p.mu.Unlock()
		return browser, nil
	}

	// Pool is full - wait for a browser to become available
	waitChan := make(chan *ManagedBrowser, 1)
	p.waiting = append(p.waiting, waitChan)
	p.mu.Unlock()

	select {
	case browser := <-waitChan:
		return browser, nil
	case <-ctx.Done():
		// Remove from waiting list
		p.mu.Lock()
		for i, ch := range p.waiting {
			if ch == waitChan {
				p.waiting = append(p.waiting[:i], p.waiting[i+1:]...)
				break
			}
		}
		p.mu.Unlock()
		return nil, ctx.Err()
	}
}

// Release returns a browser to the pool.
func (p *Pool) Release(browser *ManagedBrowser) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		p.closeBrowser(browser)
		return
	}

	browser.InUse = false
	browser.RequestCount++
	browser.LastUsedAt = time.Now()

	// Check if browser needs to be recycled
	if p.needsRecycle(browser) {
		p.recycleBrowser(browser)
		return
	}

	// Notify waiting goroutines
	if len(p.waiting) > 0 {
		waitChan := p.waiting[0]
		p.waiting = p.waiting[1:]
		browser.InUse = true
		browser.LastUsedAt = time.Now()
		waitChan <- browser
	}
}

// Close shuts down all browsers and closes the pool.
func (p *Pool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return
	}
	p.closed = true

	// Close all browsers
	for _, browser := range p.browsers {
		p.closeBrowser(browser)
	}
	p.browsers = make(map[string]*ManagedBrowser)

	// Cancel all waiting
	for _, ch := range p.waiting {
		close(ch)
	}
	p.waiting = nil
}

// Stats returns current pool statistics.
func (p *Pool) Stats() PoolStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	stats := PoolStats{
		Total:   len(p.browsers),
		MaxSize: p.cfg.BrowserPoolSize,
		Waiting: len(p.waiting),
		Ready:   p.ready,
	}

	for _, b := range p.browsers {
		if b.InUse {
			stats.InUse++
		} else {
			stats.Available++
		}
	}

	return stats
}

// PoolStats contains pool statistics.
type PoolStats struct {
	Total     int  `json:"total"`
	InUse     int  `json:"inUse"`
	Available int  `json:"available"`
	MaxSize   int  `json:"maxSize"`
	Waiting   int  `json:"waiting"`
	Ready     bool `json:"ready"`
}

// createBrowser creates a new browser instance.
func (p *Pool) createBrowser(ctx context.Context) (*ManagedBrowser, error) {
	// Configure launcher
	l := launcher.New()

	// Use custom Chrome path if specified
	if p.chromePath != "" {
		l = l.Bin(p.chromePath)
	}

	// Configure for stealth and resource efficiency
	l = l.
		Headless(p.headless).
		Set("disable-blink-features", "AutomationControlled").
		Set("disable-dev-shm-usage").
		Set("disable-gpu").
		Set("no-sandbox").
		Set("disable-setuid-sandbox").
		Set("disable-infobars").
		Set("disable-extensions").
		Set("disable-plugins-discovery").
		Set("disable-background-networking").
		Set("disable-background-timer-throttling").
		Set("disable-backgrounding-occluded-windows").
		Set("disable-renderer-backgrounding").
		Set("window-size", "1920,1080").
		Set("lang", "en-US,en")

	// Launch browser
	u, err := l.Launch()
	if err != nil {
		return nil, err
	}

	browser := rod.New().ControlURL(u)
	if err := browser.Connect(); err != nil {
		return nil, err
	}

	id := ulid.Make().String()
	p.logger.Info("browser created", "id", id)

	return &ManagedBrowser{
		ID:           id,
		Browser:      browser,
		InUse:        true,
		CreatedAt:    time.Now(),
		LastUsedAt:   time.Now(),
		RequestCount: 0,
	}, nil
}

// isHealthy checks if a browser is still healthy.
func (p *Pool) isHealthy(b *ManagedBrowser) bool {
	// Check if browser is too old
	if time.Since(b.CreatedAt) > p.cfg.BrowserMaxAge {
		return false
	}

	// Check if browser has handled too many requests
	if b.RequestCount >= p.cfg.BrowserMaxRequests {
		return false
	}

	// Check if browser is idle too long
	if !b.InUse && time.Since(b.LastUsedAt) > p.cfg.BrowserIdleTimeout {
		return false
	}

	// Try to ping the browser - simple health check
	// Use MustPages with panic recovery
	defer func() {
		recover()
	}()
	_, err := b.Browser.Pages()
	return err == nil
}

// needsRecycle checks if a browser should be recycled.
func (p *Pool) needsRecycle(b *ManagedBrowser) bool {
	if time.Since(b.CreatedAt) > p.cfg.BrowserMaxAge {
		return true
	}
	if b.RequestCount >= p.cfg.BrowserMaxRequests {
		return true
	}
	return false
}

// recycleBrowser closes an old browser and creates a new one.
func (p *Pool) recycleBrowser(b *ManagedBrowser) {
	p.logger.Info("recycling browser", "id", b.ID, "age", time.Since(b.CreatedAt), "requests", b.RequestCount)

	// Close old browser
	p.closeBrowser(b)
	delete(p.browsers, b.ID)

	// Create new browser (in background to not block)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		newBrowser, err := p.createBrowser(ctx)
		if err != nil {
			p.logger.Error("failed to create replacement browser", "error", err)
			return
		}

		p.mu.Lock()
		defer p.mu.Unlock()

		if p.closed {
			p.closeBrowser(newBrowser)
			return
		}

		newBrowser.InUse = false
		p.browsers[newBrowser.ID] = newBrowser

		// Notify waiting goroutines
		if len(p.waiting) > 0 {
			waitChan := p.waiting[0]
			p.waiting = p.waiting[1:]
			newBrowser.InUse = true
			newBrowser.LastUsedAt = time.Now()
			waitChan <- newBrowser
		}
	}()
}

// closeBrowser safely closes a browser.
func (p *Pool) closeBrowser(b *ManagedBrowser) {
	if b.Browser != nil {
		if err := b.Browser.Close(); err != nil {
			p.logger.Warn("error closing browser", "id", b.ID, "error", err)
		}
	}
	p.logger.Info("browser closed", "id", b.ID)
}

// StartCleanup starts a background goroutine that periodically cleans up idle browsers.
func (p *Pool) StartCleanup(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.cleanupIdleBrowsers()
		}
	}
}

// cleanupIdleBrowsers removes browsers that have been idle too long.
func (p *Pool) cleanupIdleBrowsers() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return
	}

	var toRemove []string
	for id, b := range p.browsers {
		if !b.InUse && time.Since(b.LastUsedAt) > p.cfg.BrowserIdleTimeout {
			toRemove = append(toRemove, id)
		}
	}

	for _, id := range toRemove {
		b := p.browsers[id]
		p.logger.Info("cleaning up idle browser", "id", id, "idle_time", time.Since(b.LastUsedAt))
		p.closeBrowser(b)
		delete(p.browsers, id)
	}
}
