package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/jmylchreest/refyne-api/internal/config"
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
	repos         *repository.Repositories
	billingCfg    *config.BillingConfig
	orClient      *llm.OpenRouterClient
	logger        *slog.Logger
}

// NewBillingService creates a new billing service.
func NewBillingService(repos *repository.Repositories, billingCfg *config.BillingConfig, orClient *llm.OpenRouterClient, logger *slog.Logger) *BillingService {
	return &BillingService{
		repos:      repos,
		billingCfg: billingCfg,
		orClient:   orClient,
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
func (s *BillingService) CheckSufficientBalance(ctx context.Context, userID string, estimatedCostUSD float64) error {
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
func (s *BillingService) EstimateCost(pages int, model string) float64 {
	const avgTokensPerPage = 2000
	baseCost := llm.EstimateCost(avgTokensPerPage*pages, avgTokensPerPage*pages/4, model)
	// Add maximum possible markup (100% for free tier)
	return baseCost * 2
}

// CalculateTotalCost calculates the total cost including markup for a given tier.
func (s *BillingService) CalculateTotalCost(llmCostUSD float64, tier string) (totalUSD, markupRate, markupUSD float64) {
	markupRate = s.billingCfg.GetMarkup(tier)
	markupUSD = llmCostUSD * markupRate
	totalUSD = llmCostUSD + markupUSD
	return
}

// GetActualCost retrieves actual cost from OpenRouter or falls back to estimation.
func (s *BillingService) GetActualCost(ctx context.Context, tokensInput, tokensOutput int, model, generationID string) float64 {
	// TODO: Query OpenRouter for actual cost if generationID is available
	// For now, use estimation
	return llm.EstimateCost(tokensInput, tokensOutput, model)
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
	if provider == "ollama" {
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
	UserID      string
	Tier        string
	JobID       string
	JobType     models.JobType
	IsBYOK      bool
	TokensInput int
	TokensOutput int
	Model        string
	Provider     string

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

	// Get actual cost
	result.LLMCostUSD = s.GetActualCost(ctx, input.TokensInput, input.TokensOutput, input.Model, input.GenerationID)

	// Apply markup for non-BYOK
	if !input.IsBYOK && result.LLMCostUSD > 0 {
		result.TotalCostUSD, result.MarkupRate, result.MarkupUSD = s.CalculateTotalCost(result.LLMCostUSD, input.Tier)

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

	if err := s.RecordUsage(ctx, usageRecord); err != nil {
		s.logger.Warn("failed to record usage", "error", err)
	}

	return result, nil
}
