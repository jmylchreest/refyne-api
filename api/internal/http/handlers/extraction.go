package handlers

import (
	"context"
	"encoding/json"

	"github.com/danielgtaylor/huma/v2"

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
		URL        string          `json:"url" minLength:"1" doc:"URL to extract data from"`
		Schema     json.RawMessage `json:"schema" doc:"Schema defining the data structure to extract (JSON or YAML format)"`
		FetchMode  string          `json:"fetch_mode,omitempty" enum:"auto,static,dynamic" default:"auto" doc:"Fetch mode: auto, static, or dynamic"`
		LLMConfig  *LLMConfigInput `json:"llm_config,omitempty" doc:"Optional LLM configuration override"`
		WebhookID  string          `json:"webhook_id,omitempty" doc:"ID of a saved webhook to call on completion"`
		Webhook    *InlineWebhookInput `json:"webhook,omitempty" doc:"Inline ephemeral webhook configuration"`
		WebhookURL string          `json:"webhook_url,omitempty" format:"uri" doc:"Simple webhook URL (backward compatible)"`
	}
}

// LLMConfigInput represents LLM config in request.
type LLMConfigInput struct {
	Provider string `json:"provider,omitempty" enum:"anthropic,openai,openrouter,ollama,credits" doc:"LLM provider"`
	APIKey   string `json:"api_key,omitempty" doc:"API key for the provider"`
	BaseURL  string `json:"base_url,omitempty" doc:"Custom base URL (for Ollama)"`
	Model    string `json:"model,omitempty" doc:"Model to use"`
}

// WebhookHeaderInput represents a custom header in webhook requests.
type WebhookHeaderInput struct {
	Name  string `json:"name" minLength:"1" maxLength:"256" doc:"Header name"`
	Value string `json:"value" maxLength:"4096" doc:"Header value"`
}

// InlineWebhookInput represents an ephemeral webhook configuration.
type InlineWebhookInput struct {
	URL     string               `json:"url" format:"uri" minLength:"1" doc:"Webhook URL"`
	Secret  string               `json:"secret,omitempty" maxLength:"256" doc:"Secret for HMAC-SHA256 signature"`
	Events  []string             `json:"events,omitempty" doc:"Event types to subscribe to (empty for all)"`
	Headers []WebhookHeaderInput `json:"headers,omitempty" maxItems:"10" doc:"Custom headers"`
}

// WebhookOptions represents webhook configuration for a request.
// Supports three modes:
// 1. webhook_id - reference to a saved webhook
// 2. webhook - inline ephemeral webhook configuration
// 3. webhook_url - simple URL (backward compatible)
type WebhookOptions struct {
	WebhookID  string              `json:"webhook_id,omitempty" doc:"ID of a saved webhook to use"`
	Webhook    *InlineWebhookInput `json:"webhook,omitempty" doc:"Inline ephemeral webhook configuration"`
	WebhookURL string              `json:"webhook_url,omitempty" format:"uri" doc:"Simple webhook URL (backward compatible)"`
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
	// Extract user context from JWT claims
	uc := ExtractUserContext(ctx)
	if !uc.IsAuthenticated() {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	// Convert input LLM config if provided
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
		jobOutput, err := h.jobSvc.CreateExtractJob(ctx, uc.UserID, service.CreateExtractJobInput{
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

	// Use ExtractWithContext to pass tier and feature eligibility information
	ectx := &service.ExtractContext{
		UserID:              uc.UserID,
		Tier:                uc.Tier,
		BYOKAllowed:         uc.BYOKAllowed,
		ModelsCustomAllowed: uc.ModelsCustomAllowed,
	}

	result, err := h.extractionSvc.ExtractWithContext(ctx, uc.UserID, service.ExtractInput{
		URL:       input.Body.URL,
		Schema:    input.Body.Schema,
		FetchMode: input.Body.FetchMode,
		LLMConfig: llmCfg,
	}, ectx)

	// Update job record with result or error
	if h.jobSvc != nil && jobID != "" {
		if err != nil {
			// Record the failure using shared error utilities
			errInfo := ExtractErrorInfo(err, isBYOK)
			_ = FailExtractJobViaService(ctx, h.jobSvc, jobID, JobFailureFromErrorInfo(errInfo))
		} else {
			// Record success using shared job utilities
			resultJSON, _ := json.Marshal(result.Data)
			_ = CompleteExtractJobViaService(ctx, h.jobSvc, jobID, JobCompletionInput{
				ResultJSON:   string(resultJSON),
				PageCount:    1,
				TokensInput:  result.Usage.InputTokens,
				TokensOutput: result.Usage.OutputTokens,
				CostUSD:      result.Usage.CostUSD,
				LLMCostUSD:   result.Usage.LLMCostUSD,
				LLMProvider:  result.Metadata.Provider,
				LLMModel:     result.Metadata.Model,
				IsBYOK:       result.Usage.IsBYOK,
			})
		}
	}

	if err != nil {
		return nil, NewJobError(err, isBYOK)
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
