package service

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/jmylchreest/refyne-api/internal/config"
	"github.com/jmylchreest/refyne-api/internal/models"
	"github.com/jmylchreest/refyne-api/internal/repository"
)

// mockBalanceRepository implements repository.BalanceRepository for testing
type mockBalanceRepository struct {
	mu       sync.RWMutex
	balances map[string]*models.UserBalance
	getErr   error
}

func newMockBalanceRepository() *mockBalanceRepository {
	return &mockBalanceRepository{
		balances: make(map[string]*models.UserBalance),
	}
}

func (m *mockBalanceRepository) Get(ctx context.Context, userID string) (*models.UserBalance, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.getErr != nil {
		return nil, m.getErr
	}
	if b, ok := m.balances[userID]; ok {
		// Return a copy to avoid data races
		copy := *b
		return &copy, nil
	}
	return nil, nil
}

func (m *mockBalanceRepository) Upsert(ctx context.Context, balance *models.UserBalance) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.balances[balance.UserID] = balance
	return nil
}

func (m *mockBalanceRepository) GetAvailableBalance(ctx context.Context, userID string, now time.Time) (float64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if b, ok := m.balances[userID]; ok {
		return b.BalanceUSD, nil
	}
	return 0, nil
}

func (m *mockBalanceRepository) UpdateSubscriptionPeriod(ctx context.Context, userID string, periodStart, periodEnd time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if b, ok := m.balances[userID]; ok {
		b.PeriodStart = &periodStart
		b.PeriodEnd = &periodEnd
	} else {
		m.balances[userID] = &models.UserBalance{
			UserID:      userID,
			PeriodStart: &periodStart,
			PeriodEnd:   &periodEnd,
		}
	}
	return nil
}

func (m *mockBalanceRepository) SetBalance(userID string, balance *models.UserBalance) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.balances[userID] = balance
}

// mockCreditTransactionRepository implements repository.CreditTransactionRepository for testing
type mockCreditTransactionRepository struct {
	mu               sync.RWMutex
	transactions     []*models.CreditTransaction
	byStripePayment  map[string]*models.CreditTransaction
	availableBalance float64
	createErr        error
	duplicateKeys    map[string]bool
}

func newMockCreditTransactionRepository() *mockCreditTransactionRepository {
	return &mockCreditTransactionRepository{
		transactions:    make([]*models.CreditTransaction, 0),
		byStripePayment: make(map[string]*models.CreditTransaction),
		duplicateKeys:   make(map[string]bool),
	}
}

func (m *mockCreditTransactionRepository) Create(ctx context.Context, tx *models.CreditTransaction) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.createErr != nil {
		return m.createErr
	}
	// Check for duplicate stripe payment ID
	if tx.StripePaymentID != nil {
		if m.duplicateKeys[*tx.StripePaymentID] {
			return errors.New("UNIQUE constraint failed: credit_transactions.stripe_payment_id")
		}
		m.duplicateKeys[*tx.StripePaymentID] = true
		m.byStripePayment[*tx.StripePaymentID] = tx
	}
	m.transactions = append(m.transactions, tx)
	return nil
}

func (m *mockCreditTransactionRepository) GetByUserID(ctx context.Context, userID string, limit, offset int) ([]*models.CreditTransaction, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*models.CreditTransaction
	for _, tx := range m.transactions {
		if tx.UserID == userID {
			result = append(result, tx)
		}
	}
	return result, nil
}

func (m *mockCreditTransactionRepository) GetByStripePaymentID(ctx context.Context, stripePaymentID string) (*models.CreditTransaction, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if tx, ok := m.byStripePayment[stripePaymentID]; ok {
		return tx, nil
	}
	return nil, sql.ErrNoRows
}

func (m *mockCreditTransactionRepository) GetNonExpiredSubscriptionCredits(ctx context.Context) ([]*models.CreditTransaction, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*models.CreditTransaction
	for _, tx := range m.transactions {
		if tx.Type == models.TxTypeSubscription && !tx.IsExpired {
			result = append(result, tx)
		}
	}
	return result, nil
}

func (m *mockCreditTransactionRepository) UpdateExpiry(ctx context.Context, id string, expiresAt *time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, tx := range m.transactions {
		if tx.ID == id {
			tx.ExpiresAt = expiresAt
			return nil
		}
	}
	return sql.ErrNoRows
}

func (m *mockCreditTransactionRepository) MarkExpired(ctx context.Context, now time.Time) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var count int64
	for _, tx := range m.transactions {
		if tx.ExpiresAt != nil && tx.ExpiresAt.Before(now) && !tx.IsExpired {
			tx.IsExpired = true
			count++
		}
	}
	return count, nil
}

func (m *mockCreditTransactionRepository) GetAvailableBalance(ctx context.Context, userID string, now time.Time) (float64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.availableBalance, nil
}

// mockUsageRepository implements repository.UsageRepository for testing
type mockUsageRepository struct {
	mu            sync.RWMutex
	monthlySpend  map[string]float64
	summaries     map[string]map[string]*repository.UsageSummary // userID -> period -> summary
	dateSummary   *repository.UsageSummary
}

func newMockUsageRepository() *mockUsageRepository {
	return &mockUsageRepository{
		monthlySpend: make(map[string]float64),
		summaries:    make(map[string]map[string]*repository.UsageSummary),
	}
}

func (m *mockUsageRepository) Create(ctx context.Context, record *models.UsageRecord) error {
	return nil
}

func (m *mockUsageRepository) GetByUserID(ctx context.Context, userID string, startDate, endDate string) ([]*models.UsageRecord, error) {
	return nil, nil
}

func (m *mockUsageRepository) GetSummary(ctx context.Context, userID string, period string) (*repository.UsageSummary, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if userSummaries, ok := m.summaries[userID]; ok {
		if summary, ok := userSummaries[period]; ok {
			return summary, nil
		}
	}
	return &repository.UsageSummary{}, nil
}

func (m *mockUsageRepository) GetSummaryByDateRange(ctx context.Context, userID string, startDate, endDate time.Time) (*repository.UsageSummary, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.dateSummary != nil {
		return m.dateSummary, nil
	}
	return &repository.UsageSummary{}, nil
}

func (m *mockUsageRepository) GetMonthlySpend(ctx context.Context, userID string, month time.Time) (float64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key := userID + month.Format("2006-01")
	return m.monthlySpend[key], nil
}

func (m *mockUsageRepository) CountByUserAndDateRange(ctx context.Context, userID string, startDate, endDate string) (int, error) {
	return 0, nil
}

func (m *mockUsageRepository) SetSummary(userID, period string, summary *repository.UsageSummary) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.summaries[userID] == nil {
		m.summaries[userID] = make(map[string]*repository.UsageSummary)
	}
	m.summaries[userID][period] = summary
}

func (m *mockUsageRepository) SetDateRangeSummary(summary *repository.UsageSummary) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dateSummary = summary
}

// Test helper to create a new test balance service
func newTestBalanceService() (*BalanceService, *mockBalanceRepository, *mockCreditTransactionRepository, *mockUsageRepository) {
	balanceRepo := newMockBalanceRepository()
	creditRepo := newMockCreditTransactionRepository()
	usageRepo := newMockUsageRepository()

	repos := &repository.Repositories{
		Balance:           balanceRepo,
		CreditTransaction: creditRepo,
		Usage:             usageRepo,
	}

	billingCfg := config.DefaultBillingConfig()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	svc := NewBalanceService(repos, &billingCfg, logger)
	return svc, balanceRepo, creditRepo, usageRepo
}

func TestCheckBalance(t *testing.T) {
	svc, _, creditRepo, _ := newTestBalanceService()
	ctx := context.Background()
	userID := "user_123"

	tests := []struct {
		name             string
		availableBalance float64
		requiredUSD      float64
		wantErr          error
	}{
		{"sufficient balance", 10.0, 5.0, nil},
		{"exact balance", 5.0, 5.0, nil},
		{"insufficient balance", 3.0, 5.0, ErrInsufficientBalance},
		{"zero balance", 0.0, 5.0, ErrInsufficientBalance},
		{"zero required", 10.0, 0.0, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			creditRepo.availableBalance = tt.availableBalance

			err := svc.CheckBalance(ctx, userID, tt.requiredUSD)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("CheckBalance() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestDeductUsageCredits(t *testing.T) {
	ctx := context.Background()
	userID := "user_123"

	tests := []struct {
		name             string
		availableBalance float64
		amountUSD        float64
		wantErr          error
	}{
		{"successful deduction", 10.0, 5.0, nil},
		{"zero amount", 10.0, 0.0, nil},
		{"negative amount", 10.0, -5.0, nil}, // Should be no-op
		{"insufficient balance", 3.0, 5.0, ErrInsufficientBalance},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, balanceRepo, creditRepo, _ := newTestBalanceService()
			creditRepo.availableBalance = tt.availableBalance

			// Set up initial balance
			balanceRepo.balances[userID] = &models.UserBalance{
				UserID:     userID,
				BalanceUSD: tt.availableBalance,
			}

			jobID := "job_123"
			err := svc.DeductUsageCredits(ctx, userID, tt.amountUSD, &jobID)

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("DeductUsageCredits() error = %v, want %v", err, tt.wantErr)
			}

			// Verify transaction was created for successful deduction
			if tt.wantErr == nil && tt.amountUSD > 0 {
				if len(creditRepo.transactions) == 0 {
					t.Error("expected transaction to be created")
				} else {
					tx := creditRepo.transactions[0]
					if tx.AmountUSD != -tt.amountUSD {
						t.Errorf("transaction amount = %v, want %v", tx.AmountUSD, -tt.amountUSD)
					}
					if tx.Type != models.TxTypeUsage {
						t.Errorf("transaction type = %v, want %v", tx.Type, models.TxTypeUsage)
					}
				}
			}
		})
	}
}

func TestAddTopUpCredits(t *testing.T) {
	svc, balanceRepo, creditRepo, _ := newTestBalanceService()
	ctx := context.Background()
	userID := "user_123"
	stripePaymentID := "pi_123456"

	// Set up initial balance (user exists but has 0 balance)
	balanceRepo.balances[userID] = &models.UserBalance{
		UserID:     userID,
		BalanceUSD: 0,
	}

	// First addition should succeed
	err := svc.AddTopUpCredits(ctx, userID, stripePaymentID, 50.0)
	if err != nil {
		t.Fatalf("AddTopUpCredits() error = %v", err)
	}

	// Verify transaction was created
	if len(creditRepo.transactions) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(creditRepo.transactions))
	}

	tx := creditRepo.transactions[0]
	if tx.AmountUSD != 50.0 {
		t.Errorf("transaction amount = %v, want 50.0", tx.AmountUSD)
	}
	if tx.Type != models.TxTypeTopUp {
		t.Errorf("transaction type = %v, want %v", tx.Type, models.TxTypeTopUp)
	}
	if tx.ExpiresAt != nil {
		t.Error("top-up credits should not expire")
	}

	// Verify balance was updated
	balance := balanceRepo.balances[userID]
	if balance == nil {
		t.Fatal("balance not created")
	}
	if balance.BalanceUSD != 50.0 {
		t.Errorf("balance = %v, want 50.0", balance.BalanceUSD)
	}
	if balance.LifetimeAdded != 50.0 {
		t.Errorf("lifetime added = %v, want 50.0", balance.LifetimeAdded)
	}

	// Duplicate payment should fail
	err = svc.AddTopUpCredits(ctx, userID, stripePaymentID, 50.0)
	if !errors.Is(err, ErrDuplicatePayment) {
		t.Errorf("duplicate payment error = %v, want %v", err, ErrDuplicatePayment)
	}
}

func TestAddAdjustment(t *testing.T) {
	svc, balanceRepo, creditRepo, _ := newTestBalanceService()
	ctx := context.Background()
	userID := "user_123"

	tests := []struct {
		name        string
		amount      float64
		description string
	}{
		{"positive adjustment", 25.0, "manual credit"},
		{"negative adjustment", -10.0, "manual debit"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset for each test
			balanceRepo.balances = make(map[string]*models.UserBalance)
			balanceRepo.balances[userID] = &models.UserBalance{UserID: userID, BalanceUSD: 0}
			creditRepo.transactions = nil

			err := svc.AddAdjustment(ctx, userID, tt.amount, tt.description)
			if err != nil {
				t.Fatalf("AddAdjustment() error = %v", err)
			}

			if len(creditRepo.transactions) != 1 {
				t.Fatalf("expected 1 transaction, got %d", len(creditRepo.transactions))
			}

			tx := creditRepo.transactions[0]
			if tx.AmountUSD != tt.amount {
				t.Errorf("transaction amount = %v, want %v", tx.AmountUSD, tt.amount)
			}
			if tx.Type != models.TxTypeAdjustment {
				t.Errorf("transaction type = %v, want %v", tx.Type, models.TxTypeAdjustment)
			}
			if tx.Description != tt.description {
				t.Errorf("description = %v, want %v", tx.Description, tt.description)
			}
		})
	}
}

func TestProcessRefund(t *testing.T) {
	svc, balanceRepo, creditRepo, _ := newTestBalanceService()
	ctx := context.Background()
	userID := "user_123"
	stripePaymentID := "pi_original"

	// Set up initial balance
	balanceRepo.balances[userID] = &models.UserBalance{UserID: userID, BalanceUSD: 100.0}

	// Set up original transaction
	original := &models.CreditTransaction{
		ID:              "tx_original",
		UserID:          userID,
		Type:            models.TxTypeSubscription,
		AmountUSD:       100.0,
		StripePaymentID: &stripePaymentID,
	}
	creditRepo.transactions = append(creditRepo.transactions, original)
	creditRepo.byStripePayment[stripePaymentID] = original

	tests := []struct {
		name          string
		refundAmount  float64
		wantCredited  float64
		wantErr       bool
	}{
		{"partial refund", 50.0, 50.0, false},
		{"full refund", 100.0, 100.0, false},
		{"over refund capped", 150.0, 100.0, false}, // Should cap at original amount
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset duplicate keys for each test (except original)
			creditRepo.duplicateKeys = map[string]bool{stripePaymentID: true}

			err := svc.ProcessRefund(ctx, userID, stripePaymentID, tt.refundAmount)
			if (err != nil) != tt.wantErr {
				t.Errorf("ProcessRefund() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				// Find the refund transaction
				var refundTx *models.CreditTransaction
				for _, tx := range creditRepo.transactions {
					if tx.Type == models.TxTypeRefund {
						refundTx = tx
						break
					}
				}

				if refundTx == nil {
					t.Fatal("refund transaction not created")
				}

				if refundTx.AmountUSD != tt.wantCredited {
					t.Errorf("refund amount = %v, want %v", refundTx.AmountUSD, tt.wantCredited)
				}
			}

			// Clean up refund transaction for next test
			var filtered []*models.CreditTransaction
			for _, tx := range creditRepo.transactions {
				if tx.Type != models.TxTypeRefund {
					filtered = append(filtered, tx)
				}
			}
			creditRepo.transactions = filtered
		})
	}
}

func TestProcessRefundUnknownPayment(t *testing.T) {
	svc, _, _, _ := newTestBalanceService()
	ctx := context.Background()
	userID := "user_123"

	err := svc.ProcessRefund(ctx, userID, "unknown_payment", 50.0)
	if err == nil {
		t.Error("ProcessRefund() should fail for unknown payment")
	}
}

func TestExpireOldCredits(t *testing.T) {
	svc, _, creditRepo, _ := newTestBalanceService()
	ctx := context.Background()

	now := time.Now()
	past := now.Add(-24 * time.Hour)
	future := now.Add(24 * time.Hour)

	// Set up transactions with various expiry dates
	creditRepo.transactions = []*models.CreditTransaction{
		{ID: "tx_1", ExpiresAt: &past, IsExpired: false},
		{ID: "tx_2", ExpiresAt: &past, IsExpired: false},
		{ID: "tx_3", ExpiresAt: &future, IsExpired: false},
		{ID: "tx_4", ExpiresAt: nil, IsExpired: false}, // Never expires
	}

	count, err := svc.ExpireOldCredits(ctx)
	if err != nil {
		t.Fatalf("ExpireOldCredits() error = %v", err)
	}

	if count != 2 {
		t.Errorf("expired count = %d, want 2", count)
	}

	// Verify the correct transactions are marked expired
	for _, tx := range creditRepo.transactions {
		switch tx.ID {
		case "tx_1", "tx_2":
			if !tx.IsExpired {
				t.Errorf("transaction %s should be expired", tx.ID)
			}
		case "tx_3", "tx_4":
			if tx.IsExpired {
				t.Errorf("transaction %s should not be expired", tx.ID)
			}
		}
	}
}

func TestUpdateSubscriptionPeriod(t *testing.T) {
	svc, balanceRepo, _, _ := newTestBalanceService()
	ctx := context.Background()
	userID := "user_123"

	periodStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)

	err := svc.UpdateSubscriptionPeriod(ctx, userID, periodStart, periodEnd)
	if err != nil {
		t.Fatalf("UpdateSubscriptionPeriod() error = %v", err)
	}

	balance := balanceRepo.balances[userID]
	if balance == nil {
		t.Fatal("balance not created")
	}

	if balance.PeriodStart == nil || !balance.PeriodStart.Equal(periodStart) {
		t.Errorf("period start = %v, want %v", balance.PeriodStart, periodStart)
	}

	if balance.PeriodEnd == nil || !balance.PeriodEnd.Equal(periodEnd) {
		t.Errorf("period end = %v, want %v", balance.PeriodEnd, periodEnd)
	}
}

func TestGetSubscriptionPeriod(t *testing.T) {
	svc, balanceRepo, _, _ := newTestBalanceService()
	ctx := context.Background()
	userID := "user_123"

	// No balance exists
	start, end, err := svc.GetSubscriptionPeriod(ctx, userID)
	if err != nil {
		t.Fatalf("GetSubscriptionPeriod() error = %v", err)
	}
	if start != nil || end != nil {
		t.Error("expected nil times for non-existent balance")
	}

	// Set up balance with period
	periodStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
	balanceRepo.balances[userID] = &models.UserBalance{
		UserID:      userID,
		PeriodStart: &periodStart,
		PeriodEnd:   &periodEnd,
	}

	start, end, err = svc.GetSubscriptionPeriod(ctx, userID)
	if err != nil {
		t.Fatalf("GetSubscriptionPeriod() error = %v", err)
	}

	if start == nil || !start.Equal(periodStart) {
		t.Errorf("start = %v, want %v", start, periodStart)
	}
	if end == nil || !end.Equal(periodEnd) {
		t.Errorf("end = %v, want %v", end, periodEnd)
	}
}

func TestBalanceServiceConcurrent(t *testing.T) {
	svc, balanceRepo, creditRepo, _ := newTestBalanceService()
	ctx := context.Background()
	userID := "user_123"

	// Set initial balance
	balanceRepo.balances[userID] = &models.UserBalance{
		UserID:     userID,
		BalanceUSD: 1000.0,
	}
	creditRepo.availableBalance = 1000.0

	// Run multiple concurrent deductions
	done := make(chan bool)
	errChan := make(chan error, 100)

	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 10; j++ {
				jobID := "job_test"
				err := svc.DeductUsageCredits(ctx, userID, 1.0, &jobID)
				if err != nil && !errors.Is(err, ErrInsufficientBalance) {
					errChan <- err
				}
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	select {
	case err := <-errChan:
		t.Fatalf("Concurrent operation failed: %v", err)
	default:
		// Success
	}
}

func TestIsDuplicateKeyError(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		want    bool
	}{
		{"nil error", nil, false},
		{"sqlite unique", errors.New("UNIQUE constraint failed: credit_transactions.stripe_payment_id"), true},
		{"postgres duplicate", errors.New("duplicate key value violates unique constraint"), true},
		{"mysql exists", errors.New("Error 1062: Duplicate entry 'x' for key 'PRIMARY' - already exists"), true},
		{"other error", errors.New("connection refused"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isDuplicateKeyError(tt.err)
			if got != tt.want {
				t.Errorf("isDuplicateKeyError() = %v, want %v", got, tt.want)
			}
		})
	}
}
