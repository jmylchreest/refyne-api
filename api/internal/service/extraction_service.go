package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/oklog/ulid/v2"

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

	// Check balance for non-BYOK users (if billing is enabled)
	if s.billing != nil && !isBYOK && len(llmConfigs) > 0 {
		estimatedCost := s.billing.EstimateCost(1, llmConfigs[0].Model)
		if err := s.billing.CheckSufficientBalance(ctx, userID, estimatedCost); err != nil {
			return nil, err
		}
	}

	// Try each config in the chain until one succeeds, with retry and backoff
	var lastErr error
	var lastLLMErr *llm.LLMError
	var lastCfg *LLMConfigInput
	totalRetries := 0

	for providerIdx, llmCfg := range llmConfigs {
		lastCfg = llmCfg

		// Retry loop for this provider with exponential backoff
		for attempt := 0; attempt < constants.MaxRetryAttempts; attempt++ {
			totalRetries++

			s.logger.Info("extraction attempt",
				"user_id", userID,
				"url", input.URL,
				"provider", llmCfg.Provider,
				"model", llmCfg.Model,
				"provider_idx", providerIdx+1,
				"of_providers", len(llmConfigs),
				"attempt", attempt+1,
				"max_attempts", constants.MaxRetryAttempts,
				"is_byok", isBYOK,
			)

			// Create refyne instance
			r, err := s.createRefyneInstance(llmCfg)
			if err != nil {
				s.logger.Warn("failed to create LLM instance",
					"provider", llmCfg.Provider,
					"model", llmCfg.Model,
					"error", err,
				)
				lastErr = err
				lastLLMErr = llm.WrapError(err, llmCfg.Provider, llmCfg.Model)
				break // Can't retry instance creation, try next provider
			}

			// Perform extraction
			result, err := r.Extract(ctx, input.URL, sch)
			r.Close()

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

			lastLLMErr = llm.WrapError(lastErr, llmCfg.Provider, llmCfg.Model)

			s.logger.Warn("extraction failed",
				"provider", llmCfg.Provider,
				"model", llmCfg.Model,
				"error", lastErr,
				"category", lastLLMErr.Category,
				"retryable", lastLLMErr.Retryable,
				"should_fallback", lastLLMErr.ShouldFallback,
				"attempt", attempt+1,
			)

			// Decide whether to retry with this provider or move to next
			if !lastLLMErr.Retryable {
				// Not retryable with same provider - check if we should fall back
				if lastLLMErr.ShouldFallback {
					s.logger.Info("error not retryable, falling back to next provider",
						"provider", llmCfg.Provider,
						"category", lastLLMErr.Category,
					)
					// Brief delay before trying next provider
					time.Sleep(constants.ProviderFallbackDelay)
				}
				break // Exit retry loop for this provider
			}

			// Calculate backoff delay
			backoff := s.calculateBackoff(attempt, lastLLMErr.Category)

			// Check if context is still valid before sleeping
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}

			s.logger.Info("backing off before retry",
				"provider", llmCfg.Provider,
				"backoff", backoff.String(),
				"attempt", attempt+1,
			)

			// Sleep with context awareness
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
				// Continue to next retry
			}
		}

		// If the error suggests we shouldn't try other providers, stop the chain
		if lastLLMErr != nil && !lastLLMErr.ShouldFallback {
			s.logger.Info("error does not suggest fallback, stopping provider chain",
				"provider", llmCfg.Provider,
				"category", lastLLMErr.Category,
			)
			break
		}
	}

	// All attempts failed - record the failure with detailed error info
	if lastErr != nil {
		s.recordFailedExtractionWithDetails(ctx, userID, input, ectx, lastCfg, isBYOK, lastErr, lastLLMErr, totalRetries, startTime)
		return nil, s.handleLLMError(lastErr, lastCfg)
	}

	return nil, fmt.Errorf("extraction failed: no LLM providers configured")
}

// recordFailedExtraction records usage for a failed extraction attempt.
func (s *ExtractionService) recordFailedExtraction(
	ctx context.Context,
	userID string,
	input ExtractInput,
	ectx *ExtractContext,
	llmCfg *LLMConfigInput,
	isBYOK bool,
	err error,
	startTime time.Time,
) {
	// Delegate to the new method with nil LLMError
	s.recordFailedExtractionWithDetails(ctx, userID, input, ectx, llmCfg, isBYOK, err, nil, 1, startTime)
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

	if recordErr := s.billing.RecordUsage(ctx, usageRecord); recordErr != nil {
		s.logger.Warn("failed to record failed extraction usage", "error", recordErr)
	}
}

// calculateBackoff calculates the backoff duration for a retry attempt.
// Uses exponential backoff with longer initial delay for rate limits.
func (s *ExtractionService) calculateBackoff(attempt int, category string) time.Duration {
	var initial time.Duration

	// Rate limits get longer initial backoff
	if category == "rate_limit" {
		initial = constants.RateLimitBackoff
	} else {
		initial = constants.InitialBackoff
	}

	// Exponential backoff: initial * (multiplier ^ attempt)
	backoff := float64(initial) * math.Pow(constants.BackoffMultiplier, float64(attempt))

	// Cap at maximum
	if backoff > float64(constants.MaxBackoff) {
		backoff = float64(constants.MaxBackoff)
	}

	return time.Duration(backoff)
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
		"cost_usd", usageInfo.CostUSD,
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

// boolToInt converts a bool to 1 or 0.
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// CrawlInput represents crawl input.
type CrawlInput struct {
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
	TotalCredits      int          `json:"total_credits"`
}

// Crawl performs a multi-page crawl extraction.
func (s *ExtractionService) Crawl(ctx context.Context, userID string, input CrawlInput) (*CrawlResult, error) {
	// Resolve LLM configuration
	llmCfg, err := s.resolveLLMConfig(ctx, userID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve LLM config: %w", err)
	}

	s.logger.Info("crawl starting",
		"user_id", userID,
		"url", input.URL,
		"provider", llmCfg.Provider,
		"model", llmCfg.Model,
	)

	// Parse schema (supports both JSON and YAML)
	sch, err := parseSchema(input.Schema)
	if err != nil {
		return nil, fmt.Errorf("invalid schema: %w", err)
	}

	// Create refyne instance
	r, err := s.createRefyneInstance(llmCfg)
	if err != nil {
		return nil, s.handleLLMError(err, llmCfg)
	}
	defer r.Close()

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
			// Log with model/provider info for debugging
			s.logger.Warn("crawl page error",
				"url", result.URL,
				"provider", llmCfg.Provider,
				"model", llmCfg.Model,
				"error", result.Error,
			)
			lastError = result.Error

			// Wrap error for user-friendly messaging
			llmErr := llm.WrapError(result.Error, llmCfg.Provider, llmCfg.Model)
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
			})
			continue
		}

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
		return nil, s.handleLLMError(lastError, llmCfg)
	}

	// Calculate cost
	totalTokens := totalTokensInput + totalTokensOutput
	totalCredits := (totalTokens + 999) / 1000

	// Record usage
	s.recordUsageLegacy(ctx, userID, "", models.JobTypeCrawl, pageCount,
		totalTokensInput, totalTokensOutput, totalCredits,
		llmCfg.Provider, llmCfg.Model)

	s.logger.Info("crawl completed",
		"user_id", userID,
		"url", input.URL,
		"page_count", pageCount,
		"total_input_tokens", totalTokensInput,
		"total_output_tokens", totalTokensOutput,
	)

	return &CrawlResult{
		Results:           data,
		PageResults:       pageResults,
		PageCount:         pageCount,
		TotalTokensInput:  totalTokensInput,
		TotalTokensOutput: totalTokensOutput,
		TotalCredits:      totalCredits,
	}, nil
}

// CrawlResultCallback is called for each page result during a crawl.
// Return an error to stop the crawl early.
type CrawlResultCallback func(result PageResult) error

// CrawlWithCallback performs a multi-page crawl extraction with a callback for each result.
// This allows incremental processing of results (e.g., saving to database as they come in).
func (s *ExtractionService) CrawlWithCallback(ctx context.Context, userID string, input CrawlInput, callback CrawlResultCallback) (*CrawlResult, error) {
	// Resolve LLM configuration
	llmCfg, err := s.resolveLLMConfig(ctx, userID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve LLM config: %w", err)
	}

	// Build seed URLs list - use SeedURLs if provided (from sitemap), otherwise just the main URL
	seedURLs := input.SeedURLs
	if len(seedURLs) == 0 {
		seedURLs = []string{input.URL}
	}

	s.logger.Info("crawl starting",
		"user_id", userID,
		"url", input.URL,
		"seed_count", len(seedURLs),
		"provider", llmCfg.Provider,
		"model", llmCfg.Model,
	)

	// Parse schema (supports both JSON and YAML)
	sch, err := parseSchema(input.Schema)
	if err != nil {
		return nil, fmt.Errorf("invalid schema: %w", err)
	}

	// Create refyne instance
	r, err := s.createRefyneInstance(llmCfg)
	if err != nil {
		return nil, s.handleLLMError(err, llmCfg)
	}
	defer r.Close()

	// Build crawl options
	crawlOpts := s.buildCrawlOptions(input.Options)

	// Perform crawl - use CrawlMany for multiple seeds (from sitemap), single Crawl otherwise
	var results <-chan *refyne.Result
	if len(seedURLs) > 1 {
		results = r.CrawlMany(ctx, seedURLs, sch, crawlOpts...)
	} else {
		results = r.Crawl(ctx, input.URL, sch, crawlOpts...)
	}

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
			// Log with model/provider info for debugging
			s.logger.Warn("crawl page error",
				"url", result.URL,
				"provider", llmCfg.Provider,
				"model", llmCfg.Model,
				"error", result.Error,
			)
			lastError = result.Error

			// Wrap error for user-friendly messaging
			llmErr := llm.WrapError(result.Error, llmCfg.Provider, llmCfg.Model)
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
			}
		} else {
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
			}
			totalTokensInput += result.TokenUsage.InputTokens
			totalTokensOutput += result.TokenUsage.OutputTokens
			pageCount++
		}

		pageResults = append(pageResults, pageResult)

		// Call the callback for this result
		if callback != nil {
			if err := callback(pageResult); err != nil {
				s.logger.Error("callback error, stopping crawl", "error", err)
				break
			}
		}
	}

	// If no results and we have an error, return the error
	if pageCount == 0 && lastError != nil {
		return nil, s.handleLLMError(lastError, llmCfg)
	}

	// Calculate cost
	totalTokens := totalTokensInput + totalTokensOutput
	totalCredits := (totalTokens + 999) / 1000

	// Record usage
	s.recordUsageLegacy(ctx, userID, "", models.JobTypeCrawl, pageCount,
		totalTokensInput, totalTokensOutput, totalCredits,
		llmCfg.Provider, llmCfg.Model)

	s.logger.Info("crawl completed",
		"user_id", userID,
		"url", input.URL,
		"page_count", pageCount,
		"total_input_tokens", totalTokensInput,
		"total_output_tokens", totalTokensOutput,
	)

	return &CrawlResult{
		Results:           data,
		PageResults:       pageResults,
		PageCount:         pageCount,
		TotalTokensInput:  totalTokensInput,
		TotalTokensOutput: totalTokensOutput,
		TotalCredits:      totalCredits,
	}, nil
}

// createRefyneInstance creates a new refyne instance with the given LLM config.
// Uses Trafilatura (text output) -> Markdown cleaner chain and a 120s timeout.
func (s *ExtractionService) createRefyneInstance(llmCfg *LLMConfigInput) (*refyne.Refyne, error) {
	// Create cleaner chain: Trafilatura (text output) -> Markdown
	// This extracts main content and converts to clean markdown for the LLM
	trafilaturaCleaner := cleaner.NewTrafilatura(&cleaner.TrafilaturaConfig{
		Output: cleaner.OutputText, // Text output for cleaner result
	})
	markdownCleaner := cleaner.NewMarkdown()
	cleanerChain := cleaner.NewChain(trafilaturaCleaner, markdownCleaner)

	opts := []refyne.Option{
		refyne.WithProvider(llmCfg.Provider),
		refyne.WithCleaner(cleanerChain),
		refyne.WithTimeout(llm.LLMTimeout), // 120s timeout for LLM requests
		refyne.WithStrictMode(llmCfg.StrictMode),
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

	return refyne.New(opts...)
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
func (s *ExtractionService) handleLLMError(err error, llmCfg *LLMConfigInput) error {
	llmErr := llm.WrapError(err, llmCfg.Provider, llmCfg.Model)

	// Log the detailed error
	s.logger.Error("LLM error",
		"error", err.Error(),
		"provider", llmCfg.Provider,
		"model", llmCfg.Model,
		"user_message", llmErr.UserMessage,
		"retryable", llmErr.Retryable,
		"suggest_upgrade", llmErr.SuggestUpgrade,
	)

	return llmErr
}

// resolveLLMConfig determines which LLM configuration to use.
// This is a simpler version that returns a single config (for Crawl operations).
func (s *ExtractionService) resolveLLMConfig(ctx context.Context, userID string, override *LLMConfigInput) (*LLMConfigInput, error) {
	// If override provided with provider, use it
	if override != nil && override.Provider != "" {
		return override, nil
	}

	// 1. Check user's fallback chain first (new system)
	configs := s.buildUserFallbackChain(ctx, userID)
	if len(configs) > 0 {
		s.logger.Info("using user fallback chain for crawl (BYOK)",
			"user_id", userID,
			"provider", configs[0].Provider,
			"model", configs[0].Model,
		)
		return configs[0], nil
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
		}, nil
	}

	// 3. Default to service keys or free tier
	s.logger.Info("using system config for crawl",
		"user_id", userID,
	)
	return s.getDefaultLLMConfig(), nil
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

// UsageInput holds all data needed to record usage.
type UsageInput struct {
	UserID          string
	JobID           string
	JobType         models.JobType
	Status          string // success, failed, partial
	TotalChargedUSD float64
	IsBYOK          bool

	// For insights table
	TargetURL         string
	SchemaID          string
	CrawlConfigJSON   string
	ErrorMessage      string
	ErrorCode         string
	TokensInput       int
	TokensOutput      int
	LLMCostUSD        float64
	MarkupRate        float64
	MarkupUSD         float64
	LLMProvider       string
	LLMModel          string
	GenerationID      string
	BYOKProvider      string
	PagesAttempted    int
	PagesSuccessful   int
	FetchDurationMs   int
	ExtractDurationMs int
	TotalDurationMs   int
	RequestID         string
	UserAgent         string
	IPCountry         string
}

// recordUsage records usage to both lean billing table and rich insights table.
func (s *ExtractionService) recordUsage(ctx context.Context, input *UsageInput) {
	usageID := ulid.Make().String()
	now := time.Now()

	// Record to lean billing table
	usageRecord := &models.UsageRecord{
		ID:              usageID,
		UserID:          input.UserID,
		JobID:           input.JobID,
		Date:            now.Format("2006-01-02"),
		Type:            input.JobType,
		Status:          input.Status,
		TotalChargedUSD: input.TotalChargedUSD,
		IsBYOK:          input.IsBYOK,
		CreatedAt:       now,
	}

	if err := s.repos.Usage.Create(ctx, usageRecord); err != nil {
		s.logger.Warn("failed to record usage", "error", err)
		return
	}

	// Record to rich insights table
	insight := &models.UsageInsight{
		ID:                ulid.Make().String(),
		UsageID:           usageID,
		TargetURL:         input.TargetURL,
		SchemaID:          input.SchemaID,
		CrawlConfigJSON:   input.CrawlConfigJSON,
		ErrorMessage:      input.ErrorMessage,
		ErrorCode:         input.ErrorCode,
		TokensInput:       input.TokensInput,
		TokensOutput:      input.TokensOutput,
		LLMCostUSD:        input.LLMCostUSD,
		MarkupRate:        input.MarkupRate,
		MarkupUSD:         input.MarkupUSD,
		LLMProvider:       input.LLMProvider,
		LLMModel:          input.LLMModel,
		GenerationID:      input.GenerationID,
		BYOKProvider:      input.BYOKProvider,
		PagesAttempted:    input.PagesAttempted,
		PagesSuccessful:   input.PagesSuccessful,
		FetchDurationMs:   input.FetchDurationMs,
		ExtractDurationMs: input.ExtractDurationMs,
		TotalDurationMs:   input.TotalDurationMs,
		RequestID:         input.RequestID,
		UserAgent:         input.UserAgent,
		IPCountry:         input.IPCountry,
		CreatedAt:         now,
	}

	if err := s.repos.UsageInsight.Create(ctx, insight); err != nil {
		s.logger.Warn("failed to record usage insight", "error", err)
	}
}

// recordUsageLegacy is a temporary wrapper for old-style usage recording.
// TODO: Update all callers to use recordUsage with UsageInput.
func (s *ExtractionService) recordUsageLegacy(ctx context.Context, userID, jobID string, jobType models.JobType,
	pages, tokensInput, tokensOutput, costCredits int, provider, model string) {

	s.recordUsage(ctx, &UsageInput{
		UserID:          userID,
		JobID:           jobID,
		JobType:         jobType,
		Status:          "success",
		TotalChargedUSD: 0, // Legacy: no USD tracking yet
		IsBYOK:          false,
		TokensInput:     tokensInput,
		TokensOutput:    tokensOutput,
		LLMProvider:     provider,
		LLMModel:        model,
		PagesSuccessful: pages,
	})
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

// detectBYOK determines if the user provided their own API key (Bring Your Own Key).
// BYOK users are not charged for API usage.
func (s *ExtractionService) detectBYOK(inputCfg, resolvedCfg *LLMConfigInput) bool {
	// If user explicitly provided an API key in the request, it's BYOK
	if inputCfg != nil && inputCfg.APIKey != "" {
		return true
	}

	// If using Ollama (local LLM), it's effectively BYOK (no API cost to us)
	if resolvedCfg.Provider == "ollama" {
		return true
	}

	// If using our service keys, it's not BYOK
	if s.cfg.ServiceOpenRouterKey != "" && resolvedCfg.APIKey == s.cfg.ServiceOpenRouterKey {
		return false
	}
	if s.cfg.ServiceAnthropicKey != "" && resolvedCfg.APIKey == s.cfg.ServiceAnthropicKey {
		return false
	}
	if s.cfg.ServiceOpenAIKey != "" && resolvedCfg.APIKey == s.cfg.ServiceOpenAIKey {
		return false
	}

	// If we get here with an API key that's not ours, it's BYOK from saved config
	if resolvedCfg.APIKey != "" {
		return true
	}

	return false
}
