package constants

import (
	"testing"
	"time"
)

func TestCalculateBackoff(t *testing.T) {
	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{-1, InitialBackoff},   // Negative defaults to initial
		{0, 2 * time.Second},   // Initial backoff
		{1, 4 * time.Second},   // 2s * 2
		{2, 8 * time.Second},   // 4s * 2
		{3, 16 * time.Second},  // 8s * 2
		{4, 30 * time.Second},  // Would be 32s but capped at MaxBackoff (30s)
		{5, 30 * time.Second},  // Capped at MaxBackoff
		{10, 30 * time.Second}, // Still capped
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := CalculateBackoff(tt.attempt)
			if got != tt.want {
				t.Errorf("CalculateBackoff(%d) = %v, want %v", tt.attempt, got, tt.want)
			}
		})
	}
}

func TestCalculateRateLimitBackoff(t *testing.T) {
	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{-1, RateLimitBackoff}, // Negative defaults to rate limit backoff
		{0, 5 * time.Second},   // Initial rate limit backoff
		{1, 10 * time.Second},  // 5s * 2
		{2, 20 * time.Second},  // 10s * 2
		{3, 30 * time.Second},  // Would be 40s but capped at MaxBackoff
		{4, 30 * time.Second},  // Capped at MaxBackoff
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := CalculateRateLimitBackoff(tt.attempt)
			if got != tt.want {
				t.Errorf("CalculateRateLimitBackoff(%d) = %v, want %v", tt.attempt, got, tt.want)
			}
		})
	}
}

func TestIsRetryableCategory(t *testing.T) {
	tests := []struct {
		category ErrorCategory
		want     bool
	}{
		{ErrorCategoryRateLimit, true},
		{ErrorCategoryProviderError, true},
		{ErrorCategoryTimeout, true},
		{ErrorCategoryModelUnsupported, false},
		{ErrorCategoryInvalidKey, false},
		{ErrorCategoryQuotaExceeded, false},
		{ErrorCategoryContentTooLong, false},
		{ErrorCategoryUnknown, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.category), func(t *testing.T) {
			got := IsRetryableCategory(tt.category)
			if got != tt.want {
				t.Errorf("IsRetryableCategory(%q) = %v, want %v", tt.category, got, tt.want)
			}
		})
	}
}

func TestShouldFallbackCategory(t *testing.T) {
	tests := []struct {
		category ErrorCategory
		want     bool
	}{
		{ErrorCategoryRateLimit, true},
		{ErrorCategoryModelUnsupported, true},
		{ErrorCategoryProviderError, true},
		{ErrorCategoryTimeout, true},
		{ErrorCategoryInvalidKey, false},
		{ErrorCategoryQuotaExceeded, false},
		{ErrorCategoryContentTooLong, false},
		{ErrorCategoryUnknown, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.category), func(t *testing.T) {
			got := ShouldFallbackCategory(tt.category)
			if got != tt.want {
				t.Errorf("ShouldFallbackCategory(%q) = %v, want %v", tt.category, got, tt.want)
			}
		})
	}
}

func TestGetSanitizedErrorMessage(t *testing.T) {
	// Test that all categories have messages
	categories := []ErrorCategory{
		ErrorCategoryRateLimit,
		ErrorCategoryModelUnsupported,
		ErrorCategoryInvalidKey,
		ErrorCategoryQuotaExceeded,
		ErrorCategoryProviderError,
		ErrorCategoryContentTooLong,
		ErrorCategoryTimeout,
		ErrorCategoryUnknown,
	}

	for _, cat := range categories {
		msg := GetSanitizedErrorMessage(cat)
		if msg == "" {
			t.Errorf("GetSanitizedErrorMessage(%q) returned empty string", cat)
		}
	}

	// Test unknown category returns default message
	unknownMsg := GetSanitizedErrorMessage(ErrorCategory("nonexistent"))
	expectedUnknown := SanitizedErrorMessages[ErrorCategoryUnknown]
	if unknownMsg != expectedUnknown {
		t.Errorf("GetSanitizedErrorMessage(nonexistent) = %q, want %q", unknownMsg, expectedUnknown)
	}
}

func TestConstantValues(t *testing.T) {
	// Verify key constants have sensible values
	if MaxRetryAttempts <= 0 {
		t.Errorf("MaxRetryAttempts should be positive, got %d", MaxRetryAttempts)
	}
	if InitialBackoff <= 0 {
		t.Errorf("InitialBackoff should be positive, got %v", InitialBackoff)
	}
	if MaxBackoff <= InitialBackoff {
		t.Errorf("MaxBackoff (%v) should be greater than InitialBackoff (%v)", MaxBackoff, InitialBackoff)
	}
	if BackoffMultiplier <= 1.0 {
		t.Errorf("BackoffMultiplier should be greater than 1.0, got %f", BackoffMultiplier)
	}
	if RateLimitBackoff <= InitialBackoff {
		t.Errorf("RateLimitBackoff (%v) should be greater than InitialBackoff (%v)", RateLimitBackoff, InitialBackoff)
	}
}
