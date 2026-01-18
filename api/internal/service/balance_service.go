package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/jmylchreest/refyne-api/internal/config"
	"github.com/jmylchreest/refyne-api/internal/constants"
	"github.com/jmylchreest/refyne-api/internal/models"
	"github.com/jmylchreest/refyne-api/internal/repository"
)

var (
	// ErrInsufficientBalance indicates the user doesn't have enough balance.
	ErrInsufficientBalance = errors.New("insufficient balance")

	// ErrDuplicatePayment indicates a duplicate Stripe payment ID.
	ErrDuplicatePayment = errors.New("duplicate payment - already processed")
)

// BalanceService handles user balance and credit operations.
type BalanceService struct {
	repos         *repository.Repositories
	billingConfig *config.BillingConfig
	logger        *slog.Logger
}

// NewBalanceService creates a new balance service.
func NewBalanceService(repos *repository.Repositories, billingConfig *config.BillingConfig, logger *slog.Logger) *BalanceService {
	return &BalanceService{
		repos:         repos,
		billingConfig: billingConfig,
		logger:        logger,
	}
}

// GetBalance retrieves a user's current balance.
func (s *BalanceService) GetBalance(ctx context.Context, userID string) (*models.UserBalance, error) {
	balance, err := s.repos.Balance.Get(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get balance: %w", err)
	}
	return balance, nil
}

// GetAvailableBalance retrieves the user's available balance (considering expired credits).
func (s *BalanceService) GetAvailableBalance(ctx context.Context, userID string) (float64, error) {
	return s.repos.CreditTransaction.GetAvailableBalance(ctx, userID, time.Now())
}

// CheckBalance verifies if a user has sufficient balance for an operation.
func (s *BalanceService) CheckBalance(ctx context.Context, userID string, requiredUSD float64) error {
	available, err := s.GetAvailableBalance(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to check balance: %w", err)
	}

	if available < requiredUSD {
		return ErrInsufficientBalance
	}

	return nil
}

// AddSubscriptionCredits adds credits for a subscription payment.
// Uses Stripe/Clerk payment ID for idempotency.
// Credit expiry is determined by the tier's CreditRolloverMonths setting:
//   - -1: never expires
//   - 0: expires at end of current billing period
//   - N: expires N additional periods after current
func (s *BalanceService) AddSubscriptionCredits(ctx context.Context, userID, stripePaymentID, tier string) error {
	// Get tier limits which include credit allocation and rollover settings
	tierLimits := constants.GetTierLimits(tier)

	// Get allocation for the tier
	allocation := tierLimits.CreditAllocationUSD
	if allocation <= 0 {
		s.logger.Info("no credit allocation for tier", "user_id", userID, "tier", tier)
		return nil
	}

	// Calculate expiry date based on tier's rollover setting
	rolloverMonths := tierLimits.CreditRolloverMonths
	var expiresAt *time.Time

	if rolloverMonths == -1 {
		// Never expires - leave expiresAt as nil
		s.logger.Debug("credits never expire", "user_id", userID, "tier", tier)
	} else {
		// Get the user's current billing period
		balance, err := s.repos.Balance.Get(ctx, userID)
		if err != nil {
			s.logger.Warn("failed to get balance for expiry calculation, using fallback",
				"user_id", userID,
				"error", err,
			)
		}

		if balance != nil && balance.PeriodEnd != nil {
			// Use subscription period end as base, add rollover months
			expiry := balance.PeriodEnd.AddDate(0, rolloverMonths, 0)
			expiresAt = &expiry
			s.logger.Debug("credit expiry set from subscription period",
				"user_id", userID,
				"period_end", balance.PeriodEnd.Format(time.RFC3339),
				"rollover_months", rolloverMonths,
				"expires_at", expiry.Format(time.RFC3339),
			)
		} else {
			// Fallback: use calendar month end + rollover
			// This should only happen if webhook hasn't set period yet
			now := time.Now()
			endOfMonth := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, time.UTC).Add(-time.Second)
			expiry := endOfMonth.AddDate(0, rolloverMonths, 0)
			expiresAt = &expiry
			s.logger.Warn("using calendar month fallback for credit expiry",
				"user_id", userID,
				"expires_at", expiry.Format(time.RFC3339),
			)
		}
	}

	return s.addCredits(ctx, userID, models.TxTypeSubscription, allocation, expiresAt, &stripePaymentID, nil,
		fmt.Sprintf("%s subscription - $%.2f credit allocation", tier, allocation))
}

// AddTopUpCredits adds credits from a manual top-up purchase.
func (s *BalanceService) AddTopUpCredits(ctx context.Context, userID, stripePaymentID string, amountUSD float64) error {
	// Top-up credits don't expire (or have longer expiry - business decision)
	// For now, they don't expire
	return s.addCredits(ctx, userID, models.TxTypeTopUp, amountUSD, nil, &stripePaymentID, nil,
		fmt.Sprintf("Top-up purchase - $%.2f", amountUSD))
}

// DeductUsageCredits deducts credits for API usage.
func (s *BalanceService) DeductUsageCredits(ctx context.Context, userID string, amountUSD float64, jobID *string) error {
	if amountUSD <= 0 {
		return nil // Nothing to deduct
	}

	// Check if sufficient balance exists
	available, err := s.GetAvailableBalance(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to check balance: %w", err)
	}

	if available < amountUSD {
		return ErrInsufficientBalance
	}

	// Deduct the credits (negative amount)
	return s.addCredits(ctx, userID, models.TxTypeUsage, -amountUSD, nil, nil, jobID,
		fmt.Sprintf("API usage - $%.4f", amountUSD))
}

// ProcessRefund processes a refund. Does not claw back spent credits.
// Only adds back the refund amount up to what was originally credited.
func (s *BalanceService) ProcessRefund(ctx context.Context, userID, stripePaymentID string, refundAmountUSD float64) error {
	// Find the original transaction
	original, err := s.repos.CreditTransaction.GetByStripePaymentID(ctx, stripePaymentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Original transaction not found - this shouldn't happen
			s.logger.Warn("refund for unknown payment", "stripe_payment_id", stripePaymentID)
		}
		return fmt.Errorf("failed to find original transaction: %w", err)
	}

	// Don't refund more than originally credited
	if refundAmountUSD > original.AmountUSD {
		refundAmountUSD = original.AmountUSD
	}

	// Generate a unique ID for the refund transaction
	refundPaymentID := stripePaymentID + "_refund"

	return s.addCredits(ctx, userID, models.TxTypeRefund, refundAmountUSD, nil, &refundPaymentID, nil,
		fmt.Sprintf("Refund for payment %s - $%.2f", stripePaymentID, refundAmountUSD))
}

// AddAdjustment adds a manual admin adjustment to user's balance.
func (s *BalanceService) AddAdjustment(ctx context.Context, userID string, amountUSD float64, description string) error {
	return s.addCredits(ctx, userID, models.TxTypeAdjustment, amountUSD, nil, nil, nil, description)
}

// ExpireOldCredits marks expired credits and updates balances.
// This should be called periodically (e.g., daily cron job).
func (s *BalanceService) ExpireOldCredits(ctx context.Context) (int64, error) {
	count, err := s.repos.CreditTransaction.MarkExpired(ctx, time.Now())
	if err != nil {
		return 0, fmt.Errorf("failed to expire credits: %w", err)
	}

	if count > 0 {
		s.logger.Info("expired credits", "count", count)
	}

	return count, nil
}

// addCredits is the internal method for adding/deducting credits.
func (s *BalanceService) addCredits(ctx context.Context, userID string, txType models.CreditTransactionType,
	amountUSD float64, expiresAt *time.Time, stripePaymentID, jobID *string, description string) error {

	// Get current balance
	balance, err := s.repos.Balance.Get(ctx, userID)
	if err != nil {
		// Create new balance record if doesn't exist
		balance = &models.UserBalance{
			UserID:    userID,
			UpdatedAt: time.Now(),
		}
	}

	// Calculate new balance
	newBalance := balance.BalanceUSD + amountUSD

	// Create transaction record
	tx := &models.CreditTransaction{
		ID:              ulid.Make().String(),
		UserID:          userID,
		Type:            txType,
		AmountUSD:       amountUSD,
		BalanceAfter:    newBalance,
		ExpiresAt:       expiresAt,
		IsExpired:       false,
		StripePaymentID: stripePaymentID,
		JobID:           jobID,
		Description:     description,
		CreatedAt:       time.Now(),
	}

	// Try to create the transaction (will fail on duplicate Stripe ID)
	if err := s.repos.CreditTransaction.Create(ctx, tx); err != nil {
		if isDuplicateKeyError(err) {
			s.logger.Info("duplicate payment ignored", "stripe_payment_id", stripePaymentID)
			return ErrDuplicatePayment
		}
		return fmt.Errorf("failed to create credit transaction: %w", err)
	}

	// Update user balance
	if amountUSD > 0 {
		balance.LifetimeAdded += amountUSD
	} else {
		balance.LifetimeSpent -= amountUSD // Make positive since amount is negative
	}
	balance.BalanceUSD = newBalance
	balance.UpdatedAt = time.Now()

	if err := s.repos.Balance.Upsert(ctx, balance); err != nil {
		return fmt.Errorf("failed to update balance: %w", err)
	}

	s.logger.Info("credit transaction recorded",
		"user_id", userID,
		"type", txType,
		"amount_usd", amountUSD,
		"balance_after", newBalance,
	)

	return nil
}

// GetTransactionHistory retrieves a user's credit transaction history.
func (s *BalanceService) GetTransactionHistory(ctx context.Context, userID string, limit, offset int) ([]*models.CreditTransaction, error) {
	return s.repos.CreditTransaction.GetByUserID(ctx, userID, limit, offset)
}

// GetMonthlySpend retrieves how much a user has spent in a given month.
func (s *BalanceService) GetMonthlySpend(ctx context.Context, userID string, month time.Time) (float64, error) {
	return s.repos.Usage.GetMonthlySpend(ctx, userID, month)
}

// UpdateSubscriptionPeriod updates the user's current billing period dates.
// This is called when we receive Clerk subscription webhooks with period information.
func (s *BalanceService) UpdateSubscriptionPeriod(ctx context.Context, userID string, periodStart, periodEnd time.Time) error {
	if err := s.repos.Balance.UpdateSubscriptionPeriod(ctx, userID, periodStart, periodEnd); err != nil {
		return fmt.Errorf("failed to update subscription period: %w", err)
	}

	s.logger.Info("subscription period updated",
		"user_id", userID,
		"period_start", periodStart.Format(time.RFC3339),
		"period_end", periodEnd.Format(time.RFC3339),
	)

	return nil
}

// GetSubscriptionPeriod retrieves the user's current billing period.
// Returns nil times if no period is set.
func (s *BalanceService) GetSubscriptionPeriod(ctx context.Context, userID string) (*time.Time, *time.Time, error) {
	balance, err := s.repos.Balance.Get(ctx, userID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get balance: %w", err)
	}
	if balance == nil {
		return nil, nil, nil
	}
	return balance.PeriodStart, balance.PeriodEnd, nil
}

// isDuplicateKeyError checks if an error is a duplicate key constraint violation.
func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	// SQLite unique constraint violation
	errStr := err.Error()
	return strings.Contains(errStr, "UNIQUE constraint failed") ||
		strings.Contains(errStr, "duplicate key") ||
		strings.Contains(errStr, "already exists")
}
