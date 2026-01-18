package service

import (
	"context"
	"database/sql"
	"log/slog"
)

// UserCleanupService handles deletion of all user data.
// This is used when a user deletes their account.
type UserCleanupService struct {
	db     *sql.DB
	logger *slog.Logger
}

// NewUserCleanupService creates a new user cleanup service.
func NewUserCleanupService(db *sql.DB, logger *slog.Logger) *UserCleanupService {
	return &UserCleanupService{
		db:     db,
		logger: logger,
	}
}

// DeleteAllUserData deletes all data associated with a user.
// This includes jobs, API keys, LLM configs, usage records, webhooks, etc.
// This operation is irreversible.
func (s *UserCleanupService) DeleteAllUserData(ctx context.Context, userID string) error {
	s.logger.Info("starting user data deletion", "user_id", userID)

	// Use a transaction to ensure atomicity
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Order matters due to foreign key constraints (delete children first)
	// Note: SQLite with foreign keys disabled might not enforce this, but good practice

	// 1. Delete job results first (child of jobs)
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM job_results WHERE job_id IN (SELECT id FROM jobs WHERE user_id = ?)
	`, userID); err != nil {
		s.logger.Error("failed to delete job results", "user_id", userID, "error", err)
		return err
	}

	// 2. Delete webhook deliveries (child of jobs and webhooks)
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM webhook_deliveries WHERE job_id IN (SELECT id FROM jobs WHERE user_id = ?)
	`, userID); err != nil {
		s.logger.Error("failed to delete webhook deliveries", "user_id", userID, "error", err)
		return err
	}

	// 3. Delete jobs
	if _, err := tx.ExecContext(ctx, `DELETE FROM jobs WHERE user_id = ?`, userID); err != nil {
		s.logger.Error("failed to delete jobs", "user_id", userID, "error", err)
		return err
	}

	// 4. Delete API keys
	if _, err := tx.ExecContext(ctx, `DELETE FROM api_keys WHERE user_id = ?`, userID); err != nil {
		s.logger.Error("failed to delete API keys", "user_id", userID, "error", err)
		return err
	}

	// 5. Delete LLM config
	if _, err := tx.ExecContext(ctx, `DELETE FROM llm_configs WHERE user_id = ?`, userID); err != nil {
		s.logger.Error("failed to delete LLM config", "user_id", userID, "error", err)
		return err
	}

	// 6. Delete user service keys (BYOK keys)
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_service_keys WHERE user_id = ?`, userID); err != nil {
		s.logger.Error("failed to delete user service keys", "user_id", userID, "error", err)
		return err
	}

	// 7. Delete user fallback chain
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_fallback_chain WHERE user_id = ?`, userID); err != nil {
		s.logger.Error("failed to delete user fallback chain", "user_id", userID, "error", err)
		return err
	}

	// 8. Delete webhooks
	if _, err := tx.ExecContext(ctx, `DELETE FROM webhooks WHERE user_id = ?`, userID); err != nil {
		s.logger.Error("failed to delete webhooks", "user_id", userID, "error", err)
		return err
	}

	// 9. Delete usage insights (child of usage records)
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM usage_insights WHERE usage_id IN (SELECT id FROM usage_records WHERE user_id = ?)
	`, userID); err != nil {
		s.logger.Error("failed to delete usage insights", "user_id", userID, "error", err)
		return err
	}

	// 10. Delete usage records
	if _, err := tx.ExecContext(ctx, `DELETE FROM usage_records WHERE user_id = ?`, userID); err != nil {
		s.logger.Error("failed to delete usage records", "user_id", userID, "error", err)
		return err
	}

	// 11. Delete credit transactions
	if _, err := tx.ExecContext(ctx, `DELETE FROM credit_transactions WHERE user_id = ?`, userID); err != nil {
		s.logger.Error("failed to delete credit transactions", "user_id", userID, "error", err)
		return err
	}

	// 12. Delete user balance
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_balances WHERE user_id = ?`, userID); err != nil {
		s.logger.Error("failed to delete user balance", "user_id", userID, "error", err)
		return err
	}

	// 13. Delete schema snapshots
	if _, err := tx.ExecContext(ctx, `DELETE FROM schema_snapshots WHERE user_id = ?`, userID); err != nil {
		s.logger.Error("failed to delete schema snapshots", "user_id", userID, "error", err)
		return err
	}

	// 14. Delete user-owned schema catalog entries
	if _, err := tx.ExecContext(ctx, `DELETE FROM schema_catalog WHERE owner_user_id = ?`, userID); err != nil {
		s.logger.Error("failed to delete schema catalog entries", "user_id", userID, "error", err)
		return err
	}

	// 15. Delete saved sites
	if _, err := tx.ExecContext(ctx, `DELETE FROM saved_sites WHERE user_id = ?`, userID); err != nil {
		s.logger.Error("failed to delete saved sites", "user_id", userID, "error", err)
		return err
	}

	// 16. Delete telemetry events (optional - may want to keep anonymized)
	if _, err := tx.ExecContext(ctx, `DELETE FROM telemetry_events WHERE user_id = ?`, userID); err != nil {
		s.logger.Error("failed to delete telemetry events", "user_id", userID, "error", err)
		return err
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		s.logger.Error("failed to commit user deletion transaction", "user_id", userID, "error", err)
		return err
	}

	s.logger.Info("completed user data deletion", "user_id", userID)
	return nil
}
