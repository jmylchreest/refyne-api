// Package session provides session management for persistent browser instances.
package session

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/oklog/ulid/v2"

	"github.com/jmylchreest/refyne-api/captcha/internal/browser"
	"github.com/jmylchreest/refyne-api/captcha/internal/config"
	"github.com/jmylchreest/refyne-api/captcha/internal/models"
)

var (
	// ErrSessionNotFound is returned when a session doesn't exist.
	ErrSessionNotFound = errors.New("session not found")
	// ErrSessionInUse is returned when a session is currently in use.
	ErrSessionInUse = errors.New("session is currently in use")
	// ErrMaxSessionsReached is returned when the maximum number of sessions is reached.
	ErrMaxSessionsReached = errors.New("maximum sessions reached")
)

// Session represents a persistent browser session.
type Session struct {
	ID           string
	Browser      *rod.Browser
	Page         *rod.Page
	UserAgent    string
	Proxy        string
	CreatedAt    time.Time
	LastUsedAt   time.Time
	RequestCount int
	InUse        bool
	Cookies      []*proto.NetworkCookie
}

// Manager manages browser sessions.
type Manager struct {
	mu          sync.RWMutex
	sessions    map[string]*Session
	waiters     map[string][]chan struct{} // Waiters for busy sessions
	cfg         *config.Config
	logger      *slog.Logger
	maxSessions int
	closed      bool
	store       *SQLiteStore // Optional persistence layer
}

// NewManager creates a new session manager.
// If cfg.SessionDBPath is set, sessions will be persisted to SQLite.
func NewManager(cfg *config.Config, logger *slog.Logger) *Manager {
	m := &Manager{
		sessions:    make(map[string]*Session),
		cfg:         cfg,
		logger:      logger,
		maxSessions: cfg.BrowserPoolSize * 2, // Allow 2x pool size for sessions
	}

	// Initialize SQLite store if path is configured
	if cfg.SessionDBPath != "" {
		store, err := NewSQLiteStore(cfg.SessionDBPath, logger)
		if err != nil {
			logger.Error("failed to initialize session store, sessions will not persist", "error", err)
		} else {
			m.store = store
			logger.Info("session persistence enabled", "path", cfg.SessionDBPath)
		}
	}

	return m
}

// Create creates a new browser session.
// If sessionID is provided, it will be used as the session identifier.
// If sessionID is empty, a ULID will be generated.
func (m *Manager) Create(ctx context.Context, sessionID string, opts *models.SessionOptions) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil, errors.New("manager is closed")
	}

	if len(m.sessions) >= m.maxSessions {
		return nil, ErrMaxSessionsReached
	}

	// Check if session ID already exists
	if sessionID != "" {
		if _, exists := m.sessions[sessionID]; exists {
			return nil, errors.New("session already exists: " + sessionID)
		}
	}

	// Configure launcher
	l := launcher.New()

	if m.cfg.ChromePath != "" {
		l = l.Bin(m.cfg.ChromePath)
	}

	// Determine headless mode
	headless := true
	if opts != nil && opts.Headless != nil {
		headless = *opts.Headless
	}

	// Configure browser
	l = l.
		Headless(headless).
		Set("disable-blink-features", "AutomationControlled").
		Set("disable-dev-shm-usage").
		Set("disable-gpu").
		Set("no-sandbox").
		Set("disable-setuid-sandbox").
		Set("disable-infobars").
		Set("lang", "en-US,en")

	// Set window size
	width := 1920
	height := 1080
	if opts != nil {
		if opts.WindowWidth > 0 {
			width = opts.WindowWidth
		}
		if opts.WindowHeight > 0 {
			height = opts.WindowHeight
		}
	}
	l = l.Set("window-size", fmt.Sprintf("%d,%d", width, height))

	// Configure proxy if specified
	var proxyURL string
	if opts != nil && opts.Proxy != nil {
		proxyURL = opts.Proxy.URL
		l = l.Proxy(proxyURL)
	} else if m.cfg.ProxyEnabled && m.cfg.ProxyURL != "" {
		proxyURL = m.cfg.ProxyURL
		l = l.Proxy(proxyURL)
	}

	// Launch browser
	u, err := l.Launch()
	if err != nil {
		return nil, err
	}

	b := rod.New().ControlURL(u)
	if err := b.Connect(); err != nil {
		return nil, err
	}

	// Create page (stealth mode unless disabled for testing)
	m.logger.Debug("creating session page", "stealth", !m.cfg.DisableStealth)
	page, err := browser.CreatePage(b, m.cfg.DisableStealth)
	if err != nil {
		b.Close()
		return nil, err
	}

	// Set user agent if specified
	userAgent := ""
	if opts != nil && opts.UserAgent != "" {
		userAgent = opts.UserAgent
		if err := page.SetUserAgent(&proto.NetworkSetUserAgentOverride{
			UserAgent: userAgent,
		}); err != nil {
			b.Close()
			return nil, err
		}
	}

	// Use provided session ID or generate one
	id := sessionID
	if id == "" {
		id = ulid.Make().String()
	}
	session := &Session{
		ID:           id,
		Browser:      b,
		Page:         page,
		UserAgent:    userAgent,
		Proxy:        proxyURL,
		CreatedAt:    time.Now(),
		LastUsedAt:   time.Now(),
		RequestCount: 0,
		InUse:        false,
	}

	m.sessions[id] = session
	m.logger.Info("session created", "id", id, "proxy", proxyURL != "")

	// Persist to store if available
	if m.store != nil {
		userID := ""
		if opts != nil {
			userID = opts.UserID
		}
		if err := m.store.Save(&PersistedSession{
			ID:           session.ID,
			UserID:       userID,
			UserAgent:    session.UserAgent,
			Proxy:        session.Proxy,
			CreatedAt:    session.CreatedAt,
			LastUsedAt:   session.LastUsedAt,
			RequestCount: session.RequestCount,
			Cookies:      session.Cookies,
		}); err != nil {
			m.logger.Error("failed to persist session", "id", id, "error", err)
		}
	}

	return session, nil
}

// Get retrieves a session by ID.
func (m *Manager) Get(id string) (*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, ok := m.sessions[id]
	if !ok {
		return nil, ErrSessionNotFound
	}

	return session, nil
}

// Acquire acquires a session for use, waiting if the session is busy.
// This blocks until the session is available or context is cancelled.
// If the session is not in memory but exists in the persistence store,
// it will be lazily restored with saved cookies.
func (m *Manager) Acquire(ctx context.Context, id string) (*Session, error) {
	m.logger.Debug("session acquire attempt",
		"id", id,
		"total_sessions", len(m.sessions),
	)

	// Try to acquire immediately first
	session, waitChan, err := m.tryAcquire(id)
	if err != nil {
		// Session not found in memory - try to restore from store
		if err == ErrSessionNotFound && m.store != nil {
			m.logger.Debug("session not in memory, checking store", "id", id)
			restored, restoreErr := m.restoreFromStore(ctx, id)
			if restoreErr != nil {
				return nil, restoreErr
			}
			if restored != nil {
				m.logger.Info("session restored from store", "id", id)
				// Try to acquire again now that it's restored
				session, _, err = m.tryAcquire(id)
				if err != nil {
					return nil, err
				}
				if session != nil {
					return session, nil
				}
			}
		}
		return nil, err
	}
	if session != nil {
		m.logger.Debug("session acquired immediately", "id", id)
		return session, nil
	}

	// Session is in use - wait for it
	m.logger.Debug("session busy, waiting", "id", id)

	select {
	case <-waitChan:
		// Session released, try to acquire again
		session, _, err := m.tryAcquire(id)
		if err != nil {
			return nil, err
		}
		if session != nil {
			m.logger.Debug("session acquired after wait", "id", id)
			return session, nil
		}
		// Someone else got it, recurse
		return m.Acquire(ctx, id)
	case <-ctx.Done():
		// Remove from waiting list
		m.removeWaiter(id, waitChan)
		return nil, ctx.Err()
	}
}

// restoreFromStore restores a session from the persistence store.
// It creates a new browser instance and injects the saved cookies.
func (m *Manager) restoreFromStore(ctx context.Context, id string) (*Session, error) {
	persisted, err := m.store.Load(id)
	if err != nil {
		return nil, fmt.Errorf("failed to load session from store: %w", err)
	}
	if persisted == nil {
		return nil, ErrSessionNotFound
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check again in case another goroutine restored it
	if _, exists := m.sessions[id]; exists {
		return m.sessions[id], nil
	}

	if len(m.sessions) >= m.maxSessions {
		return nil, ErrMaxSessionsReached
	}

	// Create new browser with saved settings
	l := launcher.New()
	if m.cfg.ChromePath != "" {
		l = l.Bin(m.cfg.ChromePath)
	}
	l = l.
		Headless(true).
		Set("disable-blink-features", "AutomationControlled").
		Set("disable-dev-shm-usage").
		Set("disable-gpu").
		Set("no-sandbox").
		Set("disable-setuid-sandbox").
		Set("disable-infobars").
		Set("lang", "en-US,en").
		Set("window-size", "1920,1080")

	if persisted.Proxy != "" {
		l = l.Proxy(persisted.Proxy)
	}

	u, err := l.Launch()
	if err != nil {
		return nil, fmt.Errorf("failed to launch browser: %w", err)
	}

	b := rod.New().ControlURL(u)
	if err := b.Connect(); err != nil {
		return nil, fmt.Errorf("failed to connect to browser: %w", err)
	}

	page, err := browser.CreatePage(b, m.cfg.DisableStealth)
	if err != nil {
		b.Close()
		return nil, fmt.Errorf("failed to create page: %w", err)
	}

	// Restore user agent
	if persisted.UserAgent != "" {
		if err := page.SetUserAgent(&proto.NetworkSetUserAgentOverride{
			UserAgent: persisted.UserAgent,
		}); err != nil {
			m.logger.Warn("failed to restore user agent", "id", id, "error", err)
		}
	}

	// Restore cookies
	if len(persisted.Cookies) > 0 {
		var cookieParams []*proto.NetworkCookieParam
		for _, c := range persisted.Cookies {
			param := &proto.NetworkCookieParam{
				Name:     c.Name,
				Value:    c.Value,
				Domain:   c.Domain,
				Path:     c.Path,
				Secure:   c.Secure,
				HTTPOnly: c.HTTPOnly,
			}
			if c.Expires > 0 {
				param.Expires = proto.TimeSinceEpoch(c.Expires)
			}
			switch c.SameSite {
			case proto.NetworkCookieSameSiteStrict:
				param.SameSite = proto.NetworkCookieSameSiteStrict
			case proto.NetworkCookieSameSiteLax:
				param.SameSite = proto.NetworkCookieSameSiteLax
			case proto.NetworkCookieSameSiteNone:
				param.SameSite = proto.NetworkCookieSameSiteNone
			}
			cookieParams = append(cookieParams, param)
		}

		setCookiesCmd := proto.NetworkSetCookies{Cookies: cookieParams}
		if err := setCookiesCmd.Call(page); err != nil {
			m.logger.Warn("failed to restore cookies", "id", id, "error", err)
		} else {
			m.logger.Info("restored cookies from store", "id", id, "count", len(cookieParams))
		}
	}

	session := &Session{
		ID:           id,
		Browser:      b,
		Page:         page,
		UserAgent:    persisted.UserAgent,
		Proxy:        persisted.Proxy,
		CreatedAt:    persisted.CreatedAt,
		LastUsedAt:   time.Now(),
		RequestCount: persisted.RequestCount,
		InUse:        false,
		Cookies:      persisted.Cookies,
	}

	m.sessions[id] = session
	m.logger.Info("session restored", "id", id, "cookies", len(persisted.Cookies))

	return session, nil
}

// tryAcquire attempts to acquire a session without blocking.
// Returns (session, nil, nil) if acquired, (nil, waitChan, nil) if busy, or (nil, nil, err) if not found.
func (m *Manager) tryAcquire(id string) (*Session, chan struct{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[id]
	if !ok {
		m.logger.Debug("session not found", "id", id, "known_sessions", m.sessionIDsLocked())
		return nil, nil, ErrSessionNotFound
	}

	if session.InUse {
		// Create a wait channel for this session
		waitChan := make(chan struct{}, 1)
		if m.waiters == nil {
			m.waiters = make(map[string][]chan struct{})
		}
		m.waiters[id] = append(m.waiters[id], waitChan)
		m.logger.Debug("session in use, queued waiter",
			"id", id,
			"request_count", session.RequestCount,
			"waiters", len(m.waiters[id]),
		)
		return nil, waitChan, nil
	}

	session.InUse = true
	session.LastUsedAt = time.Now()
	return session, nil, nil
}

// removeWaiter removes a wait channel from the waiters list.
func (m *Manager) removeWaiter(id string, waitChan chan struct{}) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.waiters == nil {
		return
	}
	waiters := m.waiters[id]
	for i, ch := range waiters {
		if ch == waitChan {
			m.waiters[id] = append(waiters[:i], waiters[i+1:]...)
			break
		}
	}
}

// sessionIDsLocked returns session IDs (must hold lock).
func (m *Manager) sessionIDsLocked() []string {
	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	return ids
}

// Release releases a session after use.
func (m *Manager) Release(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[id]
	if !ok {
		return
	}

	session.InUse = false
	session.RequestCount++
	session.LastUsedAt = time.Now()

	// Store cookies
	if session.Page != nil {
		cookies, err := session.Page.Cookies(nil)
		if err == nil {
			session.Cookies = cookies
			m.logger.Debug("session cookies saved", "id", id, "cookie_count", len(cookies))
		}
	}

	// Persist updated state to store
	if m.store != nil {
		if err := m.store.Save(&PersistedSession{
			ID:           session.ID,
			UserAgent:    session.UserAgent,
			Proxy:        session.Proxy,
			CreatedAt:    session.CreatedAt,
			LastUsedAt:   session.LastUsedAt,
			RequestCount: session.RequestCount,
			Cookies:      session.Cookies,
		}); err != nil {
			m.logger.Error("failed to persist session state", "id", id, "error", err)
		}
	}

	// Notify one waiter if any
	if m.waiters != nil && len(m.waiters[id]) > 0 {
		waiter := m.waiters[id][0]
		m.waiters[id] = m.waiters[id][1:]
		m.logger.Debug("notifying waiter", "id", id, "remaining_waiters", len(m.waiters[id]))
		select {
		case waiter <- struct{}{}:
		default:
		}
	}
}

// Destroy destroys a session.
func (m *Manager) Destroy(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[id]
	if !ok {
		return ErrSessionNotFound
	}

	if session.InUse {
		return ErrSessionInUse
	}

	m.closeSession(session)
	delete(m.sessions, id)

	// Remove from persistence store
	if m.store != nil {
		if err := m.store.Delete(id); err != nil {
			m.logger.Error("failed to delete session from store", "id", id, "error", err)
		}
	}

	m.logger.Info("session destroyed", "id", id)

	return nil
}

// List returns all session IDs.
func (m *Manager) List() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}

	return ids
}

// GetInfo returns detailed info about a session.
func (m *Manager) GetInfo(id string) (*models.SessionInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, ok := m.sessions[id]
	if !ok {
		return nil, ErrSessionNotFound
	}

	// Mask proxy credentials
	maskedProxy := ""
	if session.Proxy != "" {
		maskedProxy = maskProxyCredentials(session.Proxy)
	}

	return &models.SessionInfo{
		ID:           session.ID,
		CreatedAt:    session.CreatedAt.UnixMilli(),
		LastUsedAt:   session.LastUsedAt.UnixMilli(),
		RequestCount: session.RequestCount,
		UserAgent:    session.UserAgent,
		Proxy:        maskedProxy,
	}, nil
}

// Close closes all sessions and the manager.
func (m *Manager) Close() {
	m.logger.Info("closing session manager...")
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return
	}
	m.closed = true

	sessionCount := len(m.sessions)
	for _, session := range m.sessions {
		m.closeSession(session)
	}
	m.sessions = make(map[string]*Session)

	// Close the persistence store
	if m.store != nil {
		m.logger.Info("closing session persistence store...")
		if err := m.store.Close(); err != nil {
			m.logger.Error("failed to close session store", "error", err)
		} else {
			m.logger.Info("session store closed successfully")
		}
	}
	m.logger.Info("session manager closed", "sessions_closed", sessionCount)
}

// GetPersistedSessions returns all persisted sessions from the store.
// This is useful for listing sessions that may need to be restored.
func (m *Manager) GetPersistedSessions() ([]*PersistedSession, error) {
	if m.store == nil {
		return nil, nil
	}
	return m.store.ListAll()
}

// GetPersistedSession returns a single persisted session by ID.
func (m *Manager) GetPersistedSession(id string) (*PersistedSession, error) {
	if m.store == nil {
		return nil, nil
	}
	return m.store.Load(id)
}

// StartCleanup starts a background goroutine that cleans up idle sessions.
func (m *Manager) StartCleanup(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.cleanupIdleSessions()
		}
	}
}

// cleanupIdleSessions removes sessions that have been idle too long.
func (m *Manager) cleanupIdleSessions() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return
	}

	var toRemove []string
	for id, session := range m.sessions {
		if !session.InUse && time.Since(session.LastUsedAt) > m.cfg.SessionMaxIdle {
			toRemove = append(toRemove, id)
		}
	}

	for _, id := range toRemove {
		session := m.sessions[id]
		m.logger.Info("cleaning up idle session", "id", id, "idle_time", time.Since(session.LastUsedAt))
		m.closeSession(session)
		delete(m.sessions, id)

		// Remove from persistence store
		if m.store != nil {
			if err := m.store.Delete(id); err != nil {
				m.logger.Error("failed to delete session from store", "id", id, "error", err)
			}
		}
	}

	// Also clean up old persisted sessions that might not be in memory
	if m.store != nil {
		threshold := time.Now().Add(-m.cfg.SessionMaxIdle)
		if _, err := m.store.CleanupOlderThan(context.Background(), threshold); err != nil {
			m.logger.Error("failed to cleanup old persisted sessions", "error", err)
		}
	}
}

// closeSession safely closes a session.
func (m *Manager) closeSession(session *Session) {
	if session.Page != nil {
		session.Page.Close()
	}
	if session.Browser != nil {
		session.Browser.Close()
	}
}

// maskProxyCredentials masks the username/password in a proxy URL.
func maskProxyCredentials(proxyURL string) string {
	// Simple masking - replace username:password with ****
	// Format: protocol://user:pass@host:port
	atIdx := strings.Index(proxyURL, "@")
	if atIdx > 0 {
		schemeIdx := strings.Index(proxyURL, "://")
		if schemeIdx > 0 {
			prefix := proxyURL[:schemeIdx+3]
			suffix := proxyURL[atIdx:]
			return prefix + "****:****" + suffix
		}
	}
	return proxyURL
}
