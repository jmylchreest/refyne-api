package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/jmylchreest/refyne-api/internal/models"
)

// SQLiteFallbackChainRepository implements FallbackChainRepository using SQLite.
type SQLiteFallbackChainRepository struct {
	db *sql.DB
}

// NewSQLiteFallbackChainRepository creates a new SQLite fallback chain repository.
func NewSQLiteFallbackChainRepository(db *sql.DB) *SQLiteFallbackChainRepository {
	return &SQLiteFallbackChainRepository{db: db}
}

// scanEntry scans a fallback chain entry from a row.
func scanEntry(rows *sql.Rows) (*models.FallbackChainEntry, error) {
	entry := &models.FallbackChainEntry{}
	var tier sql.NullString
	var temperature sql.NullFloat64
	var maxTokens sql.NullInt64
	var strictMode sql.NullInt64
	var createdAt, updatedAt string
	if err := rows.Scan(
		&entry.ID,
		&tier,
		&entry.Position,
		&entry.Provider,
		&entry.Model,
		&temperature,
		&maxTokens,
		&strictMode,
		&entry.IsEnabled,
		&createdAt,
		&updatedAt,
	); err != nil {
		return nil, err
	}
	if tier.Valid {
		entry.Tier = &tier.String
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
	return entry, nil
}

// GetAll returns all fallback chain entries ordered by tier and position.
func (r *SQLiteFallbackChainRepository) GetAll(ctx context.Context) ([]*models.FallbackChainEntry, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, tier, position, provider, model, temperature, max_tokens, strict_mode, is_enabled, created_at, updated_at
		FROM fallback_chain
		ORDER BY COALESCE(tier, ''), position ASC
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var entries []*models.FallbackChainEntry
	for rows.Next() {
		entry, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}

	return entries, rows.Err()
}

// GetByTier returns all fallback chain entries for a specific tier.
// Pass nil for the default chain, or a tier name for tier-specific chain.
func (r *SQLiteFallbackChainRepository) GetByTier(ctx context.Context, tier *string) ([]*models.FallbackChainEntry, error) {
	var rows *sql.Rows
	var err error

	if tier == nil {
		rows, err = r.db.QueryContext(ctx, `
			SELECT id, tier, position, provider, model, temperature, max_tokens, strict_mode, is_enabled, created_at, updated_at
			FROM fallback_chain
			WHERE tier IS NULL
			ORDER BY position ASC
		`)
	} else {
		rows, err = r.db.QueryContext(ctx, `
			SELECT id, tier, position, provider, model, temperature, max_tokens, strict_mode, is_enabled, created_at, updated_at
			FROM fallback_chain
			WHERE tier = ?
			ORDER BY position ASC
		`, *tier)
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var entries []*models.FallbackChainEntry
	for rows.Next() {
		entry, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}

	return entries, rows.Err()
}

// GetEnabledByTier returns enabled entries for a specific tier.
// If no tier-specific chain exists, returns the default chain.
func (r *SQLiteFallbackChainRepository) GetEnabledByTier(ctx context.Context, tier string) ([]*models.FallbackChainEntry, error) {
	// First, try to get tier-specific chain
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, tier, position, provider, model, temperature, max_tokens, strict_mode, is_enabled, created_at, updated_at
		FROM fallback_chain
		WHERE tier = ? AND is_enabled = 1
		ORDER BY position ASC
	`, tier)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var entries []*models.FallbackChainEntry
	for rows.Next() {
		entry, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// If tier-specific chain exists, return it
	if len(entries) > 0 {
		return entries, nil
	}

	// Otherwise, fall back to default chain (tier IS NULL)
	return r.GetEnabled(ctx)
}

// GetEnabled returns all enabled fallback chain entries from the default chain.
func (r *SQLiteFallbackChainRepository) GetEnabled(ctx context.Context) ([]*models.FallbackChainEntry, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, tier, position, provider, model, temperature, max_tokens, strict_mode, is_enabled, created_at, updated_at
		FROM fallback_chain
		WHERE tier IS NULL AND is_enabled = 1
		ORDER BY position ASC
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var entries []*models.FallbackChainEntry
	for rows.Next() {
		entry, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}

	return entries, rows.Err()
}

// ReplaceAllByTier replaces all entries for a specific tier.
// Pass nil for the default chain, or a tier name for tier-specific chain.
func (r *SQLiteFallbackChainRepository) ReplaceAllByTier(ctx context.Context, tier *string, entries []*models.FallbackChainEntry) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }() // Rollback is no-op after commit

	// Delete existing entries for this tier
	if tier == nil {
		if _, err := tx.ExecContext(ctx, "DELETE FROM fallback_chain WHERE tier IS NULL"); err != nil {
			return err
		}
	} else {
		if _, err := tx.ExecContext(ctx, "DELETE FROM fallback_chain WHERE tier = ?", *tier); err != nil {
			return err
		}
	}

	// Insert new entries
	now := time.Now().UTC().Format(time.RFC3339)
	for i, entry := range entries {
		if entry.ID == "" {
			entry.ID = ulid.Make().String()
		}
		entry.Position = i + 1 // Ensure positions are sequential starting from 1
		entry.Tier = tier

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
			INSERT INTO fallback_chain (id, tier, position, provider, model, temperature, max_tokens, strict_mode, is_enabled, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, entry.ID, tier, entry.Position, entry.Provider, entry.Model, entry.Temperature, entry.MaxTokens, strictModeInt, entry.IsEnabled, now, now)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// ReplaceAll replaces all entries in the default fallback chain.
// This is for backwards compatibility.
func (r *SQLiteFallbackChainRepository) ReplaceAll(ctx context.Context, entries []*models.FallbackChainEntry) error {
	return r.ReplaceAllByTier(ctx, nil, entries)
}

// GetAllTiers returns a list of all tiers that have custom chains configured.
func (r *SQLiteFallbackChainRepository) GetAllTiers(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT DISTINCT tier FROM fallback_chain WHERE tier IS NOT NULL ORDER BY tier
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var tiers []string
	for rows.Next() {
		var tier string
		if err := rows.Scan(&tier); err != nil {
			return nil, err
		}
		tiers = append(tiers, tier)
	}

	return tiers, rows.Err()
}

// DeleteByTier removes all entries for a specific tier.
func (r *SQLiteFallbackChainRepository) DeleteByTier(ctx context.Context, tier string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM fallback_chain WHERE tier = ?", tier)
	return err
}

// Create adds a new entry at the end of the chain for a specific tier.
func (r *SQLiteFallbackChainRepository) Create(ctx context.Context, entry *models.FallbackChainEntry) error {
	// Get the maximum position for this tier
	var maxPos sql.NullInt64
	var err error
	if entry.Tier == nil {
		err = r.db.QueryRowContext(ctx, "SELECT MAX(position) FROM fallback_chain WHERE tier IS NULL").Scan(&maxPos)
	} else {
		err = r.db.QueryRowContext(ctx, "SELECT MAX(position) FROM fallback_chain WHERE tier = ?", *entry.Tier).Scan(&maxPos)
	}
	if err != nil {
		return err
	}

	entry.Position = int(maxPos.Int64) + 1
	if entry.ID == "" {
		entry.ID = ulid.Make().String()
	}

	now := time.Now().UTC().Format(time.RFC3339)
	// Convert StrictMode bool to int for SQLite
	var strictModeInt *int
	if entry.StrictMode != nil {
		v := 0
		if *entry.StrictMode {
			v = 1
		}
		strictModeInt = &v
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO fallback_chain (id, tier, position, provider, model, temperature, max_tokens, strict_mode, is_enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, entry.ID, entry.Tier, entry.Position, entry.Provider, entry.Model, entry.Temperature, entry.MaxTokens, strictModeInt, entry.IsEnabled, now, now)

	return err
}

// Update updates an existing entry.
func (r *SQLiteFallbackChainRepository) Update(ctx context.Context, entry *models.FallbackChainEntry) error {
	now := time.Now().UTC().Format(time.RFC3339)
	// Convert StrictMode bool to int for SQLite
	var strictModeInt *int
	if entry.StrictMode != nil {
		v := 0
		if *entry.StrictMode {
			v = 1
		}
		strictModeInt = &v
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE fallback_chain
		SET provider = ?, model = ?, temperature = ?, max_tokens = ?, strict_mode = ?, is_enabled = ?, updated_at = ?
		WHERE id = ?
	`, entry.Provider, entry.Model, entry.Temperature, entry.MaxTokens, strictModeInt, entry.IsEnabled, now, entry.ID)

	return err
}

// Delete removes an entry and reorders remaining entries within its tier.
func (r *SQLiteFallbackChainRepository) Delete(ctx context.Context, id string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }() // Rollback is no-op after commit

	// Get the tier and position of the entry being deleted
	var tier sql.NullString
	var position int
	if err := tx.QueryRowContext(ctx, "SELECT tier, position FROM fallback_chain WHERE id = ?", id).Scan(&tier, &position); err != nil {
		if err == sql.ErrNoRows {
			return nil // Already deleted
		}
		return err
	}

	// Delete the entry
	if _, err := tx.ExecContext(ctx, "DELETE FROM fallback_chain WHERE id = ?", id); err != nil {
		return err
	}

	// Reorder remaining entries within the same tier
	if tier.Valid {
		if _, err := tx.ExecContext(ctx, `
			UPDATE fallback_chain SET position = position - 1 WHERE tier = ? AND position > ?
		`, tier.String, position); err != nil {
			return err
		}
	} else {
		if _, err := tx.ExecContext(ctx, `
			UPDATE fallback_chain SET position = position - 1 WHERE tier IS NULL AND position > ?
		`, position); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// Reorder updates the positions of entries within a tier.
// The ids slice should contain all entry IDs for that tier in the desired order.
func (r *SQLiteFallbackChainRepository) Reorder(ctx context.Context, ids []string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }() // Rollback is no-op after commit

	now := time.Now().UTC().Format(time.RFC3339)
	for i, id := range ids {
		if _, err := tx.ExecContext(ctx, `
			UPDATE fallback_chain SET position = ?, updated_at = ? WHERE id = ?
		`, i+1, now, id); err != nil {
			return err
		}
	}

	return tx.Commit()
}
