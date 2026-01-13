package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/jmylchreest/refyne-api/internal/models"
)

// SQLiteUserServiceKeyRepository implements UserServiceKeyRepository for SQLite/libsql.
type SQLiteUserServiceKeyRepository struct {
	db *sql.DB
}

// NewSQLiteUserServiceKeyRepository creates a new SQLite user service key repository.
func NewSQLiteUserServiceKeyRepository(db *sql.DB) *SQLiteUserServiceKeyRepository {
	return &SQLiteUserServiceKeyRepository{db: db}
}

// Upsert creates or updates a user service key.
func (r *SQLiteUserServiceKeyRepository) Upsert(ctx context.Context, key *models.UserServiceKey) error {
	now := time.Now().Format(time.RFC3339)

	if key.ID == "" {
		key.ID = ulid.Make().String()
	}

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO user_service_keys (id, user_id, provider, api_key_encrypted, base_url, is_enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, provider) DO UPDATE SET
			api_key_encrypted = excluded.api_key_encrypted,
			base_url = excluded.base_url,
			is_enabled = excluded.is_enabled,
			updated_at = excluded.updated_at
	`, key.ID, key.UserID, key.Provider, key.APIKeyEncrypted, key.BaseURL, key.IsEnabled, now, now)

	return err
}

// GetByID retrieves a user service key by ID.
func (r *SQLiteUserServiceKeyRepository) GetByID(ctx context.Context, id string) (*models.UserServiceKey, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, user_id, provider, api_key_encrypted, base_url, is_enabled, created_at, updated_at
		FROM user_service_keys
		WHERE id = ?
	`, id)

	return r.scanKey(row)
}

// GetByUserID retrieves all service keys for a user.
func (r *SQLiteUserServiceKeyRepository) GetByUserID(ctx context.Context, userID string) ([]*models.UserServiceKey, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, provider, api_key_encrypted, base_url, is_enabled, created_at, updated_at
		FROM user_service_keys
		WHERE user_id = ?
		ORDER BY provider
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanKeys(rows)
}

// GetByUserAndProvider retrieves a specific provider key for a user.
func (r *SQLiteUserServiceKeyRepository) GetByUserAndProvider(ctx context.Context, userID, provider string) (*models.UserServiceKey, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, user_id, provider, api_key_encrypted, base_url, is_enabled, created_at, updated_at
		FROM user_service_keys
		WHERE user_id = ? AND provider = ?
	`, userID, provider)

	return r.scanKey(row)
}

// GetEnabledByUserID retrieves all enabled service keys for a user.
func (r *SQLiteUserServiceKeyRepository) GetEnabledByUserID(ctx context.Context, userID string) ([]*models.UserServiceKey, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, provider, api_key_encrypted, base_url, is_enabled, created_at, updated_at
		FROM user_service_keys
		WHERE user_id = ? AND is_enabled = 1
		ORDER BY provider
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanKeys(rows)
}

// Delete removes a user service key by ID.
func (r *SQLiteUserServiceKeyRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM user_service_keys WHERE id = ?`, id)
	return err
}

// scanKey scans a single row into a UserServiceKey.
func (r *SQLiteUserServiceKeyRepository) scanKey(row *sql.Row) (*models.UserServiceKey, error) {
	var key models.UserServiceKey
	var createdAt, updatedAt string
	var baseURL sql.NullString

	err := row.Scan(
		&key.ID,
		&key.UserID,
		&key.Provider,
		&key.APIKeyEncrypted,
		&baseURL,
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

	key.BaseURL = baseURL.String
	key.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	key.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

	return &key, nil
}

// scanKeys scans multiple rows into UserServiceKey slice.
func (r *SQLiteUserServiceKeyRepository) scanKeys(rows *sql.Rows) ([]*models.UserServiceKey, error) {
	var keys []*models.UserServiceKey

	for rows.Next() {
		var key models.UserServiceKey
		var createdAt, updatedAt string
		var baseURL sql.NullString

		err := rows.Scan(
			&key.ID,
			&key.UserID,
			&key.Provider,
			&key.APIKeyEncrypted,
			&baseURL,
			&key.IsEnabled,
			&createdAt,
			&updatedAt,
		)
		if err != nil {
			return nil, err
		}

		key.BaseURL = baseURL.String
		key.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		key.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

		keys = append(keys, &key)
	}

	return keys, rows.Err()
}
