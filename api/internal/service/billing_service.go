package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/jmylchreest/refyne-api/internal/config"
	"github.com/jmylchreest/refyne-api/internal/constants"
	"github.com/jmylchreest/refyne-api/internal/llm"
	"github.com/jmylchreest/refyne-api/internal/models"
	"github.com/jmylchreest/refyne-api/internal/repository"
)

// BillingService handles all billing-related operations.
// This provides a unified interface for:
// - Balance management (credits, deductions)
// - Usage recording (lean + insights tables)
// - Cost estimation and calculation
// - Schema snapshot management
type BillingService struct {
	repos      *repository.Repositories
	billingCfg *config.BillingConfig
	pricingSvc *PricingService
	logger     *slog.Logger
}

// NewBillingService creates a new billing service.
func NewBillingService(repos *repository.Repositories, billingCfg *config.BillingConfig, pricingSvc *PricingService, logger *slog.Logger) *BillingService {
	return &BillingService{
		repos:      repos,
		billingCfg: billingCfg,
		pricingSvc: pricingSvc,
		logger:     logger,
	}
}

// ========================================
// Balance Operations
// ========================================

// GetAvailableBalance returns the user's available balance in USD.
func (s *BillingService) GetAvailableBalance(ctx context.Context, userID string) (float64, error) {
	return s.repos.CreditTransaction.GetAvailableBalance(ctx, userID, time.Now())
}

// CheckSufficientBalance verifies the user has enough balance for an estimated operation.
// Returns an error if insufficient balance.
// For free tier users, skip the balance check - they're limited by extraction quota instead.
func (s *BillingService) CheckSufficientBalance(ctx context.Context, userID, tier string, estimatedCostUSD float64) error {
	// Skip balance check for free tier - they're limited by monthly extraction quota
	normalizedTier := constants.NormalizeTierName(tier)
	if normalizedTier == constants.TierFree {
		s.logger.Debug("skipping balance check for free tier user",
			"user_id", userID,
			"tier", tier,
		)
		return nil
	}

	available, err := s.GetAvailableBalance(ctx, userID)
	if err != nil {
		s.logger.Warn("failed to check balance, allowing operation", "user_id", userID, "error", err)
		return nil // Don't block on balance check errors
	}

	if available < estimatedCostUSD {
		return llm.NewInsufficientCreditsError("", int(estimatedCostUSD*100), int(available*100))
	}

	return nil
}

// DeductUsage deducts credits for API usage.
func (s *BillingService) DeductUsage(ctx context.Context, userID string, amountUSD float64, jobID *string) error {
	if amountUSD <= 0 {
		return nil
	}

	// Get current balance
	balance, err := s.repos.Balance.Get(ctx, userID)
	if err != nil {
		balance = &models.UserBalance{
			UserID:    userID,
			UpdatedAt: time.Now(),
		}
	}

	newBalance := balance.BalanceUSD - amountUSD

	// Create transaction record
	tx := &models.CreditTransaction{
		ID:           ulid.Make().String(),
		UserID:       userID,
		Type:         models.TxTypeUsage,
		AmountUSD:    -amountUSD,
		BalanceAfter: newBalance,
		JobID:        jobID,
		Description:  fmt.Sprintf("API usage - $%.4f", amountUSD),
		CreatedAt:    time.Now(),
	}

	if err := s.repos.CreditTransaction.Create(ctx, tx); err != nil {
		return fmt.Errorf("failed to create credit transaction: %w", err)
	}

	balance.BalanceUSD = newBalance
	balance.LifetimeSpent += amountUSD
	balance.UpdatedAt = time.Now()

	if err := s.repos.Balance.Upsert(ctx, balance); err != nil {
		return fmt.Errorf("failed to update balance: %w", err)
	}

	return nil
}

// ========================================
// Cost Estimation & Calculation
// ========================================

// EstimateCost provides a conservative cost estimate for pre-flight balance checks.
func (s *BillingService) EstimateCost(pages int, model, provider string) float64 {
	const avgTokensPerPage = 2000
	inputTokens := avgTokensPerPage * pages
	outputTokens := avgTokensPerPage * pages / 4

	baseCost := s.pricingSvc.EstimateCost(provider, model, inputTokens, outputTokens)
	// Add maximum possible markup (100% for free tier)
	return baseCost * 2
}

// CalculateTotalCost calculates the total cost including markup and per-transaction fees.
// Returns the total, markup percentage, and markup amount.
// Markup and per-transaction costs are configured per-tier in constants.TierLimits.
// For BYOK transactions, pass isBYOK=true to skip markup (user pays provider directly).
func (s *BillingService) CalculateTotalCost(llmCostUSD float64, tier string, isBYOK bool) (totalUSD, markupRate, markupUSD float64) {
	// BYOK transactions have no markup - user pays provider directly
	if isBYOK {
		return llmCostUSD, 0, 0
	}

	tierLimits := constants.GetTierLimits(tier)
	markupRate = tierLimits.MarkupPercentage
	markupUSD = llmCostUSD * markupRate
	perTxCost := tierLimits.CostPerTransaction
	totalUSD = llmCostUSD + markupUSD + perTxCost
	return
}

// GetActualCost calculates cost based on token counts using cached pricing data.
// For actual recorded cost from the provider, use GetActualCostFromProvider with the generation ID.
func (s *BillingService) GetActualCost(ctx context.Context, tokensInput, tokensOutput int, model, provider string) float64 {
	return s.pricingSvc.EstimateCost(provider, model, tokensInput, tokensOutput)
}

// GetActualCostFromProvider retrieves the actual recorded cost from the provider.
// This makes an API call, so should only be used for final cost recording.
// Returns the cost and any error. If error, falls back to estimation.
// apiKey is the user's API key for BYOK users (used to query their generation stats).
func (s *BillingService) GetActualCostFromProvider(ctx context.Context, provider, generationID, apiKey string, tokensInput, tokensOutput int, model string) float64 {
	if s.pricingSvc != nil && generationID != "" {
		cost, err := s.pricingSvc.GetActualCost(ctx, provider, generationID, apiKey)
		if err == nil && cost > 0 {
			s.logger.Debug("using actual cost from provider",
				"provider", provider,
				"generation_id", generationID,
				"cost_usd", cost,
			)
			return cost
		}
		// Log the error but don't fail - fall back to estimation
		if err != nil {
			s.logger.Debug("failed to get actual cost from provider, using estimation",
				"provider", provider,
				"generation_id", generationID,
				"error", err,
			)
		} else if cost == 0 {
			s.logger.Debug("provider returned zero cost, using estimation",
				"provider", provider,
				"generation_id", generationID,
			)
		}
	} else if generationID == "" {
		s.logger.Debug("no generation ID available, using estimation",
			"provider", provider,
			"model", model,
		)
	}
	// Fall back to estimation
	cost := s.GetActualCost(ctx, tokensInput, tokensOutput, model, provider)
	s.logger.Debug("cost estimated",
		"provider", provider,
		"model", model,
		"input_tokens", tokensInput,
		"output_tokens", tokensOutput,
		"cost_usd", cost,
	)
	return cost
}

// ========================================
// Cost Calculation Helpers
// ========================================

// CostResult holds calculated costs for an operation.
type CostResult struct {
	LLMCostUSD  float64 // Actual cost from LLM provider
	UserCostUSD float64 // Total cost charged to user (with markup)
	MarkupRate  float64 // Markup percentage applied
	MarkupUSD   float64 // Markup amount in USD
}

// CostInput holds input parameters for cost calculation.
type CostInput struct {
	TokensInput  int
	TokensOutput int
	Model        string
	Provider     string
	Tier         string
	IsBYOK       bool
	GenerationID string // Optional: for fetching actual cost from provider
	APIKey       string // Optional: for BYOK actual cost lookup
}

// CalculateCosts is a helper that calculates both LLM and user costs in one call.
// This reduces duplication across extraction services.
func (s *BillingService) CalculateCosts(ctx context.Context, input CostInput) CostResult {
	var result CostResult

	// Get LLM cost - try actual first if we have generation ID
	if input.GenerationID != "" {
		result.LLMCostUSD = s.GetActualCostFromProvider(ctx, input.Provider, input.GenerationID, input.APIKey, input.TokensInput, input.TokensOutput, input.Model)
	} else {
		result.LLMCostUSD = s.GetActualCost(ctx, input.TokensInput, input.TokensOutput, input.Model, input.Provider)
	}

	// Calculate user cost with markup
	result.UserCostUSD, result.MarkupRate, result.MarkupUSD = s.CalculateTotalCost(result.LLMCostUSD, input.Tier, input.IsBYOK)

	return result
}

// ========================================
// Schema Snapshot Management
// ========================================

// GetOrCreateSchemaSnapshot returns an existing schema snapshot or creates a new one.
func (s *BillingService) GetOrCreateSchemaSnapshot(ctx context.Context, userID, schemaJSON string) (*models.SchemaSnapshot, error) {
	hash := hashSchema(schemaJSON)

	// Check if schema already exists for this user
	existing, err := s.repos.SchemaSnapshot.GetByUserAndHash(ctx, userID, hash)
	if err == nil && existing != nil {
		if err := s.repos.SchemaSnapshot.IncrementUsageCount(ctx, existing.ID); err != nil {
			s.logger.Warn("failed to increment schema usage count", "id", existing.ID, "error", err)
		}
		return existing, nil
	}

	version, err := s.repos.SchemaSnapshot.GetNextVersion(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get next version: %w", err)
	}

	snapshot := &models.SchemaSnapshot{
		ID:         ulid.Make().String(),
		UserID:     userID,
		Hash:       hash,
		SchemaJSON: schemaJSON,
		Version:    version,
		UsageCount: 1,
		CreatedAt:  time.Now(),
	}

	if err := s.repos.SchemaSnapshot.Create(ctx, snapshot); err != nil {
		return nil, fmt.Errorf("failed to create schema snapshot: %w", err)
	}

	return snapshot, nil
}

// ========================================
// Usage Recording
// ========================================

// UsageRecord holds all data needed to record usage.
type UsageRecord struct {
	UserID          string
	JobID           string
	JobType         models.JobType
	Status          string // success, failed, partial
	TotalChargedUSD float64
	IsBYOK          bool

	// Insight details
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

// RecordUsage records usage to both lean billing table and rich insights table.
func (s *BillingService) RecordUsage(ctx context.Context, record *UsageRecord) error {
	usageID := ulid.Make().String()
	now := time.Now()

	// Record to lean billing table
	usageRec := &models.UsageRecord{
		ID:              usageID,
		UserID:          record.UserID,
		JobID:           record.JobID,
		Date:            now.Format("2006-01-02"),
		Type:            record.JobType,
		Status:          record.Status,
		TotalChargedUSD: record.TotalChargedUSD,
		IsBYOK:          record.IsBYOK,
		CreatedAt:       now,
	}

	if err := s.repos.Usage.Create(ctx, usageRec); err != nil {
		return fmt.Errorf("failed to record usage: %w", err)
	}

	// Record to rich insights table
	insight := &models.UsageInsight{
		ID:                ulid.Make().String(),
		UsageID:           usageID,
		TargetURL:         record.TargetURL,
		SchemaID:          record.SchemaID,
		CrawlConfigJSON:   record.CrawlConfigJSON,
		ErrorMessage:      record.ErrorMessage,
		ErrorCode:         record.ErrorCode,
		TokensInput:       record.TokensInput,
		TokensOutput:      record.TokensOutput,
		LLMCostUSD:        record.LLMCostUSD,
		MarkupRate:        record.MarkupRate,
		MarkupUSD:         record.MarkupUSD,
		LLMProvider:       record.LLMProvider,
		LLMModel:          record.LLMModel,
		GenerationID:      record.GenerationID,
		BYOKProvider:      record.BYOKProvider,
		PagesAttempted:    record.PagesAttempted,
		PagesSuccessful:   record.PagesSuccessful,
		FetchDurationMs:   record.FetchDurationMs,
		ExtractDurationMs: record.ExtractDurationMs,
		TotalDurationMs:   record.TotalDurationMs,
		RequestID:         record.RequestID,
		UserAgent:         record.UserAgent,
		IPCountry:         record.IPCountry,
		CreatedAt:         now,
	}

	if err := s.repos.UsageInsight.Create(ctx, insight); err != nil {
		s.logger.Warn("failed to record usage insight", "error", err)
		// Don't fail - lean record is more important
	}

	return nil
}

// ========================================
// BYOK Detection
// ========================================

// IsBYOK determines if the request is using user's own API key.
func (s *BillingService) IsBYOK(provider, apiKey, serviceOpenRouterKey, serviceAnthropicKey, serviceOpenAIKey string) bool {
	// Ollama is local, so effectively BYOK (no API cost)
	if provider == llm.ProviderOllama {
		return true
	}

	// Check if API key matches any of our service keys
	if apiKey != "" {
		if serviceOpenRouterKey != "" && apiKey == serviceOpenRouterKey {
			return false
		}
		if serviceAnthropicKey != "" && apiKey == serviceAnthropicKey {
			return false
		}
		if serviceOpenAIKey != "" && apiKey == serviceOpenAIKey {
			return false
		}
		// If we have an API key that's not ours, it's BYOK
		return true
	}

	return false
}

// ========================================
// Combined Operations
// ========================================

// ChargeForUsageInput contains all info needed to charge and record usage.
type ChargeForUsageInput struct {
	UserID       string
	Tier         string
	JobID        string
	JobType      models.JobType
	IsBYOK       bool
	TokensInput  int
	TokensOutput int
	Model        string
	Provider     string
	APIKey       string // User's API key for BYOK (used to query actual cost)

	// For recording insights
	TargetURL         string
	SchemaID          string
	CrawlConfigJSON   string
	ErrorMessage      string
	ErrorCode         string
	GenerationID      string
	PagesAttempted    int
	PagesSuccessful   int
	FetchDurationMs   int
	ExtractDurationMs int
	TotalDurationMs   int
	RequestID         string
	UserAgent         string
	IPCountry         string
}

// ChargeForUsageResult contains the billing results.
type ChargeForUsageResult struct {
	LLMCostUSD   float64
	MarkupRate   float64
	MarkupUSD    float64
	TotalCostUSD float64
}

// ChargeForUsage handles the complete billing flow: calculate cost, deduct credits, record usage.
func (s *BillingService) ChargeForUsage(ctx context.Context, input *ChargeForUsageInput) (*ChargeForUsageResult, error) {
	result := &ChargeForUsageResult{}

	// Calculate costs using the shared helper
	costs := s.CalculateCosts(ctx, CostInput{
		TokensInput:  input.TokensInput,
		TokensOutput: input.TokensOutput,
		Model:        input.Model,
		Provider:     input.Provider,
		Tier:         input.Tier,
		IsBYOK:       input.IsBYOK,
		GenerationID: input.GenerationID,
		APIKey:       input.APIKey,
	})
	result.LLMCostUSD = costs.LLMCostUSD
	result.TotalCostUSD = costs.UserCostUSD
	result.MarkupRate = costs.MarkupRate
	result.MarkupUSD = costs.MarkupUSD

	// Deduct credits for non-BYOK
	if !input.IsBYOK && result.TotalCostUSD > 0 {

		// Deduct credits
		jobID := &input.JobID
		if input.JobID == "" {
			jobID = nil
		}
		if err := s.DeductUsage(ctx, input.UserID, result.TotalCostUSD, jobID); err != nil {
			s.logger.Warn("failed to deduct credits", "user_id", input.UserID, "error", err)
			// Continue - don't fail the operation for billing errors
		}
	}

	// Determine status
	status := "success"
	if input.ErrorMessage != "" {
		status = "failed"
	} else if input.PagesAttempted > 0 && input.PagesSuccessful < input.PagesAttempted {
		status = "partial"
	}

	// Record usage
	usageRecord := &UsageRecord{
		UserID:            input.UserID,
		JobID:             input.JobID,
		JobType:           input.JobType,
		Status:            status,
		TotalChargedUSD:   result.TotalCostUSD,
		IsBYOK:            input.IsBYOK,
		TargetURL:         input.TargetURL,
		SchemaID:          input.SchemaID,
		CrawlConfigJSON:   input.CrawlConfigJSON,
		ErrorMessage:      input.ErrorMessage,
		ErrorCode:         input.ErrorCode,
		TokensInput:       input.TokensInput,
		TokensOutput:      input.TokensOutput,
		LLMCostUSD:        result.LLMCostUSD,
		MarkupRate:        result.MarkupRate,
		MarkupUSD:         result.MarkupUSD,
		LLMProvider:       input.Provider,
		LLMModel:          input.Model,
		GenerationID:      input.GenerationID,
		PagesAttempted:    input.PagesAttempted,
		PagesSuccessful:   input.PagesSuccessful,
		FetchDurationMs:   input.FetchDurationMs,
		ExtractDurationMs: input.ExtractDurationMs,
		TotalDurationMs:   input.TotalDurationMs,
		RequestID:         input.RequestID,
		UserAgent:         input.UserAgent,
		IPCountry:         input.IPCountry,
	}

	if input.IsBYOK {
		usageRecord.BYOKProvider = input.Provider
	}

	// Use detached context - we want to record usage even if request timed out
	if err := s.RecordUsage(context.WithoutCancel(ctx), usageRecord); err != nil {
		s.logger.Warn("failed to record usage", "error", err)
	}

	return result, nil
}
