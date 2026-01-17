package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/refyne/refyne/pkg/cleaner"
	"github.com/refyne/refyne/pkg/refyne"
	"github.com/refyne/refyne/pkg/schema"

	"github.com/jmylchreest/refyne-api/internal/config"
	"github.com/jmylchreest/refyne-api/internal/constants"
	"github.com/jmylchreest/refyne-api/internal/crypto"
	"github.com/jmylchreest/refyne-api/internal/llm"
	"github.com/jmylchreest/refyne-api/internal/models"
	"github.com/jmylchreest/refyne-api/internal/repository"
)

// ExtractionService handles synchronous single-page extraction.
type ExtractionService struct {
	cfg        *config.Config
	repos      *repository.Repositories
	billing    *BillingService
	logger     *slog.Logger
	encryptor  *crypto.Encryptor
}

// NewExtractionService creates a new extraction service (legacy constructor).
func NewExtractionService(cfg *config.Config, repos *repository.Repositories, logger *slog.Logger) *ExtractionService {
	return NewExtractionServiceWithBilling(cfg, repos, nil, logger)
}

// NewExtractionServiceWithBilling creates an extraction service with billing integration.
func NewExtractionServiceWithBilling(cfg *config.Config, repos *repository.Repositories, billing *BillingService, logger *slog.Logger) *ExtractionService {
	// Create encryptor for decrypting stored API keys
	encryptor, err := crypto.NewEncryptor(cfg.EncryptionKey)
	if err != nil {
		logger.Error("failed to create encryptor for extraction service", "error", err)
		// Continue without encryptor - will fail gracefully if needed
	}

	return &ExtractionService{
		cfg:       cfg,
		repos:     repos,
		billing:   billing,
		logger:    logger,
		encryptor: encryptor,
	}
}

// ExtractInput represents extraction input.
type ExtractInput struct {
	URL       string          `json:"url"`
	Schema    json.RawMessage `json:"schema"`
	FetchMode string          `json:"fetch_mode,omitempty"`
	LLMConfig *LLMConfigInput `json:"llm_config,omitempty"`
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
	Data      any         `json:"data"`
	URL       string      `json:"url"`
	FetchedAt time.Time   `json:"fetched_at"`
	Usage     UsageInfo   `json:"usage"`
	Metadata  ExtractMeta `json:"metadata"`
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
	UserID   string
	Tier     string // From JWT claims
	SchemaID string
	IsBYOK   bool
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

	// Parse schema first (same for all attempts)
	// Try JSON first, then YAML if JSON fails
	sch, err := parseSchema(input.Schema)
	if err != nil {
		return nil, fmt.Errorf("invalid schema: %w", err)
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
	llmConfigs, isBYOK := s.resolveLLMConfigsWithFallback(ctx, userID, input.LLMConfig, ectx.Tier)
	ectx.IsBYOK = isBYOK

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

		// Create refyne instance
		r, _, err := s.createRefyneInstance(llmCfg)
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

	return &ExtractOutput{
		Data:      result.Data,
		URL:       result.URL,
		FetchedAt: result.FetchedAt,
		Usage:     usageInfo,
		Metadata: ExtractMeta{
			FetchDurationMs:   int(result.FetchDuration.Milliseconds()),
			ExtractDurationMs: int(result.ExtractDuration.Milliseconds()),
			Model:             result.Model,
			Provider:          result.Provider,
		},
	}, nil
}

// resolveLLMConfigsWithFallback returns the list of LLM configs to try, in order.
// Returns (configs, isBYOK) where isBYOK is true if user provided their own config.
// The tier parameter is used to select a tier-specific fallback chain if configured.
func (s *ExtractionService) resolveLLMConfigsWithFallback(ctx context.Context, userID string, override *LLMConfigInput, tier string) ([]*LLMConfigInput, bool) {
	// If override provided with provider, use it directly (no fallback)
	if override != nil && override.Provider != "" {
		return []*LLMConfigInput{override}, true
	}

	// Check if user has a custom fallback chain configured
	configs := s.buildUserFallbackChain(ctx, userID)
	if len(configs) > 0 {
		// Log which providers/models are in the user's chain
		providerModels := make([]string, len(configs))
		for i, c := range configs {
			providerModels[i] = c.Provider + ":" + c.Model
		}
		s.logger.Info("using user fallback chain (BYOK)",
			"user_id", userID,
			"entries", len(configs),
			"chain", providerModels,
		)
		return configs, true
	}
	s.logger.Debug("no user fallback chain found, checking legacy config",
		"user_id", userID,
	)

	// Check user's legacy saved config (single provider)
	savedCfg, err := s.repos.LLMConfig.GetByUserID(ctx, userID)
	if err != nil {
		s.logger.Warn("failed to get user LLM config", "user_id", userID, "error", err)
	}

	if savedCfg != nil && savedCfg.Provider != "" {
		// Decrypt API key if present
		var apiKey string
		if savedCfg.APIKeyEncrypted != "" && s.encryptor != nil {
			decrypted, err := s.encryptor.Decrypt(savedCfg.APIKeyEncrypted)
			if err != nil {
				s.logger.Warn("failed to decrypt API key", "user_id", userID, "error", err)
			} else {
				apiKey = decrypted
			}
		}

		// User has BYOK config, use it directly (no fallback)
		return []*LLMConfigInput{{
			Provider: savedCfg.Provider,
			APIKey:   apiKey,
			BaseURL:  savedCfg.BaseURL,
			Model:    savedCfg.Model,
		}}, true
	}

	// No user config - use the tier-specific fallback chain
	s.logger.Info("using system fallback chain",
		"user_id", userID,
		"tier", tier,
	)
	return s.getDefaultLLMConfigsForTier(tier), false
}

// buildUserFallbackChain builds the LLM config list from user's fallback chain and service keys.
// Returns empty slice if user has no chain configured or no valid keys.
func (s *ExtractionService) buildUserFallbackChain(ctx context.Context, userID string) []*LLMConfigInput {
	if s.repos.UserFallbackChain == nil || s.repos.UserServiceKey == nil {
		s.logger.Debug("user fallback chain repos not initialized", "user_id", userID)
		return nil
	}

	// Get user's enabled fallback chain entries
	chain, err := s.repos.UserFallbackChain.GetEnabledByUserID(ctx, userID)
	if err != nil {
		s.logger.Warn("failed to get user fallback chain", "user_id", userID, "error", err)
		return nil
	}
	if len(chain) == 0 {
		s.logger.Debug("user has no enabled fallback chain entries", "user_id", userID)
		return nil
	}
	s.logger.Debug("found user fallback chain entries",
		"user_id", userID,
		"count", len(chain),
	)

	// Get user's service keys
	keys, err := s.repos.UserServiceKey.GetEnabledByUserID(ctx, userID)
	if err != nil {
		s.logger.Warn("failed to get user service keys", "user_id", userID, "error", err)
		return nil
	}
	s.logger.Debug("found user service keys",
		"user_id", userID,
		"count", len(keys),
	)

	// Build a map of provider -> decrypted key
	keyMap := make(map[string]*models.UserServiceKey)
	for _, k := range keys {
		keyMap[k.Provider] = k
	}

	// Build configs from chain, only including entries where we have a valid key
	var configs []*LLMConfigInput
	for _, entry := range chain {
		key, ok := keyMap[entry.Provider]
		if !ok {
			// No key for this provider, skip
			s.logger.Debug("skipping chain entry - no key configured",
				"user_id", userID,
				"provider", entry.Provider,
			)
			continue
		}

		// Ollama doesn't require an API key
		var apiKey string
		if entry.Provider != "ollama" {
			if key.APIKeyEncrypted == "" {
				s.logger.Debug("skipping chain entry - no API key",
					"user_id", userID,
					"provider", entry.Provider,
				)
				continue
			}
			// Decrypt the API key
			if s.encryptor != nil {
				decrypted, err := s.encryptor.Decrypt(key.APIKeyEncrypted)
				if err != nil {
					s.logger.Warn("failed to decrypt user API key",
						"user_id", userID,
						"provider", entry.Provider,
						"error", err,
					)
					continue
				}
				apiKey = decrypted
			} else {
				apiKey = key.APIKeyEncrypted
			}
		}

		// Get model settings (including strict mode) - chain entry can override defaults
		_, _, strictMode := llm.GetModelSettings(entry.Provider, entry.Model, entry.Temperature, entry.MaxTokens, entry.StrictMode)

		configs = append(configs, &LLMConfigInput{
			Provider:   entry.Provider,
			APIKey:     apiKey,
			BaseURL:    key.BaseURL,
			Model:      entry.Model,
			StrictMode: strictMode,
		})
	}

	return configs
}

// CrawlInput represents crawl input.
type CrawlInput struct {
	JobID    string          `json:"job_id,omitempty"`    // Job ID for logging/tracking
	URL      string          `json:"url"`
	SeedURLs []string        `json:"seed_urls,omitempty"` // Additional seed URLs (from sitemap discovery)
	Schema   json.RawMessage `json:"schema"`
	Options  CrawlOptions    `json:"options"`
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
}

// CrawlResult represents the result of a crawl operation.
type CrawlResult struct {
	Results           []any        `json:"results"`      // Aggregated data (backward compat)
	PageResults       []PageResult `json:"page_results"` // Individual page results for SSE streaming
	PageCount         int          `json:"page_count"`
	TotalTokensInput  int          `json:"total_tokens_input"`
	TotalTokensOutput int          `json:"total_tokens_output"`
	TotalCostUSD      float64      `json:"total_cost_usd"` // Actual USD cost charged to user
	StoppedEarly      bool         `json:"stopped_early"`  // True if crawl terminated before completion
	StopReason        string       `json:"stop_reason"`    // Reason for early stop (e.g., "insufficient_balance")
}

// Crawl performs a multi-page crawl extraction.
func (s *ExtractionService) Crawl(ctx context.Context, userID string, input CrawlInput) (*CrawlResult, error) {
	// Resolve LLM configuration
	llmCfg, isBYOK, err := s.resolveLLMConfig(ctx, userID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve LLM config: %w", err)
	}

	// Parse schema (supports both JSON and YAML)
	sch, err := parseSchema(input.Schema)
	if err != nil {
		return nil, fmt.Errorf("invalid schema: %w", err)
	}

	// Create refyne instance
	r, cleanerName, err := s.createRefyneInstance(llmCfg)
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
			pageUserCost, _, _ = s.billing.CalculateTotalCost(pageLLMCost, "")
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

		data = append(data, result.Data)
		pageResults = append(pageResults, PageResult{
			URL:               result.URL,
			ParentURL:         parentURL,
			Depth:             depth,
			Data:              result.Data,
			TokenUsageInput:   result.TokenUsage.InputTokens,
			TokenUsageOutput:  result.TokenUsage.OutputTokens,
			FetchDurationMs:   int(result.FetchDuration.Milliseconds()),
			ExtractDurationMs: int(result.ExtractDuration.Milliseconds()),
			LLMProvider:       result.Provider,
			LLMModel:          result.Model,
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
		userCostUSD, _, _ = s.billing.CalculateTotalCost(llmCostUSD, "")
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
	// Resolve LLM configuration
	llmCfg, isBYOK, err := s.resolveLLMConfig(ctx, userID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve LLM config: %w", err)
	}

	// Get available balance for non-BYOK users (for mid-crawl balance enforcement)
	var availableBalance float64
	var checkBalance bool
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

	// Create refyne instance
	r, cleanerName, err := s.createRefyneInstance(llmCfg)
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
				pageUserCost, _, _ = s.billing.CalculateTotalCost(pageLLMCost, "")
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

			data = append(data, result.Data)
			pageResult = PageResult{
				URL:               result.URL,
				ParentURL:         parentURL,
				Depth:             depth,
				Data:              result.Data,
				TokenUsageInput:   result.TokenUsage.InputTokens,
				TokenUsageOutput:  result.TokenUsage.OutputTokens,
				FetchDurationMs:   int(result.FetchDuration.Milliseconds()),
				ExtractDurationMs: int(result.ExtractDuration.Milliseconds()),
				LLMProvider:       result.Provider,
				LLMModel:          result.Model,
				GenerationID:      result.GenerationID,
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
		userCostUSD, _, _ = s.billing.CalculateTotalCost(llmCostUSD, "")
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
		StoppedEarly:      stoppedEarly,
		StopReason:        stopReason,
	}, nil
}

// createRefyneInstance creates a new refyne instance with the given LLM config.
// Returns the refyne instance and the cleaner name for logging.
func (s *ExtractionService) createRefyneInstance(llmCfg *LLMConfigInput) (*refyne.Refyne, string, error) {
	// Use Trafilatura -> Markdown chain
	// 1. Trafilatura extracts main content (more inclusive than Readability)
	//    - Keeps tables, links, images (important for data extraction)
	//    - Works better for non-article pages (product listings, job boards, etc.)
	// 2. Markdown converts HTML to markdown (strips tags, preserves structure)
	contentCleaner := cleaner.NewChain(
		cleaner.NewTrafilatura(&cleaner.TrafilaturaConfig{
			Output: cleaner.OutputHTML,
			Tables: cleaner.Include, // Keep tables for structured data
			Links:  cleaner.Include, // Keep links for URL extraction
			Images: cleaner.Include, // Keep image URLs
		}),
		cleaner.NewMarkdown(),
	)

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
	return r, contentCleaner.Name(), nil
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

// resolveLLMConfig determines which LLM configuration to use.
// This is a simpler version that returns a single config (for Crawl operations).
// Returns (config, isBYOK, error) where isBYOK is true if the user is using their own API key.
func (s *ExtractionService) resolveLLMConfig(ctx context.Context, userID string, override *LLMConfigInput) (*LLMConfigInput, bool, error) {
	// If override provided with provider, use it (assume BYOK since user provided it)
	if override != nil && override.Provider != "" {
		return override, true, nil
	}

	// 1. Check user's fallback chain first (new system)
	configs := s.buildUserFallbackChain(ctx, userID)
	if len(configs) > 0 {
		s.logger.Info("using user fallback chain for crawl (BYOK)",
			"user_id", userID,
			"provider", configs[0].Provider,
			"model", configs[0].Model,
		)
		return configs[0], true, nil
	}

	// 2. Check user's legacy saved config
	savedCfg, err := s.repos.LLMConfig.GetByUserID(ctx, userID)
	if err != nil {
		s.logger.Warn("failed to get user LLM config", "user_id", userID, "error", err)
		// Continue to defaults
	}

	if savedCfg != nil && savedCfg.Provider != "" {
		// Decrypt API key if present
		var apiKey string
		if savedCfg.APIKeyEncrypted != "" && s.encryptor != nil {
			decrypted, err := s.encryptor.Decrypt(savedCfg.APIKeyEncrypted)
			if err != nil {
				s.logger.Warn("failed to decrypt API key", "user_id", userID, "error", err)
				// Continue without API key
			} else {
				apiKey = decrypted
			}
		}

		s.logger.Info("using legacy user config for crawl (BYOK)",
			"user_id", userID,
			"provider", savedCfg.Provider,
		)
		return &LLMConfigInput{
			Provider: savedCfg.Provider,
			APIKey:   apiKey,
			BaseURL:  savedCfg.BaseURL,
			Model:    savedCfg.Model,
		}, true, nil // BYOK = true for user's legacy config
	}

	// 3. Default to service keys or free tier
	s.logger.Info("using system config for crawl",
		"user_id", userID,
	)
	return s.getDefaultLLMConfig(), false, nil // BYOK = false for system config
}

// getDefaultLLMConfigs returns the fallback chain of LLM configurations for users without custom config.
// Returns a list in priority order - the extraction service will try each until one succeeds.
// Uses the default (tier=nil) fallback chain.
func (s *ExtractionService) getDefaultLLMConfigs() []*LLMConfigInput {
	return s.getDefaultLLMConfigsForTier("")
}

// getDefaultLLMConfigsForTier returns the fallback chain for a specific tier.
// If the tier has a custom chain configured, it uses that. Otherwise falls back to the default chain.
// Returns a list in priority order - the extraction service will try each until one succeeds.
func (s *ExtractionService) getDefaultLLMConfigsForTier(tier string) []*LLMConfigInput {
	// First, try to get the configured fallback chain from the database
	if s.repos != nil && s.repos.FallbackChain != nil {
		var chain []*models.FallbackChainEntry
		var err error

		// Get tier-specific chain (falls back to default if none exists)
		if tier != "" {
			chain, err = s.repos.FallbackChain.GetEnabledByTier(context.Background(), tier)
		} else {
			chain, err = s.repos.FallbackChain.GetEnabled(context.Background())
		}

		if err == nil && len(chain) > 0 {
			serviceKeys := s.getServiceKeys()
			configs := make([]*LLMConfigInput, 0, len(chain))

			for _, entry := range chain {
				// Get model settings - chain entry can override defaults
				_, _, strictMode := llm.GetModelSettings(entry.Provider, entry.Model, entry.Temperature, entry.MaxTokens, entry.StrictMode)

				config := &LLMConfigInput{
					Provider:   entry.Provider,
					Model:      entry.Model,
					StrictMode: strictMode,
				}

				// Add API key for the provider if we have one
				switch entry.Provider {
				case "openrouter":
					config.APIKey = serviceKeys.OpenRouterKey
				case "anthropic":
					config.APIKey = serviceKeys.AnthropicKey
				case "openai":
					config.APIKey = serviceKeys.OpenAIKey
				case "ollama":
					// Ollama doesn't need an API key
				}

				// Only add if we have the required API key (or it's ollama)
				if config.APIKey != "" || entry.Provider == "ollama" {
					configs = append(configs, config)
				}
			}

			if len(configs) > 0 {
				s.logger.Debug("using fallback chain",
					"tier", tier,
					"entries", len(configs),
				)
				return configs
			}
		}
	}

	// Fall back to the hardcoded default chain
	return s.getHardcodedDefaultChain()
}

// getHardcodedDefaultChain returns the default fallback chain when no custom chain is configured.
//
// Default chain (all OpenRouter free models):
//  1. xiaomi/mimo-v2-flash:free   - Fast, capable free model
//  2. openai/gpt-oss-120b:free    - Large open-source model
//  3. google/gemma-3-27b-it:free  - Google's instruction-tuned model
//  4. ollama:llama3.2             - Local fallback (no API key needed)
//
// This chain prioritizes free OpenRouter models for cost efficiency.
// Configure a custom chain via the admin dashboard to override these defaults.
func (s *ExtractionService) getHardcodedDefaultChain() []*LLMConfigInput {
	serviceKeys := s.getServiceKeys()
	configs := make([]*LLMConfigInput, 0, 4)

	// OpenRouter free models chain (requires OpenRouter API key)
	if serviceKeys.OpenRouterKey != "" {
		// 1. Xiaomi MiMo - fast and capable
		_, _, strictMode1 := llm.GetModelSettings("openrouter", "xiaomi/mimo-v2-flash:free", nil, nil, nil)
		configs = append(configs, &LLMConfigInput{
			Provider:   "openrouter",
			APIKey:     serviceKeys.OpenRouterKey,
			Model:      "xiaomi/mimo-v2-flash:free",
			StrictMode: strictMode1,
		})

		// 2. GPT-OSS-120B - large open-source model
		_, _, strictMode2 := llm.GetModelSettings("openrouter", "openai/gpt-oss-120b:free", nil, nil, nil)
		configs = append(configs, &LLMConfigInput{
			Provider:   "openrouter",
			APIKey:     serviceKeys.OpenRouterKey,
			Model:      "openai/gpt-oss-120b:free",
			StrictMode: strictMode2,
		})

		// 3. Gemma 3 27B - Google's instruction-tuned model
		_, _, strictMode3 := llm.GetModelSettings("openrouter", "google/gemma-3-27b-it:free", nil, nil, nil)
		configs = append(configs, &LLMConfigInput{
			Provider:   "openrouter",
			APIKey:     serviceKeys.OpenRouterKey,
			Model:      "google/gemma-3-27b-it:free",
			StrictMode: strictMode3,
		})
	}

	// Final fallback: Ollama (no API key needed, requires local setup)
	_, _, strictModeOllama := llm.GetModelSettings("ollama", "llama3.2", nil, nil, nil)
	configs = append(configs, &LLMConfigInput{
		Provider:   "ollama",
		Model:      "llama3.2",
		StrictMode: strictModeOllama,
	})

	return configs
}

// getDefaultLLMConfig returns the first available LLM configuration (for backward compatibility).
func (s *ExtractionService) getDefaultLLMConfig() *LLMConfigInput {
	configs := s.getDefaultLLMConfigs()
	if len(configs) > 0 {
		return configs[0]
	}
	// Ultimate fallback
	return &LLMConfigInput{
		Provider: "ollama",
		Model:    "llama3.2",
	}
}

// ServiceKeys holds the service API keys (loaded from DB or env vars).
type ServiceKeys struct {
	OpenRouterKey string
	AnthropicKey  string
	OpenAIKey     string
}

// getServiceKeys retrieves service keys, preferring DB over env vars.
func (s *ExtractionService) getServiceKeys() ServiceKeys {
	keys := ServiceKeys{}

	// Try to load from database first
	if s.repos != nil && s.repos.ServiceKey != nil {
		dbKeys, err := s.repos.ServiceKey.GetAll(context.Background())
		if err == nil {
			for _, k := range dbKeys {
				// Decrypt the key if we have an encryptor
				apiKey := k.APIKeyEncrypted
				if s.encryptor != nil && k.APIKeyEncrypted != "" {
					if decrypted, err := s.encryptor.Decrypt(k.APIKeyEncrypted); err == nil {
						apiKey = decrypted
					}
				}

				switch k.Provider {
				case "openrouter":
					keys.OpenRouterKey = apiKey
				case "anthropic":
					keys.AnthropicKey = apiKey
				case "openai":
					keys.OpenAIKey = apiKey
				}
			}
		}
	}

	// Fall back to env vars for any missing keys
	if keys.OpenRouterKey == "" {
		keys.OpenRouterKey = s.cfg.ServiceOpenRouterKey
	}
	if keys.AnthropicKey == "" {
		keys.AnthropicKey = s.cfg.ServiceAnthropicKey
	}
	if keys.OpenAIKey == "" {
		keys.OpenAIKey = s.cfg.ServiceOpenAIKey
	}

	return keys
}

// ========================================
// Helper Methods
// ========================================

// parseSchema attempts to parse schema data as JSON first, then YAML if JSON fails.
// This allows the API to accept schemas in either format.
func parseSchema(data []byte) (schema.Schema, error) {
	// Try JSON first (most common)
	sch, err := schema.FromJSON(data)
	if err == nil {
		return sch, nil
	}

	// If JSON fails, try YAML
	sch, yamlErr := schema.FromYAML(data)
	if yamlErr == nil {
		return sch, nil
	}

	// Both failed - return the JSON error as it's the primary format
	return schema.Schema{}, fmt.Errorf("invalid schema format (tried JSON and YAML): %w", err)
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
