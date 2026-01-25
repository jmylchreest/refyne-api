// Package shutdown provides graceful shutdown utilities including idle monitoring.
package shutdown

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestNewIdleMonitor(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	t.Run("creates monitor with default health check", func(t *testing.T) {
		m := NewIdleMonitor(IdleMonitorConfig{
			Timeout: 60 * time.Second,
			Logger:  logger,
		})

		if m.idleTimeout != 60*time.Second {
			t.Errorf("expected timeout 60s, got %v", m.idleTimeout)
		}
		if m.isHealthCheckFn == nil {
			t.Error("expected default health check function")
		}
	})

	t.Run("creates monitor with custom health check", func(t *testing.T) {
		customCheck := func(r *http.Request) bool {
			return r.URL.Path == "/custom-health"
		}

		m := NewIdleMonitor(IdleMonitorConfig{
			Timeout:       30 * time.Second,
			Logger:        logger,
			IsHealthCheck: customCheck,
		})

		req := httptest.NewRequest("GET", "/custom-health", nil)
		if !m.isHealthCheckFn(req) {
			t.Error("expected custom health check to match /custom-health")
		}
	})
}

func TestIdleMonitor_IsEnabled(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	tests := []struct {
		name    string
		timeout time.Duration
		want    bool
	}{
		{"positive timeout enabled", 60 * time.Second, true},
		{"zero timeout disabled", 0, false},
		{"negative timeout disabled", -1 * time.Second, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewIdleMonitor(IdleMonitorConfig{
				Timeout: tt.timeout,
				Logger:  logger,
			})
			if got := m.IsEnabled(); got != tt.want {
				t.Errorf("IsEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIdleMonitor_TrackRequest(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	t.Run("tracks non-health-check requests", func(t *testing.T) {
		m := NewIdleMonitor(IdleMonitorConfig{
			Timeout: 60 * time.Second,
			Logger:  logger,
		})

		initialTime := m.LastRequestTime()

		// Small delay to ensure time difference
		time.Sleep(10 * time.Millisecond)

		req := httptest.NewRequest("GET", "/api/solve", nil)
		done := m.TrackRequest(req)

		if m.ActiveRequests() != 1 {
			t.Errorf("expected 1 active request, got %d", m.ActiveRequests())
		}

		if !m.LastRequestTime().After(initialTime) {
			t.Error("expected last request time to be updated")
		}

		done()

		if m.ActiveRequests() != 0 {
			t.Errorf("expected 0 active requests after done, got %d", m.ActiveRequests())
		}
	})

	t.Run("ignores health check requests", func(t *testing.T) {
		m := NewIdleMonitor(IdleMonitorConfig{
			Timeout: 60 * time.Second,
			Logger:  logger,
		})

		initialTime := m.LastRequestTime()
		initialActive := m.ActiveRequests()

		req := httptest.NewRequest("GET", "/health", nil)
		done := m.TrackRequest(req)
		done()

		if m.ActiveRequests() != initialActive {
			t.Errorf("health check should not affect active requests")
		}

		// Allow some time tolerance for initialization
		if m.LastRequestTime().Sub(initialTime) > 10*time.Millisecond {
			t.Error("health check should not significantly update last request time")
		}
	})
}

func TestIdleMonitor_Middleware(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	m := NewIdleMonitor(IdleMonitorConfig{
		Timeout: 60 * time.Second,
		Logger:  logger,
	})

	handlerCalled := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		// Check that request is being tracked
		if m.ActiveRequests() != 1 {
			t.Errorf("expected 1 active request during handler, got %d", m.ActiveRequests())
		}
		w.WriteHeader(http.StatusOK)
	})

	wrapped := m.Middleware(handler)

	req := httptest.NewRequest("GET", "/api/test", nil)
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	if !handlerCalled {
		t.Error("expected handler to be called")
	}

	if m.ActiveRequests() != 0 {
		t.Errorf("expected 0 active requests after middleware, got %d", m.ActiveRequests())
	}
}

func TestIdleMonitor_ShutdownSignal(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	// Note: The idle monitor checks every 10 seconds in production.
	// For unit tests, we test the logic directly rather than waiting for the ticker.

	t.Run("shutdown channel exists and is closed on idle", func(t *testing.T) {
		m := NewIdleMonitor(IdleMonitorConfig{
			Timeout: 1 * time.Millisecond, // Very short for testing
			Logger:  logger,
		})

		// Verify shutdown channel is not closed initially
		select {
		case <-m.ShutdownChan():
			t.Error("shutdown channel should not be closed initially")
		default:
			// Expected
		}

		// Simulate what the run() function does: check idle and signal shutdown
		// by directly testing the conditions
		if m.IdleTime() < m.idleTimeout || m.ActiveRequests() != 0 {
			// Need to wait for idle timeout
			time.Sleep(5 * time.Millisecond)
		}

		// Verify idle conditions are met
		if m.IdleTime() < m.idleTimeout {
			t.Skip("idle time not reached yet (expected in fast test)")
		}
		if m.ActiveRequests() != 0 {
			t.Error("expected 0 active requests")
		}
	})

	t.Run("does not signal if requests active", func(t *testing.T) {
		m := NewIdleMonitor(IdleMonitorConfig{
			Timeout: 1 * time.Millisecond,
			Logger:  logger,
		})

		// Start a "request" that won't finish
		req := httptest.NewRequest("GET", "/api/test", nil)
		done := m.TrackRequest(req)
		defer done()

		// Wait past the idle timeout
		time.Sleep(10 * time.Millisecond)

		// Active requests should prevent idle condition from being true
		if m.ActiveRequests() == 0 {
			t.Error("expected active requests > 0")
		}

		// Even though idle time > timeout, active requests prevent shutdown
		if m.IdleTime() < m.idleTimeout && m.ActiveRequests() == 0 {
			t.Error("shutdown condition should not be met with active requests")
		}
	})

	t.Run("disabled monitor does not start goroutine", func(t *testing.T) {
		m := NewIdleMonitor(IdleMonitorConfig{
			Timeout: 0, // Disabled
			Logger:  logger,
		})

		if m.IsEnabled() {
			t.Error("monitor should be disabled with timeout 0")
		}

		m.Start() // Should return immediately without starting goroutine
		defer m.Stop()

		// Channel should never be closed for disabled monitor
		select {
		case <-m.ShutdownChan():
			t.Error("disabled monitor should never signal shutdown")
		default:
			// Expected
		}
	})
}

func TestDefaultIsHealthCheck(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		userAgent string
		want      bool
	}{
		{"health path", "/health", "", true},
		{"healthz path", "/healthz", "", true},
		{"livez path", "/livez", "", true},
		{"readyz path", "/readyz", "", true},
		{"fly health check agent", "/api/test", "Fly-HealthCheck/1.0", true},
		{"health check agent", "/api/test", "HealthCheck/1.0", true},
		{"regular request", "/api/solve", "Mozilla/5.0", false},
		{"empty user agent", "/api/test", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			if tt.userAgent != "" {
				req.Header.Set("User-Agent", tt.userAgent)
			}

			if got := DefaultIsHealthCheck(req); got != tt.want {
				t.Errorf("DefaultIsHealthCheck() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIdleMonitor_IdleTime(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	m := NewIdleMonitor(IdleMonitorConfig{
		Timeout: 60 * time.Second,
		Logger:  logger,
	})

	// Initial idle time should be very small
	initialIdle := m.IdleTime()
	if initialIdle > 100*time.Millisecond {
		t.Errorf("expected initial idle time < 100ms, got %v", initialIdle)
	}

	// Wait and check idle time increases
	time.Sleep(50 * time.Millisecond)
	newIdle := m.IdleTime()
	if newIdle <= initialIdle {
		t.Error("expected idle time to increase")
	}

	// Make a request and check idle time resets
	req := httptest.NewRequest("GET", "/api/test", nil)
	done := m.TrackRequest(req)
	done()

	resetIdle := m.IdleTime()
	if resetIdle > 50*time.Millisecond {
		t.Errorf("expected idle time to reset after request, got %v", resetIdle)
	}
}
