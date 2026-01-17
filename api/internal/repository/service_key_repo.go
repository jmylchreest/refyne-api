package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/jmylchreest/refyne-api/internal/models"
)

// SQLiteServiceKeyRepository implements ServiceKeyRepository for SQLite/libsql.
type SQLiteServiceKeyRepository struct {
	db *sql.DB
}

// NewSQLiteServiceKeyRepository creates a new SQLite service key repository.
func NewSQLiteServiceKeyRepository(db *sql.DB) *SQLiteServiceKeyRepository {
	return &SQLiteServiceKeyRepository{db: db}
}

// Upsert creates or updates a service key.
func (r *SQLiteServiceKeyRepository) Upsert(ctx context.Context, key *models.ServiceKey) error {
	now := time.Now().Format(time.RFC3339)

	if key.ID == "" {
		key.ID = ulid.Make().String()
	}

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO service_keys (id, provider, api_key_encrypted, default_model, is_enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(provider) DO UPDATE SET
			api_key_encrypted = excluded.api_key_encrypted,
			default_model = excluded.default_model,
			is_enabled = excluded.is_enabled,
			updated_at = excluded.updated_at
	`, key.ID, key.Provider, key.APIKeyEncrypted, key.DefaultModel, key.IsEnabled, now, now)

	return err
}

// GetByProvider retrieves a service key by provider.
func (r *SQLiteServiceKeyRepository) GetByProvider(ctx context.Context, provider string) (*models.ServiceKey, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, provider, api_key_encrypted, default_model, is_enabled, created_at, updated_at
		FROM service_keys
		WHERE provider = ?
	`, provider)

	return r.scanKey(row)
}

// GetAll retrieves all service keys.
func (r *SQLiteServiceKeyRepository) GetAll(ctx context.Context) ([]*models.ServiceKey, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, provider, api_key_encrypted, default_model, is_enabled, created_at, updated_at
		FROM service_keys
		ORDER BY provider
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return r.scanKeys(rows)
}

// GetEnabled retrieves all enabled service keys.
func (r *SQLiteServiceKeyRepository) GetEnabled(ctx context.Context) ([]*models.ServiceKey, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, provider, api_key_encrypted, default_model, is_enabled, created_at, updated_at
		FROM service_keys
		WHERE is_enabled = 1
		ORDER BY provider
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return r.scanKeys(rows)
}

// Delete removes a service key by provider.
func (r *SQLiteServiceKeyRepository) Delete(ctx context.Context, provider string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM service_keys WHERE provider = ?`, provider)
	return err
}

// scanKey scans a single row into a ServiceKey.
func (r *SQLiteServiceKeyRepository) scanKey(row *sql.Row) (*models.ServiceKey, error) {
	var key models.ServiceKey
	var createdAt, updatedAt string

	err := row.Scan(
		&key.ID,
		&key.Provider,
		&key.APIKeyEncrypted,
		&key.DefaultModel,
		&key.IsEnabled,
		&createdAt,
		&updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	key.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	key.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

	return &key, nil
}

// scanKeys scans multiple rows into ServiceKey slice.
func (r *SQLiteServiceKeyRepository) scanKeys(rows *sql.Rows) ([]*models.ServiceKey, error) {
	var keys []*models.ServiceKey

	for rows.Next() {
		var key models.ServiceKey
		var createdAt, updatedAt string

		err := rows.Scan(
			&key.ID,
			&key.Provider,
			&key.APIKeyEncrypted,
			&key.DefaultModel,
			&key.IsEnabled,
			&createdAt,
			&updatedAt,
		)
		if err != nil {
			return nil, err
		}

		key.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		key.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

		keys = append(keys, &key)
	}

	return keys, rows.Err()
}
