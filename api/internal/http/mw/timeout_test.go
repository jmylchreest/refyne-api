package mw

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// ========================================
// TimeoutConfig Tests
// ========================================

func TestTimeoutConfig_Fields(t *testing.T) {
	cfg := TimeoutConfig{
		Default:          30 * time.Second,
		Extended:         120 * time.Second,
		ExtendedPatterns: []string{"/analyze", "/extract"},
		SkipPatterns:     []string{"/stream", "/ws"},
	}

	if cfg.Default != 30*time.Second {
		t.Errorf("Default = %v, want 30s", cfg.Default)
	}
	if cfg.Extended != 120*time.Second {
		t.Errorf("Extended = %v, want 120s", cfg.Extended)
	}
	if len(cfg.ExtendedPatterns) != 2 {
		t.Errorf("ExtendedPatterns length = %d, want 2", len(cfg.ExtendedPatterns))
	}
	if len(cfg.SkipPatterns) != 2 {
		t.Errorf("SkipPatterns length = %d, want 2", len(cfg.SkipPatterns))
	}
}

// ========================================
// Timeout Middleware Tests
// ========================================

func TestTimeout_DefaultPath(t *testing.T) {
	cfg := TimeoutConfig{
		Default:          50 * time.Millisecond,
		Extended:         100 * time.Millisecond,
		ExtendedPatterns: []string{"/analyze"},
		SkipPatterns:     []string{"/stream"},
	}

	handler := Timeout(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Respond immediately
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestTimeout_ExtendedPath(t *testing.T) {
	cfg := TimeoutConfig{
		Default:          10 * time.Millisecond,
		Extended:         100 * time.Millisecond,
		ExtendedPatterns: []string{"/analyze"},
		SkipPatterns:     []string{"/stream"},
	}

	handler := Timeout(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Sleep longer than default but less than extended
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/analyze/job", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d (extended timeout should allow request)", rec.Code, http.StatusOK)
	}
}

func TestTimeout_SkipPath(t *testing.T) {
	cfg := TimeoutConfig{
		Default:          10 * time.Millisecond,
		Extended:         50 * time.Millisecond,
		ExtendedPatterns: []string{"/analyze"},
		SkipPatterns:     []string{"/stream"},
	}

	handler := Timeout(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Sleep longer than all timeouts
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/stream/events", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Skip paths have no timeout, so this should succeed
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d (skip pattern should bypass timeout)", rec.Code, http.StatusOK)
	}
}

func TestTimeout_DefaultTimedOut(t *testing.T) {
	cfg := TimeoutConfig{
		Default:          10 * time.Millisecond,
		Extended:         100 * time.Millisecond,
		ExtendedPatterns: []string{"/analyze"},
		SkipPatterns:     []string{"/stream"},
	}

	handler := Timeout(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Sleep longer than default timeout
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusGatewayTimeout {
		t.Errorf("status = %d, want %d (should timeout)", rec.Code, http.StatusGatewayTimeout)
	}
}

func TestTimeout_MultipleExtendedPatterns(t *testing.T) {
	cfg := TimeoutConfig{
		Default:          10 * time.Millisecond,
		Extended:         100 * time.Millisecond,
		ExtendedPatterns: []string{"/analyze", "/extract", "/crawl"},
		SkipPatterns:     []string{},
	}

	tests := []struct {
		path    string
		isLong  bool
		comment string
	}{
		{"/api/v1/analyze/job", true, "analyze should be extended"},
		{"/api/v1/extract", true, "extract should be extended"},
		{"/api/v1/crawl/start", true, "crawl should be extended"},
		{"/api/v1/jobs", false, "jobs should use default"},
		{"/api/v1/status", false, "status should use default"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			handler := Timeout(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Sleep for a duration between default and extended
				time.Sleep(50 * time.Millisecond)
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if tt.isLong {
				if rec.Code != http.StatusOK {
					t.Errorf("%s: status = %d, want %d", tt.comment, rec.Code, http.StatusOK)
				}
			} else {
				if rec.Code != http.StatusGatewayTimeout {
					t.Errorf("%s: status = %d, want %d", tt.comment, rec.Code, http.StatusGatewayTimeout)
				}
			}
		})
	}
}

func TestTimeout_MultipleSkipPatterns(t *testing.T) {
	cfg := TimeoutConfig{
		Default:          10 * time.Millisecond,
		Extended:         50 * time.Millisecond,
		ExtendedPatterns: []string{},
		SkipPatterns:     []string{"/stream", "/ws", "/sse"},
	}

	tests := []struct {
		path     string
		shouldOK bool
	}{
		{"/api/v1/stream/events", true},
		{"/api/v1/ws/connect", true},
		{"/api/v1/sse/subscribe", true},
		{"/api/v1/jobs", false}, // Should timeout
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			handler := Timeout(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Sleep longer than all timeouts
				time.Sleep(100 * time.Millisecond)
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if tt.shouldOK {
				if rec.Code != http.StatusOK {
					t.Errorf("%s: status = %d, want %d (skip pattern)", tt.path, rec.Code, http.StatusOK)
				}
			} else {
				if rec.Code != http.StatusGatewayTimeout {
					t.Errorf("%s: status = %d, want %d", tt.path, rec.Code, http.StatusGatewayTimeout)
				}
			}
		})
	}
}

func TestTimeout_EmptyConfig(t *testing.T) {
	cfg := TimeoutConfig{
		Default:          50 * time.Millisecond,
		Extended:         100 * time.Millisecond,
		ExtendedPatterns: []string{},
		SkipPatterns:     []string{},
	}

	handler := Timeout(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/any/path", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}
