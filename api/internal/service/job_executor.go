package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jmylchreest/refyne-api/internal/models"
)

// JobExecutor is the core interface for all job types.
// It provides a unified lifecycle for job execution with consistent
// handling of webhooks, billing, error handling, and state management.
type JobExecutor interface {
	// Execute runs the job and returns the result.
	// The executor is responsible for all business logic.
	Execute(ctx context.Context) (*JobExecutionResult, error)

	// JobType returns the type of job (extract, analyze, crawl).
	JobType() models.JobType

	// IsBYOK returns whether the job uses user's own API keys.
	IsBYOK() bool

	// GetURL returns the primary URL for this job.
	GetURL() string

	// GetSchema returns the schema/prompt for this job (may be nil for some job types).
	GetSchema() json.RawMessage

	// SetJobID sets the job ID on the executor after job record creation.
	// This is called by RunJob before Execute to provide tracking context.
	SetJobID(jobID string)
}

// JobExecutionResult contains the output of job execution.
// All job types produce this standardized result.
type JobExecutionResult struct {
	// Core result data
	Data any // Extracted/analyzed data (type varies by job)

	// Token usage and costs
	TokensInput  int
	TokensOutput int
	CostUSD      float64
	LLMCostUSD   float64

	// LLM metadata
	LLMProvider string
	LLMModel    string

	// Execution metadata
	PageCount  int
	IsBYOK     bool
	ResultJSON string // JSON serialized result for storage

	// Debug capture (optional)
	DebugCapture *DebugCaptureData

	// Webhook payload data
	WebhookData map[string]any
}

// DebugCaptureData contains debug information for LLM requests.
type DebugCaptureData struct {
	URL            string
	FetchMode      string
	RawContent     string // Page content sent to LLM
	RawLLMResponse string // Raw LLM output
	Prompt         string
	Schema         string // Schema used for extraction (YAML format)
	DurationMs     int64
}

// RunJobOptions contains options for running a job via JobService.
type RunJobOptions struct {
	// User identification
	UserID string
	Tier   string

	// Job configuration
	JobID        string // Optional: if set, use existing job record
	CaptureDebug bool   // Whether to capture debug information

	// Webhook configuration
	EphemeralWebhook *WebhookConfig // Inline webhook for this request
	WebhookID        string         // ID of saved webhook to use
}

// JobLifecycleManager provides job lifecycle operations.
// This is implemented by JobService to manage jobs.
type JobLifecycleManager interface {
	// RunJob executes a job with full lifecycle management.
	// This is the single entry point for synchronous job types.
	RunJob(ctx context.Context, executor JobExecutor, opts *RunJobOptions) (*JobExecutionResult, error)

	// CreateJob creates a job record without executing it (for async jobs).
	CreateJob(ctx context.Context, executor JobExecutor, opts *RunJobOptions) (*models.Job, error)

	// CompleteJob marks a job as completed with results.
	CompleteJob(ctx context.Context, job *models.Job, result *JobExecutionResult) error

	// FailJob marks a job as failed with error information.
	FailJob(ctx context.Context, job *models.Job, err error, isBYOK bool) error
}

// ErrorInfo contains classified error information.
type ErrorInfo struct {
	UserMessage string // User-visible error message (sanitized for non-BYOK)
	Details     string // Full error details (BYOK only)
	Category    string // Error classification
	Provider    string // LLM provider if applicable
	Model       string // LLM model if applicable
}

// ClassifyError extracts and classifies error information.
// For BYOK users, full details are included. For non-BYOK users, error details are sanitized.
func ClassifyError(err error, isBYOK bool) ErrorInfo {
	if err == nil {
		return ErrorInfo{}
	}

	info := ErrorInfo{
		UserMessage: err.Error(),
		Details:     err.Error(),
		Category:    "unknown",
	}

	// Extract error category from error type if available
	if catErr, ok := err.(interface{ Category() string }); ok {
		info.Category = catErr.Category()
	}

	// Extract provider/model info if available
	if provErr, ok := err.(interface{ Provider() string }); ok {
		info.Provider = provErr.Provider()
	}
	if modelErr, ok := err.(interface{ Model() string }); ok {
		info.Model = modelErr.Model()
	}

	// Sanitize error message for non-BYOK users
	if !isBYOK {
		info.UserMessage = sanitizeErrorMessage(info.UserMessage, info.Category)
		info.Details = "" // Don't expose details to non-BYOK users
	}

	return info
}

// sanitizeErrorMessage removes sensitive information from error messages for non-BYOK users.
func sanitizeErrorMessage(msg string, category string) string {
	switch category {
	case "rate_limit":
		return "Rate limit exceeded. Please try again later."
	case "quota_exceeded":
		return "Quota exceeded. Please upgrade your plan or wait for quota reset."
	case "provider_error":
		return "LLM provider error. Please try again."
	case "invalid_key":
		return "API key validation failed."
	case "context_length":
		return "Content too long for extraction. Try a smaller page or simpler schema."
	case "invalid_response":
		return "Failed to parse LLM response. Please try again."
	case "fetch_error":
		return "Failed to fetch the page. Please check the URL and try again."
	default:
		// Return a generic message for unknown errors
		if len(msg) > 100 {
			return "An error occurred during processing. Please try again."
		}
		return msg
	}
}

// JobStartedEvent is emitted when a job starts running.
type JobStartedEvent struct {
	JobID     string
	JobType   models.JobType
	UserID    string
	URL       string
	StartedAt time.Time
}

// JobCompletedEvent is emitted when a job completes successfully.
type JobCompletedEvent struct {
	JobID       string
	JobType     models.JobType
	UserID      string
	URL         string
	PageCount   int
	CostUSD     float64
	CompletedAt time.Time
	Result      *JobExecutionResult
}

// JobFailedEvent is emitted when a job fails.
type JobFailedEvent struct {
	JobID     string
	JobType   models.JobType
	UserID    string
	URL       string
	Error     ErrorInfo
	FailedAt  time.Time
}
