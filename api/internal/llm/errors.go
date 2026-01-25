// Package llm provides LLM client utilities and error handling.
package llm

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// Error categories for LLM operations.
var (
	// ErrFreeTierUnavailable indicates the OpenRouter free model is experiencing issues.
	ErrFreeTierUnavailable = errors.New("free tier unavailable")

	// ErrFreeTierRateLimited indicates the OpenRouter free model rate limit was hit.
	ErrFreeTierRateLimited = errors.New("free tier rate limited")

	// ErrFreeTierQuotaExhausted indicates the OpenRouter free model quota is exhausted.
	ErrFreeTierQuotaExhausted = errors.New("free tier quota exhausted")

	// ErrModelUnavailable indicates a specific model is unavailable.
	ErrModelUnavailable = errors.New("model unavailable")

	// ErrModelFeatureUnsupported indicates the model doesn't support a required feature
	// (e.g., response_format for structured output).
	ErrModelFeatureUnsupported = errors.New("model feature unsupported")

	// ErrInvalidAPIKey indicates the API key is invalid or expired.
	ErrInvalidAPIKey = errors.New("invalid API key")

	// ErrProviderError indicates a general provider error.
	ErrProviderError = errors.New("provider error")

	// ErrTierQuotaExceeded indicates the user has exceeded their subscription tier's quota.
	// This is different from ErrFreeTierQuotaExhausted which is about OpenRouter's free model limits.
	ErrTierQuotaExceeded = errors.New("tier quota exceeded")

	// ErrTierFeatureDisabled indicates a feature is not available on the user's tier.
	ErrTierFeatureDisabled = errors.New("feature not available on tier")

	// ErrInsufficientCredits indicates the user doesn't have enough credits for the operation.
	ErrInsufficientCredits = errors.New("insufficient credits")

	// ErrNoModelsConfigured indicates no valid LLM models are available in the user's configuration.
	ErrNoModelsConfigured = errors.New("no models configured")
)

// LLMError represents an error from an LLM provider with user-friendly messaging.
type LLMError struct {
	// Original error from the provider
	Err error

	// HTTP status code (if applicable)
	StatusCode int

	// Provider name (openrouter, anthropic, openai, ollama)
	Provider string

	// Model that was being used
	Model string

	// User-friendly message to display
	UserMessage string

	// Raw error message (for admin/BYOK visibility)
	RawMessage string

	// Error category for classification (rate_limit, model_unsupported, etc.)
	Category string

	// Whether this is a retryable error (with same provider/model)
	Retryable bool

	// Whether this error should trigger fallback to next provider
	ShouldFallback bool

	// Whether user should consider upgrading (BYOK or higher tier)
	SuggestUpgrade bool

	// User's subscription tier (free, starter, pro) - for tier quota errors
	Tier string

	// Limit that was exceeded (for quota errors)
	Limit int

	// Current usage (for quota errors)
	Used int
}

func (e *LLMError) Error() string {
	if e.UserMessage != "" {
		return e.UserMessage
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return "unknown LLM error"
}

func (e *LLMError) Unwrap() error {
	return e.Err
}

// IsFreeTierError returns true if this is an OpenRouter free model error.
func (e *LLMError) IsFreeTierError() bool {
	return errors.Is(e.Err, ErrFreeTierUnavailable) ||
		errors.Is(e.Err, ErrFreeTierRateLimited) ||
		errors.Is(e.Err, ErrFreeTierQuotaExhausted)
}

// IsTierQuotaError returns true if this is a subscription tier quota error.
func (e *LLMError) IsTierQuotaError() bool {
	return errors.Is(e.Err, ErrTierQuotaExceeded) ||
		errors.Is(e.Err, ErrTierFeatureDisabled) ||
		errors.Is(e.Err, ErrInsufficientCredits)
}

// IsQuotaError returns true if this is any kind of quota error (tier or LLM).
func (e *LLMError) IsQuotaError() bool {
	return e.IsFreeTierError() || e.IsTierQuotaError()
}

// ClassifyError analyzes an error from an LLM call and returns a classified LLMError.
// This handles OpenRouter-specific errors and other provider errors.
// The isBYOK parameter indicates whether the user is using their own API key - if true,
// they should see actual error details rather than generic "free tier" messages.
func ClassifyError(err error, provider, model string, statusCode int, isBYOK bool) *LLMError {
	if err == nil {
		return nil
	}

	errStr := strings.ToLower(err.Error())
	llmErr := &LLMError{
		Err:        err,
		StatusCode: statusCode,
		Provider:   provider,
		Model:      model,
		RawMessage: err.Error(),
	}

	// Check if using free model WITHOUT BYOK - BYOK users should see actual errors
	// even when using free models, since they're paying for their own usage
	isFreeTier := (strings.Contains(model, ":free") || provider == "credits") && !isBYOK

	// First check error message for specific patterns that override status code classification
	// This is important because 400 Bad Request can mean many different things
	if containsFeatureUnsupported(errStr) {
		llmErr.Err = ErrModelFeatureUnsupported
		llmErr.Category = "model_unsupported"
		llmErr.UserMessage = "This model doesn't support structured output. Trying next model."
		llmErr.Retryable = false
		llmErr.ShouldFallback = true // Try next provider
		return llmErr
	}

	// Classify by HTTP status code
	switch statusCode {
	case http.StatusTooManyRequests: // 429
		llmErr.Category = "rate_limit"
		llmErr.ShouldFallback = true
		if isFreeTier {
			llmErr.Err = ErrFreeTierRateLimited
			llmErr.UserMessage = "The free tier is experiencing high demand. Please try again in a few minutes, or configure your own API key for reliable access."
			llmErr.Retryable = true
			llmErr.SuggestUpgrade = true
		} else {
			llmErr.UserMessage = "Rate limit exceeded. Please wait before retrying."
			llmErr.Retryable = true
		}

	case http.StatusPaymentRequired: // 402
		llmErr.Category = "quota_exceeded"
		llmErr.ShouldFallback = true
		if isFreeTier {
			llmErr.Err = ErrFreeTierQuotaExhausted
			llmErr.UserMessage = "Free tier quota has been exhausted. Configure your own API key to continue using the service."
			llmErr.Retryable = false
			llmErr.SuggestUpgrade = true
		} else {
			llmErr.UserMessage = "Payment required. Please check your API key's billing status."
			llmErr.Retryable = false
		}

	case http.StatusServiceUnavailable: // 503
		llmErr.Category = "provider_error"
		llmErr.ShouldFallback = true
		if isFreeTier {
			llmErr.Err = ErrFreeTierUnavailable
			llmErr.UserMessage = "The free model is temporarily unavailable. Please try again later, or configure your own API key for better reliability."
			llmErr.Retryable = true
			llmErr.SuggestUpgrade = true
		} else {
			llmErr.Err = ErrModelUnavailable
			llmErr.UserMessage = "The model is temporarily unavailable. Please try again later."
			llmErr.Retryable = true
		}

	case http.StatusUnauthorized: // 401
		llmErr.Err = ErrInvalidAPIKey
		llmErr.Category = "invalid_key"
		llmErr.UserMessage = "Invalid API key. Please check your LLM configuration."
		llmErr.Retryable = false
		llmErr.ShouldFallback = false // Don't try other models with same bad key

	case http.StatusBadGateway, http.StatusGatewayTimeout: // 502, 504
		llmErr.Category = "provider_error"
		llmErr.ShouldFallback = true
		if isFreeTier {
			llmErr.Err = ErrFreeTierUnavailable
			llmErr.UserMessage = "The free model backend is experiencing issues. Consider using your own API key for more reliable access."
			llmErr.Retryable = true
			llmErr.SuggestUpgrade = true
		} else {
			llmErr.Err = ErrProviderError
			llmErr.UserMessage = "The LLM provider is experiencing issues. Please try again."
			llmErr.Retryable = true
		}

	case http.StatusBadRequest: // 400
		// 400 errors need further classification by message
		llmErr = classifyByErrorMessage(llmErr, errStr, isFreeTier)

	default:
		// Check error message content for OpenRouter-specific errors
		llmErr = classifyByErrorMessage(llmErr, errStr, isFreeTier)
	}

	return llmErr
}

// containsFeatureUnsupported checks if the error indicates a model feature is unsupported.
func containsFeatureUnsupported(errStr string) bool {
	patterns := []string{
		"response_format is not supported",
		"response_format not supported",
		"structured output not supported",
		"json mode not supported",
		"json_object not supported",
		"does not support response_format",
		"does not support structured",
	}
	for _, p := range patterns {
		if strings.Contains(errStr, p) {
			return true
		}
	}
	return false
}

// classifyByErrorMessage analyzes error message content for specific patterns.
func classifyByErrorMessage(llmErr *LLMError, errStr string, isFreeTier bool) *LLMError {
	// OpenRouter-specific error patterns
	switch {
	case strings.Contains(errStr, "rate limit") || strings.Contains(errStr, "ratelimit"):
		llmErr.Category = "rate_limit"
		llmErr.ShouldFallback = true
		if isFreeTier {
			llmErr.Err = ErrFreeTierRateLimited
			llmErr.UserMessage = "The free tier is rate limited. Please wait a moment and try again, or use your own API key."
			llmErr.Retryable = true
			llmErr.SuggestUpgrade = true
		} else {
			llmErr.UserMessage = "Rate limit exceeded. Please wait before retrying."
			llmErr.Retryable = true
		}

	case strings.Contains(errStr, "overloaded") || strings.Contains(errStr, "capacity"):
		llmErr.Category = "provider_error"
		llmErr.ShouldFallback = true
		if isFreeTier {
			llmErr.Err = ErrFreeTierUnavailable
			llmErr.UserMessage = "Free tier is under heavy load. Try again later or configure your own API key."
			llmErr.Retryable = true
			llmErr.SuggestUpgrade = true
		} else {
			llmErr.Err = ErrModelUnavailable
			llmErr.UserMessage = "Model is overloaded. Please try again later."
			llmErr.Retryable = true
		}

	case strings.Contains(errStr, "model not found") || strings.Contains(errStr, "invalid model"):
		llmErr.Err = ErrModelUnavailable
		llmErr.Category = "model_unsupported"
		llmErr.UserMessage = "The specified model is not available. Please check your LLM configuration."
		llmErr.Retryable = false
		llmErr.ShouldFallback = true

	case strings.Contains(errStr, "invalid api key") || strings.Contains(errStr, "authentication"):
		llmErr.Err = ErrInvalidAPIKey
		llmErr.Category = "invalid_key"
		llmErr.UserMessage = "Invalid API key. Please check your LLM configuration."
		llmErr.Retryable = false
		llmErr.ShouldFallback = false

	case strings.Contains(errStr, "insufficient") && strings.Contains(errStr, "credit"):
		llmErr.Category = "quota_exceeded"
		llmErr.ShouldFallback = true
		if isFreeTier {
			llmErr.Err = ErrFreeTierQuotaExhausted
			llmErr.UserMessage = "Free tier credits exhausted. Configure your own API key to continue."
			llmErr.Retryable = false
			llmErr.SuggestUpgrade = true
		} else {
			llmErr.UserMessage = "Insufficient credits. Please top up your account."
			llmErr.Retryable = false
		}

	case strings.Contains(errStr, "context") && strings.Contains(errStr, "length"):
		llmErr.Err = ErrProviderError
		llmErr.Category = "content_too_long"
		llmErr.UserMessage = "The content is too long for the model. Try with a smaller page or simpler schema."
		llmErr.Retryable = false
		llmErr.ShouldFallback = false // Content issue, not provider issue

	case strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline exceeded"):
		llmErr.Err = ErrProviderError
		llmErr.Category = "timeout"
		llmErr.ShouldFallback = true
		if isFreeTier {
			llmErr.UserMessage = "Request timed out waiting for the free model. Free models can be slow under load. Consider using your own API key for faster, more reliable extractions."
			llmErr.SuggestUpgrade = true
		} else {
			llmErr.UserMessage = "Request timed out. The model took too long to respond. Please try again."
		}
		llmErr.Retryable = true

	default:
		// Generic error
		llmErr.Err = ErrProviderError
		llmErr.Category = "unknown"
		llmErr.ShouldFallback = true // Try next provider for unknown errors
		if isFreeTier {
			llmErr.UserMessage = "An error occurred with the free tier. Consider using your own API key for better reliability."
			llmErr.SuggestUpgrade = true
		} else {
			llmErr.UserMessage = fmt.Sprintf("LLM error: %s", llmErr.Err.Error())
		}
		llmErr.Retryable = false
	}

	return llmErr
}

// WrapError wraps a raw error into an LLMError with classification.
// This is a convenience function for use in services.
// The isBYOK parameter indicates whether the user is using their own API key.
func WrapError(err error, provider, model string, isBYOK bool) *LLMError {
	if err == nil {
		return nil
	}

	// Try to extract status code from error if it's already wrapped
	var llmErr *LLMError
	if errors.As(err, &llmErr) {
		return llmErr
	}

	// Check for HTTP status in error message (common pattern)
	statusCode := extractStatusCode(err.Error())

	return ClassifyError(err, provider, model, statusCode, isBYOK)
}

// extractStatusCode attempts to extract an HTTP status code from an error message.
func extractStatusCode(errMsg string) int {
	// Common patterns: "status 429", "HTTP 503", "returned 502"
	patterns := []struct {
		prefix string
		code   int
	}{
		{"status: 429", http.StatusTooManyRequests},
		{"status: 402", http.StatusPaymentRequired},
		{"status: 401", http.StatusUnauthorized},
		{"status: 503", http.StatusServiceUnavailable},
		{"status: 502", http.StatusBadGateway},
		{"status: 504", http.StatusGatewayTimeout},
		{"status: 500", http.StatusInternalServerError},
		{"429", http.StatusTooManyRequests},
		{"503", http.StatusServiceUnavailable},
	}

	errLower := strings.ToLower(errMsg)
	for _, p := range patterns {
		if strings.Contains(errLower, p.prefix) {
			return p.code
		}
	}

	return 0
}

// IsRetryable returns true if the error is retryable.
func IsRetryable(err error) bool {
	var llmErr *LLMError
	if errors.As(err, &llmErr) {
		return llmErr.Retryable
	}
	return false
}

// ShouldSuggestUpgrade returns true if the error suggests user should upgrade.
func ShouldSuggestUpgrade(err error) bool {
	var llmErr *LLMError
	if errors.As(err, &llmErr) {
		return llmErr.SuggestUpgrade
	}
	return false
}

// GetUserMessage returns a user-friendly message for the error.
func GetUserMessage(err error) string {
	var llmErr *LLMError
	if errors.As(err, &llmErr) {
		return llmErr.UserMessage
	}
	return "An unexpected error occurred. Please try again."
}

// NewTierQuotaError creates an error for when a user exceeds their subscription tier quota.
func NewTierQuotaError(tier string, limit, used int) *LLMError {
	var msg string
	var suggestUpgrade bool

	switch tier {
	case "free":
		msg = fmt.Sprintf("You've reached your free tier limit of %d extractions this month. Upgrade to Starter for 1,000 monthly extractions.", limit)
		suggestUpgrade = true
	case "starter":
		msg = fmt.Sprintf("You've reached your Starter plan limit of %d extractions this month. Upgrade to Pro for unlimited extractions.", limit)
		suggestUpgrade = true
	case "pro":
		// Pro has unlimited, but we still handle it in case limits change
		msg = "You've reached your monthly extraction limit. Please contact support."
		suggestUpgrade = false
	default:
		msg = fmt.Sprintf("You've reached your monthly limit of %d extractions. Please upgrade your plan for more capacity.", limit)
		suggestUpgrade = true
	}

	return &LLMError{
		Err:            ErrTierQuotaExceeded,
		StatusCode:     http.StatusTooManyRequests,
		UserMessage:    msg,
		Retryable:      false,
		SuggestUpgrade: suggestUpgrade,
		Tier:           tier,
		Limit:          limit,
		Used:           used,
	}
}

// NewTierFeatureError creates an error for when a feature is not available on the user's tier.
func NewTierFeatureError(tier, feature string) *LLMError {
	var msg string

	switch feature {
	case "webhooks":
		msg = "Webhooks are not available on the free tier. Upgrade to Starter or Pro to use webhooks."
	case "byok":
		msg = "Bring Your Own Key (BYOK) is not available on the free tier. Upgrade to Starter or Pro to use your own API keys."
	case "antibot":
		msg = "Anti-bot features are only available on the Pro plan. Upgrade to Pro for advanced anti-bot capabilities."
	default:
		msg = fmt.Sprintf("The '%s' feature is not available on your current plan. Please upgrade to access this feature.", feature)
	}

	return &LLMError{
		Err:            ErrTierFeatureDisabled,
		StatusCode:     http.StatusForbidden,
		UserMessage:    msg,
		Retryable:      false,
		SuggestUpgrade: true,
		Tier:           tier,
	}
}

// NewInsufficientCreditsError creates an error for when a user doesn't have enough credits.
func NewInsufficientCreditsError(tier string, required, available int) *LLMError {
	msg := fmt.Sprintf("Insufficient credits for this operation. Required: %d, Available: %d.", required, available)
	if tier == "free" || tier == "starter" {
		msg += " Consider upgrading your plan for more credits."
	}

	return &LLMError{
		Err:            ErrInsufficientCredits,
		StatusCode:     http.StatusPaymentRequired,
		UserMessage:    msg,
		Retryable:      false,
		SuggestUpgrade: tier != "pro",
		Tier:           tier,
	}
}

// IsTierQuotaExceeded checks if an error is a tier quota exceeded error.
func IsTierQuotaExceeded(err error) bool {
	var llmErr *LLMError
	if errors.As(err, &llmErr) {
		return errors.Is(llmErr.Err, ErrTierQuotaExceeded)
	}
	return errors.Is(err, ErrTierQuotaExceeded)
}

// IsTierFeatureDisabled checks if an error is a tier feature disabled error.
func IsTierFeatureDisabled(err error) bool {
	var llmErr *LLMError
	if errors.As(err, &llmErr) {
		return errors.Is(llmErr.Err, ErrTierFeatureDisabled)
	}
	return errors.Is(err, ErrTierFeatureDisabled)
}

// NewNoModelsConfiguredError creates an error for when no valid LLM models are configured.
// This can happen when:
// - User has no models in their fallback chain
// - User's API keys are missing or invalid for configured models
// - System keys are not available for the tier
func NewNoModelsConfiguredError(reason string) *LLMError {
	msg := "No valid LLM models are configured."
	if reason != "" {
		msg = fmt.Sprintf("No valid LLM models are configured: %s", reason)
	}
	msg += " Please add models to your fallback chain or configure API keys in your account settings."

	return &LLMError{
		Err:            ErrNoModelsConfigured,
		StatusCode:     http.StatusUnprocessableEntity, // 422 - request valid but cannot be processed
		UserMessage:    msg,
		Category:       "no_models",
		Retryable:      false,
		ShouldFallback: false, // No models to fall back to
		SuggestUpgrade: true,  // BYOK or higher tier might help
	}
}

// IsNoModelsConfigured checks if an error is a "no models configured" error.
func IsNoModelsConfigured(err error) bool {
	var llmErr *LLMError
	if errors.As(err, &llmErr) {
		return errors.Is(llmErr.Err, ErrNoModelsConfigured)
	}
	return errors.Is(err, ErrNoModelsConfigured)
}
