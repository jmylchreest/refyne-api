package mw

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jmylchreest/refyne-api/internal/constants"
)

// ========================================
// intToString Tests
// ========================================

func TestIntToString(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{1, "1"},
		{-1, "-1"},
		{100, "100"},
		{999999, "999999"},
	}

	for _, tt := range tests {
		got := intToString(tt.input)
		if got != tt.expected {
			t.Errorf("intToString(%d) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

// ========================================
// GetTierLimitsFromContext Tests
// ========================================

func TestGetTierLimitsFromContext(t *testing.T) {
	t.Run("limits present in context", func(t *testing.T) {
		expectedLimits := constants.TierLimits{
			MonthlyExtractions: 1000,
			RequestsPerMinute:  60,
			MaxConcurrentJobs:  5,
		}
		ctx := context.WithValue(context.Background(), TierLimitsKey, expectedLimits)

		got := GetTierLimitsFromContext(ctx)
		if got == nil {
			t.Fatal("expected limits, got nil")
		}
		if got.MonthlyExtractions != expectedLimits.MonthlyExtractions {
			t.Errorf("MonthlyExtractions = %d, want %d", got.MonthlyExtractions, expectedLimits.MonthlyExtractions)
		}
		if got.RequestsPerMinute != expectedLimits.RequestsPerMinute {
			t.Errorf("RequestsPerMinute = %d, want %d", got.RequestsPerMinute, expectedLimits.RequestsPerMinute)
		}
	})

	t.Run("no limits in context - returns free tier defaults", func(t *testing.T) {
		got := GetTierLimitsFromContext(context.Background())
		if got == nil {
			t.Fatal("expected default limits, got nil")
		}
		// Should return free tier defaults
		freeLimits := constants.Tiers[constants.TierFree]
		if got.MonthlyExtractions != freeLimits.MonthlyExtractions {
			t.Errorf("MonthlyExtractions = %d, want %d (free tier)", got.MonthlyExtractions, freeLimits.MonthlyExtractions)
		}
	})

	t.Run("wrong type in context - returns free tier defaults", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), TierLimitsKey, "not limits")
		got := GetTierLimitsFromContext(ctx)
		if got == nil {
			t.Fatal("expected default limits, got nil")
		}
	})
}

// ========================================
// TierGate Middleware Tests
// ========================================

func TestTierGate_NoClaims(t *testing.T) {
	handler := TierGate(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestTierGate_WithClaims(t *testing.T) {
	var receivedLimits *TierLimits
	handler := TierGate(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedLimits = GetTierLimitsFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	claims := &UserClaims{
		UserID: "user-123",
		Tier:   "pro",
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req = req.WithContext(context.WithValue(req.Context(), UserClaimsKey, claims))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if receivedLimits == nil {
		t.Error("expected tier limits to be set in context")
	}
}

// ========================================
// RequireFeature Middleware Tests
// ========================================

func TestRequireFeature_NoClaims(t *testing.T) {
	handler := RequireFeature("provider_byok")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestRequireFeature_MissingFeature(t *testing.T) {
	handler := RequireFeature("provider_byok")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	claims := &UserClaims{
		UserID:   "user-123",
		Tier:     "free",
		Features: []string{"webhooks"}, // Does not have provider_byok
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req = req.WithContext(context.WithValue(req.Context(), UserClaimsKey, claims))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}

	// Check response body
	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if response["error"] != "feature_not_available" {
		t.Errorf("error = %q, want %q", response["error"], "feature_not_available")
	}
	if response["feature"] != "provider_byok" {
		t.Errorf("feature = %q, want %q", response["feature"], "provider_byok")
	}
}

func TestRequireFeature_HasFeature(t *testing.T) {
	handler := RequireFeature("provider_byok")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))

	claims := &UserClaims{
		UserID:   "user-123",
		Tier:     "pro",
		Features: []string{"provider_byok", "models_custom"},
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
// RequireUsageQuota Middleware Tests
// ========================================

// Note: RequireUsageQuota requires a real UsageService with GetBillingPeriodUsage method.
// These tests verify middleware behavior with mock claims and tier limits.

func TestRequireUsageQuota_NoClaims(t *testing.T) {
	handler := RequireUsageQuota(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/extract", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

// ========================================
// Mock JobService
// ========================================

type mockJobService struct {
	count int
	err   error
}

func (m *mockJobService) CountActiveJobsByUser(ctx context.Context, userID string) (int, error) {
	return m.count, m.err
}

// ========================================
// RequireConcurrentJobLimit Middleware Tests
// ========================================

func TestRequireConcurrentJobLimit_NoClaims(t *testing.T) {
	handler := RequireConcurrentJobLimit(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestRequireConcurrentJobLimit_UnderLimit(t *testing.T) {
	// Free tier has MaxConcurrentJobs=1, so count must be 0 to be under the limit
	jobSvc := &mockJobService{count: 0, err: nil}
	handler := RequireConcurrentJobLimit(jobSvc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Set up claims with a tier that has job limits
	claims := &UserClaims{
		UserID: "user-123",
		Tier:   "free", // Free tier has MaxConcurrentJobs=1
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", nil)
	req = req.WithContext(context.WithValue(req.Context(), UserClaimsKey, claims))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestRequireConcurrentJobLimit_AtLimit(t *testing.T) {
	// Get free tier limits to know what the limit is
	freeLimits := constants.GetTierLimits("free")
	if freeLimits.MaxConcurrentJobs == 0 {
		t.Skip("free tier has unlimited concurrent jobs, skipping test")
	}

	jobSvc := &mockJobService{count: freeLimits.MaxConcurrentJobs, err: nil}
	handler := RequireConcurrentJobLimit(jobSvc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	claims := &UserClaims{
		UserID: "user-123",
		Tier:   "free",
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", nil)
	req = req.WithContext(context.WithValue(req.Context(), UserClaimsKey, claims))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusTooManyRequests)
	}

	// Check response body
	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if response["error"] != "concurrent_job_limit_exceeded" {
		t.Errorf("error = %q, want %q", response["error"], "concurrent_job_limit_exceeded")
	}

	// Check headers
	if rec.Header().Get("X-Concurrent-Limit") == "" {
		t.Error("expected X-Concurrent-Limit header")
	}
	if rec.Header().Get("X-Concurrent-Active") == "" {
		t.Error("expected X-Concurrent-Active header")
	}
}

func TestRequireConcurrentJobLimit_ServiceError(t *testing.T) {
	jobSvc := &mockJobService{count: 0, err: errors.New("database error")}
	handler := RequireConcurrentJobLimit(jobSvc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Get free tier limits to ensure it has a non-zero limit
	freeLimits := constants.GetTierLimits("free")
	if freeLimits.MaxConcurrentJobs == 0 {
		t.Skip("free tier has unlimited concurrent jobs, skipping test")
	}

	claims := &UserClaims{
		UserID: "user-123",
		Tier:   "free",
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", nil)
	req = req.WithContext(context.WithValue(req.Context(), UserClaimsKey, claims))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestRequireConcurrentJobLimit_UnlimitedTier(t *testing.T) {
	// Self-hosted tier typically has unlimited concurrent jobs (0)
	selfHostedLimits := constants.GetTierLimits("selfhosted")
	if selfHostedLimits.MaxConcurrentJobs != 0 {
		t.Skip("selfhosted tier has limited concurrent jobs, skipping test")
	}

	jobSvc := &mockJobService{count: 1000, err: nil} // High count shouldn't matter
	handler := RequireConcurrentJobLimit(jobSvc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	claims := &UserClaims{
		UserID: "user-123",
		Tier:   "selfhosted",
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", nil)
	req = req.WithContext(context.WithValue(req.Context(), UserClaimsKey, claims))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d (unlimited tier)", rec.Code, http.StatusOK)
	}
}

// ========================================
// TierContextKey Tests
// ========================================

func TestTierContextKey(t *testing.T) {
	if TierLimitsKey != TierContextKey("tier_limits") {
		t.Errorf("TierLimitsKey = %q, want %q", TierLimitsKey, "tier_limits")
	}
}
