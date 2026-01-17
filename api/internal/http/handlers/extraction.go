package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/jmylchreest/refyne-api/internal/http/mw"
	"github.com/jmylchreest/refyne-api/internal/llm"
	"github.com/jmylchreest/refyne-api/internal/service"
)

// ExtractionHandler handles extraction endpoints.
type ExtractionHandler struct {
	extractionSvc *service.ExtractionService
	jobSvc        *service.JobService
}

// NewExtractionHandler creates a new extraction handler.
func NewExtractionHandler(extractionSvc *service.ExtractionService, jobSvc *service.JobService) *ExtractionHandler {
	return &ExtractionHandler{
		extractionSvc: extractionSvc,
		jobSvc:        jobSvc,
	}
}

// ExtractInput represents extraction request.
type ExtractInput struct {
	Body struct {
		URL       string          `json:"url" minLength:"1" doc:"URL to extract data from"`
		Schema    json.RawMessage `json:"schema" doc:"Schema defining the data structure to extract (JSON or YAML format)"`
		FetchMode string          `json:"fetch_mode,omitempty" enum:"auto,static,dynamic" default:"auto" doc:"Fetch mode: auto, static, or dynamic"`
		LLMConfig *LLMConfigInput `json:"llm_config,omitempty" doc:"Optional LLM configuration override"`
	}
}

// LLMConfigInput represents LLM config in request.
type LLMConfigInput struct {
	Provider string `json:"provider,omitempty" enum:"anthropic,openai,openrouter,ollama,credits" doc:"LLM provider"`
	APIKey   string `json:"api_key,omitempty" doc:"API key for the provider"`
	BaseURL  string `json:"base_url,omitempty" doc:"Custom base URL (for Ollama)"`
	Model    string `json:"model,omitempty" doc:"Model to use"`
}

// ExtractOutput represents extraction response.
type ExtractOutput struct {
	Body struct {
		JobID     string           `json:"job_id" doc:"Job ID for this extraction (for history/tracking)"`
		Data      any              `json:"data" doc:"Extracted data matching the schema"`
		URL       string           `json:"url" doc:"URL that was extracted"`
		FetchedAt string           `json:"fetched_at" doc:"Timestamp when the page was fetched"`
		Usage     UsageResponse    `json:"usage" doc:"Token usage information"`
		Metadata  MetadataResponse `json:"metadata" doc:"Extraction metadata"`
	}
}

// UsageResponse represents usage info in response.
type UsageResponse struct {
	InputTokens  int     `json:"input_tokens" doc:"Number of input tokens used"`
	OutputTokens int     `json:"output_tokens" doc:"Number of output tokens used"`
	CostUSD      float64 `json:"cost_usd" doc:"Total USD cost charged for this extraction"`
	LLMCostUSD   float64 `json:"llm_cost_usd" doc:"Actual LLM cost from provider"`
	IsBYOK       bool    `json:"is_byok" doc:"True if user's own API key was used (no charge)"`
}

// MetadataResponse represents metadata in response.
type MetadataResponse struct {
	FetchDurationMs   int    `json:"fetch_duration_ms" doc:"Time to fetch the page in milliseconds"`
	ExtractDurationMs int    `json:"extract_duration_ms" doc:"Time to extract data in milliseconds"`
	Model             string `json:"model" doc:"Model used for extraction"`
	Provider          string `json:"provider" doc:"LLM provider used"`
}

// Extract handles single-page extraction.
func (h *ExtractionHandler) Extract(ctx context.Context, input *ExtractInput) (*ExtractOutput, error) {
	userID := getUserID(ctx)
	if userID == "" {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	// Get user's tier from claims for tier-specific fallback chain
	tier := "free"
	if claims := mw.GetUserClaims(ctx); claims != nil && claims.Tier != "" {
		tier = claims.Tier
	}

	var llmCfg *service.LLMConfigInput
	isBYOK := false
	if input.Body.LLMConfig != nil {
		llmCfg = &service.LLMConfigInput{
			Provider: input.Body.LLMConfig.Provider,
			APIKey:   input.Body.LLMConfig.APIKey,
			BaseURL:  input.Body.LLMConfig.BaseURL,
			Model:    input.Body.LLMConfig.Model,
		}
		// If user provides their own API key, it's BYOK
		isBYOK = input.Body.LLMConfig.APIKey != ""
	}

	// Create job record before extraction (for history/tracking)
	var jobID string
	if h.jobSvc != nil {
		jobOutput, err := h.jobSvc.CreateExtractJob(ctx, userID, service.CreateExtractJobInput{
			URL:       input.Body.URL,
			Schema:    input.Body.Schema,
			FetchMode: input.Body.FetchMode,
			IsBYOK:    isBYOK,
		})
		if err != nil {
			// Log but don't fail - job tracking is secondary to extraction
			// Continue without job tracking
		} else {
			jobID = jobOutput.JobID
		}
	}

	// Use ExtractWithContext to pass tier information
	ectx := &service.ExtractContext{
		UserID: userID,
		Tier:   tier,
	}

	result, err := h.extractionSvc.ExtractWithContext(ctx, userID, service.ExtractInput{
		URL:       input.Body.URL,
		Schema:    input.Body.Schema,
		FetchMode: input.Body.FetchMode,
		LLMConfig: llmCfg,
	}, ectx)

	// Update job record with result or error
	if h.jobSvc != nil && jobID != "" {
		if err != nil {
			// Record the failure in job record
			errMsg, errDetails, errCategory, provider, model := extractErrorDetails(err)
			_ = h.jobSvc.FailExtractJob(ctx, jobID, service.FailExtractJobInput{
				ErrorMessage:  errMsg,
				ErrorDetails:  errDetails,
				ErrorCategory: errCategory,
				LLMProvider:   provider,
				LLMModel:      model,
			})
		} else {
			// Record success in job record
			resultJSON, _ := json.Marshal(result.Data)
			_ = h.jobSvc.CompleteExtractJob(ctx, jobID, service.CompleteExtractJobInput{
				ResultJSON:       string(resultJSON),
				PageCount:        1,
				TokenUsageInput:  result.Usage.InputTokens,
				TokenUsageOutput: result.Usage.OutputTokens,
				LLMProvider:      result.Metadata.Provider,
				LLMModel:         result.Metadata.Model,
			})
		}
	}

	if err != nil {
		return nil, handleExtractionError(err, isBYOK)
	}

	return &ExtractOutput{
		Body: struct {
			JobID     string           `json:"job_id" doc:"Job ID for this extraction (for history/tracking)"`
			Data      any              `json:"data" doc:"Extracted data matching the schema"`
			URL       string           `json:"url" doc:"URL that was extracted"`
			FetchedAt string           `json:"fetched_at" doc:"Timestamp when the page was fetched"`
			Usage     UsageResponse    `json:"usage" doc:"Token usage information"`
			Metadata  MetadataResponse `json:"metadata" doc:"Extraction metadata"`
		}{
			JobID:     jobID,
			Data:      result.Data,
			URL:       result.URL,
			FetchedAt: result.FetchedAt.Format("2006-01-02T15:04:05Z07:00"),
			Usage: UsageResponse{
				InputTokens:  result.Usage.InputTokens,
				OutputTokens: result.Usage.OutputTokens,
				CostUSD:      result.Usage.CostUSD,
				LLMCostUSD:   result.Usage.LLMCostUSD,
				IsBYOK:       result.Usage.IsBYOK,
			},
			Metadata: MetadataResponse{
				FetchDurationMs:   result.Metadata.FetchDurationMs,
				ExtractDurationMs: result.Metadata.ExtractDurationMs,
				Model:             result.Metadata.Model,
				Provider:          result.Metadata.Provider,
			},
		},
	}, nil
}

// extractErrorDetails extracts user message, details, category, provider and model from an error.
func extractErrorDetails(err error) (userMsg, details, category, provider, model string) {
	var llmErr *llm.LLMError
	if errors.As(err, &llmErr) {
		userMsg = llmErr.UserMessage
		details = llmErr.Error()
		provider = llmErr.Provider
		model = llmErr.Model
		category = categorizeError(llmErr)
		return
	}
	// Generic error
	userMsg = "Extraction failed"
	details = err.Error()
	category = "unknown"
	return
}

// categorizeError returns an error category string based on the LLM error type.
func categorizeError(llmErr *llm.LLMError) string {
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

// ExtractionError is a custom error type that includes additional context for the frontend.
// It implements huma.StatusError interface so it can be returned from handlers.
type ExtractionError struct {
	Status        int    `json:"-"`
	Title         string `json:"title,omitempty"`
	Detail        string `json:"detail,omitempty"`
	ErrorCategory string `json:"error_category,omitempty"`
	ErrorDetails  string `json:"error_details,omitempty"` // Only for BYOK users
	IsBYOK        bool   `json:"is_byok,omitempty"`
	Provider      string `json:"provider,omitempty"`
	Model         string `json:"model,omitempty"`
}

func (e *ExtractionError) Error() string {
	return e.Detail
}

func (e *ExtractionError) GetStatus() int {
	return e.Status
}

// handleExtractionError converts extraction errors to appropriate HTTP errors with category.
func handleExtractionError(err error, isBYOK bool) error {
	// Check if it's an LLM error
	var llmErr *llm.LLMError
	if errors.As(err, &llmErr) {
		category := categorizeError(llmErr)
		status := determineStatusCode(llmErr)

		extractionErr := &ExtractionError{
			Status:        status,
			Title:         http.StatusText(status),
			Detail:        llmErr.UserMessage,
			ErrorCategory: category,
			IsBYOK:        isBYOK,
		}

		// Include sensitive details only for BYOK users
		if isBYOK {
			extractionErr.ErrorDetails = llmErr.Error()
			extractionErr.Provider = llmErr.Provider
			extractionErr.Model = llmErr.Model
		}

		return extractionErr
	}

	// For non-LLM errors, return a generic error
	return &ExtractionError{
		Status:        http.StatusInternalServerError,
		Title:         "Internal Server Error",
		Detail:        "Extraction failed. Please try again later.",
		ErrorCategory: "unknown",
		IsBYOK:        isBYOK,
	}
}

// determineStatusCode maps LLM errors to appropriate HTTP status codes.
func determineStatusCode(llmErr *llm.LLMError) int {
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
