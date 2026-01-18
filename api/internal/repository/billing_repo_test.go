package repository

import (
	"context"
	"testing"
	"time"

	"github.com/jmylchreest/refyne-api/internal/models"
)

// ========================================
// Balance Repository Tests
// ========================================

func TestBalanceRepository_GetNonExistent(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	balance, err := repos.Balance.Get(ctx, "non-existent-user")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if balance != nil {
		t.Error("expected nil balance for non-existent user")
	}
}

func TestBalanceRepository_Upsert(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	// Insert new balance
	balance := &models.UserBalance{
		UserID:        "user-1",
		BalanceUSD:    100.50,
		LifetimeAdded: 200.00,
		LifetimeSpent: 99.50,
		UpdatedAt:     time.Now().UTC(),
	}

	if err := repos.Balance.Upsert(ctx, balance); err != nil {
		t.Fatalf("failed to insert balance: %v", err)
	}

	// Retrieve and verify
	got, err := repos.Balance.Get(ctx, "user-1")
	if err != nil {
		t.Fatalf("failed to get balance: %v", err)
	}
	if got == nil {
		t.Fatal("expected balance to be found")
	}
	if got.BalanceUSD != 100.50 {
		t.Errorf("balance = %v, want 100.50", got.BalanceUSD)
	}
	if got.LifetimeAdded != 200.00 {
		t.Errorf("lifetime added = %v, want 200.00", got.LifetimeAdded)
	}
	if got.LifetimeSpent != 99.50 {
		t.Errorf("lifetime spent = %v, want 99.50", got.LifetimeSpent)
	}

	// Update existing balance
	balance.BalanceUSD = 50.00
	balance.LifetimeSpent = 150.00
	if err := repos.Balance.Upsert(ctx, balance); err != nil {
		t.Fatalf("failed to update balance: %v", err)
	}

	got, err = repos.Balance.Get(ctx, "user-1")
	if err != nil {
		t.Fatalf("failed to get updated balance: %v", err)
	}
	if got.BalanceUSD != 50.00 {
		t.Errorf("updated balance = %v, want 50.00", got.BalanceUSD)
	}
}

func TestBalanceRepository_UpdateSubscriptionPeriod(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	periodStart := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	// Update for non-existent user should create a new record
	if err := repos.Balance.UpdateSubscriptionPeriod(ctx, "user-1", periodStart, periodEnd); err != nil {
		t.Fatalf("failed to update subscription period: %v", err)
	}

	got, err := repos.Balance.Get(ctx, "user-1")
	if err != nil {
		t.Fatalf("failed to get balance: %v", err)
	}
	if got == nil {
		t.Fatal("expected balance to be created")
	}
	if got.PeriodStart == nil || !got.PeriodStart.Equal(periodStart) {
		t.Errorf("period start = %v, want %v", got.PeriodStart, periodStart)
	}
	if got.PeriodEnd == nil || !got.PeriodEnd.Equal(periodEnd) {
		t.Errorf("period end = %v, want %v", got.PeriodEnd, periodEnd)
	}

	// Update again with new period
	newPeriodStart := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	newPeriodEnd := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	if err := repos.Balance.UpdateSubscriptionPeriod(ctx, "user-1", newPeriodStart, newPeriodEnd); err != nil {
		t.Fatalf("failed to update subscription period: %v", err)
	}

	got, err = repos.Balance.Get(ctx, "user-1")
	if err != nil {
		t.Fatalf("failed to get balance: %v", err)
	}
	if got.PeriodStart == nil || !got.PeriodStart.Equal(newPeriodStart) {
		t.Errorf("updated period start = %v, want %v", got.PeriodStart, newPeriodStart)
	}
}

func TestBalanceRepository_GetAvailableBalance(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// Create some non-expired credit transactions
	tx1 := &models.CreditTransaction{
		ID:          "tx-1",
		UserID:      "user-1",
		Type:        models.TxTypeTopUp,
		AmountUSD:   50.00,
		BalanceAfter: 50.00,
		IsExpired:   false,
		Description: "Top-up",
		CreatedAt:   now,
	}
	if err := repos.CreditTransaction.Create(ctx, tx1); err != nil {
		t.Fatalf("failed to create tx1: %v", err)
	}

	// Create another non-expired transaction
	tx2 := &models.CreditTransaction{
		ID:          "tx-2",
		UserID:      "user-1",
		Type:        models.TxTypeSubscription,
		AmountUSD:   25.00,
		BalanceAfter: 75.00,
		IsExpired:   false,
		Description: "Subscription",
		CreatedAt:   now,
	}
	if err := repos.CreditTransaction.Create(ctx, tx2); err != nil {
		t.Fatalf("failed to create tx2: %v", err)
	}

	// Create an expired transaction (should not be counted)
	tx3 := &models.CreditTransaction{
		ID:          "tx-3",
		UserID:      "user-1",
		Type:        models.TxTypeTopUp,
		AmountUSD:   100.00,
		BalanceAfter: 175.00,
		IsExpired:   true,
		Description: "Expired top-up",
		CreatedAt:   now,
	}
	if err := repos.CreditTransaction.Create(ctx, tx3); err != nil {
		t.Fatalf("failed to create tx3: %v", err)
	}

	available, err := repos.Balance.GetAvailableBalance(ctx, "user-1", now)
	if err != nil {
		t.Fatalf("failed to get available balance: %v", err)
	}
	// Should be tx1 + tx2 = 75.00 (not tx3 which is expired)
	if available != 75.00 {
		t.Errorf("available balance = %v, want 75.00", available)
	}
}

// ========================================
// Credit Transaction Repository Tests
// ========================================

func TestCreditTransactionRepository_Create(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()
	now := time.Now().UTC()

	stripeID := "pi_test123"
	expiresAt := now.AddDate(0, 1, 0)

	tx := &models.CreditTransaction{
		ID:              "tx-1",
		UserID:          "user-1",
		Type:            models.TxTypeTopUp,
		AmountUSD:       50.00,
		BalanceAfter:    150.00,
		ExpiresAt:       &expiresAt,
		IsExpired:       false,
		StripePaymentID: &stripeID,
		Description:     "Test top-up",
		CreatedAt:       now,
	}

	if err := repos.CreditTransaction.Create(ctx, tx); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}
}

func TestCreditTransactionRepository_GetByUserID(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// Create multiple transactions for user
	for i := 0; i < 5; i++ {
		tx := &models.CreditTransaction{
			ID:           "tx-" + string(rune('a'+i)),
			UserID:       "user-1",
			Type:         models.TxTypeTopUp,
			AmountUSD:    float64(i * 10),
			BalanceAfter: float64(i * 10),
			IsExpired:    false,
			Description:  "Test",
			CreatedAt:    now.Add(time.Duration(i) * time.Hour),
		}
		if err := repos.CreditTransaction.Create(ctx, tx); err != nil {
			t.Fatalf("failed to create transaction %d: %v", i, err)
		}
	}

	// Test pagination
	txs, err := repos.CreditTransaction.GetByUserID(ctx, "user-1", 3, 0)
	if err != nil {
		t.Fatalf("failed to get transactions: %v", err)
	}
	if len(txs) != 3 {
		t.Errorf("got %d transactions, want 3", len(txs))
	}

	// Test offset
	txs, err = repos.CreditTransaction.GetByUserID(ctx, "user-1", 10, 2)
	if err != nil {
		t.Fatalf("failed to get transactions with offset: %v", err)
	}
	if len(txs) != 3 {
		t.Errorf("got %d transactions with offset, want 3", len(txs))
	}
}

func TestCreditTransactionRepository_GetByStripePaymentID(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()
	now := time.Now().UTC()

	stripeID := "pi_unique123"
	tx := &models.CreditTransaction{
		ID:              "tx-1",
		UserID:          "user-1",
		Type:            models.TxTypeTopUp,
		AmountUSD:       100.00,
		BalanceAfter:    100.00,
		IsExpired:       false,
		StripePaymentID: &stripeID,
		Description:     "Stripe payment",
		CreatedAt:       now,
	}
	if err := repos.CreditTransaction.Create(ctx, tx); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	// Find by stripe ID
	found, err := repos.CreditTransaction.GetByStripePaymentID(ctx, stripeID)
	if err != nil {
		t.Fatalf("failed to get by stripe ID: %v", err)
	}
	if found == nil {
		t.Fatal("expected to find transaction")
	}
	if found.ID != "tx-1" {
		t.Errorf("found wrong transaction: %s", found.ID)
	}

	// Non-existent stripe ID
	notFound, err := repos.CreditTransaction.GetByStripePaymentID(ctx, "pi_nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if notFound != nil {
		t.Error("expected nil for non-existent stripe ID")
	}
}

func TestCreditTransactionRepository_GetNonExpiredSubscriptionCredits(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// Create subscription credit (non-expired)
	tx1 := &models.CreditTransaction{
		ID:           "tx-1",
		UserID:       "user-1",
		Type:         models.TxTypeSubscription,
		AmountUSD:    50.00,
		BalanceAfter: 50.00,
		IsExpired:    false,
		Description:  "Subscription credit",
		CreatedAt:    now,
	}
	if err := repos.CreditTransaction.Create(ctx, tx1); err != nil {
		t.Fatalf("failed to create tx1: %v", err)
	}

	// Create expired subscription credit (should not be returned)
	tx2 := &models.CreditTransaction{
		ID:           "tx-2",
		UserID:       "user-1",
		Type:         models.TxTypeSubscription,
		AmountUSD:    100.00,
		BalanceAfter: 150.00,
		IsExpired:    true,
		Description:  "Expired subscription",
		CreatedAt:    now,
	}
	if err := repos.CreditTransaction.Create(ctx, tx2); err != nil {
		t.Fatalf("failed to create tx2: %v", err)
	}

	// Create non-subscription credit (should not be returned)
	tx3 := &models.CreditTransaction{
		ID:           "tx-3",
		UserID:       "user-1",
		Type:         models.TxTypeTopUp,
		AmountUSD:    25.00,
		BalanceAfter: 175.00,
		IsExpired:    false,
		Description:  "Top-up credit",
		CreatedAt:    now,
	}
	if err := repos.CreditTransaction.Create(ctx, tx3); err != nil {
		t.Fatalf("failed to create tx3: %v", err)
	}

	credits, err := repos.CreditTransaction.GetNonExpiredSubscriptionCredits(ctx)
	if err != nil {
		t.Fatalf("failed to get non-expired subscription credits: %v", err)
	}
	if len(credits) != 1 {
		t.Errorf("got %d credits, want 1", len(credits))
	}
	if len(credits) > 0 && credits[0].ID != "tx-1" {
		t.Errorf("got wrong transaction: %s", credits[0].ID)
	}
}

func TestCreditTransactionRepository_UpdateExpiry(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()
	now := time.Now().UTC()

	tx := &models.CreditTransaction{
		ID:           "tx-1",
		UserID:       "user-1",
		Type:         models.TxTypeSubscription,
		AmountUSD:    50.00,
		BalanceAfter: 50.00,
		IsExpired:    false,
		Description:  "Test",
		CreatedAt:    now,
	}
	if err := repos.CreditTransaction.Create(ctx, tx); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	// Update expiry
	newExpiry := now.AddDate(0, 2, 0)
	if err := repos.CreditTransaction.UpdateExpiry(ctx, "tx-1", &newExpiry); err != nil {
		t.Fatalf("failed to update expiry: %v", err)
	}

	// Verify by getting from DB (via stripe payment lookup since we don't have GetByID)
	txs, _ := repos.CreditTransaction.GetByUserID(ctx, "user-1", 10, 0)
	if len(txs) == 0 {
		t.Fatal("expected to find transaction")
	}
	if txs[0].ExpiresAt == nil {
		t.Error("expected expires_at to be set")
	}
}

func TestCreditTransactionRepository_MarkExpired(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()
	now := time.Now().UTC()
	past := now.Add(-24 * time.Hour)

	// Create transaction with past expiry
	expiresAt := past
	tx := &models.CreditTransaction{
		ID:           "tx-1",
		UserID:       "user-1",
		Type:         models.TxTypeSubscription,
		AmountUSD:    50.00,
		BalanceAfter: 50.00,
		ExpiresAt:    &expiresAt,
		IsExpired:    false,
		Description:  "Should expire",
		CreatedAt:    past.Add(-48 * time.Hour),
	}
	if err := repos.CreditTransaction.Create(ctx, tx); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	// Create transaction with future expiry
	futureExpiry := now.AddDate(0, 1, 0)
	tx2 := &models.CreditTransaction{
		ID:           "tx-2",
		UserID:       "user-1",
		Type:         models.TxTypeSubscription,
		AmountUSD:    100.00,
		BalanceAfter: 150.00,
		ExpiresAt:    &futureExpiry,
		IsExpired:    false,
		Description:  "Should not expire",
		CreatedAt:    now,
	}
	if err := repos.CreditTransaction.Create(ctx, tx2); err != nil {
		t.Fatalf("failed to create tx2: %v", err)
	}

	// Mark expired
	count, err := repos.CreditTransaction.MarkExpired(ctx, now)
	if err != nil {
		t.Fatalf("failed to mark expired: %v", err)
	}
	if count != 1 {
		t.Errorf("marked %d as expired, want 1", count)
	}

	// Verify available balance only includes non-expired
	available, _ := repos.Balance.GetAvailableBalance(ctx, "user-1", now)
	if available != 100.00 {
		t.Errorf("available balance = %v, want 100.00", available)
	}
}

func TestCreditTransactionRepository_GetAvailableBalance(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// Create non-expired credits
	tx1 := &models.CreditTransaction{
		ID:           "tx-1",
		UserID:       "user-1",
		Type:         models.TxTypeTopUp,
		AmountUSD:    100.00,
		BalanceAfter: 100.00,
		IsExpired:    false,
		Description:  "Credit",
		CreatedAt:    now,
	}
	repos.CreditTransaction.Create(ctx, tx1)

	tx2 := &models.CreditTransaction{
		ID:           "tx-2",
		UserID:       "user-1",
		Type:         models.TxTypeUsage,
		AmountUSD:    -30.00,
		BalanceAfter: 70.00,
		IsExpired:    false,
		Description:  "Usage deduction",
		CreatedAt:    now,
	}
	repos.CreditTransaction.Create(ctx, tx2)

	available, err := repos.CreditTransaction.GetAvailableBalance(ctx, "user-1", now)
	if err != nil {
		t.Fatalf("failed to get available balance: %v", err)
	}
	// 100 - 30 = 70
	if available != 70.00 {
		t.Errorf("available balance = %v, want 70.00", available)
	}
}

// ========================================
// Schema Snapshot Repository Tests
// ========================================

func TestSchemaSnapshotRepository_Create(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()
	now := time.Now().UTC()

	snapshot := &models.SchemaSnapshot{
		ID:         "snap-1",
		UserID:     "user-1",
		Hash:       "abc123hash",
		SchemaJSON: `{"type":"object"}`,
		Name:       "Test Schema",
		Version:    1,
		UsageCount: 0,
		CreatedAt:  now,
	}

	if err := repos.SchemaSnapshot.Create(ctx, snapshot); err != nil {
		t.Fatalf("failed to create snapshot: %v", err)
	}

	// Verify by retrieving
	got, err := repos.SchemaSnapshot.GetByID(ctx, "snap-1")
	if err != nil {
		t.Fatalf("failed to get snapshot: %v", err)
	}
	if got == nil {
		t.Fatal("expected to find snapshot")
	}
	if got.SchemaJSON != `{"type":"object"}` {
		t.Errorf("wrong schema JSON: %s", got.SchemaJSON)
	}
}

func TestSchemaSnapshotRepository_GetByID_NotFound(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	snapshot, err := repos.SchemaSnapshot.GetByID(ctx, "non-existent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snapshot != nil {
		t.Error("expected nil for non-existent snapshot")
	}
}

func TestSchemaSnapshotRepository_GetByUserAndHash(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()
	now := time.Now().UTC()

	snapshot := &models.SchemaSnapshot{
		ID:         "snap-1",
		UserID:     "user-1",
		Hash:       "uniquehash123",
		SchemaJSON: `{"test":true}`,
		Version:    1,
		UsageCount: 0,
		CreatedAt:  now,
	}
	repos.SchemaSnapshot.Create(ctx, snapshot)

	// Find by user and hash
	found, err := repos.SchemaSnapshot.GetByUserAndHash(ctx, "user-1", "uniquehash123")
	if err != nil {
		t.Fatalf("failed to find snapshot: %v", err)
	}
	if found == nil {
		t.Fatal("expected to find snapshot")
	}
	if found.ID != "snap-1" {
		t.Errorf("found wrong snapshot: %s", found.ID)
	}

	// Different user, same hash
	notFound, err := repos.SchemaSnapshot.GetByUserAndHash(ctx, "user-2", "uniquehash123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if notFound != nil {
		t.Error("expected nil for different user")
	}
}

func TestSchemaSnapshotRepository_GetByUserID(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// Create multiple snapshots
	for i := 0; i < 5; i++ {
		snapshot := &models.SchemaSnapshot{
			ID:         "snap-" + string(rune('a'+i)),
			UserID:     "user-1",
			Hash:       "hash-" + string(rune('a'+i)),
			SchemaJSON: `{}`,
			Version:    i + 1,
			UsageCount: 0,
			CreatedAt:  now.Add(time.Duration(i) * time.Hour),
		}
		repos.SchemaSnapshot.Create(ctx, snapshot)
	}

	snapshots, err := repos.SchemaSnapshot.GetByUserID(ctx, "user-1", 3, 0)
	if err != nil {
		t.Fatalf("failed to get snapshots: %v", err)
	}
	if len(snapshots) != 3 {
		t.Errorf("got %d snapshots, want 3", len(snapshots))
	}
}

func TestSchemaSnapshotRepository_GetNextVersion(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// No snapshots yet
	version, err := repos.SchemaSnapshot.GetNextVersion(ctx, "user-1")
	if err != nil {
		t.Fatalf("failed to get next version: %v", err)
	}
	if version != 1 {
		t.Errorf("next version = %d, want 1", version)
	}

	// Create snapshot
	repos.SchemaSnapshot.Create(ctx, &models.SchemaSnapshot{
		ID:         "snap-1",
		UserID:     "user-1",
		Hash:       "hash1",
		SchemaJSON: `{}`,
		Version:    1,
		UsageCount: 0,
		CreatedAt:  now,
	})

	version, err = repos.SchemaSnapshot.GetNextVersion(ctx, "user-1")
	if err != nil {
		t.Fatalf("failed to get next version: %v", err)
	}
	if version != 2 {
		t.Errorf("next version = %d, want 2", version)
	}
}

func TestSchemaSnapshotRepository_IncrementUsageCount(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()
	now := time.Now().UTC()

	repos.SchemaSnapshot.Create(ctx, &models.SchemaSnapshot{
		ID:         "snap-1",
		UserID:     "user-1",
		Hash:       "hash1",
		SchemaJSON: `{}`,
		Version:    1,
		UsageCount: 0,
		CreatedAt:  now,
	})

	// Increment twice
	repos.SchemaSnapshot.IncrementUsageCount(ctx, "snap-1")
	repos.SchemaSnapshot.IncrementUsageCount(ctx, "snap-1")

	snapshot, _ := repos.SchemaSnapshot.GetByID(ctx, "snap-1")
	if snapshot.UsageCount != 2 {
		t.Errorf("usage count = %d, want 2", snapshot.UsageCount)
	}
}

// ========================================
// Usage Repository Tests
// ========================================

func TestUsageRepository_Create(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()
	now := time.Now().UTC()

	record := &models.UsageRecord{
		ID:              "usage-1",
		UserID:          "user-1",
		Date:            "2026-01-18",
		Type:            "extract",
		Status:          "success",
		TotalChargedUSD: 0.05,
		IsBYOK:          false,
		CreatedAt:       now,
	}

	if err := repos.Usage.Create(ctx, record); err != nil {
		t.Fatalf("failed to create usage record: %v", err)
	}
}

func TestUsageRepository_GetByUserID(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// Create records across different dates
	dates := []string{"2026-01-15", "2026-01-16", "2026-01-17", "2026-01-18"}
	for i, date := range dates {
		repos.Usage.Create(ctx, &models.UsageRecord{
			ID:              "usage-" + string(rune('a'+i)),
			UserID:          "user-1",
			Date:            date,
			Type:            "extract",
			Status:          "success",
			TotalChargedUSD: float64(i) * 0.01,
			IsBYOK:          i%2 == 0,
			CreatedAt:       now,
		})
	}

	// Get records in date range
	records, err := repos.Usage.GetByUserID(ctx, "user-1", "2026-01-16", "2026-01-17")
	if err != nil {
		t.Fatalf("failed to get records: %v", err)
	}
	if len(records) != 2 {
		t.Errorf("got %d records, want 2", len(records))
	}
}

func TestUsageRepository_GetSummary(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()
	now := time.Now().UTC()
	today := now.Format("2006-01-02")

	// Create records for today
	repos.Usage.Create(ctx, &models.UsageRecord{
		ID:              "usage-1",
		UserID:          "user-1",
		Date:            today,
		Type:            "extract",
		Status:          "success",
		TotalChargedUSD: 1.00,
		IsBYOK:          false,
		CreatedAt:       now,
	})
	repos.Usage.Create(ctx, &models.UsageRecord{
		ID:              "usage-2",
		UserID:          "user-1",
		Date:            today,
		Type:            "extract",
		Status:          "success",
		TotalChargedUSD: 0.50,
		IsBYOK:          true,
		CreatedAt:       now,
	})

	summary, err := repos.Usage.GetSummary(ctx, "user-1", "day")
	if err != nil {
		t.Fatalf("failed to get summary: %v", err)
	}
	if summary.TotalJobs != 2 {
		t.Errorf("total jobs = %d, want 2", summary.TotalJobs)
	}
	if summary.TotalChargedUSD != 1.50 {
		t.Errorf("total charged = %v, want 1.50", summary.TotalChargedUSD)
	}
	if summary.BYOKJobs != 1 {
		t.Errorf("byok jobs = %d, want 1", summary.BYOKJobs)
	}
}

func TestUsageRepository_GetSummaryByDateRange(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// Create records across dates
	repos.Usage.Create(ctx, &models.UsageRecord{
		ID:              "usage-1",
		UserID:          "user-1",
		Date:            "2026-01-10",
		Type:            "extract",
		Status:          "success",
		TotalChargedUSD: 1.00,
		IsBYOK:          false,
		CreatedAt:       now,
	})
	repos.Usage.Create(ctx, &models.UsageRecord{
		ID:              "usage-2",
		UserID:          "user-1",
		Date:            "2026-01-15",
		Type:            "extract",
		Status:          "success",
		TotalChargedUSD: 2.00,
		IsBYOK:          false,
		CreatedAt:       now,
	})
	repos.Usage.Create(ctx, &models.UsageRecord{
		ID:              "usage-3",
		UserID:          "user-1",
		Date:            "2026-01-20",
		Type:            "extract",
		Status:          "success",
		TotalChargedUSD: 3.00,
		IsBYOK:          false,
		CreatedAt:       now,
	})

	startDate := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(2026, 1, 16, 0, 0, 0, 0, time.UTC)

	summary, err := repos.Usage.GetSummaryByDateRange(ctx, "user-1", startDate, endDate)
	if err != nil {
		t.Fatalf("failed to get summary: %v", err)
	}
	if summary.TotalJobs != 2 {
		t.Errorf("total jobs = %d, want 2", summary.TotalJobs)
	}
	if summary.TotalChargedUSD != 3.00 {
		t.Errorf("total charged = %v, want 3.00", summary.TotalChargedUSD)
	}
}

func TestUsageRepository_GetMonthlySpend(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// Create records for January 2026
	repos.Usage.Create(ctx, &models.UsageRecord{
		ID:              "usage-1",
		UserID:          "user-1",
		Date:            "2026-01-05",
		Type:            "extract",
		Status:          "success",
		TotalChargedUSD: 5.00,
		IsBYOK:          false,
		CreatedAt:       now,
	})
	repos.Usage.Create(ctx, &models.UsageRecord{
		ID:              "usage-2",
		UserID:          "user-1",
		Date:            "2026-01-15",
		Type:            "extract",
		Status:          "success",
		TotalChargedUSD: 10.00,
		IsBYOK:          false,
		CreatedAt:       now,
	})
	// February record (should not be counted)
	repos.Usage.Create(ctx, &models.UsageRecord{
		ID:              "usage-3",
		UserID:          "user-1",
		Date:            "2026-02-01",
		Type:            "extract",
		Status:          "success",
		TotalChargedUSD: 20.00,
		IsBYOK:          false,
		CreatedAt:       now,
	})

	month := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	spend, err := repos.Usage.GetMonthlySpend(ctx, "user-1", month)
	if err != nil {
		t.Fatalf("failed to get monthly spend: %v", err)
	}
	if spend != 15.00 {
		t.Errorf("monthly spend = %v, want 15.00", spend)
	}
}

func TestUsageRepository_CountByUserAndDateRange(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()
	now := time.Now().UTC()

	dates := []string{"2026-01-10", "2026-01-11", "2026-01-12", "2026-01-13"}
	for i, date := range dates {
		repos.Usage.Create(ctx, &models.UsageRecord{
			ID:              "usage-" + string(rune('a'+i)),
			UserID:          "user-1",
			Date:            date,
			Type:            "extract",
			Status:          "success",
			TotalChargedUSD: 0.10,
			IsBYOK:          false,
			CreatedAt:       now,
		})
	}

	count, err := repos.Usage.CountByUserAndDateRange(ctx, "user-1", "2026-01-10", "2026-01-13")
	if err != nil {
		t.Fatalf("failed to count records: %v", err)
	}
	if count != 3 { // 10, 11, 12 (13 is excluded by < comparison)
		t.Errorf("count = %d, want 3", count)
	}
}

// ========================================
// Usage Insight Repository Tests
// ========================================

func TestUsageInsightRepository_Create(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// First create a usage record (foreign key requirement)
	repos.Usage.Create(ctx, &models.UsageRecord{
		ID:              "usage-1",
		UserID:          "user-1",
		Date:            "2026-01-18",
		Type:            "extract",
		Status:          "success",
		TotalChargedUSD: 0.10,
		IsBYOK:          false,
		CreatedAt:       now,
	})

	insight := &models.UsageInsight{
		ID:                "insight-1",
		UsageID:           "usage-1",
		TargetURL:         "https://example.com",
		TokensInput:       1000,
		TokensOutput:      500,
		LLMCostUSD:        0.08,
		MarkupRate:        0.25,
		MarkupUSD:         0.02,
		LLMProvider:       "openrouter",
		LLMModel:          "anthropic/claude-3-5-sonnet",
		PagesAttempted:    1,
		PagesSuccessful:   1,
		FetchDurationMs:   500,
		ExtractDurationMs: 1500,
		TotalDurationMs:   2000,
		CreatedAt:         now,
	}

	if err := repos.UsageInsight.Create(ctx, insight); err != nil {
		t.Fatalf("failed to create insight: %v", err)
	}
}

func TestUsageInsightRepository_GetByUsageID(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// Create usage record and insight
	repos.Usage.Create(ctx, &models.UsageRecord{
		ID:              "usage-1",
		UserID:          "user-1",
		Date:            "2026-01-18",
		Type:            "extract",
		Status:          "success",
		TotalChargedUSD: 0.10,
		IsBYOK:          false,
		CreatedAt:       now,
	})

	repos.UsageInsight.Create(ctx, &models.UsageInsight{
		ID:              "insight-1",
		UsageID:         "usage-1",
		TargetURL:       "https://example.com",
		TokensInput:     1000,
		TokensOutput:    500,
		LLMCostUSD:      0.08,
		MarkupRate:      0.02,
		MarkupUSD:       0.0016,
		LLMProvider:     "openrouter",
		LLMModel:        "anthropic/claude-3-5-sonnet",
		PagesAttempted:  1,
		PagesSuccessful: 1,
		CreatedAt:       now,
	})

	insight, err := repos.UsageInsight.GetByUsageID(ctx, "usage-1")
	if err != nil {
		t.Fatalf("failed to get insight: %v", err)
	}
	if insight == nil {
		t.Fatal("expected to find insight")
	}
	if insight.LLMProvider != "openrouter" {
		t.Errorf("provider = %s, want openrouter", insight.LLMProvider)
	}
	if insight.TokensInput != 1000 {
		t.Errorf("tokens input = %d, want 1000", insight.TokensInput)
	}

	// Non-existent
	notFound, err := repos.UsageInsight.GetByUsageID(ctx, "non-existent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if notFound != nil {
		t.Error("expected nil for non-existent")
	}
}

func TestUsageInsightRepository_GetByUserID(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// Create multiple usage records and insights
	for i := 0; i < 5; i++ {
		usageID := "usage-" + string(rune('a'+i))
		repos.Usage.Create(ctx, &models.UsageRecord{
			ID:              usageID,
			UserID:          "user-1",
			Date:            "2026-01-18",
			Type:            "extract",
			Status:          "success",
			TotalChargedUSD: float64(i) * 0.01,
			IsBYOK:          false,
			CreatedAt:       now.Add(time.Duration(i) * time.Hour),
		})

		repos.UsageInsight.Create(ctx, &models.UsageInsight{
			ID:              "insight-" + string(rune('a'+i)),
			UsageID:         usageID,
			TargetURL:       "https://example.com/" + string(rune('a'+i)),
			TokensInput:     i * 100,
			TokensOutput:    i * 50,
			LLMCostUSD:      float64(i) * 0.01,
			MarkupRate:      0.02,
			MarkupUSD:       float64(i) * 0.0002,
			LLMProvider:     "openrouter",
			LLMModel:        "anthropic/claude-3-5-sonnet",
			PagesAttempted:  1,
			PagesSuccessful: 1,
			CreatedAt:       now.Add(time.Duration(i) * time.Hour),
		})
	}

	// Test pagination
	insights, err := repos.UsageInsight.GetByUserID(ctx, "user-1", 3, 0)
	if err != nil {
		t.Fatalf("failed to get insights: %v", err)
	}
	if len(insights) != 3 {
		t.Errorf("got %d insights, want 3", len(insights))
	}

	// Test offset
	insights, err = repos.UsageInsight.GetByUserID(ctx, "user-1", 10, 2)
	if err != nil {
		t.Fatalf("failed to get insights with offset: %v", err)
	}
	if len(insights) != 3 {
		t.Errorf("got %d insights with offset, want 3", len(insights))
	}
}
