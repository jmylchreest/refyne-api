package service

import (
	"context"
	"database/sql"
	"log/slog"
	"time"
)

// UserCleanupService handles deletion of all user data.
// This is used when a user deletes their account.
type UserCleanupService struct {
	db         *sql.DB
	storageSvc *StorageService
	logger     *slog.Logger
}

// NewUserCleanupService creates a new user cleanup service.
func NewUserCleanupService(db *sql.DB, storageSvc *StorageService, logger *slog.Logger) *UserCleanupService {
	return &UserCleanupService{
		db:         db,
		storageSvc: storageSvc,
		logger:     logger,
	}
}

// DeleteAllUserData deletes user's personal data while retaining audit records.
//
// RETAINED for audit/compliance (user_id preserved for correlation):
//   - usage_records: billing and usage history
//   - usage_insights: detailed usage analytics (includes target_url for context)
//   - credit_transactions: payment history
//   - telemetry_events: system events
//
// DELETED (personal/operational data):
//   - jobs: job records (usage_records has billing info)
//   - job_results: extraction output data
//   - S3 storage: result files in object storage
//   - api_keys, user_service_keys: authentication credentials
//   - webhooks, webhook_deliveries: notification configs
//   - user_fallback_chain: LLM preferences
//   - schema_snapshots, schema_catalog: user schemas
//   - saved_sites: saved site configurations
//   - user_balances: current balance (transactions retained)
//
// This operation is irreversible.
func (s *UserCleanupService) DeleteAllUserData(ctx context.Context, userID string) error {
	s.logger.Info("starting user data deletion", "user_id", userID)

	// Step 1: Get job IDs BEFORE deletion (needed for S3 cleanup)
	jobIDs, err := s.getUserJobIDs(ctx, userID)
	if err != nil {
		s.logger.Error("failed to get user job IDs", "user_id", userID, "error", err)
		return err
	}
	s.logger.Info("found user jobs to delete", "user_id", userID, "job_count", len(jobIDs))

	// Step 2: Delete S3 objects for each job (outside transaction - S3 ops can't rollback)
	if s.storageSvc != nil && s.storageSvc.IsEnabled() {
		for _, jobID := range jobIDs {
			if err := s.storageSvc.DeleteJobResults(ctx, jobID); err != nil {
				// Log but continue - S3 cleanup will catch orphaned objects
				s.logger.Warn("failed to delete S3 object for job", "job_id", jobID, "error", err)
			}
		}
		s.logger.Info("deleted S3 objects", "user_id", userID, "count", len(jobIDs))
	}

	// Step 3: Database cleanup in transaction
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Order matters due to foreign key constraints (delete children first)

	// 1. Delete job results (extraction output)
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM job_results WHERE job_id IN (SELECT id FROM jobs WHERE user_id = ?)
	`, userID); err != nil {
		s.logger.Error("failed to delete job results", "user_id", userID, "error", err)
		return err
	}

	// 2. Delete webhook deliveries
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM webhook_deliveries WHERE job_id IN (SELECT id FROM jobs WHERE user_id = ?)
	`, userID); err != nil {
		s.logger.Error("failed to delete webhook deliveries", "user_id", userID, "error", err)
		return err
	}

	// 3. Delete jobs (usage_records retains billing info with job_id reference)
	if _, err := tx.ExecContext(ctx, `DELETE FROM jobs WHERE user_id = ?`, userID); err != nil {
		s.logger.Error("failed to delete jobs", "user_id", userID, "error", err)
		return err
	}

	// 4. Delete API keys (authentication credentials)
	if _, err := tx.ExecContext(ctx, `DELETE FROM api_keys WHERE user_id = ?`, userID); err != nil {
		s.logger.Error("failed to delete API keys", "user_id", userID, "error", err)
		return err
	}

	// 5. Delete user service keys (BYOK credentials)
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_service_keys WHERE user_id = ?`, userID); err != nil {
		s.logger.Error("failed to delete user service keys", "user_id", userID, "error", err)
		return err
	}

	// 6. Delete user fallback chain (LLM preferences)
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_fallback_chain WHERE user_id = ?`, userID); err != nil {
		s.logger.Error("failed to delete user fallback chain", "user_id", userID, "error", err)
		return err
	}

	// 7. Delete webhooks (notification configs)
	if _, err := tx.ExecContext(ctx, `DELETE FROM webhooks WHERE user_id = ?`, userID); err != nil {
		s.logger.Error("failed to delete webhooks", "user_id", userID, "error", err)
		return err
	}

	// NOTE: usage_insights, usage_records, credit_transactions, telemetry_events
	// are RETAINED for audit - provides billing history for abuse/dispute investigation

	// 8. Delete user balance (current state - transactions retained for history)
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_balances WHERE user_id = ?`, userID); err != nil {
		s.logger.Error("failed to delete user balance", "user_id", userID, "error", err)
		return err
	}

	// 9. Delete schema snapshots (user's saved schemas)
	if _, err := tx.ExecContext(ctx, `DELETE FROM schema_snapshots WHERE user_id = ?`, userID); err != nil {
		s.logger.Error("failed to delete schema snapshots", "user_id", userID, "error", err)
		return err
	}

	// 10. Delete user-owned schema catalog entries
	if _, err := tx.ExecContext(ctx, `DELETE FROM schema_catalog WHERE owner_user_id = ?`, userID); err != nil {
		s.logger.Error("failed to delete schema catalog entries", "user_id", userID, "error", err)
		return err
	}

	// 11. Delete saved sites
	if _, err := tx.ExecContext(ctx, `DELETE FROM saved_sites WHERE user_id = ?`, userID); err != nil {
		s.logger.Error("failed to delete saved sites", "user_id", userID, "error", err)
		return err
	}

	// 12. Record the user deletion for audit tracking
	deletedAt := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO deleted_users (user_id, deleted_at, reason) VALUES (?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET deleted_at = excluded.deleted_at
	`, userID, deletedAt, "user_requested"); err != nil {
		s.logger.Error("failed to record user deletion", "user_id", userID, "error", err)
		return err
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		s.logger.Error("failed to commit user deletion transaction", "user_id", userID, "error", err)
		return err
	}

	s.logger.Info("completed user data deletion", "user_id", userID, "deleted_at", deletedAt)
	return nil
}

// getUserJobIDs returns all job IDs for a user.
func (s *UserCleanupService) getUserJobIDs(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM jobs WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		jobIDs = append(jobIDs, id)
	}
	return jobIDs, rows.Err()
}
