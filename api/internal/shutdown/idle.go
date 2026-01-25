// Package shutdown provides idle monitoring for scale-to-zero deployments.
package shutdown

import (
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// BackgroundWorkChecker is a function that returns true if background work is in progress.
// This is used to prevent idle shutdown while background jobs are running.
type BackgroundWorkChecker func() bool

// IdleMonitor tracks request activity and signals when the server has been idle
// for a configurable duration. This enables scale-to-zero deployments on platforms
// like Fly.io that can stop machines when idle.
type IdleMonitor struct {
	timeout             time.Duration
	logger              *slog.Logger
	activeRequests      int64
	lastActivity        time.Time
	mu                  sync.RWMutex
	shutdownChan        chan struct{}
	stopChan            chan struct{}
	excludePaths        []string              // Paths to exclude from activity tracking (e.g., /healthz)
	backgroundWorkCheck BackgroundWorkChecker // Optional check for background work (e.g., job worker)
}

// IdleMonitorConfig holds configuration for the idle monitor.
type IdleMonitorConfig struct {
	Timeout             time.Duration         // How long to wait before considering idle
	Logger              *slog.Logger
	ExcludePaths        []string              // URL paths that don't count as activity (e.g., health checks)
	BackgroundWorkCheck BackgroundWorkChecker // Optional: returns true if background work is in progress
}

// NewIdleMonitor creates a new idle monitor.
// If timeout is 0, the monitor is effectively disabled.
func NewIdleMonitor(cfg IdleMonitorConfig) *IdleMonitor {
	return &IdleMonitor{
		timeout:             cfg.Timeout,
		logger:              cfg.Logger,
		lastActivity:        time.Now(),
		shutdownChan:        make(chan struct{}),
		stopChan:            make(chan struct{}),
		excludePaths:        cfg.ExcludePaths,
		backgroundWorkCheck: cfg.BackgroundWorkCheck,
	}
}

// Start begins monitoring for idle periods.
// When the timeout is reached with no active requests, it signals shutdown.
func (m *IdleMonitor) Start() {
	if m.timeout <= 0 {
		m.logger.Debug("idle monitoring disabled (timeout=0)")
		return
	}

	m.logger.Info("idle monitoring started", "timeout", m.timeout, "exclude_paths", m.excludePaths)

	go m.run()
}

// Stop stops the idle monitor.
func (m *IdleMonitor) Stop() {
	if m.timeout <= 0 {
		return
	}
	close(m.stopChan)
}

// ShutdownChan returns a channel that is closed when idle timeout is reached.
func (m *IdleMonitor) ShutdownChan() <-chan struct{} {
	return m.shutdownChan
}

// Middleware returns an HTTP middleware that tracks request activity.
// It excludes configured paths (like health checks) from activity tracking.
func (m *IdleMonitor) Middleware(next http.Handler) http.Handler {
	if m.timeout <= 0 {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if this path should be excluded from activity tracking
		path := r.URL.Path
		excluded := false
		for _, excludePath := range m.excludePaths {
			if strings.HasPrefix(path, excludePath) {
				excluded = true
				break
			}
		}

		if !excluded {
			m.requestStart()
			defer m.requestEnd()
		}

		next.ServeHTTP(w, r)
	})
}

// requestStart marks the beginning of a request.
func (m *IdleMonitor) requestStart() {
	atomic.AddInt64(&m.activeRequests, 1)
	m.mu.Lock()
	m.lastActivity = time.Now()
	m.mu.Unlock()
}

// requestEnd marks the end of a request.
func (m *IdleMonitor) requestEnd() {
	atomic.AddInt64(&m.activeRequests, -1)
	m.mu.Lock()
	m.lastActivity = time.Now()
	m.mu.Unlock()
}

// run is the main monitoring loop.
func (m *IdleMonitor) run() {
	// Check more frequently than the timeout to be responsive
	checkInterval := m.timeout / 6
	if checkInterval < 5*time.Second {
		checkInterval = 5 * time.Second
	}
	if checkInterval > 30*time.Second {
		checkInterval = 30 * time.Second
	}

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopChan:
			return
		case <-ticker.C:
			active := atomic.LoadInt64(&m.activeRequests)
			m.mu.RLock()
			idleTime := time.Since(m.lastActivity)
			m.mu.RUnlock()

			// Check if background work is in progress (e.g., job worker processing, pprof connections)
			backgroundBusy := false
			if m.backgroundWorkCheck != nil {
				backgroundBusy = m.backgroundWorkCheck()
			}

			// If background work is in progress, reset the idle timer
			// This gives a full grace period after work completes before shutdown,
			// allowing pollers and other periodic tasks to work correctly
			if active > 0 || backgroundBusy {
				m.mu.Lock()
				m.lastActivity = time.Now()
				m.mu.Unlock()
				// Re-read idle time after reset
				idleTime = 0
			}

			// Only trigger shutdown if:
			// 1. No active HTTP requests
			// 2. No background work in progress
			// 3. Idle time exceeds timeout
			if active == 0 && !backgroundBusy && idleTime >= m.timeout {
				m.logger.Info("idle timeout reached, signaling graceful shutdown",
					"idle_time", idleTime,
					"timeout", m.timeout,
				)
				close(m.shutdownChan)
				return
			}

			m.logger.Debug("idle check",
				"idle_time", idleTime,
				"active_requests", active,
				"background_busy", backgroundBusy,
				"timeout", m.timeout,
			)
		}
	}
}
