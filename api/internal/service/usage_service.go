package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/jmylchreest/refyne-api/internal/repository"
)

// UsageService handles usage tracking operations.
type UsageService struct {
	repos  *repository.Repositories
	logger *slog.Logger
}

// NewUsageService creates a new usage service.
func NewUsageService(repos *repository.Repositories, logger *slog.Logger) *UsageService {
	return &UsageService{
		repos:  repos,
		logger: logger,
	}
}

// GetUsageSummary retrieves usage summary for a user by period name.
// For quota checks, use GetBillingPeriodUsage instead.
func (s *UsageService) GetUsageSummary(ctx context.Context, userID string, period string) (*repository.UsageSummary, error) {
	if period == "" {
		period = "month"
	}
	return s.repos.Usage.GetSummary(ctx, userID, period)
}

// GetBillingPeriodUsage retrieves usage for the user's current billing period.
// All users have subscriptions (including free tier), so period dates should always be available.
// Falls back to calendar month only if webhook hasn't been received yet (temporary edge case).
func (s *UsageService) GetBillingPeriodUsage(ctx context.Context, userID string) (*repository.UsageSummary, error) {
	// Get user's subscription period
	balance, err := s.repos.Balance.Get(ctx, userID)
	if err != nil {
		s.logger.Warn("failed to get balance for period lookup, using calendar month fallback",
			"user_id", userID,
			"error", err,
		)
		return s.repos.Usage.GetSummary(ctx, userID, "month")
	}

	// Use subscription period dates if available
	// Note: We check both dates exist but don't require period_end to be in future
	// because cancelled subscriptions may have period_end in the past but we still
	// want to count usage for that final period
	if balance != nil && balance.PeriodStart != nil && balance.PeriodEnd != nil {
		s.logger.Debug("using subscription period for usage calculation",
			"user_id", userID,
			"period_start", balance.PeriodStart.Format(time.RFC3339),
			"period_end", balance.PeriodEnd.Format(time.RFC3339),
		)
		return s.repos.Usage.GetSummaryByDateRange(ctx, userID, *balance.PeriodStart, *balance.PeriodEnd)
	}

	// Fallback to calendar month - this should only happen if:
	// 1. Brand new user whose subscription webhook hasn't arrived yet
	// 2. Webhook processing failed
	// This is a temporary state that should self-correct when the webhook arrives
	s.logger.Warn("no subscription period found, using calendar month fallback - webhook may be delayed",
		"user_id", userID,
	)
	return s.repos.Usage.GetSummary(ctx, userID, "month")
}

// GetDailyUsage retrieves today's usage for a user.
func (s *UsageService) GetDailyUsage(ctx context.Context, userID string) (*repository.UsageSummary, error) {
	return s.repos.Usage.GetSummary(ctx, userID, "day")
}
