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

// CleanerConfigInput represents a cleaner in the chain.
type CleanerConfigInput struct {
	Name    string                 `json:"name" minLength:"1" doc:"Cleaner name (noop, refyne)"`
	Options *CleanerOptionsInput   `json:"options,omitempty" doc:"Cleaner-specific options"`
}

// CleanerOptionsInput represents cleaner configuration options for refyne cleaner.
type CleanerOptionsInput struct {
	// Output format
	Output  string `json:"output,omitempty" enum:"html,text,markdown" default:"html" doc:"Output format: html, text, or markdown"`
	BaseURL string `json:"base_url,omitempty" doc:"Base URL for resolving relative links"`

	// Preset and selectors
	Preset          string   `json:"preset,omitempty" enum:"default,minimal,aggressive" doc:"Preset: default, minimal, or aggressive"`
	RemoveSelectors []string `json:"remove_selectors,omitempty" doc:"CSS selectors for elements to remove"`
	KeepSelectors   []string `json:"keep_selectors,omitempty" doc:"CSS selectors for elements to always keep"`

	// Markdown output options
	IncludeFrontmatter bool `json:"include_frontmatter,omitempty" doc:"Prepend YAML frontmatter with metadata (markdown output)"`
	ExtractImages      bool `json:"extract_images,omitempty" doc:"Extract images to frontmatter with {{IMG_001}} placeholders (markdown)"`
	ExtractHeadings    bool `json:"extract_headings,omitempty" doc:"Extract heading structure to frontmatter (markdown)"`
	ResolveURLs        bool `json:"resolve_urls,omitempty" doc:"Resolve relative URLs to absolute using base_url"`
}

// ExtractInput represents extraction request.
type ExtractInput struct {
	Body struct {
		URL          string               `json:"url" minLength:"1" doc:"URL to extract data from"`
		Schema       json.RawMessage      `json:"schema" minLength:"1" doc:"Extraction instructions - either a structured schema (YAML/JSON with 'name' and 'fields') or freeform natural language prompt. The API auto-detects the format and returns 'input_format' in the response."`
		FetchMode    string               `json:"fetch_mode,omitempty" enum:"auto,static,dynamic" default:"auto" doc:"Fetch mode: auto, static, or dynamic"`
		LLMConfig    *LLMConfigInput      `json:"llm_config,omitempty" doc:"Optional LLM configuration override"`
		CleanerChain []CleanerConfigInput `json:"cleaner_chain,omitempty" doc:"Content cleaner chain (default: [markdown])"`
		CaptureDebug bool                 `json:"capture_debug,omitempty" doc:"Enable debug capture to store raw LLM request/response for troubleshooting"`
		WebhookID    string               `json:"webhook_id,omitempty" doc:"ID of a saved webhook to call on completion"`
		Webhook      *InlineWebhookInput  `json:"webhook,omitempty" doc:"Inline ephemeral webhook configuration"`
		WebhookURL   string               `json:"webhook_url,omitempty" format:"uri" doc:"Simple webhook URL (backward compatible)"`
	}
}

// LLMConfigInput represents LLM config in request.
type LLMConfigInput struct {
	Provider       string `json:"provider,omitempty" enum:"anthropic,openai,openrouter,ollama,helicone,credits" doc:"LLM provider"`
	APIKey         string `json:"api_key,omitempty" doc:"API key for the provider"`
	BaseURL        string `json:"base_url,omitempty" doc:"Custom base URL (for Ollama or self-hosted Helicone)"`
	Model          string `json:"model,omitempty" doc:"Model to use"`
	TargetProvider string `json:"target_provider,omitempty" doc:"Underlying provider for Helicone self-hosted mode"`
	TargetAPIKey   string `json:"target_api_key,omitempty" doc:"Underlying provider's API key for Helicone self-hosted mode"`
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
		JobID       string           `json:"job_id" doc:"Job ID for this extraction (for history/tracking)"`
		Data        any              `json:"data" doc:"Extracted data matching the schema"`
		URL         string           `json:"url" doc:"URL that was extracted"`
		FetchedAt   string           `json:"fetched_at" doc:"Timestamp when the page was fetched"`
		InputFormat string           `json:"input_format" doc:"How the input was interpreted: 'schema' (structured YAML/JSON) or 'prompt' (freeform text)"`
		Usage       UsageResponse    `json:"usage" doc:"Token usage information"`
		Metadata    MetadataResponse `json:"metadata" doc:"Extraction metadata"`
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
// Uses the unified JobService.RunJob for consistent job lifecycle management including webhooks.
func (h *ExtractionHandler) Extract(ctx context.Context, input *ExtractInput) (*ExtractOutput, error) {
	// Extract user context from JWT claims
	uc := ExtractUserContext(ctx)
	if !uc.IsAuthenticated() {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	// Validate that schema/prompt input is provided
	if len(input.Body.Schema) == 0 {
		return nil, huma.Error400BadRequest("'schema' is required - provide either a structured schema (YAML/JSON) or freeform extraction instructions")
	}

	// Convert cleaner chain using shared utility
	cleanerChain := ConvertCleanerChain(input.Body.CleanerChain)

	// Build extraction context
	ectx := BuildExtractContext(uc, input.Body.LLMConfig)

	// Convert LLM config
	llmCfg := ConvertLLMConfig(input.Body.LLMConfig)
	isBYOK := IsBYOKFromLLMConfig(input.Body.LLMConfig)

	// Create executor
	executor := service.NewExtractExecutor(h.extractionSvc, service.ExtractInput{
		URL:          input.Body.URL,
		Schema:       input.Body.Schema,
		FetchMode:    input.Body.FetchMode,
		LLMConfig:    llmCfg,
		CleanerChain: cleanerChain,
	}, ectx)

	// Build ephemeral webhook config if provided
	ephemeralWebhook := BuildEphemeralWebhook(input.Body.Webhook, input.Body.WebhookURL)

	// Run job with full lifecycle management (creates job, executes, handles webhooks)
	var jobID string
	var result *service.ExtractOutput

	if h.jobSvc != nil {
		runResult, err := h.jobSvc.RunJob(ctx, executor, &service.RunJobOptions{
			UserID:           uc.UserID,
			Tier:             uc.Tier,
			CaptureDebug:     input.Body.CaptureDebug,
			EphemeralWebhook: ephemeralWebhook,
			WebhookID:        input.Body.WebhookID,
		})
		if err != nil {
			return nil, NewJobError(err, isBYOK)
		}

		// Get job ID from the run result
		jobID = runResult.JobID

		// Extract the full ExtractOutput from the execution result (stored in Data field)
		if runResult.Result != nil && runResult.Result.Data != nil {
			if extractResult, ok := runResult.Result.Data.(*service.ExtractOutput); ok {
				result = extractResult
			}
		}
	}

	// Fallback: direct extraction without job tracking (if jobSvc is nil or result extraction failed)
	if result == nil {
		directResult, directErr := h.extractionSvc.ExtractWithContext(ctx, uc.UserID, service.ExtractInput{
			URL:          input.Body.URL,
			Schema:       input.Body.Schema,
			FetchMode:    input.Body.FetchMode,
			LLMConfig:    llmCfg,
			CleanerChain: cleanerChain,
		}, ectx)
		if directErr != nil {
			return nil, NewJobError(directErr, isBYOK)
		}
		result = directResult
	}

	return &ExtractOutput{
		Body: struct {
			JobID       string           `json:"job_id" doc:"Job ID for this extraction (for history/tracking)"`
			Data        any              `json:"data" doc:"Extracted data matching the schema"`
			URL         string           `json:"url" doc:"URL that was extracted"`
			FetchedAt   string           `json:"fetched_at" doc:"Timestamp when the page was fetched"`
			InputFormat string           `json:"input_format" doc:"How the input was interpreted: 'schema' (structured YAML/JSON) or 'prompt' (freeform text)"`
			Usage       UsageResponse    `json:"usage" doc:"Token usage information"`
			Metadata    MetadataResponse `json:"metadata" doc:"Extraction metadata"`
		}{
			JobID:       jobID,
			Data:        result.Data,
			URL:         result.URL,
			FetchedAt:   result.FetchedAt.Format("2006-01-02T15:04:05Z07:00"),
			InputFormat: string(result.InputFormat),
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
