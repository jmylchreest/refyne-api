package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/refyne/refyne/pkg/refyne"
	"github.com/refyne/refyne/pkg/schema"

	"github.com/jmylchreest/refyne-api/internal/config"
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
	Provider string `json:"provider,omitempty"`
	APIKey   string `json:"api_key,omitempty"`
	BaseURL  string `json:"base_url,omitempty"`
	Model    string `json:"model,omitempty"`
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
	sch, err := schema.FromJSON(input.Schema)
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

	// Try each config in the chain until one succeeds
	var lastErr error
	var lastCfg *LLMConfigInput

	for i, llmCfg := range llmConfigs {
		lastCfg = llmCfg

		s.logger.Info("extraction attempt",
			"user_id", userID,
			"url", input.URL,
			"provider", llmCfg.Provider,
			"model", llmCfg.Model,
			"attempt", i+1,
			"of", len(llmConfigs),
			"is_byok", isBYOK,
		)

		// Create refyne instance
		r, err := s.createRefyneInstance(llmCfg)
		if err != nil {
			s.logger.Warn("failed to create LLM instance, trying next",
				"provider", llmCfg.Provider,
				"model", llmCfg.Model,
				"error", err,
			)
			lastErr = err
			continue
		}

		// Perform extraction
		result, err := r.Extract(ctx, input.URL, sch)
		r.Close()

		// Check for success
		if err == nil && result != nil && result.Error == nil {
			// Success! Handle billing and return
			return s.handleSuccessfulExtraction(ctx, userID, input, ectx, llmCfg, result, isBYOK, startTime)
		}

		// Extraction failed, log and try next
		if err != nil {
			lastErr = err
		} else if result != nil && result.Error != nil {
			lastErr = result.Error
		}

		s.logger.Warn("extraction failed, trying next provider",
			"provider", llmCfg.Provider,
			"model", llmCfg.Model,
			"error", lastErr,
			"attempt", i+1,
		)
	}

	// All attempts failed
	if lastErr != nil {
		return nil, s.handleLLMError(lastErr, lastCfg)
	}

	return nil, fmt.Errorf("extraction failed: no LLM providers configured")
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

	// Check user's saved config
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
	return s.getDefaultLLMConfigsForTier(tier), false
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
	URL     string          `json:"url"`
	Schema  json.RawMessage `json:"schema"`
	Options CrawlOptions    `json:"options"`
}

// Note: CrawlOptions is defined in job_service.go to avoid duplication

// CrawlResult represents the result of a crawl operation.
type CrawlResult struct {
	Results           []any `json:"results"`
	PageCount         int   `json:"page_count"`
	TotalTokensInput  int   `json:"total_tokens_input"`
	TotalTokensOutput int   `json:"total_tokens_output"`
	TotalCredits      int   `json:"total_credits"`
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

	// Parse schema
	sch, err := schema.FromJSON(input.Schema)
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
	var (
		data              []any
		totalTokensInput  int
		totalTokensOutput int
		pageCount         int
		lastError         error
	)

	for result := range results {
		if result.Error != nil {
			s.logger.Warn("crawl page error", "url", result.URL, "error", result.Error)
			lastError = result.Error
			continue
		}

		data = append(data, result.Data)
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
		PageCount:         pageCount,
		TotalTokensInput:  totalTokensInput,
		TotalTokensOutput: totalTokensOutput,
		TotalCredits:      totalCredits,
	}, nil
}

// createRefyneInstance creates a new refyne instance with the given LLM config.
func (s *ExtractionService) createRefyneInstance(llmCfg *LLMConfigInput) (*refyne.Refyne, error) {
	opts := []refyne.Option{
		refyne.WithProvider(llmCfg.Provider),
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
func (s *ExtractionService) resolveLLMConfig(ctx context.Context, userID string, override *LLMConfigInput) (*LLMConfigInput, error) {
	// If override provided with provider, use it
	if override != nil && override.Provider != "" {
		return override, nil
	}

	// Check user's saved config
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

		return &LLMConfigInput{
			Provider: savedCfg.Provider,
			APIKey:   apiKey,
			BaseURL:  savedCfg.BaseURL,
			Model:    savedCfg.Model,
		}, nil
	}

	// Default to service keys or free tier
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
				config := &LLMConfigInput{
					Provider: entry.Provider,
					Model:    entry.Model,
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
		configs = append(configs, &LLMConfigInput{
			Provider: "openrouter",
			APIKey:   serviceKeys.OpenRouterKey,
			Model:    "xiaomi/mimo-v2-flash:free",
		})

		// 2. GPT-OSS-120B - large open-source model
		configs = append(configs, &LLMConfigInput{
			Provider: "openrouter",
			APIKey:   serviceKeys.OpenRouterKey,
			Model:    "openai/gpt-oss-120b:free",
		})

		// 3. Gemma 3 27B - Google's instruction-tuned model
		configs = append(configs, &LLMConfigInput{
			Provider: "openrouter",
			APIKey:   serviceKeys.OpenRouterKey,
			Model:    "google/gemma-3-27b-it:free",
		})
	}

	// Final fallback: Ollama (no API key needed, requires local setup)
	configs = append(configs, &LLMConfigInput{
		Provider: "ollama",
		Model:    "llama3.2",
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
