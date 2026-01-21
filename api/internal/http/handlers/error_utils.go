package handlers

import (
	"errors"
	"net/http"

	"github.com/jmylchreest/refyne-api/internal/llm"
)

// ErrorInfo contains extracted error details for consistent error handling across handlers.
type ErrorInfo struct {
	UserMessage   string // User-visible error message (sanitized for non-BYOK)
	Details       string // Full error details (admin/BYOK only)
	Category      string // Error classification
	Provider      string // LLM provider involved
	Model         string // LLM model involved
	StatusCode    int    // HTTP status code
	IsBYOK        bool   // Whether user's own API key was used
}

// ExtractErrorInfo extracts user message, details, category, provider and model from an error.
func ExtractErrorInfo(err error, isBYOK bool) ErrorInfo {
	info := ErrorInfo{
		IsBYOK: isBYOK,
	}

	var llmErr *llm.LLMError
	if errors.As(err, &llmErr) {
		info.UserMessage = llmErr.UserMessage
		info.Details = llmErr.Error()
		info.Provider = llmErr.Provider
		info.Model = llmErr.Model
		info.Category = CategorizeError(llmErr)
		info.StatusCode = DetermineStatusCode(llmErr)
		return info
	}

	// Generic error
	info.UserMessage = "Operation failed"
	info.Details = err.Error()
	info.Category = "unknown"
	info.StatusCode = http.StatusInternalServerError
	return info
}

// CategorizeError returns an error category string based on the LLM error type.
func CategorizeError(llmErr *llm.LLMError) string {
	switch {
	case errors.Is(llmErr.Err, llm.ErrInvalidAPIKey):
		return "invalid_api_key"
	case errors.Is(llmErr.Err, llm.ErrInsufficientCredits):
		return "insufficient_credits"
	case errors.Is(llmErr.Err, llm.ErrTierQuotaExceeded):
		return "quota_exceeded"
	case errors.Is(llmErr.Err, llm.ErrTierFeatureDisabled):
		return "feature_disabled"
	case errors.Is(llmErr.Err, llm.ErrFreeTierRateLimited):
		return "rate_limited"
	case errors.Is(llmErr.Err, llm.ErrFreeTierQuotaExhausted):
		return "quota_exhausted"
	case errors.Is(llmErr.Err, llm.ErrFreeTierUnavailable):
		return "provider_unavailable"
	case errors.Is(llmErr.Err, llm.ErrModelUnavailable):
		return "model_unavailable"
	case llmErr.Retryable:
		return "provider_error"
	default:
		return "extraction_error"
	}
}

// DetermineStatusCode maps LLM errors to appropriate HTTP status codes.
func DetermineStatusCode(llmErr *llm.LLMError) int {
	switch {
	case errors.Is(llmErr.Err, llm.ErrTierQuotaExceeded):
		return http.StatusTooManyRequests
	case errors.Is(llmErr.Err, llm.ErrTierFeatureDisabled):
		return http.StatusForbidden
	case errors.Is(llmErr.Err, llm.ErrInsufficientCredits):
		return http.StatusPaymentRequired
	case errors.Is(llmErr.Err, llm.ErrFreeTierRateLimited):
		return http.StatusTooManyRequests
	case errors.Is(llmErr.Err, llm.ErrFreeTierQuotaExhausted):
		return http.StatusPaymentRequired
	case errors.Is(llmErr.Err, llm.ErrFreeTierUnavailable):
		return http.StatusServiceUnavailable
	case errors.Is(llmErr.Err, llm.ErrModelUnavailable):
		return http.StatusServiceUnavailable
	case errors.Is(llmErr.Err, llm.ErrInvalidAPIKey):
		return http.StatusUnauthorized
	case llmErr.Retryable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusBadRequest
	}
}

// JobError is a standardized error type for job operations.
// It implements huma.StatusError interface so it can be returned from handlers.
type JobError struct {
	Status        int    `json:"-"`
	Title         string `json:"title,omitempty"`
	Detail        string `json:"detail,omitempty"`
	ErrorCategory string `json:"error_category,omitempty"`
	ErrorDetails  string `json:"error_details,omitempty"` // Only for BYOK users
	IsBYOK        bool   `json:"is_byok,omitempty"`
	Provider      string `json:"provider,omitempty"`
	Model         string `json:"model,omitempty"`
}

func (e *JobError) Error() string {
	return e.Detail
}

func (e *JobError) GetStatus() int {
	return e.Status
}

// NewJobError creates a JobError from an error and BYOK status.
// It extracts LLM-specific details when available and sanitizes output for non-BYOK users.
func NewJobError(err error, isBYOK bool) *JobError {
	info := ExtractErrorInfo(err, isBYOK)

	jobErr := &JobError{
		Status:        info.StatusCode,
		Title:         http.StatusText(info.StatusCode),
		Detail:        info.UserMessage,
		ErrorCategory: info.Category,
		IsBYOK:        isBYOK,
	}

	// Include sensitive details only for BYOK users
	if isBYOK {
		jobErr.ErrorDetails = info.Details
		jobErr.Provider = info.Provider
		jobErr.Model = info.Model
	}

	return jobErr
}
