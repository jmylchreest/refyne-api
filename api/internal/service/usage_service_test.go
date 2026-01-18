package service

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/jmylchreest/refyne-api/internal/models"
	"github.com/jmylchreest/refyne-api/internal/repository"
)

// ========================================
// GetUsageSummary Tests
// ========================================

func TestUsageService_GetUsageSummary(t *testing.T) {
	mockUsageRepo := newMockUsageRepository()
	repos := &repository.Repositories{
		Usage: mockUsageRepo,
	}

	logger := slog.Default()
	svc := NewUsageService(repos, logger)

	// Set up test data
	mockUsageRepo.SetSummary("user-123", "month", &repository.UsageSummary{
		TotalJobs:       50,
		TotalChargedUSD: 2.50,
		BYOKJobs:        10,
	})
	mockUsageRepo.SetSummary("user-123", "day", &repository.UsageSummary{
		TotalJobs:       5,
		TotalChargedUSD: 0.25,
		BYOKJobs:        2,
	})

	t.Run("returns monthly summary", func(t *testing.T) {
		summary, err := svc.GetUsageSummary(context.Background(), "user-123", "month")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if summary == nil {
			t.Fatal("expected summary, got nil")
		}
		if summary.TotalJobs != 50 {
			t.Errorf("TotalJobs = %d, want 50", summary.TotalJobs)
		}
		if summary.TotalChargedUSD != 2.50 {
			t.Errorf("TotalChargedUSD = %f, want 2.50", summary.TotalChargedUSD)
		}
	})

	t.Run("defaults to month when period is empty", func(t *testing.T) {
		summary, err := svc.GetUsageSummary(context.Background(), "user-123", "")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if summary.TotalJobs != 50 {
			t.Errorf("TotalJobs = %d, want 50 (monthly summary)", summary.TotalJobs)
		}
	})

	t.Run("returns daily summary", func(t *testing.T) {
		summary, err := svc.GetUsageSummary(context.Background(), "user-123", "day")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if summary.TotalJobs != 5 {
			t.Errorf("TotalJobs = %d, want 5", summary.TotalJobs)
		}
	})

	t.Run("returns empty summary for user with no usage", func(t *testing.T) {
		summary, err := svc.GetUsageSummary(context.Background(), "user-no-usage", "month")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if summary == nil {
			t.Fatal("expected summary, got nil")
		}
		if summary.TotalJobs != 0 {
			t.Errorf("TotalJobs = %d, want 0", summary.TotalJobs)
		}
	})
}

// ========================================
// GetBillingPeriodUsage Tests
// ========================================

func TestUsageService_GetBillingPeriodUsage(t *testing.T) {
	mockUsageRepo := newMockUsageRepository()
	mockBalanceRepo := newMockBalanceRepository()
	repos := &repository.Repositories{
		Usage:   mockUsageRepo,
		Balance: mockBalanceRepo,
	}

	logger := slog.Default()
	svc := NewUsageService(repos, logger)

	t.Run("uses subscription period when available", func(t *testing.T) {
		periodStart := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
		periodEnd := time.Date(2024, 2, 15, 0, 0, 0, 0, time.UTC)

		mockBalanceRepo.SetBalance("user-with-sub", &models.UserBalance{
			UserID:      "user-with-sub",
			BalanceUSD:  10.00,
			PeriodStart: &periodStart,
			PeriodEnd:   &periodEnd,
		})

		mockUsageRepo.SetDateRangeSummary(&repository.UsageSummary{
			TotalJobs:       25,
			TotalChargedUSD: 1.25,
			BYOKJobs:        5,
		})

		summary, err := svc.GetBillingPeriodUsage(context.Background(), "user-with-sub")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if summary == nil {
			t.Fatal("expected summary, got nil")
		}
		// Should use date range summary
		if summary.TotalJobs != 25 {
			t.Errorf("TotalJobs = %d, want 25", summary.TotalJobs)
		}
	})

	t.Run("falls back to month when no period set", func(t *testing.T) {
		mockBalanceRepo.SetBalance("user-no-period", &models.UserBalance{
			UserID:     "user-no-period",
			BalanceUSD: 5.00,
			// No PeriodStart or PeriodEnd
		})

		mockUsageRepo.SetSummary("user-no-period", "month", &repository.UsageSummary{
			TotalJobs:       30,
			TotalChargedUSD: 1.50,
		})

		summary, err := svc.GetBillingPeriodUsage(context.Background(), "user-no-period")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		// Should fall back to monthly summary
		if summary.TotalJobs != 30 {
			t.Errorf("TotalJobs = %d, want 30", summary.TotalJobs)
		}
	})

	t.Run("falls back to month when balance not found", func(t *testing.T) {
		mockUsageRepo.SetSummary("user-new", "month", &repository.UsageSummary{
			TotalJobs:       10,
			TotalChargedUSD: 0.50,
		})

		summary, err := svc.GetBillingPeriodUsage(context.Background(), "user-new")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		// Should fall back to monthly summary
		if summary.TotalJobs != 10 {
			t.Errorf("TotalJobs = %d, want 10", summary.TotalJobs)
		}
	})

	t.Run("uses period even if end date is in past", func(t *testing.T) {
		// Cancelled subscription may have period_end in the past
		pastStart := time.Now().Add(-60 * 24 * time.Hour)
		pastEnd := time.Now().Add(-30 * 24 * time.Hour)

		mockBalanceRepo.SetBalance("user-cancelled", &models.UserBalance{
			UserID:      "user-cancelled",
			BalanceUSD:  0,
			PeriodStart: &pastStart,
			PeriodEnd:   &pastEnd,
		})

		mockUsageRepo.SetDateRangeSummary(&repository.UsageSummary{
			TotalJobs:       15,
			TotalChargedUSD: 0.75,
		})

		summary, err := svc.GetBillingPeriodUsage(context.Background(), "user-cancelled")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		// Should still use the period dates
		if summary.TotalJobs != 15 {
			t.Errorf("TotalJobs = %d, want 15", summary.TotalJobs)
		}
	})
}

// ========================================
// GetDailyUsage Tests
// ========================================

func TestUsageService_GetDailyUsage(t *testing.T) {
	mockUsageRepo := newMockUsageRepository()
	repos := &repository.Repositories{
		Usage: mockUsageRepo,
	}

	logger := slog.Default()
	svc := NewUsageService(repos, logger)

	mockUsageRepo.SetSummary("user-daily", "day", &repository.UsageSummary{
		TotalJobs:       8,
		TotalChargedUSD: 0.40,
		BYOKJobs:        3,
	})

	t.Run("returns daily usage", func(t *testing.T) {
		summary, err := svc.GetDailyUsage(context.Background(), "user-daily")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if summary == nil {
			t.Fatal("expected summary, got nil")
		}
		if summary.TotalJobs != 8 {
			t.Errorf("TotalJobs = %d, want 8", summary.TotalJobs)
		}
		if summary.BYOKJobs != 3 {
			t.Errorf("BYOKJobs = %d, want 3", summary.BYOKJobs)
		}
	})

	t.Run("returns empty for user with no daily usage", func(t *testing.T) {
		summary, err := svc.GetDailyUsage(context.Background(), "user-no-daily")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if summary.TotalJobs != 0 {
			t.Errorf("TotalJobs = %d, want 0", summary.TotalJobs)
		}
	})
}
