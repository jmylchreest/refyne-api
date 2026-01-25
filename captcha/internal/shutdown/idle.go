// Package shutdown provides graceful shutdown utilities including idle monitoring.
package shutdown

import (
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// IdleMonitor tracks server activity and signals shutdown when idle.
// Use ShutdownChan() to receive the shutdown signal.
type IdleMonitor struct {
	idleTimeout      time.Duration
	lastRequest      atomic.Value // time.Time
	activeRequests   atomic.Int64
	logger           *slog.Logger
	stopCh           chan struct{}
	shutdownCh       chan struct{}
	wg               sync.WaitGroup
	isHealthCheckFn  func(*http.Request) bool
}

// IdleMonitorConfig configures the idle monitor.
type IdleMonitorConfig struct {
	// Timeout is the duration of inactivity before triggering shutdown.
	// Set to 0 or negative to disable idle monitoring.
	Timeout time.Duration

	// Logger for idle monitoring events.
	Logger *slog.Logger

	// IsHealthCheck is an optional function to identify health check requests.
	// Health checks do not reset the idle timer.
	// If nil, uses DefaultIsHealthCheck.
	IsHealthCheck func(*http.Request) bool
}

// NewIdleMonitor creates a new idle monitor.
// If timeout is <= 0, the monitor will be disabled.
func NewIdleMonitor(cfg IdleMonitorConfig) *IdleMonitor {
	isHealthCheck := cfg.IsHealthCheck
	if isHealthCheck == nil {
		isHealthCheck = DefaultIsHealthCheck
	}

	m := &IdleMonitor{
		idleTimeout:     cfg.Timeout,
		logger:          cfg.Logger,
		stopCh:          make(chan struct{}),
		shutdownCh:      make(chan struct{}),
		isHealthCheckFn: isHealthCheck,
	}
	m.lastRequest.Store(time.Now())
	return m
}

// Start begins monitoring for idle state.
// When idle timeout is reached and no requests are active, signals shutdown via ShutdownChan().
// Idle monitoring is disabled if timeout is <= 0.
func (m *IdleMonitor) Start() {
	if m.idleTimeout <= 0 {
		m.logger.Info("idle monitoring disabled (set IDLE_TIMEOUT to enable)")
		return
	}

	m.logger.Info("idle monitoring started", "timeout", m.idleTimeout)

	m.wg.Add(1)
	go m.run()
}

// IsEnabled returns true if idle monitoring is enabled (timeout > 0).
func (m *IdleMonitor) IsEnabled() bool {
	return m.idleTimeout > 0
}

// Stop stops the idle monitor.
func (m *IdleMonitor) Stop() {
	close(m.stopCh)
	m.wg.Wait()
}

func (m *IdleMonitor) run() {
	defer m.wg.Done()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			lastReq := m.lastRequest.Load().(time.Time)
			idleTime := time.Since(lastReq)
			active := m.activeRequests.Load()

			if idleTime > m.idleTimeout && active == 0 {
				m.logger.Info("idle timeout reached, signaling graceful shutdown",
					"idle_time", idleTime.Round(time.Second),
					"timeout", m.idleTimeout,
				)
				// Signal shutdown - main will handle graceful exit
				close(m.shutdownCh)
				return
			}

			// Log status periodically for debugging
			if idleTime > m.idleTimeout/2 {
				m.logger.Debug("idle check",
					"idle_time", idleTime.Round(time.Second),
					"active_requests", active,
					"timeout", m.idleTimeout,
				)
			}
		}
	}
}

// TrackRequest marks that a request has started.
// Returns a function to call when the request completes.
func (m *IdleMonitor) TrackRequest(r *http.Request) func() {
	// Don't count health checks toward activity
	if m.isHealthCheckFn(r) {
		return func() {} // No-op
	}

	m.activeRequests.Add(1)
	m.lastRequest.Store(time.Now())

	return func() {
		m.activeRequests.Add(-1)
		m.lastRequest.Store(time.Now())
	}
}

// Middleware returns HTTP middleware that tracks requests.
func (m *IdleMonitor) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		done := m.TrackRequest(r)
		defer done()
		next.ServeHTTP(w, r)
	})
}

// ShutdownChan returns a channel that is closed when idle shutdown is triggered.
// Main should select on this channel alongside SIGTERM to handle idle shutdown.
func (m *IdleMonitor) ShutdownChan() <-chan struct{} {
	return m.shutdownCh
}

// ActiveRequests returns the current number of active requests.
func (m *IdleMonitor) ActiveRequests() int64 {
	return m.activeRequests.Load()
}

// LastRequestTime returns the time of the last non-health-check request.
func (m *IdleMonitor) LastRequestTime() time.Time {
	return m.lastRequest.Load().(time.Time)
}

// IdleTime returns how long the server has been idle.
func (m *IdleMonitor) IdleTime() time.Duration {
	return time.Since(m.LastRequestTime())
}

// DefaultIsHealthCheck returns true if this is a health check request.
// Detects Fly.io health checks by User-Agent and common health check paths.
func DefaultIsHealthCheck(r *http.Request) bool {
	ua := r.Header.Get("User-Agent")
	if strings.Contains(ua, "Fly-HealthCheck") || strings.Contains(ua, "HealthCheck") {
		return true
	}
	path := r.URL.Path
	switch path {
	case "/health", "/healthz", "/livez", "/readyz":
		return true
	}
	return false
}
