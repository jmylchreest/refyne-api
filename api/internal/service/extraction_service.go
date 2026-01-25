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

// getMaxTokens returns the recommended max tokens for a model.
// Delegates to the resolver which uses S3-backed or hardcoded defaults.
func (s *ExtractionService) getMaxTokens(ctx context.Context, provider, model string, chainMaxTokens *int) int {
	if s.resolver != nil {
		return s.resolver.GetMaxTokens(ctx, provider, model, chainMaxTokens)
	}
	// Fall back to static defaults if resolver not set
	_, maxTokens, _ := llm.GetModelSettings(provider, model, nil, chainMaxTokens, nil)
	return maxTokens
}

// ExtractInput represents extraction input.
type ExtractInput struct {
	URL          string           `json:"url"`
	Schema       json.RawMessage  `json:"schema"`                  // Can be structured schema (YAML/JSON) or freeform prompt - auto-detected
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
	MaxTokens  int    `json:"max_tokens,omitempty"`  // Max output tokens for LLM responses
	StrictMode bool   `json:"strict_mode,omitempty"` // Whether to use strict JSON schema mode
}

// InputFormat represents the detected input format for extraction.
type InputFormat string

const (
	// InputFormatSchema indicates the input was parsed as a structured schema.
	InputFormatSchema InputFormat = "schema"
	// InputFormatPrompt indicates the input was treated as a freeform prompt.
	InputFormatPrompt InputFormat = "prompt"
)

// ExtractOutput represents extraction output.
type ExtractOutput struct {
	Data        any         `json:"data"`
	URL         string      `json:"url"`
	FetchedAt   time.Time   `json:"fetched_at"`
	Usage       UsageInfo   `json:"usage"`
	Metadata    ExtractMeta `json:"metadata"`
	InputFormat InputFormat `json:"input_format"` // "schema" or "prompt" - indicates how the input was interpreted
	RawContent  string      `json:"-"`            // Raw page content (not serialized, for debug capture only)
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
	FetchDurationMs   int          `json:"fetch_duration_ms"`
	ExtractDurationMs int          `json:"extract_duration_ms"`
	Model             string       `json:"model"`
	Provider          string       `json:"provider"`
	BudgetSkips       []BudgetSkip `json:"budget_skips,omitempty"` // Models skipped due to budget constraints
}

// BudgetSkip represents a model that was skipped due to budget constraints.
type BudgetSkip struct {
	Provider      string  `json:"provider"`
	Model         string  `json:"model"`
	EstimatedCost float64 `json:"estimated_cost_usd"`
	AvailableBudget float64 `json:"available_budget_usd"`
	Reason        string  `json:"reason"`
}

// ExtractContext holds context for billing tracking.
type ExtractContext struct {
	UserID              string
	Tier                string // From JWT claims
	SchemaID            string
	IsBYOK              bool
	BYOKAllowed         bool   // From JWT claims - whether user has the "provider_byok" feature
	ModelsCustomAllowed bool   // From JWT claims - whether user has the "models_custom" feature
	ModelsPremiumAllowed bool  // From JWT claims - whether user has the "models_premium" feature (budget-based fallback)
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

	// Auto-detect input format using shared helper
	// If parsing fails, treat the input as a freeform prompt
	inputFormat, sch, schemaErr := DetectInputFormat(input.Schema)
	if inputFormat == InputFormatPrompt {
		// Input is a freeform prompt - use prompt-based extraction
		s.logger.Info("input detected as freeform prompt",
			"user_id", userID,
			"parse_error", schemaErr.Error(),
		)
		return s.extractWithPrompt(ctx, userID, input, ectx, startTime)
	}
	if schemaErr != nil {
		// Schema parsing failed for some other reason
		return nil, schemaErr
	}

	// Valid schema - continue with schema-based extraction
	s.logger.Debug("input detected as structured schema", "user_id", userID)

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

	// For models_premium users, get available balance for per-model budget checking
	// This enables budget-based fallback - skip expensive models if insufficient balance
	var availableBudget float64
	var budgetSkips []BudgetSkip
	useBudgetFallback := !isBYOK && ectx.ModelsPremiumAllowed && s.billing != nil

	if useBudgetFallback {
		var err error
		availableBudget, err = s.billing.GetAvailableBalance(ctx, userID)
		if err != nil {
			s.logger.Warn("failed to get available balance, disabling budget fallback",
				"user_id", userID,
				"error", err,
			)
			useBudgetFallback = false
		} else {
			s.logger.Debug("budget fallback enabled",
				"user_id", userID,
				"available_budget_usd", availableBudget,
				"model_count", len(llmConfigs),
			)
		}
	} else if s.billing != nil && len(llmConfigs) > 0 && !isBYOK {
		// Standard pre-flight balance check for non-premium users
		estimatedCost := s.billing.EstimateCost(1, llmConfigs[0].Model, llmConfigs[0].Provider)
		s.logger.Debug("pre-flight cost estimate",
			"user_id", userID,
			"provider", llmConfigs[0].Provider,
			"model", llmConfigs[0].Model,
			"estimated_cost_usd", estimatedCost,
			"is_byok", isBYOK,
		)
		if err := s.billing.CheckSufficientBalance(ctx, userID, estimatedCost); err != nil {
			return nil, err
		}
	}

	// Try each config in the chain until one succeeds (no retries on same model)
	var lastErr error
	var lastLLMErr *llm.LLMError
	var lastCfg *LLMConfigInput
	modelsSkippedDueToBudget := 0

	for providerIdx, llmCfg := range llmConfigs {
		lastCfg = llmCfg

		// Budget-based fallback: check if this model is affordable
		if useBudgetFallback {
			estimatedCost := s.billing.EstimateCost(1, llmCfg.Model, llmCfg.Provider)

			if estimatedCost > availableBudget {
				// Skip this model - too expensive for remaining budget
				skip := BudgetSkip{
					Provider:        llmCfg.Provider,
					Model:           llmCfg.Model,
					EstimatedCost:   estimatedCost,
					AvailableBudget: availableBudget,
					Reason:          "estimated_cost_exceeds_budget",
				}
				budgetSkips = append(budgetSkips, skip)
				modelsSkippedDueToBudget++

				s.logger.Info("skipping model due to budget constraint",
					"user_id", userID,
					"provider", llmCfg.Provider,
					"model", llmCfg.Model,
					"estimated_cost_usd", estimatedCost,
					"available_budget_usd", availableBudget,
					"provider_idx", providerIdx+1,
					"of_providers", len(llmConfigs),
				)
				continue // Try next model in the fallback chain
			}

			s.logger.Debug("model within budget",
				"user_id", userID,
				"provider", llmCfg.Provider,
				"model", llmCfg.Model,
				"estimated_cost_usd", estimatedCost,
				"available_budget_usd", availableBudget,
			)
		}

		s.logger.Info("extraction attempt",
			"user_id", userID,
			"url", input.URL,
			"provider", llmCfg.Provider,
			"model", llmCfg.Model,
			"max_tokens", llmCfg.MaxTokens,
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
			return s.handleSuccessfulExtraction(ctx, userID, input, ectx, llmCfg, result, isBYOK, startTime, budgetSkips)
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
		s.recordFailedExtractionWithDetails(ctx, userID, input, ectx, lastCfg, isBYOK, lastErr, lastLLMErr, len(llmConfigs), startTime, budgetSkips)
		return nil, s.handleLLMError(lastErr, lastCfg, isBYOK)
	}

	// Check if all models were skipped due to budget constraints
	if modelsSkippedDueToBudget > 0 && modelsSkippedDueToBudget == len(llmConfigs) {
		s.logger.Warn("all models skipped due to budget constraints",
			"user_id", userID,
			"models_skipped", modelsSkippedDueToBudget,
			"available_budget_usd", availableBudget,
		)
		// Record the budget failure
		s.recordBudgetExhaustedFailure(ctx, userID, input, ectx, startTime, budgetSkips)
		return nil, llm.NewInsufficientCreditsError("all models exceed available budget", 0, int(availableBudget*100))
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
	budgetSkips []BudgetSkip,
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

	// Add budget skip information to error message if any models were skipped
	if len(budgetSkips) > 0 {
		errorMessage = fmt.Sprintf("%s (budget skips: %d models)", errorMessage, len(budgetSkips))
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

// recordBudgetExhaustedFailure records usage when all models were skipped due to budget.
func (s *ExtractionService) recordBudgetExhaustedFailure(
	ctx context.Context,
	userID string,
	input ExtractInput,
	ectx *ExtractContext,
	startTime time.Time,
	budgetSkips []BudgetSkip,
) {
	if s.billing == nil {
		return
	}

	// Build detailed error message with all skipped models
	var skipDetails []string
	for _, skip := range budgetSkips {
		skipDetails = append(skipDetails, fmt.Sprintf("%s/%s ($%.4f)", skip.Provider, skip.Model, skip.EstimatedCost))
	}
	errorMessage := fmt.Sprintf("all %d models skipped due to budget constraints: %s",
		len(budgetSkips), strings.Join(skipDetails, ", "))

	usageRecord := &UsageRecord{
		UserID:          userID,
		JobType:         models.JobTypeExtract,
		Status:          "failed",
		TotalChargedUSD: 0, // No charge - nothing was attempted
		IsBYOK:          false,
		TargetURL:       input.URL,
		SchemaID:        ectx.SchemaID,
		ErrorMessage:    errorMessage,
		ErrorCode:       "budget_exhausted",
		PagesAttempted:  0,
		PagesSuccessful: 0,
		TotalDurationMs: int(time.Since(startTime).Milliseconds()),
	}

	// Use detached context - we want to record usage even if request timed out
	if recordErr := s.billing.RecordUsage(context.WithoutCancel(ctx), usageRecord); recordErr != nil {
		s.logger.Warn("failed to record budget exhausted failure", "error", recordErr)
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
	budgetSkips []BudgetSkip,
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

	// Log budget skips if any
	if len(budgetSkips) > 0 {
		s.logger.Info("extraction completed with budget skips",
			"user_id", userID,
			"models_skipped", len(budgetSkips),
		)
	}

	return &ExtractOutput{
		Data:        processedData,
		URL:         result.URL,
		FetchedAt:   result.FetchedAt,
		InputFormat: InputFormatSchema,
		Usage:       usageInfo,
		RawContent:  result.RawContent,
		Metadata: ExtractMeta{
			FetchDurationMs:   int(result.FetchDuration.Milliseconds()),
			ExtractDurationMs: int(result.ExtractDuration.Milliseconds()),
			Model:             result.Model,
			Provider:          result.Provider,
			BudgetSkips:       budgetSkips,
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
			MaxTokens:  s.getMaxTokens(ctx, injectedProvider, injectedModel, nil),
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

// NOTE: Crawl and CrawlWithCallback methods are in extraction_crawl.go


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
	if llmCfg.MaxTokens > 0 {
		opts = append(opts, refyne.WithMaxTokens(llmCfg.MaxTokens))
	}

	r, err := refyne.New(opts...)
	if err != nil {
		return nil, "", err
	}
	return r, chainName, nil
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
		MaxTokens:  s.getMaxTokens(ctx, "ollama", "llama3.2", nil),
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

// DetectInputFormat attempts to parse input as a structured schema.
// Returns the detected format (schema or prompt) and the parsed schema if successful.
// This is the shared validation helper for both extract and crawl endpoints.
func DetectInputFormat(data []byte) (InputFormat, schema.Schema, error) {
	sch, err := parseSchema(data)
	if err == nil {
		return InputFormatSchema, sch, nil
	}
	// If schema parsing fails, it's a freeform prompt
	return InputFormatPrompt, schema.Schema{}, err
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
