package service

import (
	"log/slog"
	"testing"

	appconfig "github.com/jmylchreest/refyne-api/internal/config"
)

// ========================================
// UserCleanupService Tests
// ========================================

// Note: UserCleanupService requires a real database connection for meaningful tests.
// These tests cover constructor behavior and nil handling.
// Full integration tests would require a test database setup.

// ----------------------------------------
// Constructor Tests
// ----------------------------------------

func TestNewUserCleanupService(t *testing.T) {
	logger := slog.Default()

	// Test with nil db and storage (should not panic)
	svc := NewUserCleanupService(nil, nil, logger)
	if svc == nil {
		t.Fatal("expected service, got nil")
	}
	if svc.logger != logger {
		t.Error("logger not set correctly")
	}
	if svc.db != nil {
		t.Error("expected db to be nil when passed nil")
	}
	if svc.storageSvc != nil {
		t.Error("expected storageSvc to be nil when passed nil")
	}
}

func TestNewUserCleanupService_WithStorage(t *testing.T) {
	logger := slog.Default()
	cfg := &appconfig.Config{StorageEnabled: false}
	storageSvc, _ := NewStorageService(cfg, logger)

	svc := NewUserCleanupService(nil, storageSvc, logger)
	if svc == nil {
		t.Fatal("expected service, got nil")
	}
	if svc.storageSvc != storageSvc {
		t.Error("storageSvc not set correctly")
	}
}

// ----------------------------------------
// Behavior Notes
// ----------------------------------------

// The DeleteAllUserData method:
// 1. Gets job IDs from the database
// 2. Deletes S3 objects for each job (if storage is enabled)
// 3. Runs DELETE statements in a transaction for:
//    - job_results
//    - webhook_deliveries
//    - jobs
//    - api_keys
//    - user_service_keys
//    - user_fallback_chain
//    - webhooks
//    - user_balances
//    - schema_snapshots
//    - schema_catalog (owner_user_id)
//    - saved_sites
// 4. Records deletion in deleted_users table
//
// Retained for audit/compliance:
// - usage_records
// - usage_insights
// - credit_transactions
// - telemetry_events
//
// Full testing requires:
// - sqlmock or test database
// - Mock storage service for S3 operations

// Note: Integration tests for this service should be done with a real
// test database to verify the SQL statements and transaction handling.
// Unit testing with sqlmock is possible but adds complexity.

// Example integration test setup (not implemented due to database requirement):
//
// func TestUserCleanupService_DeleteAllUserData_Integration(t *testing.T) {
//     if testing.Short() {
//         t.Skip("skipping integration test")
//     }
//
//     // Setup test database with test data
//     db := setupTestDB(t)
//     defer db.Close()
//
//     // Insert test user data
//     insertTestUserData(t, db, "test-user")
//
//     // Create service
//     cfg := &appconfig.Config{StorageEnabled: false}
//     storageSvc, _ := NewStorageService(cfg, slog.Default())
//     svc := NewUserCleanupService(db, storageSvc, slog.Default())
//
//     // Execute deletion
//     err := svc.DeleteAllUserData(context.Background(), "test-user")
//     if err != nil {
//         t.Fatalf("unexpected error: %v", err)
//     }
//
//     // Verify data was deleted
//     verifyUserDataDeleted(t, db, "test-user")
//
//     // Verify audit records retained
//     verifyAuditRecordsRetained(t, db, "test-user")
// }
