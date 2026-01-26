package service

import (
	"context"
	"log/slog"
)

// PreflightCheck performs pre-operation validation including cost estimation and balance checking.
// Returns nil if checks pass, or an error if the user cannot afford the operation.
type PreflightCheck struct {
	billing *BillingService
	logger  *slog.Logger
}

// NewPreflightCheck creates a new preflight check utility.
func NewPreflightCheck(billing *BillingService, logger *slog.Logger) *PreflightCheck {
	return &PreflightCheck{
		billing: billing,
		logger:  logger,
	}
}

// PreflightInput contains the parameters needed for preflight validation.
type PreflightInput struct {
	UserID   string
	Tier     string // User's subscription tier
	Provider string
	Model    string
	IsBYOK   bool
	Pages    int // Estimated number of pages (use 1 for single operations)
}

// PreflightResult contains the results of preflight validation.
type PreflightResult struct {
	EstimatedCost float64
	Passed        bool
}

// Check performs the preflight validation.
// For BYOK users, it only estimates cost (no balance check).
// For non-BYOK users, it verifies sufficient balance.
func (p *PreflightCheck) Check(ctx context.Context, input PreflightInput) (*PreflightResult, error) {
	result := &PreflightResult{Passed: true}

	if p.billing == nil {
		return result, nil
	}

	pages := input.Pages
	if pages < 1 {
		pages = 1
	}

	result.EstimatedCost = p.billing.EstimateCost(pages, input.Model, input.Provider)

	p.logger.Debug("pre-flight cost estimate",
		"user_id", input.UserID,
		"provider", input.Provider,
		"model", input.Model,
		"estimated_cost_usd", result.EstimatedCost,
		"is_byok", input.IsBYOK,
	)

	// Only check balance for non-BYOK users
	if !input.IsBYOK {
		if err := p.billing.CheckSufficientBalance(ctx, input.UserID, input.Tier, result.EstimatedCost); err != nil {
			result.Passed = false
			return result, err
		}
	}

	return result, nil
}

// CheckWithConfig performs preflight validation using an LLMConfigInput.
// This is a convenience wrapper for Check.
func (p *PreflightCheck) CheckWithConfig(ctx context.Context, userID, tier string, config *LLMConfigInput, isBYOK bool) (*PreflightResult, error) {
	if config == nil {
		return &PreflightResult{Passed: true}, nil
	}

	return p.Check(ctx, PreflightInput{
		UserID:   userID,
		Tier:     tier,
		Provider: config.Provider,
		Model:    config.Model,
		IsBYOK:   isBYOK,
		Pages:    1,
	})
}

// CheckWithConfigs performs preflight validation using the first config in a chain.
// This is useful for crawl operations that have a fallback chain.
func (p *PreflightCheck) CheckWithConfigs(ctx context.Context, userID, tier string, configs []*LLMConfigInput, isBYOK bool) (*PreflightResult, error) {
	if len(configs) == 0 {
		return &PreflightResult{Passed: true}, nil
	}

	return p.CheckWithConfig(ctx, userID, tier, configs[0], isBYOK)
}
