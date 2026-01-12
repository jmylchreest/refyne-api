package service

import (
	"context"
	"log/slog"

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

// GetUsageSummary retrieves usage summary for a user.
func (s *UsageService) GetUsageSummary(ctx context.Context, userID string, period string) (*repository.UsageSummary, error) {
	if period == "" {
		period = "month"
	}
	return s.repos.Usage.GetSummary(ctx, userID, period)
}

// GetMonthlyUsage retrieves the current month's usage for a user.
// This is used for tier-based rate limiting.
func (s *UsageService) GetMonthlyUsage(ctx context.Context, userID string) (*repository.UsageSummary, error) {
	return s.repos.Usage.GetSummary(ctx, userID, "month")
}

// GetDailyUsage retrieves today's usage for a user.
func (s *UsageService) GetDailyUsage(ctx context.Context, userID string) (*repository.UsageSummary, error) {
	return s.repos.Usage.GetSummary(ctx, userID, "day")
}
