package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/jmylchreest/refyne-api/internal/models"
)

// SQLiteWebhookRepository implements WebhookRepository for SQLite/libsql.
type SQLiteWebhookRepository struct {
	db *sql.DB
}

// NewSQLiteWebhookRepository creates a new SQLite webhook repository.
func NewSQLiteWebhookRepository(db *sql.DB) *SQLiteWebhookRepository {
	return &SQLiteWebhookRepository{db: db}
}

// Create creates a new webhook.
func (r *SQLiteWebhookRepository) Create(ctx context.Context, webhook *models.Webhook) error {
	now := time.Now().Format(time.RFC3339)

	if webhook.ID == "" {
		webhook.ID = ulid.Make().String()
	}

	eventsJSON, err := json.Marshal(webhook.Events)
	if err != nil {
		return err
	}

	var headersJSON *string
	if len(webhook.Headers) > 0 {
		data, err := json.Marshal(webhook.Headers)
		if err != nil {
			return err
		}
		s := string(data)
		headersJSON = &s
	}

	_, err = r.db.ExecContext(ctx, `
		INSERT INTO webhooks (id, user_id, name, url, secret_encrypted, events, headers_json, is_active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, webhook.ID, webhook.UserID, webhook.Name, webhook.URL, webhook.SecretEncrypted, string(eventsJSON), headersJSON, webhook.IsActive, now, now)

	return err
}

// GetByID retrieves a webhook by ID.
func (r *SQLiteWebhookRepository) GetByID(ctx context.Context, id string) (*models.Webhook, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, user_id, name, url, secret_encrypted, events, headers_json, is_active, created_at, updated_at
		FROM webhooks
		WHERE id = ?
	`, id)

	return r.scanWebhook(row)
}

// GetByUserID retrieves all webhooks for a user.
func (r *SQLiteWebhookRepository) GetByUserID(ctx context.Context, userID string) ([]*models.Webhook, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, name, url, secret_encrypted, events, headers_json, is_active, created_at, updated_at
		FROM webhooks
		WHERE user_id = ?
		ORDER BY name
	`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return r.scanWebhooks(rows)
}

// GetActiveByUserID retrieves all active webhooks for a user.
func (r *SQLiteWebhookRepository) GetActiveByUserID(ctx context.Context, userID string) ([]*models.Webhook, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, name, url, secret_encrypted, events, headers_json, is_active, created_at, updated_at
		FROM webhooks
		WHERE user_id = ? AND is_active = 1
		ORDER BY name
	`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return r.scanWebhooks(rows)
}

// GetByUserAndName retrieves a webhook by user ID and name.
func (r *SQLiteWebhookRepository) GetByUserAndName(ctx context.Context, userID, name string) (*models.Webhook, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, user_id, name, url, secret_encrypted, events, headers_json, is_active, created_at, updated_at
		FROM webhooks
		WHERE user_id = ? AND name = ?
	`, userID, name)

	return r.scanWebhook(row)
}

// Update updates an existing webhook.
func (r *SQLiteWebhookRepository) Update(ctx context.Context, webhook *models.Webhook) error {
	now := time.Now().Format(time.RFC3339)

	eventsJSON, err := json.Marshal(webhook.Events)
	if err != nil {
		return err
	}

	var headersJSON *string
	if len(webhook.Headers) > 0 {
		data, err := json.Marshal(webhook.Headers)
		if err != nil {
			return err
		}
		s := string(data)
		headersJSON = &s
	}

	_, err = r.db.ExecContext(ctx, `
		UPDATE webhooks
		SET name = ?, url = ?, secret_encrypted = ?, events = ?, headers_json = ?, is_active = ?, updated_at = ?
		WHERE id = ?
	`, webhook.Name, webhook.URL, webhook.SecretEncrypted, string(eventsJSON), headersJSON, webhook.IsActive, now, webhook.ID)

	return err
}

// Delete deletes a webhook by ID.
func (r *SQLiteWebhookRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM webhooks WHERE id = ?`, id)
	return err
}

// scanWebhook scans a single row into a Webhook.
func (r *SQLiteWebhookRepository) scanWebhook(row *sql.Row) (*models.Webhook, error) {
	var webhook models.Webhook
	var secretEncrypted sql.NullString
	var eventsJSON string
	var headersJSON sql.NullString
	var createdAt, updatedAt string

	err := row.Scan(
		&webhook.ID,
		&webhook.UserID,
		&webhook.Name,
		&webhook.URL,
		&secretEncrypted,
		&eventsJSON,
		&headersJSON,
		&webhook.IsActive,
		&createdAt,
		&updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	webhook.SecretEncrypted = secretEncrypted.String

	if err := json.Unmarshal([]byte(eventsJSON), &webhook.Events); err != nil {
		return nil, err
	}

	if headersJSON.Valid {
		if err := json.Unmarshal([]byte(headersJSON.String), &webhook.Headers); err != nil {
			return nil, err
		}
	}

	webhook.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	webhook.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

	return &webhook, nil
}

// scanWebhooks scans multiple rows into Webhook slice.
func (r *SQLiteWebhookRepository) scanWebhooks(rows *sql.Rows) ([]*models.Webhook, error) {
	var webhooks []*models.Webhook

	for rows.Next() {
		var webhook models.Webhook
		var secretEncrypted sql.NullString
		var eventsJSON string
		var headersJSON sql.NullString
		var createdAt, updatedAt string

		err := rows.Scan(
			&webhook.ID,
			&webhook.UserID,
			&webhook.Name,
			&webhook.URL,
			&secretEncrypted,
			&eventsJSON,
			&headersJSON,
			&webhook.IsActive,
			&createdAt,
			&updatedAt,
		)
		if err != nil {
			return nil, err
		}

		webhook.SecretEncrypted = secretEncrypted.String

		if err := json.Unmarshal([]byte(eventsJSON), &webhook.Events); err != nil {
			return nil, err
		}

		if headersJSON.Valid {
			if err := json.Unmarshal([]byte(headersJSON.String), &webhook.Headers); err != nil {
				return nil, err
			}
		}

		webhook.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		webhook.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

		webhooks = append(webhooks, &webhook)
	}

	return webhooks, rows.Err()
}
