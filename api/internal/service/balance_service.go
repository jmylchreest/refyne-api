package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/jmylchreest/refyne-api/internal/config"
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
// Uses Stripe payment ID for idempotency.
func (s *BalanceService) AddSubscriptionCredits(ctx context.Context, userID, stripePaymentID, tier string) error {
	// Get allocation for the tier
	allocation := s.billingConfig.GetAllocation(tier)
	if allocation <= 0 {
		s.logger.Info("no credit allocation for tier", "user_id", userID, "tier", tier)
		return nil
	}

	// Calculate expiry date based on rollover config
	var expiresAt *time.Time
	if s.billingConfig.CreditRolloverMonths > 0 {
		expiry := time.Now().AddDate(0, s.billingConfig.CreditRolloverMonths+1, 0) // +1 for current month
		expiresAt = &expiry
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

// RecalculateExpiry updates expiry dates for all non-expired subscription credits.
// Useful when the rollover configuration changes.
func (s *BalanceService) RecalculateExpiry(ctx context.Context) error {
	credits, err := s.repos.CreditTransaction.GetNonExpiredSubscriptionCredits(ctx)
	if err != nil {
		return fmt.Errorf("failed to get subscription credits: %w", err)
	}

	for _, credit := range credits {
		var newExpiry *time.Time

		if s.billingConfig.CreditRolloverMonths > 0 {
			// Calculate new expiry based on creation date + rollover months + 1
			expiry := credit.CreatedAt.AddDate(0, s.billingConfig.CreditRolloverMonths+1, 0)
			newExpiry = &expiry
		}
		// If rollover is 0, newExpiry stays nil (no expiry)

		// Only update if different
		if !expiryEqual(credit.ExpiresAt, newExpiry) {
			if err := s.repos.CreditTransaction.UpdateExpiry(ctx, credit.ID, newExpiry); err != nil {
				s.logger.Error("failed to update expiry", "id", credit.ID, "error", err)
				continue
			}
		}
	}

	s.logger.Info("recalculated expiry dates", "count", len(credits))
	return nil
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

// expiryEqual compares two expiry times, handling nil values.
func expiryEqual(a, b *time.Time) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Equal(*b)
}

// isDuplicateKeyError checks if an error is a duplicate key constraint violation.
func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	// SQLite unique constraint violation
	errStr := err.Error()
	return contains(errStr, "UNIQUE constraint failed") ||
		contains(errStr, "duplicate key") ||
		contains(errStr, "already exists")
}

// contains checks if s contains substr (simple implementation).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsImpl(s, substr))
}

func containsImpl(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
