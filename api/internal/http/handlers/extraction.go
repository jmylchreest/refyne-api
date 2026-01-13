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
}

// NewExtractionHandler creates a new extraction handler.
func NewExtractionHandler(extractionSvc *service.ExtractionService) *ExtractionHandler {
	return &ExtractionHandler{extractionSvc: extractionSvc}
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
	if input.Body.LLMConfig != nil {
		llmCfg = &service.LLMConfigInput{
			Provider: input.Body.LLMConfig.Provider,
			APIKey:   input.Body.LLMConfig.APIKey,
			BaseURL:  input.Body.LLMConfig.BaseURL,
			Model:    input.Body.LLMConfig.Model,
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
	if err != nil {
		return nil, handleExtractionError(err)
	}

	return &ExtractOutput{
		Body: struct {
			Data      any              `json:"data" doc:"Extracted data matching the schema"`
			URL       string           `json:"url" doc:"URL that was extracted"`
			FetchedAt string           `json:"fetched_at" doc:"Timestamp when the page was fetched"`
			Usage     UsageResponse    `json:"usage" doc:"Token usage information"`
			Metadata  MetadataResponse `json:"metadata" doc:"Extraction metadata"`
		}{
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

// handleExtractionError converts extraction errors to appropriate HTTP errors.
func handleExtractionError(err error) error {
	// Check if it's an LLM error
	var llmErr *llm.LLMError
	if errors.As(err, &llmErr) {
		// Map LLM errors to appropriate HTTP status codes
		switch {
		// Tier quota errors (subscription-based limits)
		case errors.Is(llmErr.Err, llm.ErrTierQuotaExceeded):
			return huma.NewError(http.StatusTooManyRequests, llmErr.UserMessage)

		case errors.Is(llmErr.Err, llm.ErrTierFeatureDisabled):
			return huma.NewError(http.StatusForbidden, llmErr.UserMessage)

		case errors.Is(llmErr.Err, llm.ErrInsufficientCredits):
			return huma.NewError(http.StatusPaymentRequired, llmErr.UserMessage)

		// OpenRouter free model errors
		case errors.Is(llmErr.Err, llm.ErrFreeTierRateLimited):
			return huma.NewError(http.StatusTooManyRequests, llmErr.UserMessage)

		case errors.Is(llmErr.Err, llm.ErrFreeTierQuotaExhausted):
			return huma.NewError(http.StatusPaymentRequired, llmErr.UserMessage)

		case errors.Is(llmErr.Err, llm.ErrFreeTierUnavailable):
			return huma.NewError(http.StatusServiceUnavailable, llmErr.UserMessage)

		// General LLM errors
		case errors.Is(llmErr.Err, llm.ErrModelUnavailable):
			return huma.NewError(http.StatusServiceUnavailable, llmErr.UserMessage)

		case errors.Is(llmErr.Err, llm.ErrInvalidAPIKey):
			return huma.NewError(http.StatusUnauthorized, llmErr.UserMessage)

		default:
			// For other LLM errors, use the user message
			if llmErr.Retryable {
				return huma.NewError(http.StatusServiceUnavailable, llmErr.UserMessage)
			}
			return huma.NewError(http.StatusBadRequest, llmErr.UserMessage)
		}
	}

	// For non-LLM errors, return a generic error
	return huma.Error500InternalServerError("extraction failed: " + err.Error())
}
