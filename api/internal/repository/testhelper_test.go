package repository

import (
	"database/sql"
	"testing"

	"github.com/jmylchreest/refyne-api/internal/database/migrations"
	_ "github.com/tursodatabase/go-libsql"
)

// setupTestDB creates an in-memory SQLite database for testing.
// It runs migrations and returns a database connection that will be cleaned up
// when the test completes.
func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()

	// Create in-memory database
	db, err := sql.Open("libsql", ":memory:")
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("failed to enable foreign keys: %v", err)
	}

	// Run migrations
	if err := migrations.Run(db, nil); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	// Clean up when test completes
	t.Cleanup(func() {
		_ = db.Close()
	})

	return db
}

// setupTestRepos creates all repositories using a test database.
func setupTestRepos(t *testing.T) *Repositories {
	t.Helper()
	db := setupTestDB(t)
	return NewRepositories(db)
}

// InsertTestJob is a helper to insert a test job directly.
func InsertTestJob(t *testing.T, db *sql.DB, id, userID, status string) {
	t.Helper()
	query := `
		INSERT INTO jobs (id, user_id, type, status, url, schema_json, created_at, updated_at)
		VALUES (?, ?, 'extract', ?, 'https://example.com', '{}', datetime('now'), datetime('now'))
	`
	if _, err := db.Exec(query, id, userID, status); err != nil {
		t.Fatalf("failed to insert test job: %v", err)
	}
}

// InsertTestAPIKey is a helper to insert a test API key directly.
func InsertTestAPIKey(t *testing.T, db *sql.DB, id, userID, keyHash, keyPrefix string) {
	t.Helper()
	query := `
		INSERT INTO api_keys (id, user_id, name, key_hash, key_prefix, scopes, created_at)
		VALUES (?, ?, 'Test Key', ?, ?, '["*"]', datetime('now'))
	`
	if _, err := db.Exec(query, id, userID, keyHash, keyPrefix); err != nil {
		t.Fatalf("failed to insert test API key: %v", err)
	}
}

// InsertTestUsageRecord is a helper to insert a test usage record.
func InsertTestUsageRecord(t *testing.T, db *sql.DB, id, userID, jobID, date string, chargedUSD float64) {
	t.Helper()
	query := `
		INSERT INTO usage_records (id, user_id, job_id, date, type, status, total_charged_usd, is_byok, created_at)
		VALUES (?, ?, ?, ?, 'extract', 'success', ?, 0, datetime('now'))
	`
	if _, err := db.Exec(query, id, userID, jobID, date, chargedUSD); err != nil {
		t.Fatalf("failed to insert test usage record: %v", err)
	}
}

// InsertTestBalance is a helper to insert a test user balance.
func InsertTestBalance(t *testing.T, db *sql.DB, userID string, balanceUSD float64) {
	t.Helper()
	query := `
		INSERT INTO user_balances (user_id, balance_usd, lifetime_spent, lifetime_added, created_at, updated_at)
		VALUES (?, ?, 0, 0, datetime('now'), datetime('now'))
	`
	if _, err := db.Exec(query, userID, balanceUSD); err != nil {
		t.Fatalf("failed to insert test balance: %v", err)
	}
}

// InsertTestCreditTransaction is a helper to insert a test credit transaction.
func InsertTestCreditTransaction(t *testing.T, db *sql.DB, id, userID, txType string, amountUSD, balanceAfter float64) {
	t.Helper()
	query := `
		INSERT INTO credit_transactions (id, user_id, type, amount_usd, balance_after, description, created_at)
		VALUES (?, ?, ?, ?, ?, 'Test transaction', datetime('now'))
	`
	if _, err := db.Exec(query, id, userID, txType, amountUSD, balanceAfter); err != nil {
		t.Fatalf("failed to insert test credit transaction: %v", err)
	}
}

// InsertTestServiceKey is a helper to insert a test service key.
func InsertTestServiceKey(t *testing.T, db *sql.DB, provider, apiKey string, enabled bool) {
	t.Helper()
	isEnabled := 0
	if enabled {
		isEnabled = 1
	}
	query := `
		INSERT INTO service_keys (provider, api_key_encrypted, is_enabled, created_at, updated_at)
		VALUES (?, ?, ?, datetime('now'), datetime('now'))
	`
	if _, err := db.Exec(query, provider, apiKey, isEnabled); err != nil {
		t.Fatalf("failed to insert test service key: %v", err)
	}
}

// InsertTestFallbackChainEntry is a helper to insert a fallback chain entry.
func InsertTestFallbackChainEntry(t *testing.T, db *sql.DB, id, provider, model string, position int, enabled bool) {
	t.Helper()
	isEnabled := 0
	if enabled {
		isEnabled = 1
	}
	query := `
		INSERT INTO fallback_chain (id, tier, provider, model, position, is_enabled, created_at, updated_at)
		VALUES (?, NULL, ?, ?, ?, ?, datetime('now'), datetime('now'))
	`
	if _, err := db.Exec(query, id, provider, model, position, isEnabled); err != nil {
		t.Fatalf("failed to insert test fallback chain entry: %v", err)
	}
}

// InsertTestWebhook is a helper to insert a test webhook.
func InsertTestWebhook(t *testing.T, db *sql.DB, id, userID, name, url string, enabled bool) {
	t.Helper()
	isActive := 0
	if enabled {
		isActive = 1
	}
	query := `
		INSERT INTO webhooks (id, user_id, name, url, secret, events, is_active, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'test-secret', '["job.completed"]', ?, datetime('now'), datetime('now'))
	`
	if _, err := db.Exec(query, id, userID, name, url, isActive); err != nil {
		t.Fatalf("failed to insert test webhook: %v", err)
	}
}
