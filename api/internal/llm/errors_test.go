package llm

import (
	"errors"
	"net/http"
	"testing"
)

// ========================================
// LLMError Tests
// ========================================

func TestLLMError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *LLMError
		expected string
	}{
		{
			name:     "with user message",
			err:      &LLMError{UserMessage: "User-friendly message"},
			expected: "User-friendly message",
		},
		{
			name:     "with wrapped error",
			err:      &LLMError{Err: errors.New("wrapped error")},
			expected: "wrapped error",
		},
		{
			name:     "empty error",
			err:      &LLMError{},
			expected: "unknown LLM error",
		},
		{
			name:     "user message takes priority",
			err:      &LLMError{UserMessage: "User message", Err: errors.New("wrapped")},
			expected: "User message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.err.Error()
			if result != tt.expected {
				t.Errorf("Error() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestLLMError_Unwrap(t *testing.T) {
	wrapped := errors.New("wrapped error")
	llmErr := &LLMError{Err: wrapped}

	if llmErr.Unwrap() != wrapped {
		t.Error("Unwrap() should return the wrapped error")
	}

	// Verify errors.Is works
	if !errors.Is(llmErr, wrapped) {
		t.Error("errors.Is should work with Unwrap")
	}
}

func TestLLMError_IsFreeTierError(t *testing.T) {
	tests := []struct {
		name     string
		err      *LLMError
		expected bool
	}{
		{
			name:     "free tier unavailable",
			err:      &LLMError{Err: ErrFreeTierUnavailable},
			expected: true,
		},
		{
			name:     "free tier rate limited",
			err:      &LLMError{Err: ErrFreeTierRateLimited},
			expected: true,
		},
		{
			name:     "free tier quota exhausted",
			err:      &LLMError{Err: ErrFreeTierQuotaExhausted},
			expected: true,
		},
		{
			name:     "invalid API key",
			err:      &LLMError{Err: ErrInvalidAPIKey},
			expected: false,
		},
		{
			name:     "tier quota exceeded",
			err:      &LLMError{Err: ErrTierQuotaExceeded},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.err.IsFreeTierError()
			if result != tt.expected {
				t.Errorf("IsFreeTierError() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestLLMError_IsTierQuotaError(t *testing.T) {
	tests := []struct {
		name     string
		err      *LLMError
		expected bool
	}{
		{
			name:     "tier quota exceeded",
			err:      &LLMError{Err: ErrTierQuotaExceeded},
			expected: true,
		},
		{
			name:     "tier feature disabled",
			err:      &LLMError{Err: ErrTierFeatureDisabled},
			expected: true,
		},
		{
			name:     "insufficient credits",
			err:      &LLMError{Err: ErrInsufficientCredits},
			expected: true,
		},
		{
			name:     "free tier unavailable",
			err:      &LLMError{Err: ErrFreeTierUnavailable},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.err.IsTierQuotaError()
			if result != tt.expected {
				t.Errorf("IsTierQuotaError() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestLLMError_IsQuotaError(t *testing.T) {
	tests := []struct {
		name     string
		err      *LLMError
		expected bool
	}{
		{
			name:     "free tier quota",
			err:      &LLMError{Err: ErrFreeTierQuotaExhausted},
			expected: true,
		},
		{
			name:     "tier quota",
			err:      &LLMError{Err: ErrTierQuotaExceeded},
			expected: true,
		},
		{
			name:     "provider error",
			err:      &LLMError{Err: ErrProviderError},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.err.IsQuotaError()
			if result != tt.expected {
				t.Errorf("IsQuotaError() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// ========================================
// ClassifyError Tests
// ========================================

func TestClassifyError_NilError(t *testing.T) {
	result := ClassifyError(nil, "openrouter", "model", 0, false)
	if result != nil {
		t.Error("expected nil for nil error")
	}
}

func TestClassifyError_429RateLimit(t *testing.T) {
	err := errors.New("rate limit exceeded")

	// Free tier
	result := ClassifyError(err, "openrouter", "model:free", http.StatusTooManyRequests, false)
	if !errors.Is(result.Err, ErrFreeTierRateLimited) {
		t.Error("expected ErrFreeTierRateLimited for free tier 429")
	}
	if !result.SuggestUpgrade {
		t.Error("expected SuggestUpgrade for free tier")
	}

	// BYOK
	result = ClassifyError(err, "openrouter", "model:free", http.StatusTooManyRequests, true)
	if errors.Is(result.Err, ErrFreeTierRateLimited) {
		t.Error("BYOK user should not get free tier error")
	}
}

func TestClassifyError_402PaymentRequired(t *testing.T) {
	err := errors.New("payment required")

	// Free tier
	result := ClassifyError(err, "openrouter", "model:free", http.StatusPaymentRequired, false)
	if !errors.Is(result.Err, ErrFreeTierQuotaExhausted) {
		t.Error("expected ErrFreeTierQuotaExhausted for free tier 402")
	}
	if result.Retryable {
		t.Error("402 should not be retryable")
	}
}

func TestClassifyError_503ServiceUnavailable(t *testing.T) {
	err := errors.New("service unavailable")

	// Free tier
	result := ClassifyError(err, "openrouter", "model:free", http.StatusServiceUnavailable, false)
	if !errors.Is(result.Err, ErrFreeTierUnavailable) {
		t.Error("expected ErrFreeTierUnavailable for free tier 503")
	}

	// Paid tier
	result = ClassifyError(err, "openrouter", "model", http.StatusServiceUnavailable, true)
	if !errors.Is(result.Err, ErrModelUnavailable) {
		t.Error("expected ErrModelUnavailable for paid tier 503")
	}
}

func TestClassifyError_401Unauthorized(t *testing.T) {
	err := errors.New("unauthorized")

	result := ClassifyError(err, "openrouter", "model", http.StatusUnauthorized, false)
	if !errors.Is(result.Err, ErrInvalidAPIKey) {
		t.Error("expected ErrInvalidAPIKey for 401")
	}
	if result.ShouldFallback {
		t.Error("401 should not trigger fallback")
	}
}

func TestClassifyError_FeatureUnsupported(t *testing.T) {
	tests := []string{
		"response_format is not supported by this model",
		"structured output not supported",
		"json mode not supported for this model",
	}

	for _, errMsg := range tests {
		err := errors.New(errMsg)
		result := ClassifyError(err, "openrouter", "model", http.StatusBadRequest, false)
		if !errors.Is(result.Err, ErrModelFeatureUnsupported) {
			t.Errorf("expected ErrModelFeatureUnsupported for %q", errMsg)
		}
		if !result.ShouldFallback {
			t.Error("feature unsupported should trigger fallback")
		}
	}
}

// ========================================
// containsFeatureUnsupported Tests
// ========================================

func TestContainsFeatureUnsupported(t *testing.T) {
	tests := []struct {
		errStr   string
		expected bool
	}{
		{"response_format is not supported", true},
		{"response_format not supported", true},
		{"structured output not supported", true},
		{"json mode not supported", true},
		{"json_object not supported", true},
		{"does not support response_format", true},
		{"does not support structured", true},
		{"rate limit exceeded", false},
		{"invalid api key", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.errStr, func(t *testing.T) {
			result := containsFeatureUnsupported(tt.errStr)
			if result != tt.expected {
				t.Errorf("containsFeatureUnsupported(%q) = %v, want %v", tt.errStr, result, tt.expected)
			}
		})
	}
}

// ========================================
// extractStatusCode Tests
// ========================================

func TestExtractStatusCode(t *testing.T) {
	tests := []struct {
		errMsg   string
		expected int
	}{
		{"status: 429 rate limit", http.StatusTooManyRequests},
		{"status: 402 payment required", http.StatusPaymentRequired},
		{"status: 401 unauthorized", http.StatusUnauthorized},
		{"status: 503 service unavailable", http.StatusServiceUnavailable},
		{"status: 502 bad gateway", http.StatusBadGateway},
		{"status: 504 gateway timeout", http.StatusGatewayTimeout},
		{"returned 429", http.StatusTooManyRequests},
		{"HTTP 503", http.StatusServiceUnavailable},
		{"unknown error", 0},
		{"", 0},
	}

	for _, tt := range tests {
		t.Run(tt.errMsg, func(t *testing.T) {
			result := extractStatusCode(tt.errMsg)
			if result != tt.expected {
				t.Errorf("extractStatusCode(%q) = %d, want %d", tt.errMsg, result, tt.expected)
			}
		})
	}
}

// ========================================
// WrapError Tests
// ========================================

func TestWrapError_NilError(t *testing.T) {
	result := WrapError(nil, "provider", "model", false)
	if result != nil {
		t.Error("expected nil for nil error")
	}
}

func TestWrapError_AlreadyLLMError(t *testing.T) {
	original := &LLMError{
		Err:         ErrProviderError,
		UserMessage: "Original message",
	}

	result := WrapError(original, "provider", "model", false)
	if result != original {
		t.Error("expected same LLMError to be returned")
	}
}

func TestWrapError_RegularError(t *testing.T) {
	err := errors.New("status: 429 rate limit")

	result := WrapError(err, "openrouter", "model:free", false)
	if result == nil {
		t.Fatal("expected LLMError")
	}
	if result.StatusCode != http.StatusTooManyRequests {
		t.Errorf("StatusCode = %d, want %d", result.StatusCode, http.StatusTooManyRequests)
	}
}

// ========================================
// Helper Function Tests
// ========================================

func TestIsRetryable(t *testing.T) {
	retryable := &LLMError{Retryable: true}
	notRetryable := &LLMError{Retryable: false}
	regularErr := errors.New("regular error")

	if !IsRetryable(retryable) {
		t.Error("expected true for retryable LLMError")
	}
	if IsRetryable(notRetryable) {
		t.Error("expected false for non-retryable LLMError")
	}
	if IsRetryable(regularErr) {
		t.Error("expected false for regular error")
	}
}

func TestShouldSuggestUpgrade(t *testing.T) {
	suggest := &LLMError{SuggestUpgrade: true}
	noSuggest := &LLMError{SuggestUpgrade: false}
	regularErr := errors.New("regular error")

	if !ShouldSuggestUpgrade(suggest) {
		t.Error("expected true for SuggestUpgrade=true")
	}
	if ShouldSuggestUpgrade(noSuggest) {
		t.Error("expected false for SuggestUpgrade=false")
	}
	if ShouldSuggestUpgrade(regularErr) {
		t.Error("expected false for regular error")
	}
}

func TestGetUserMessage(t *testing.T) {
	llmErr := &LLMError{UserMessage: "Custom message"}
	regularErr := errors.New("regular error")

	msg := GetUserMessage(llmErr)
	if msg != "Custom message" {
		t.Errorf("GetUserMessage() = %q, want %q", msg, "Custom message")
	}

	msg = GetUserMessage(regularErr)
	if msg != "An unexpected error occurred. Please try again." {
		t.Errorf("GetUserMessage() for regular error = %q", msg)
	}
}

// ========================================
// Tier Error Factory Tests
// ========================================

func TestNewTierQuotaError(t *testing.T) {
	tests := []struct {
		tier           string
		limit          int
		used           int
		expectUpgrade  bool
	}{
		{"free", 50, 50, true},
		{"starter", 1000, 1000, true},
		{"pro", 0, 0, false},
		{"unknown", 100, 100, true},
	}

	for _, tt := range tests {
		t.Run(tt.tier, func(t *testing.T) {
			err := NewTierQuotaError(tt.tier, tt.limit, tt.used)
			if err == nil {
				t.Fatal("expected error")
			}
			if !errors.Is(err.Err, ErrTierQuotaExceeded) {
				t.Error("expected ErrTierQuotaExceeded")
			}
			if err.SuggestUpgrade != tt.expectUpgrade {
				t.Errorf("SuggestUpgrade = %v, want %v", err.SuggestUpgrade, tt.expectUpgrade)
			}
			if err.Tier != tt.tier {
				t.Errorf("Tier = %q, want %q", err.Tier, tt.tier)
			}
			if err.Limit != tt.limit {
				t.Errorf("Limit = %d, want %d", err.Limit, tt.limit)
			}
		})
	}
}

func TestNewTierFeatureError(t *testing.T) {
	tests := []struct {
		tier    string
		feature string
	}{
		{"free", "webhooks"},
		{"free", "byok"},
		{"starter", "antibot"},
		{"free", "custom_feature"},
	}

	for _, tt := range tests {
		t.Run(tt.feature, func(t *testing.T) {
			err := NewTierFeatureError(tt.tier, tt.feature)
			if err == nil {
				t.Fatal("expected error")
			}
			if !errors.Is(err.Err, ErrTierFeatureDisabled) {
				t.Error("expected ErrTierFeatureDisabled")
			}
			if err.Tier != tt.tier {
				t.Errorf("Tier = %q, want %q", err.Tier, tt.tier)
			}
			if !err.SuggestUpgrade {
				t.Error("expected SuggestUpgrade to be true")
			}
		})
	}
}

func TestNewInsufficientCreditsError(t *testing.T) {
	tests := []struct {
		tier          string
		required      int
		available     int
		expectUpgrade bool
	}{
		{"free", 100, 50, true},
		{"starter", 200, 100, true},
		{"pro", 300, 150, false},
	}

	for _, tt := range tests {
		t.Run(tt.tier, func(t *testing.T) {
			err := NewInsufficientCreditsError(tt.tier, tt.required, tt.available)
			if err == nil {
				t.Fatal("expected error")
			}
			if !errors.Is(err.Err, ErrInsufficientCredits) {
				t.Error("expected ErrInsufficientCredits")
			}
			if err.SuggestUpgrade != tt.expectUpgrade {
				t.Errorf("SuggestUpgrade = %v, want %v", err.SuggestUpgrade, tt.expectUpgrade)
			}
		})
	}
}

func TestIsTierQuotaExceeded(t *testing.T) {
	llmErr := &LLMError{Err: ErrTierQuotaExceeded}
	other := &LLMError{Err: ErrProviderError}
	directErr := ErrTierQuotaExceeded

	if !IsTierQuotaExceeded(llmErr) {
		t.Error("expected true for LLMError with ErrTierQuotaExceeded")
	}
	if IsTierQuotaExceeded(other) {
		t.Error("expected false for other error")
	}
	if !IsTierQuotaExceeded(directErr) {
		t.Error("expected true for direct ErrTierQuotaExceeded")
	}
}

func TestIsTierFeatureDisabled(t *testing.T) {
	llmErr := &LLMError{Err: ErrTierFeatureDisabled}
	other := &LLMError{Err: ErrProviderError}
	directErr := ErrTierFeatureDisabled

	if !IsTierFeatureDisabled(llmErr) {
		t.Error("expected true for LLMError with ErrTierFeatureDisabled")
	}
	if IsTierFeatureDisabled(other) {
		t.Error("expected false for other error")
	}
	if !IsTierFeatureDisabled(directErr) {
		t.Error("expected true for direct ErrTierFeatureDisabled")
	}
}
