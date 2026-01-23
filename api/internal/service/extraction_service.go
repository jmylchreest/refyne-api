package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/jmylchreest/refyne/pkg/refyne"
	"github.com/jmylchreest/refyne/pkg/schema"

	"github.com/jmylchreest/refyne-api/internal/config"
	"github.com/jmylchreest/refyne-api/internal/constants"
	"github.com/jmylchreest/refyne-api/internal/crypto"
	"github.com/jmylchreest/refyne-api/internal/llm"
	"github.com/jmylchreest/refyne-api/internal/models"
	"github.com/jmylchreest/refyne-api/internal/repository"
)

// ExtractionService handles synchronous single-page extraction.
type ExtractionService struct {
	cfg       *config.Config
	repos     *repository.Repositories
	billing   *BillingService
	resolver  *LLMConfigResolver
	logger    *slog.Logger
	encryptor *crypto.Encryptor
}

// NewExtractionService creates a new extraction service (legacy constructor).
func NewExtractionService(cfg *config.Config, repos *repository.Repositories, resolver *LLMConfigResolver, encryptor *crypto.Encryptor, logger *slog.Logger) *ExtractionService {
	return NewExtractionServiceWithBilling(cfg, repos, nil, resolver, encryptor, logger)
}

// NewExtractionServiceWithBilling creates an extraction service with billing integration.
func NewExtractionServiceWithBilling(cfg *config.Config, repos *repository.Repositories, billing *BillingService, resolver *LLMConfigResolver, encryptor *crypto.Encryptor, logger *slog.Logger) *ExtractionService {
	return &ExtractionService{
		cfg:       cfg,
		repos:     repos,
		billing:   billing,
		resolver:  resolver,
		logger:    logger,
		encryptor: encryptor,
	}
}

// getStrictMode determines if a model supports strict JSON schema mode.
// Delegates to the resolver which uses cached capabilities when available.
func (s *ExtractionService) getStrictMode(ctx context.Context, provider, model string, chainStrictMode *bool) bool {
	if s.resolver != nil {
		return s.resolver.GetStrictMode(ctx, provider, model, chainStrictMode)
	}
	// Fall back to static defaults if resolver not set
	if chainStrictMode != nil {
		return *chainStrictMode
	}
	_, _, strictMode := llm.GetModelSettings(provider, model, nil, nil, nil)
	return strictMode
}

// ExtractInput represents extraction input.
type ExtractInput struct {
	URL          string           `json:"url"`
	Schema       json.RawMessage  `json:"schema"`
	FetchMode    string           `json:"fetch_mode,omitempty"`
	LLMConfig    *LLMConfigInput  `json:"llm_config,omitempty"`
	CleanerChain []CleanerConfig  `json:"cleaner_chain,omitempty"` // Content cleaner chain: [{name: "trafilatura", options: {...}}, {name: "markdown"}]
}

// LLMConfigInput represents user-provided LLM configuration.
type LLMConfigInput struct {
	Provider   string `json:"provider,omitempty"`
	APIKey     string `json:"api_key,omitempty"`
	BaseURL    string `json:"base_url,omitempty"`
	Model      string `json:"model,omitempty"`
	StrictMode bool   `json:"strict_mode,omitempty"` // Whether to use strict JSON schema mode
}

// ExtractOutput represents extraction output.
type ExtractOutput struct {
	Data       any         `json:"data"`
	URL        string      `json:"url"`
	FetchedAt  time.Time   `json:"fetched_at"`
	Usage      UsageInfo   `json:"usage"`
	Metadata   ExtractMeta `json:"metadata"`
	RawContent string      `json:"-"` // Raw page content (not serialized, for debug capture only)
}

// UsageInfo represents token usage and cost information.
type UsageInfo struct {
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`      // Total charged to user (LLM cost + markup)
	LLMCostUSD   float64 `json:"llm_cost_usd"`  // Actual LLM cost from OpenRouter
	IsBYOK       bool    `json:"is_byok"`       // True if user's own API key was used (no charge)
}

// ExtractMeta represents extraction metadata.
type ExtractMeta struct {
	FetchDurationMs   int    `json:"fetch_duration_ms"`
	ExtractDurationMs int    `json:"extract_duration_ms"`
	Model             string `json:"model"`
	Provider          string `json:"provider"`
}

// ExtractContext holds context for billing tracking.
type ExtractContext struct {
	UserID              string
	Tier                string // From JWT claims
	SchemaID            string
	IsBYOK              bool
	BYOKAllowed         bool   // From JWT claims - whether user has the "provider_byok" feature
	ModelsCustomAllowed bool   // From JWT claims - whether user has the "models_custom" feature
	LLMProvider         string // For S3 API keys: forced LLM provider (bypasses fallback chain)
	LLMModel            string // For S3 API keys: forced LLM model (bypasses fallback chain)
}

// Extract performs a single-page extraction.
func (s *ExtractionService) Extract(ctx context.Context, userID string, input ExtractInput) (*ExtractOutput, error) {
	return s.ExtractWithContext(ctx, userID, input, nil)
}

// ExtractWithContext performs extraction with additional billing context.
// Uses the fallback chain - tries each configured LLM in order until one succeeds.
func (s *ExtractionService) ExtractWithContext(ctx context.Context, userID string, input ExtractInput, ectx *ExtractContext) (*ExtractOutput, error) {
	startTime := time.Now()

	// Initialize extraction context if not provided
	if ectx == nil {
		ectx = &ExtractContext{
			UserID: userID,
			Tier:   "free", // Default tier
		}
	}

	s.logger.Debug("ExtractWithContext starting",
		"user_id", userID,
		"tier", ectx.Tier,
		"llm_provider", ectx.LLMProvider,
		"llm_model", ectx.LLMModel,
	)

	// Parse schema first (same for all attempts)
	// Try JSON first, then YAML if JSON fails
	sch, err := parseSchema(input.Schema)
	if err != nil {
		s.logger.Error("schema parsing failed", "error", err)
		return nil, err // Return SchemaError directly for proper 400 response
	}

	// Create or get schema snapshot for tracking (if billing is enabled)
	if s.billing != nil {
		schemaSnapshot, err := s.billing.GetOrCreateSchemaSnapshot(ctx, userID, string(input.Schema))
		if err != nil {
			s.logger.Warn("failed to create schema snapshot", "error", err)
		} else if schemaSnapshot != nil {
			ectx.SchemaID = schemaSnapshot.ID
		}
	}

	// Get the list of LLM configs to try (using tier-specific chain)
	// BYOKAllowed comes from JWT claims - this is the authoritative check for BYOK eligibility
	// ModelsCustomAllowed controls whether user's custom model chain is used
	// LLMProvider/LLMModel from S3 API keys bypass the entire fallback chain
	llmConfigs, isBYOK := s.resolveLLMConfigsWithFallback(ctx, userID, input.LLMConfig, ectx.Tier, ectx.BYOKAllowed, ectx.ModelsCustomAllowed, ectx.LLMProvider, ectx.LLMModel)
	ectx.IsBYOK = isBYOK

	s.logger.Debug("LLM configs resolved",
		"config_count", len(llmConfigs),
		"is_byok", isBYOK,
	)

	// Estimate cost (triggers pricing cache refresh if needed) and check balance for non-BYOK
	if s.billing != nil && len(llmConfigs) > 0 {
		estimatedCost := s.billing.EstimateCost(1, llmConfigs[0].Model, llmConfigs[0].Provider)
		s.logger.Debug("pre-flight cost estimate",
			"user_id", userID,
			"provider", llmConfigs[0].Provider,
			"model", llmConfigs[0].Model,
			"estimated_cost_usd", estimatedCost,
			"is_byok", isBYOK,
		)
		// Only check balance for non-BYOK users
		if !isBYOK {
			if err := s.billing.CheckSufficientBalance(ctx, userID, estimatedCost); err != nil {
				return nil, err
			}
		}
	}

	// Try each config in the chain until one succeeds (no retries on same model)
	var lastErr error
	var lastLLMErr *llm.LLMError
	var lastCfg *LLMConfigInput

	for providerIdx, llmCfg := range llmConfigs {
		lastCfg = llmCfg

		s.logger.Info("extraction attempt",
			"user_id", userID,
			"url", input.URL,
			"provider", llmCfg.Provider,
			"model", llmCfg.Model,
			"provider_idx", providerIdx+1,
			"of_providers", len(llmConfigs),
			"is_byok", isBYOK,
		)

		// Create refyne instance with configured cleaner
		r, _, err := s.createRefyneInstance(llmCfg, input.CleanerChain)
		if err != nil {
			s.logger.Warn("failed to create LLM instance",
				"provider", llmCfg.Provider,
				"model", llmCfg.Model,
				"error", err,
			)
			lastErr = err
			lastLLMErr = llm.WrapError(err, llmCfg.Provider, llmCfg.Model, isBYOK)
			continue // Try next provider in chain
		}

		// Perform extraction
		result, err := r.Extract(ctx, input.URL, sch)
		_ = r.Close()

		// Check for success
		if err == nil && result != nil && result.Error == nil {
			// Success! Handle billing and return
			return s.handleSuccessfulExtraction(ctx, userID, input, ectx, llmCfg, result, isBYOK, startTime)
		}

		// Extraction failed - classify the error
		if err != nil {
			lastErr = err
		} else if result != nil && result.Error != nil {
			lastErr = result.Error
		}

		lastLLMErr = llm.WrapError(lastErr, llmCfg.Provider, llmCfg.Model, isBYOK)

		s.logger.Warn("extraction failed",
			"provider", llmCfg.Provider,
			"model", llmCfg.Model,
			"error", lastErr,
			"category", lastLLMErr.Category,
			"should_fallback", lastLLMErr.ShouldFallback,
		)

		// If the error suggests we shouldn't try other providers, stop the chain
		if !lastLLMErr.ShouldFallback {
			s.logger.Info("error does not suggest fallback, stopping provider chain",
				"provider", llmCfg.Provider,
				"category", lastLLMErr.Category,
			)
			break
		}

		// Brief delay before trying next provider
		s.logger.Info("falling back to next provider",
			"provider", llmCfg.Provider,
			"category", lastLLMErr.Category,
		)
		time.Sleep(constants.ProviderFallbackDelay)
	}

	// All attempts failed - record the failure with detailed error info
	if lastErr != nil {
		s.recordFailedExtractionWithDetails(ctx, userID, input, ectx, lastCfg, isBYOK, lastErr, lastLLMErr, len(llmConfigs), startTime)
		return nil, s.handleLLMError(lastErr, lastCfg, isBYOK)
	}

	return nil, fmt.Errorf("extraction failed: no LLM providers configured")
}

// recordFailedExtractionWithDetails records usage for a failed extraction with detailed error info.
func (s *ExtractionService) recordFailedExtractionWithDetails(
	ctx context.Context,
	userID string,
	input ExtractInput,
	ectx *ExtractContext,
	llmCfg *LLMConfigInput,
	isBYOK bool,
	err error,
	llmErr *llm.LLMError,
	totalRetries int,
	startTime time.Time,
) {
	if s.billing == nil {
		return
	}

	var byokProvider string
	if isBYOK {
		byokProvider = llmCfg.Provider
	}

	// Determine error message based on BYOK status
	var errorMessage string
	var errorCode string
	if llmErr != nil {
		errorCode = llmErr.Category
		if isBYOK {
			// BYOK users see full error details
			errorMessage = llmErr.RawMessage
		} else {
			// Non-BYOK users see sanitized message
			errorMessage = llmErr.UserMessage
		}
	} else {
		errorMessage = err.Error()
	}

	usageRecord := &UsageRecord{
		UserID:          userID,
		JobType:         models.JobTypeExtract,
		Status:          "failed",
		TotalChargedUSD: 0, // No charge for failed attempts
		IsBYOK:          isBYOK,
		TargetURL:       input.URL,
		SchemaID:        ectx.SchemaID,
		ErrorMessage:    errorMessage,
		ErrorCode:       errorCode,
		LLMProvider:     llmCfg.Provider,
		LLMModel:        llmCfg.Model,
		BYOKProvider:    byokProvider,
		PagesAttempted:  totalRetries,
		PagesSuccessful: 0,
		TotalDurationMs: int(time.Since(startTime).Milliseconds()),
	}

	// Use detached context - we want to record usage even if request timed out
	if recordErr := s.billing.RecordUsage(context.WithoutCancel(ctx), usageRecord); recordErr != nil {
		s.logger.Warn("failed to record failed extraction usage", "error", recordErr)
	}
}

// handleSuccessfulExtraction processes a successful extraction result.
func (s *ExtractionService) handleSuccessfulExtraction(
	ctx context.Context,
	userID string,
	input ExtractInput,
	ectx *ExtractContext,
	llmCfg *LLMConfigInput,
	result *refyne.Result,
	isBYOK bool,
	startTime time.Time,
) (*ExtractOutput, error) {
	// Handle billing (charge and record usage)
	var billingResult *ChargeForUsageResult
	if s.billing != nil {
		billingResult, _ = s.billing.ChargeForUsage(ctx, &ChargeForUsageInput{
			UserID:            userID,
			Tier:              ectx.Tier,
			JobType:           models.JobTypeExtract,
			IsBYOK:            isBYOK,
			TokensInput:       result.TokenUsage.InputTokens,
			TokensOutput:      result.TokenUsage.OutputTokens,
			Model:             llmCfg.Model,
			Provider:          llmCfg.Provider,
			APIKey:            llmCfg.APIKey,
			GenerationID:      result.GenerationID,
			TargetURL:         input.URL,
			SchemaID:          ectx.SchemaID,
			PagesAttempted:    1,
			PagesSuccessful:   1,
			FetchDurationMs:   int(result.FetchDuration.Milliseconds()),
			ExtractDurationMs: int(result.ExtractDuration.Milliseconds()),
			TotalDurationMs:   int(time.Since(startTime).Milliseconds()),
		})
	}

	// Prepare usage info for response
	usageInfo := UsageInfo{
		InputTokens:  result.TokenUsage.InputTokens,
		OutputTokens: result.TokenUsage.OutputTokens,
		IsBYOK:       isBYOK,
	}
	if billingResult != nil {
		usageInfo.CostUSD = billingResult.TotalCostUSD
		usageInfo.LLMCostUSD = billingResult.LLMCostUSD
	}

	s.logger.Info("extraction completed",
		"user_id", userID,
		"url", input.URL,
		"provider", llmCfg.Provider,
		"model", llmCfg.Model,
		"input_tokens", result.TokenUsage.InputTokens,
		"output_tokens", result.TokenUsage.OutputTokens,
		"llm_cost_usd", usageInfo.LLMCostUSD,
		"user_cost_usd", usageInfo.CostUSD,
		"is_byok", isBYOK,
	)

	// Apply shared post-extraction processing (URL resolution, etc.)
	processedData := s.processExtractionResult(result.Data, result.URL)

	return &ExtractOutput{
		Data:       processedData,
		URL:        result.URL,
		FetchedAt:  result.FetchedAt,
		Usage:      usageInfo,
		RawContent: result.RawContent,
		Metadata: ExtractMeta{
			FetchDurationMs:   int(result.FetchDuration.Milliseconds()),
			ExtractDurationMs: int(result.ExtractDuration.Milliseconds()),
			Model:             result.Model,
			Provider:          result.Provider,
		},
	}, nil
}

// resolveLLMConfigsWithFallback returns the list of LLM configs to try, in order.
// Returns (configs, isBYOK) where isBYOK is true if user provided their own API key.
// Delegates to LLMConfigResolver for the feature matrix implementation.
//
// If injectedProvider and injectedModel are set (from S3 API key config), they bypass
// the entire fallback chain and use system keys for that specific provider/model.
func (s *ExtractionService) resolveLLMConfigsWithFallback(ctx context.Context, userID string, override *LLMConfigInput, tier string, byokAllowed bool, modelsCustomAllowed bool, injectedProvider, injectedModel string) ([]*LLMConfigInput, bool) {
	// Check for injected LLM config from S3 API keys first - this bypasses the entire fallback chain
	if injectedProvider != "" && injectedModel != "" {
		s.logger.Info("using injected LLM config from S3 API key",
			"user_id", userID,
			"provider", injectedProvider,
			"model", injectedModel,
		)

		// Get system keys to use with the injected provider
		config := &LLMConfigInput{
			Provider:   injectedProvider,
			Model:      injectedModel,
			StrictMode: s.getStrictMode(ctx, injectedProvider, injectedModel, nil),
		}

		// Use system keys for the specified provider
		if s.resolver != nil {
			serviceKeys := s.resolver.GetServiceKeys(ctx)
			switch injectedProvider {
			case "openrouter":
				config.APIKey = serviceKeys.OpenRouterKey
			case "anthropic":
				config.APIKey = serviceKeys.AnthropicKey
			case "openai":
				config.APIKey = serviceKeys.OpenAIKey
			case "ollama":
				// Ollama doesn't require an API key
			}
		}

		return []*LLMConfigInput{config}, false // Not BYOK - using system keys
	}

	// Delegate to resolver for all standard cases
	if s.resolver != nil {
		return s.resolver.ResolveConfigs(ctx, userID, override, tier, byokAllowed, modelsCustomAllowed)
	}

	// Fallback if resolver not set (shouldn't happen in normal operation)
	s.logger.Warn("resolver not set, using hardcoded defaults")
	return s.getHardcodedDefaultChain(ctx), false
}

// CrawlInput represents crawl input.
type CrawlInput struct {
	JobID        string            `json:"job_id,omitempty"`        // Job ID for logging/tracking
	URL          string            `json:"url"`
	SeedURLs     []string          `json:"seed_urls,omitempty"`     // Additional seed URLs (from sitemap discovery)
	Schema       json.RawMessage   `json:"schema"`
	Options      CrawlOptions      `json:"options"`
	LLMConfigs   []*LLMConfigInput `json:"llm_configs,omitempty"`   // Pre-resolved LLM config chain
	Tier         string            `json:"tier,omitempty"`          // User's subscription tier at job creation time
	IsBYOK       bool              `json:"is_byok,omitempty"`       // Whether using user's own API keys
	CleanerChain []CleanerConfig   `json:"cleaner_chain,omitempty"` // Content cleaner chain
}

// Note: CrawlOptions is defined in job_service.go to avoid duplication

// PageResult represents an individual page result from a crawl.
type PageResult struct {
	URL               string  `json:"url"`
	ParentURL         *string `json:"parent_url,omitempty"` // URL that linked to this page (nil for seed)
	Depth             int     `json:"depth"`                // Distance from seed URL (0 for seed)
	Data              any     `json:"data,omitempty"`
	Error             string  `json:"error,omitempty"`           // User-visible error (sanitized)
	ErrorDetails      string  `json:"error_details,omitempty"`   // Full error (admin/BYOK only)
	ErrorCategory     string  `json:"error_category,omitempty"`  // Error classification
	LLMProvider       string  `json:"llm_provider,omitempty"`    // Provider used
	LLMModel          string  `json:"llm_model,omitempty"`       // Model used
	GenerationID      string  `json:"generation_id,omitempty"`   // Provider generation ID for cost tracking
	IsBYOK            bool    `json:"is_byok"`                   // True if user's own key
	RetryCount        int     `json:"retry_count"`               // Number of retries
	TokenUsageInput   int     `json:"token_usage_input"`
	TokenUsageOutput  int     `json:"token_usage_output"`
	FetchDurationMs   int     `json:"fetch_duration_ms,omitempty"`
	ExtractDurationMs int     `json:"extract_duration_ms,omitempty"`
	RawContent        string  `json:"-"` // Raw page content (not serialized, for debug capture only)
}

// CrawlResult represents the result of a crawl operation.
type CrawlResult struct {
	Results           []any        `json:"results"`      // Aggregated data (backward compat)
	PageResults       []PageResult `json:"page_results"` // Individual page results for SSE streaming
	PageCount         int          `json:"page_count"`
	TotalTokensInput  int          `json:"total_tokens_input"`
	TotalTokensOutput int          `json:"total_tokens_output"`
	TotalCostUSD      float64      `json:"total_cost_usd"`     // Actual USD cost charged to user
	TotalLLMCostUSD   float64      `json:"total_llm_cost_usd"` // Actual LLM provider cost
	LLMProvider       string       `json:"llm_provider"`       // LLM provider used
	LLMModel          string       `json:"llm_model"`          // LLM model used
	StoppedEarly      bool         `json:"stopped_early"`      // True if crawl terminated before completion
	StopReason        string       `json:"stop_reason"`        // Reason for early stop (e.g., "insufficient_balance")
}

// Crawl performs a multi-page crawl extraction.
func (s *ExtractionService) Crawl(ctx context.Context, userID string, input CrawlInput) (*CrawlResult, error) {
	// Use pre-resolved LLM configs from input
	if len(input.LLMConfigs) == 0 {
		return nil, fmt.Errorf("no LLM configs provided")
	}
	llmCfg := input.LLMConfigs[0]
	isBYOK := input.IsBYOK

	// Parse schema (supports both JSON and YAML)
	sch, err := parseSchema(input.Schema)
	if err != nil {
		return nil, fmt.Errorf("invalid schema: %w", err)
	}

	// Enrich cleaner chain with crawl selectors as keep selectors
	// This ensures elements matching FollowSelector/NextSelector are preserved during cleaning
	enrichedCleanerChain := EnrichCleanerChainWithCrawlSelectors(
		input.CleanerChain,
		input.Options.FollowSelector,
		input.Options.NextSelector,
	)

	// Create refyne instance with configured cleaner
	r, cleanerName, err := s.createRefyneInstance(llmCfg, enrichedCleanerChain)
	if err != nil {
		return nil, s.handleLLMError(err, llmCfg, isBYOK)
	}
	defer func() { _ = r.Close() }()

	s.logger.Info("crawl starting",
		"job_id", input.JobID,
		"user_id", userID,
		"url", input.URL,
		"cleaner", cleanerName,
		"provider", llmCfg.Provider,
		"model", llmCfg.Model,
	)

	// Build crawl options
	crawlOpts := s.buildCrawlOptions(input.Options)

	// Perform crawl - Crawl returns a channel of results
	results := r.Crawl(ctx, input.URL, sch, crawlOpts...)

	// Aggregate results
	// Track URLs we've seen to determine depth (seed URL = depth 0, others = depth 1+)
	seedURL := input.URL
	seenURLs := make(map[string]int) // URL -> depth
	seenURLs[seedURL] = 0

	var (
		data              []any
		pageResults       []PageResult
		totalTokensInput  int
		totalTokensOutput int
		pageCount         int
		lastError         error
	)

	for result := range results {
		// Determine depth and parent URL based on whether this is the seed
		var parentURL *string
		depth := 0
		if result.URL != seedURL {
			// Non-seed URL - set parent to seed and depth based on discovery order
			parentURL = &seedURL
			// If we haven't seen this URL yet, assign next depth level
			if existingDepth, seen := seenURLs[result.URL]; seen {
				depth = existingDepth
			} else {
				// URLs discovered during crawl get depth = 1 (directly linked from seed)
				// Note: refyne doesn't expose actual depth, so we approximate
				depth = 1
				seenURLs[result.URL] = depth
			}
		}

		if result.Error != nil {
			// Log with job/user/model/provider/cleaner info for debugging
			s.logger.Warn("crawl page error",
				"job_id", input.JobID,
				"user_id", userID,
				"url", result.URL,
				"cleaner", cleanerName,
				"provider", llmCfg.Provider,
				"model", llmCfg.Model,
				"error", result.Error,
			)
			lastError = result.Error

			// Wrap error for user-friendly messaging
			llmErr := llm.WrapError(result.Error, llmCfg.Provider, llmCfg.Model, isBYOK)
			userMsg := result.Error.Error()
			category := "unknown"
			if llmErr != nil {
				userMsg = llmErr.UserMessage
				category = llmErr.Category
			}
			// Still track failed pages for SSE streaming and crawl map
			pageResults = append(pageResults, PageResult{
				URL:           result.URL,
				ParentURL:     parentURL,
				Depth:         depth,
				Error:         userMsg,
				ErrorDetails:  result.Error.Error(), // Full error for BYOK/admin
				ErrorCategory: category,
				LLMProvider:   llmCfg.Provider,
				LLMModel:      llmCfg.Model,
				IsBYOK:        isBYOK,
			})
			continue
		}

		// Calculate per-page cost for logging - use actual cost from provider when available
		var pageLLMCost, pageUserCost float64
		if s.billing != nil {
			pageLLMCost = s.billing.GetActualCostFromProvider(ctx, llmCfg.Provider, result.GenerationID, llmCfg.APIKey, result.TokenUsage.InputTokens, result.TokenUsage.OutputTokens, llmCfg.Model)
			pageUserCost, _, _ = s.billing.CalculateTotalCost(pageLLMCost, input.Tier, isBYOK)
		}

		// Log successful extraction
		s.logger.Info("extracted",
			"job_id", input.JobID,
			"user_id", userID,
			"url", result.URL,
			"cleaner", cleanerName,
			"provider", llmCfg.Provider,
			"model", llmCfg.Model,
			"fetch_ms", result.FetchDuration.Milliseconds(),
			"extract_ms", result.ExtractDuration.Milliseconds(),
			"input_tokens", result.TokenUsage.InputTokens,
			"output_tokens", result.TokenUsage.OutputTokens,
			"llm_cost_usd", pageLLMCost,
			"user_cost_usd", pageUserCost,
			"is_byok", isBYOK,
		)

		// Apply shared post-extraction processing (URL resolution, etc.)
		processedData := s.processExtractionResult(result.Data, result.URL)

		data = append(data, processedData)
		pageResults = append(pageResults, PageResult{
			URL:               result.URL,
			ParentURL:         parentURL,
			Depth:             depth,
			Data:              processedData,
			TokenUsageInput:   result.TokenUsage.InputTokens,
			TokenUsageOutput:  result.TokenUsage.OutputTokens,
			FetchDurationMs:   int(result.FetchDuration.Milliseconds()),
			ExtractDurationMs: int(result.ExtractDuration.Milliseconds()),
			LLMProvider:       result.Provider,
			LLMModel:          result.Model,
			RawContent:        result.RawContent,
		})
		totalTokensInput += result.TokenUsage.InputTokens
		totalTokensOutput += result.TokenUsage.OutputTokens
		pageCount++
	}

	// If no results and we have an error, return the error
	if pageCount == 0 && lastError != nil {
		return nil, s.handleLLMError(lastError, llmCfg, isBYOK)
	}

	// Calculate actual costs
	var llmCostUSD, userCostUSD float64
	if s.billing != nil {
		llmCostUSD = s.billing.GetActualCost(ctx, totalTokensInput, totalTokensOutput, llmCfg.Model, llmCfg.Provider)
		userCostUSD, _, _ = s.billing.CalculateTotalCost(llmCostUSD, input.Tier, isBYOK)
	}

	s.logger.Info("crawl completed",
		"job_id", input.JobID,
		"user_id", userID,
		"url", input.URL,
		"cleaner", cleanerName,
		"provider", llmCfg.Provider,
		"model", llmCfg.Model,
		"page_count", pageCount,
		"total_input_tokens", totalTokensInput,
		"total_output_tokens", totalTokensOutput,
		"llm_cost_usd", llmCostUSD,
		"user_cost_usd", userCostUSD,
	)

	return &CrawlResult{
		Results:           data,
		PageResults:       pageResults,
		PageCount:         pageCount,
		TotalTokensInput:  totalTokensInput,
		TotalTokensOutput: totalTokensOutput,
		TotalCostUSD:      userCostUSD,
		TotalLLMCostUSD:   llmCostUSD,
		LLMProvider:       llmCfg.Provider,
		LLMModel:          llmCfg.Model,
		StoppedEarly:      false, // Simple Crawl doesn't have mid-crawl balance check
		StopReason:        "",
	}, nil
}

// CrawlResultCallback is called for each page result during a crawl.
// Return an error to stop the crawl early.
type CrawlResultCallback func(result PageResult) error

// URLsQueuedCallback is called when URLs are discovered and queued for processing.
// The count parameter is the total number of URLs currently queued.
type URLsQueuedCallback func(queuedCount int)

// CrawlCallbacks holds callbacks for crawl events.
type CrawlCallbacks struct {
	// OnResult is called for each page result (success or failure).
	// Return an error to stop the crawl early.
	OnResult CrawlResultCallback

	// OnURLsQueued is called when URLs are discovered and queued.
	// This is useful for progress tracking when the total is not known upfront.
	OnURLsQueued URLsQueuedCallback
}

// CrawlWithCallback performs a multi-page crawl extraction with callbacks for events.
// This allows incremental processing of results (e.g., saving to database as they come in)
// and tracking of URL discovery for progress reporting.
func (s *ExtractionService) CrawlWithCallback(ctx context.Context, userID string, input CrawlInput, callbacks CrawlCallbacks) (*CrawlResult, error) {
	// Use pre-resolved LLM configs from input
	if len(input.LLMConfigs) == 0 {
		return nil, fmt.Errorf("no LLM configs provided")
	}
	llmCfg := input.LLMConfigs[0]
	isBYOK := input.IsBYOK

	// Get available balance for non-BYOK users (for mid-crawl balance enforcement)
	var availableBalance float64
	var checkBalance bool
	var err error
	if !isBYOK && s.billing != nil {
		checkBalance = true
		availableBalance, err = s.billing.GetAvailableBalance(ctx, userID)
		if err != nil {
			s.logger.Warn("failed to get available balance, skipping mid-crawl balance check",
				"user_id", userID,
				"error", err,
			)
			checkBalance = false
		}
	}

	// Create a cancellable context so we can stop the crawler if balance is exhausted
	crawlCtx, cancelCrawl := context.WithCancel(ctx)
	defer cancelCrawl()

	// Build seed URLs list - use SeedURLs if provided (from sitemap), otherwise just the main URL
	seedURLs := input.SeedURLs
	if len(seedURLs) == 0 {
		seedURLs = []string{input.URL}
	}

	// Parse schema (supports both JSON and YAML)
	sch, err := parseSchema(input.Schema)
	if err != nil {
		return nil, fmt.Errorf("invalid schema: %w", err)
	}

	// Enrich cleaner chain with crawl selectors as keep selectors
	// This ensures elements matching FollowSelector/NextSelector are preserved during cleaning
	enrichedCleanerChain := EnrichCleanerChainWithCrawlSelectors(
		input.CleanerChain,
		input.Options.FollowSelector,
		input.Options.NextSelector,
	)

	// Create refyne instance with configured cleaner
	r, cleanerName, err := s.createRefyneInstance(llmCfg, enrichedCleanerChain)
	if err != nil {
		return nil, s.handleLLMError(err, llmCfg, isBYOK)
	}
	defer func() { _ = r.Close() }()

	s.logger.Info("crawl starting",
		"job_id", input.JobID,
		"user_id", userID,
		"url", input.URL,
		"seed_count", len(seedURLs),
		"cleaner", cleanerName,
		"provider", llmCfg.Provider,
		"model", llmCfg.Model,
	)

	// Build crawl options
	crawlOpts := s.buildCrawlOptions(input.Options)
	crawlOpts = s.addURLsQueuedCallback(crawlOpts, callbacks.OnURLsQueued)

	s.logger.Debug("initiating crawler",
		"job_id", input.JobID,
		"seed_count", len(seedURLs),
		"options", fmt.Sprintf("%+v", input.Options),
	)

	// Perform crawl - use CrawlMany for multiple seeds (from sitemap), single Crawl otherwise
	// Use crawlCtx so we can cancel if balance is exhausted
	var results <-chan *refyne.Result
	if len(seedURLs) > 1 {
		results = r.CrawlMany(crawlCtx, seedURLs, sch, crawlOpts...)
	} else {
		results = r.Crawl(crawlCtx, input.URL, sch, crawlOpts...)
	}

	s.logger.Debug("crawler channel ready, waiting for results",
		"job_id", input.JobID,
	)

	// Aggregate results while calling callback for each
	seedURL := input.URL
	seenURLs := make(map[string]int)
	for _, url := range seedURLs {
		seenURLs[url] = 0 // All seed URLs start at depth 0
	}

	var (
		data              []any
		pageResults       []PageResult
		totalTokensInput  int
		totalTokensOutput int
		pageCount         int
		lastError         error
		cumulativeCostUSD float64 // Running total of costs (for balance tracking)
		stoppedEarly      bool    // True if we stopped due to balance or other limit
		stopReason        string  // Reason for early stop
	)

	for result := range results {
		// Determine depth and parent URL based on whether this is the seed
		var parentURL *string
		depth := 0
		if result.URL != seedURL {
			parentURL = &seedURL
			if existingDepth, seen := seenURLs[result.URL]; seen {
				depth = existingDepth
			} else {
				depth = 1
				seenURLs[result.URL] = depth
			}
		}

		var pageResult PageResult
		if result.Error != nil {
			// Log with job/user/model/provider/cleaner info for debugging
			s.logger.Warn("crawl page error",
				"job_id", input.JobID,
				"user_id", userID,
				"url", result.URL,
				"cleaner", cleanerName,
				"provider", llmCfg.Provider,
				"model", llmCfg.Model,
				"error", result.Error,
			)
			lastError = result.Error

			// Wrap error for user-friendly messaging
			llmErr := llm.WrapError(result.Error, llmCfg.Provider, llmCfg.Model, isBYOK)
			userMsg := result.Error.Error()
			category := "unknown"
			if llmErr != nil {
				userMsg = llmErr.UserMessage
				category = llmErr.Category
			}
			pageResult = PageResult{
				URL:           result.URL,
				ParentURL:     parentURL,
				Depth:         depth,
				Error:         userMsg,
				ErrorDetails:  result.Error.Error(), // Full error for BYOK/admin
				ErrorCategory: category,
				LLMProvider:   llmCfg.Provider,
				LLMModel:      llmCfg.Model,
				IsBYOK:        isBYOK,
			}
		} else {
			// Calculate per-page cost for logging - use actual cost from provider when available
			var pageLLMCost, pageUserCost float64
			if s.billing != nil {
				pageLLMCost = s.billing.GetActualCostFromProvider(ctx, llmCfg.Provider, result.GenerationID, llmCfg.APIKey, result.TokenUsage.InputTokens, result.TokenUsage.OutputTokens, llmCfg.Model)
				pageUserCost, _, _ = s.billing.CalculateTotalCost(pageLLMCost, input.Tier, isBYOK)
			}

			// Track cumulative cost for balance enforcement
			cumulativeCostUSD += pageUserCost

			// Log successful extraction
			s.logger.Info("extracted",
				"job_id", input.JobID,
				"user_id", userID,
				"url", result.URL,
				"cleaner", cleanerName,
				"provider", llmCfg.Provider,
				"model", llmCfg.Model,
				"fetch_ms", result.FetchDuration.Milliseconds(),
				"extract_ms", result.ExtractDuration.Milliseconds(),
				"input_tokens", result.TokenUsage.InputTokens,
				"output_tokens", result.TokenUsage.OutputTokens,
				"llm_cost_usd", pageLLMCost,
				"user_cost_usd", pageUserCost,
				"cumulative_cost_usd", cumulativeCostUSD,
				"is_byok", isBYOK,
			)

			// Apply shared post-extraction processing (URL resolution, etc.)
			processedData := s.processExtractionResult(result.Data, result.URL)

			data = append(data, processedData)
			pageResult = PageResult{
				URL:               result.URL,
				ParentURL:         parentURL,
				Depth:             depth,
				Data:              processedData,
				TokenUsageInput:   result.TokenUsage.InputTokens,
				TokenUsageOutput:  result.TokenUsage.OutputTokens,
				FetchDurationMs:   int(result.FetchDuration.Milliseconds()),
				ExtractDurationMs: int(result.ExtractDuration.Milliseconds()),
				LLMProvider:       result.Provider,
				LLMModel:          result.Model,
				GenerationID:      result.GenerationID,
				RawContent:        result.RawContent,
			}
			totalTokensInput += result.TokenUsage.InputTokens
			totalTokensOutput += result.TokenUsage.OutputTokens
			pageCount++

			// Check if we have enough balance for the next page (non-BYOK only)
			// Use the cost of this page as estimate for the next page
			if checkBalance && pageUserCost > 0 {
				remainingBalance := availableBalance - cumulativeCostUSD
				if remainingBalance < pageUserCost {
					s.logger.Warn("insufficient balance for next page, stopping crawl",
						"job_id", input.JobID,
						"user_id", userID,
						"available_balance", availableBalance,
						"cumulative_cost", cumulativeCostUSD,
						"remaining_balance", remainingBalance,
						"estimated_next_page_cost", pageUserCost,
						"pages_completed", pageCount,
					)
					stoppedEarly = true
					stopReason = "insufficient_balance"
					cancelCrawl() // Stop the crawler
					// Continue processing to drain any remaining results from the channel
				}
			}
		}

		pageResults = append(pageResults, pageResult)

		// Call the result callback
		if callbacks.OnResult != nil {
			if err := callbacks.OnResult(pageResult); err != nil {
				s.logger.Error("callback error, stopping crawl", "error", err)
				stoppedEarly = true
				stopReason = "callback_error"
				break
			}
		}
	}

	// If no results and we have an error, return the error
	if pageCount == 0 && lastError != nil {
		return nil, s.handleLLMError(lastError, llmCfg, isBYOK)
	}

	// Calculate actual costs
	var llmCostUSD, userCostUSD float64
	if s.billing != nil {
		llmCostUSD = s.billing.GetActualCost(ctx, totalTokensInput, totalTokensOutput, llmCfg.Model, llmCfg.Provider)
		userCostUSD, _, _ = s.billing.CalculateTotalCost(llmCostUSD, input.Tier, isBYOK)
	}

	s.logger.Info("crawl completed",
		"job_id", input.JobID,
		"user_id", userID,
		"url", input.URL,
		"cleaner", cleanerName,
		"provider", llmCfg.Provider,
		"model", llmCfg.Model,
		"page_count", pageCount,
		"total_input_tokens", totalTokensInput,
		"total_output_tokens", totalTokensOutput,
		"llm_cost_usd", llmCostUSD,
		"user_cost_usd", userCostUSD,
		"stopped_early", stoppedEarly,
		"stop_reason", stopReason,
	)

	return &CrawlResult{
		Results:           data,
		PageResults:       pageResults,
		PageCount:         pageCount,
		TotalTokensInput:  totalTokensInput,
		TotalTokensOutput: totalTokensOutput,
		TotalCostUSD:      userCostUSD,
		TotalLLMCostUSD:   llmCostUSD,
		LLMProvider:       llmCfg.Provider,
		LLMModel:          llmCfg.Model,
		StoppedEarly:      stoppedEarly,
		StopReason:        stopReason,
	}, nil
}

// createRefyneInstance creates a new refyne instance with the given LLM config.
// Returns the refyne instance and the cleaner chain name for logging.
// The cleanerChain parameter specifies which cleaners to use (empty uses default).
func (s *ExtractionService) createRefyneInstance(llmCfg *LLMConfigInput, cleanerChain []CleanerConfig) (*refyne.Refyne, string, error) {
	// Create cleaner chain from factory, using default if not specified
	factory := NewCleanerFactory()
	contentCleaner, err := factory.CreateChainWithDefault(cleanerChain, DefaultExtractionCleanerChain)
	if err != nil {
		return nil, "", fmt.Errorf("invalid cleaner chain: %w", err)
	}

	// Get chain name for logging
	chainName := GetChainName(cleanerChain)
	if len(cleanerChain) == 0 {
		chainName = GetChainName(DefaultExtractionCleanerChain)
	}

	opts := []refyne.Option{
		refyne.WithProvider(llmCfg.Provider),
		refyne.WithCleaner(contentCleaner),
		refyne.WithTimeout(llm.LLMTimeout), // 120s timeout for LLM requests
		refyne.WithStrictMode(llmCfg.StrictMode),
		refyne.WithLogger(s.logger), // Inject our logger into refyne
	}

	if llmCfg.APIKey != "" {
		opts = append(opts, refyne.WithAPIKey(llmCfg.APIKey))
	}
	if llmCfg.BaseURL != "" {
		opts = append(opts, refyne.WithBaseURL(llmCfg.BaseURL))
	}
	if llmCfg.Model != "" {
		opts = append(opts, refyne.WithModel(llmCfg.Model))
	}

	r, err := refyne.New(opts...)
	if err != nil {
		return nil, "", err
	}
	return r, chainName, nil
}

// addURLsQueuedCallback adds the OnURLsQueued callback to crawl options.
func (s *ExtractionService) addURLsQueuedCallback(opts []refyne.CrawlOption, callback URLsQueuedCallback) []refyne.CrawlOption {
	if callback == nil {
		return opts
	}
	return append(opts, refyne.WithOnURLsQueued(callback))
}

// buildCrawlOptions converts CrawlOptions to refyne.CrawlOption slice.
func (s *ExtractionService) buildCrawlOptions(opts CrawlOptions) []refyne.CrawlOption {
	crawlOpts := []refyne.CrawlOption{}

	if opts.FollowSelector != "" {
		crawlOpts = append(crawlOpts, refyne.WithFollowSelector(opts.FollowSelector))
	}
	if opts.FollowPattern != "" {
		crawlOpts = append(crawlOpts, refyne.WithFollowPattern(opts.FollowPattern))
	}
	if opts.MaxDepth > 0 {
		crawlOpts = append(crawlOpts, refyne.WithMaxDepth(opts.MaxDepth))
	}
	if opts.NextSelector != "" {
		crawlOpts = append(crawlOpts, refyne.WithNextSelector(opts.NextSelector))
	}
	if opts.MaxPages > 0 {
		crawlOpts = append(crawlOpts, refyne.WithMaxPages(opts.MaxPages))
	}
	if opts.MaxURLs > 0 {
		crawlOpts = append(crawlOpts, refyne.WithMaxURLs(opts.MaxURLs))
	}
	if opts.Delay != "" {
		if d, err := time.ParseDuration(opts.Delay); err == nil {
			crawlOpts = append(crawlOpts, refyne.WithDelay(d))
		}
	}
	if opts.Concurrency > 0 {
		crawlOpts = append(crawlOpts, refyne.WithConcurrency(opts.Concurrency))
	}
	if opts.SameDomainOnly {
		crawlOpts = append(crawlOpts, refyne.WithSameDomainOnly(true))
	}
	if opts.ExtractFromSeeds {
		crawlOpts = append(crawlOpts, refyne.WithExtractFromSeeds(true))
	}

	return crawlOpts
}

// handleLLMError wraps an LLM error with user-friendly messaging.
// The isBYOK parameter indicates whether the user is using their own API key.
func (s *ExtractionService) handleLLMError(err error, llmCfg *LLMConfigInput, isBYOK bool) error {
	llmErr := llm.WrapError(err, llmCfg.Provider, llmCfg.Model, isBYOK)

	// Log the detailed error
	s.logger.Error("LLM error",
		"error", err.Error(),
		"provider", llmCfg.Provider,
		"model", llmCfg.Model,
		"user_message", llmErr.UserMessage,
		"retryable", llmErr.Retryable,
		"suggest_upgrade", llmErr.SuggestUpgrade,
		"is_byok", isBYOK,
	)

	return llmErr
}

// getHardcodedDefaultChain returns the default fallback chain when no custom chain is configured.
//
// getHardcodedDefaultChain returns the hardcoded fallback chain when resolver isn't available.
// This is a minimal fallback that should rarely be used in production.
func (s *ExtractionService) getHardcodedDefaultChain(ctx context.Context) []*LLMConfigInput {
	// If resolver is available, use its method which has access to service keys
	if s.resolver != nil {
		return s.resolver.GetDefaultConfigsForTier(ctx, "")
	}

	// Ultimate fallback: just Ollama (no API key needed)
	return []*LLMConfigInput{{
		Provider:   "ollama",
		Model:      "llama3.2",
		StrictMode: s.getStrictMode(ctx, "ollama", "llama3.2", nil),
	}}
}

// getDefaultLLMConfig returns the first available LLM configuration (for backward compatibility).
func (s *ExtractionService) getDefaultLLMConfig(ctx context.Context) *LLMConfigInput {
	if s.resolver != nil {
		return s.resolver.GetDefaultConfig(ctx, "")
	}
	// Fallback if resolver not set
	configs := s.getHardcodedDefaultChain(ctx)
	if len(configs) > 0 {
		return configs[0]
	}
	return &LLMConfigInput{
		Provider:   "ollama",
		Model:      "llama3.2",
		StrictMode: s.getStrictMode(ctx, "ollama", "llama3.2", nil),
	}
}

// ========================================
// Shared Post-Extraction Processing
// ========================================

// processExtractionResult applies common post-processing to raw extraction results.
// This is the single place where URL resolution and any other post-processing happens.
// All extraction paths (single-page Extract, Crawl, CrawlWithCallback) should use this.
func (s *ExtractionService) processExtractionResult(rawData any, pageURL string) any {
	// 1. Resolve relative URLs to absolute
	resolved := ResolveRelativeURLs(rawData, pageURL)

	// 2. Future: any other post-processing (sanitization, validation, etc.)

	return resolved
}

// ========================================
// Helper Methods
// ========================================

// SchemaError represents a schema parsing error.
// This is a user-facing error that should be returned as 400 Bad Request.
type SchemaError struct {
	Message string
}

func (e *SchemaError) Error() string {
	return e.Message
}

// parseSchema attempts to parse schema data as JSON first, then YAML if JSON fails.
// This allows the API to accept schemas in either format.
// Also handles JSON string-wrapped schemas (e.g., from jq --arg).
func parseSchema(data []byte) (schema.Schema, error) {
	// If data looks like a JSON string (starts with "), unwrap it first
	// This handles cases where the schema was sent as a JSON string value
	if len(data) >= 2 && data[0] == '"' {
		var unwrapped string
		if err := json.Unmarshal(data, &unwrapped); err == nil {
			data = []byte(unwrapped)
		}
	}

	// Try JSON first (most common)
	sch, jsonErr := schema.FromJSON(data)
	if jsonErr == nil {
		return sch, nil
	}

	// If JSON fails, try YAML
	sch, yamlErr := schema.FromYAML(data)
	if yamlErr == nil {
		return sch, nil
	}

	// Both failed - build a helpful error message
	return schema.Schema{}, &SchemaError{
		Message: formatSchemaError(data, jsonErr, yamlErr),
	}
}

// formatSchemaError creates a user-friendly error message for schema parsing failures.
func formatSchemaError(data []byte, jsonErr, yamlErr error) string {
	// Check for schema validation errors (field-level issues)
	var validationErr schema.ValidationError
	if errors.As(jsonErr, &validationErr) || errors.As(yamlErr, &validationErr) {
		if validationErr.Field != "" {
			return fmt.Sprintf("schema validation failed: field '%s' - %s", validationErr.Field, validationErr.Message)
		}
		return fmt.Sprintf("schema validation failed: %s", validationErr.Message)
	}

	// Check for JSON syntax errors
	var syntaxErr *json.SyntaxError
	if errors.As(jsonErr, &syntaxErr) {
		// Find the line number from the offset
		line, col := findPosition(data, int(syntaxErr.Offset))
		return fmt.Sprintf("invalid JSON syntax at line %d, column %d", line, col)
	}

	// Check for JSON type errors
	var typeErr *json.UnmarshalTypeError
	if errors.As(jsonErr, &typeErr) {
		if typeErr.Field != "" {
			return fmt.Sprintf("invalid type for field '%s': expected %s", typeErr.Field, typeErr.Type.String())
		}
		return fmt.Sprintf("invalid type: expected %s", typeErr.Type.String())
	}

	// For YAML errors, try to extract line info from the error message
	yamlErrStr := yamlErr.Error()
	if strings.Contains(yamlErrStr, "line") {
		// YAML errors often include "line X:" - extract just that part
		if idx := strings.Index(yamlErrStr, "line"); idx >= 0 {
			// Find the end of the line reference
			endIdx := strings.Index(yamlErrStr[idx:], ":")
			if endIdx > 0 && endIdx < 20 {
				return fmt.Sprintf("invalid YAML syntax at %s", yamlErrStr[idx:idx+endIdx])
			}
		}
	}

	// Check if schema is empty
	if len(bytes.TrimSpace(data)) == 0 {
		return "schema cannot be empty"
	}

	// Check if it looks like neither JSON nor YAML
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && trimmed[0] != '{' && trimmed[0] != '[' && !isYAMLStart(trimmed) {
		return "invalid schema format: must be valid JSON object or YAML"
	}

	// Generic fallback
	return "invalid schema format: failed to parse as JSON or YAML"
}

// findPosition calculates line and column from byte offset.
func findPosition(data []byte, offset int) (line, col int) {
	line = 1
	col = 1
	for i := 0; i < offset && i < len(data); i++ {
		if data[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return line, col
}

// isYAMLStart checks if data looks like it could be YAML.
func isYAMLStart(data []byte) bool {
	// YAML typically starts with a key (word followed by colon) or a dash for lists
	if len(data) == 0 {
		return false
	}
	// Check for common YAML starters
	if data[0] == '-' || data[0] == '#' {
		return true
	}
	// Check for "key:" pattern
	for i := 0; i < len(data) && i < 50; i++ {
		if data[i] == ':' {
			return true
		}
		if data[i] == '\n' {
			break
		}
	}
	return false
}

// ResolveRelativeURLs recursively walks through extracted data and resolves
// any relative URLs against the base page URL. This ensures all URL fields
// in the extraction results are absolute URLs.
//
// Common URL field names that are resolved:
// - url, link, href
// - image_url, image, img_url, thumbnail_url
// - source_url, page_url
func ResolveRelativeURLs(data any, baseURL string) any {
	if data == nil || baseURL == "" {
		return data
	}

	parsedBase, err := url.Parse(baseURL)
	if err != nil {
		return data // Can't parse base URL, return as-is
	}

	return resolveURLsRecursive(data, parsedBase)
}

// resolveURLsRecursive recursively processes data structures to resolve URLs.
func resolveURLsRecursive(data any, baseURL *url.URL) any {
	switch v := data.(type) {
	case map[string]any:
		result := make(map[string]any, len(v))
		for key, val := range v {
			if isURLField(key) {
				if strVal, ok := val.(string); ok {
					result[key] = resolveURL(strVal, baseURL)
				} else {
					result[key] = val
				}
			} else {
				result[key] = resolveURLsRecursive(val, baseURL)
			}
		}
		return result

	case []any:
		result := make([]any, len(v))
		for i, item := range v {
			result[i] = resolveURLsRecursive(item, baseURL)
		}
		return result

	default:
		return data
	}
}

// isURLField returns true if the field name typically contains a URL.
func isURLField(fieldName string) bool {
	name := strings.ToLower(fieldName)
	urlFields := []string{
		"url", "link", "href",
		"image_url", "image", "img_url", "img", "thumbnail_url", "thumbnail",
		"source_url", "page_url", "canonical_url",
		"video_url", "audio_url", "media_url",
		"profile_url", "avatar_url",
	}
	for _, field := range urlFields {
		if name == field {
			return true
		}
	}
	// Also match fields ending in _url or _link
	return strings.HasSuffix(name, "_url") || strings.HasSuffix(name, "_link")
}

// resolveURL resolves a potentially relative URL against a base URL.
func resolveURL(rawURL string, baseURL *url.URL) string {
	if rawURL == "" {
		return rawURL
	}

	// Already absolute (has scheme)
	if strings.HasPrefix(rawURL, "http://") || strings.HasPrefix(rawURL, "https://") {
		return rawURL
	}

	// Protocol-relative URL
	if strings.HasPrefix(rawURL, "//") {
		return baseURL.Scheme + ":" + rawURL
	}

	// Parse and resolve against base
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL // Can't parse, return as-is
	}

	resolved := baseURL.ResolveReference(parsed)
	return resolved.String()
}
