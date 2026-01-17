package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/jmylchreest/refyne-api/internal/models"
)

// SQLiteUserFallbackChainRepository implements UserFallbackChainRepository for SQLite/libsql.
type SQLiteUserFallbackChainRepository struct {
	db *sql.DB
}

// NewSQLiteUserFallbackChainRepository creates a new SQLite user fallback chain repository.
func NewSQLiteUserFallbackChainRepository(db *sql.DB) *SQLiteUserFallbackChainRepository {
	return &SQLiteUserFallbackChainRepository{db: db}
}

// GetByUserID retrieves all fallback chain entries for a user.
func (r *SQLiteUserFallbackChainRepository) GetByUserID(ctx context.Context, userID string) ([]*models.UserFallbackChainEntry, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, position, provider, model, temperature, max_tokens, strict_mode, is_enabled, created_at, updated_at
		FROM user_fallback_chain
		WHERE user_id = ?
		ORDER BY position
	`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return r.scanEntries(rows)
}

// GetEnabledByUserID retrieves enabled fallback chain entries for a user in position order.
func (r *SQLiteUserFallbackChainRepository) GetEnabledByUserID(ctx context.Context, userID string) ([]*models.UserFallbackChainEntry, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, position, provider, model, temperature, max_tokens, strict_mode, is_enabled, created_at, updated_at
		FROM user_fallback_chain
		WHERE user_id = ? AND is_enabled = 1
		ORDER BY position
	`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return r.scanEntries(rows)
}

// ReplaceAll replaces all fallback chain entries for a user.
func (r *SQLiteUserFallbackChainRepository) ReplaceAll(ctx context.Context, userID string, entries []*models.UserFallbackChainEntry) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }() // Rollback is no-op after commit

	// Delete existing entries for user
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_fallback_chain WHERE user_id = ?`, userID); err != nil {
		return err
	}

	// Insert new entries
	now := time.Now().Format(time.RFC3339)
	for i, entry := range entries {
		if entry.ID == "" {
			entry.ID = ulid.Make().String()
		}
		entry.UserID = userID
		entry.Position = i + 1

		// Convert StrictMode bool to int for SQLite
		var strictModeInt *int
		if entry.StrictMode != nil {
			v := 0
			if *entry.StrictMode {
				v = 1
			}
			strictModeInt = &v
		}

		_, err := tx.ExecContext(ctx, `
			INSERT INTO user_fallback_chain (id, user_id, position, provider, model, temperature, max_tokens, strict_mode, is_enabled, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, entry.ID, entry.UserID, entry.Position, entry.Provider, entry.Model, entry.Temperature, entry.MaxTokens, strictModeInt, entry.IsEnabled, now, now)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// scanEntries scans multiple rows into UserFallbackChainEntry slice.
func (r *SQLiteUserFallbackChainRepository) scanEntries(rows *sql.Rows) ([]*models.UserFallbackChainEntry, error) {
	var entries []*models.UserFallbackChainEntry

	for rows.Next() {
		var entry models.UserFallbackChainEntry
		var temperature sql.NullFloat64
		var maxTokens sql.NullInt64
		var strictMode sql.NullInt64
		var createdAt, updatedAt string

		err := rows.Scan(
			&entry.ID,
			&entry.UserID,
			&entry.Position,
			&entry.Provider,
			&entry.Model,
			&temperature,
			&maxTokens,
			&strictMode,
			&entry.IsEnabled,
			&createdAt,
			&updatedAt,
		)
		if err != nil {
			return nil, err
		}

		if temperature.Valid {
			entry.Temperature = &temperature.Float64
		}
		if maxTokens.Valid {
			v := int(maxTokens.Int64)
			entry.MaxTokens = &v
		}
		if strictMode.Valid {
			v := strictMode.Int64 == 1
			entry.StrictMode = &v
		}
		entry.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		entry.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

		entries = append(entries, &entry)
	}

	return entries, rows.Err()
}
