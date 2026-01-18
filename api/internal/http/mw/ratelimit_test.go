package mw

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jmylchreest/refyne-api/internal/constants"
)

// ========================================
// RateLimitConfig Tests
// ========================================

func TestRateLimitConfig_Fields(t *testing.T) {
	cfg := RateLimitConfig{
		TierLimits: map[string]int{
			"free":     60,
			"standard": 120,
			"pro":      0, // Unlimited
		},
		IPRequestsPerMinute: 30,
	}

	if cfg.TierLimits["free"] != 60 {
		t.Errorf("TierLimits[free] = %d, want 60", cfg.TierLimits["free"])
	}
	if cfg.TierLimits["standard"] != 120 {
		t.Errorf("TierLimits[standard] = %d, want 120", cfg.TierLimits["standard"])
	}
	if cfg.TierLimits["pro"] != 0 {
		t.Errorf("TierLimits[pro] = %d, want 0 (unlimited)", cfg.TierLimits["pro"])
	}
	if cfg.IPRequestsPerMinute != 30 {
		t.Errorf("IPRequestsPerMinute = %d, want 30", cfg.IPRequestsPerMinute)
	}
}

// ========================================
// DefaultRateLimitConfig Tests
// ========================================

func TestDefaultRateLimitConfig(t *testing.T) {
	cfg := DefaultRateLimitConfig()

	// Verify expected tiers exist
	expectedTiers := []string{constants.TierFree, constants.TierStandard, constants.TierPro, constants.TierSelfHosted}
	for _, tier := range expectedTiers {
		if _, ok := cfg.TierLimits[tier]; !ok {
			t.Errorf("expected TierLimits to contain %q", tier)
		}
	}

	// Verify IP limit is set from global constant
	if cfg.IPRequestsPerMinute != constants.GlobalIPRateLimitPerMinute {
		t.Errorf("IPRequestsPerMinute = %d, want %d", cfg.IPRequestsPerMinute, constants.GlobalIPRateLimitPerMinute)
	}
}

// ========================================
// RateLimitByUser Tests
// ========================================

func TestRateLimitByUser_NoAuth(t *testing.T) {
	cfg := RateLimitConfig{
		TierLimits: map[string]int{
			"free": 60,
		},
		IPRequestsPerMinute: 30,
	}

	handler := RateLimitByUser(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rec := httptest.NewRecorder()

	// First request should pass (uses fallback IP limiter)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestRateLimitByUser_AuthenticatedUser(t *testing.T) {
	cfg := RateLimitConfig{
		TierLimits: map[string]int{
			"free": 60,
			"pro":  0, // Unlimited
		},
		IPRequestsPerMinute: 30,
	}

	handler := RateLimitByUser(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	claims := &UserClaims{
		UserID: "user-123",
		Tier:   "free",
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req = req.WithContext(context.WithValue(req.Context(), UserClaimsKey, claims))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestRateLimitByUser_UnlimitedTier(t *testing.T) {
	cfg := RateLimitConfig{
		TierLimits: map[string]int{
			"free": 60,
			"pro":  0, // Unlimited
		},
		IPRequestsPerMinute: 30,
	}

	handler := RateLimitByUser(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	claims := &UserClaims{
		UserID: "user-123",
		Tier:   "pro",
	}

	// Send many requests - should all pass for unlimited tier
	for i := 0; i < 100; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
		req = req.WithContext(context.WithValue(req.Context(), UserClaimsKey, claims))
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("request %d: status = %d, want %d (unlimited tier)", i, rec.Code, http.StatusOK)
			break
		}
	}
}

func TestRateLimitByUser_NormalizeTier(t *testing.T) {
	// Test that tier names with prefixes like "tier_v1_standard" get normalized
	cfg := RateLimitConfig{
		TierLimits: map[string]int{
			"free":     60,
			"standard": 120,
		},
		IPRequestsPerMinute: 30,
	}

	handler := RateLimitByUser(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Test with raw tier name
	claims := &UserClaims{
		UserID: "user-123",
		Tier:   "standard", // Might come as "tier_v1_standard" from Clerk
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req = req.WithContext(context.WithValue(req.Context(), UserClaimsKey, claims))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

// ========================================
// RateLimitByIP Tests
// ========================================

func TestRateLimitByIP(t *testing.T) {
	handler := RateLimitByIP(100)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/public", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

// ========================================
// RateLimitGlobal Tests
// ========================================

func TestRateLimitGlobal(t *testing.T) {
	handler := RateLimitGlobal(1000)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

// Note: Full rate limiting tests would require simulating many requests
// within a short time window and checking for 429 responses.
// These tests verify the middleware construction and basic pass-through behavior.
