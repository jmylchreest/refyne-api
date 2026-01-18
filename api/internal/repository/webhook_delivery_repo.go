package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/jmylchreest/refyne-api/internal/models"
)

// SQLiteWebhookDeliveryRepository implements WebhookDeliveryRepository for SQLite/libsql.
type SQLiteWebhookDeliveryRepository struct {
	db *sql.DB
}

// NewSQLiteWebhookDeliveryRepository creates a new SQLite webhook delivery repository.
func NewSQLiteWebhookDeliveryRepository(db *sql.DB) *SQLiteWebhookDeliveryRepository {
	return &SQLiteWebhookDeliveryRepository{db: db}
}

// Create creates a new webhook delivery record.
func (r *SQLiteWebhookDeliveryRepository) Create(ctx context.Context, delivery *models.WebhookDelivery) error {
	now := time.Now().Format(time.RFC3339)

	if delivery.ID == "" {
		delivery.ID = ulid.Make().String()
	}

	var requestHeadersJSON *string
	if len(delivery.RequestHeaders) > 0 {
		data, err := json.Marshal(delivery.RequestHeaders)
		if err != nil {
			return err
		}
		s := string(data)
		requestHeadersJSON = &s
	}

	var nextRetryAt *string
	if delivery.NextRetryAt != nil {
		s := delivery.NextRetryAt.Format(time.RFC3339)
		nextRetryAt = &s
	}

	var deliveredAt *string
	if delivery.DeliveredAt != nil {
		s := delivery.DeliveredAt.Format(time.RFC3339)
		deliveredAt = &s
	}

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO webhook_deliveries (
			id, webhook_id, job_id, event_type, url, payload_json, request_headers_json,
			status_code, response_body, response_time_ms, status, error_message,
			attempt_number, max_attempts, next_retry_at, created_at, delivered_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, delivery.ID, delivery.WebhookID, delivery.JobID, delivery.EventType, delivery.URL,
		delivery.PayloadJSON, requestHeadersJSON, delivery.StatusCode, delivery.ResponseBody,
		delivery.ResponseTimeMs, delivery.Status, delivery.ErrorMessage, delivery.AttemptNumber,
		delivery.MaxAttempts, nextRetryAt, now, deliveredAt)

	return err
}

// Update updates an existing webhook delivery record.
func (r *SQLiteWebhookDeliveryRepository) Update(ctx context.Context, delivery *models.WebhookDelivery) error {
	var requestHeadersJSON *string
	if len(delivery.RequestHeaders) > 0 {
		data, err := json.Marshal(delivery.RequestHeaders)
		if err != nil {
			return err
		}
		s := string(data)
		requestHeadersJSON = &s
	}

	var nextRetryAt *string
	if delivery.NextRetryAt != nil {
		s := delivery.NextRetryAt.Format(time.RFC3339)
		nextRetryAt = &s
	}

	var deliveredAt *string
	if delivery.DeliveredAt != nil {
		s := delivery.DeliveredAt.Format(time.RFC3339)
		deliveredAt = &s
	}

	_, err := r.db.ExecContext(ctx, `
		UPDATE webhook_deliveries SET
			status_code = ?, response_body = ?, response_time_ms = ?, status = ?,
			error_message = ?, attempt_number = ?, next_retry_at = ?, delivered_at = ?,
			request_headers_json = ?
		WHERE id = ?
	`, delivery.StatusCode, delivery.ResponseBody, delivery.ResponseTimeMs, delivery.Status,
		delivery.ErrorMessage, delivery.AttemptNumber, nextRetryAt, deliveredAt,
		requestHeadersJSON, delivery.ID)

	return err
}

// GetByID retrieves a webhook delivery by ID.
func (r *SQLiteWebhookDeliveryRepository) GetByID(ctx context.Context, id string) (*models.WebhookDelivery, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, webhook_id, job_id, event_type, url, payload_json, request_headers_json,
			   status_code, response_body, response_time_ms, status, error_message,
			   attempt_number, max_attempts, next_retry_at, created_at, delivered_at
		FROM webhook_deliveries
		WHERE id = ?
	`, id)

	return r.scanDelivery(row)
}

// GetByJobID retrieves all webhook deliveries for a job.
func (r *SQLiteWebhookDeliveryRepository) GetByJobID(ctx context.Context, jobID string) ([]*models.WebhookDelivery, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, webhook_id, job_id, event_type, url, payload_json, request_headers_json,
			   status_code, response_body, response_time_ms, status, error_message,
			   attempt_number, max_attempts, next_retry_at, created_at, delivered_at
		FROM webhook_deliveries
		WHERE job_id = ?
		ORDER BY created_at DESC
	`, jobID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return r.scanDeliveries(rows)
}

// GetByWebhookID retrieves all deliveries for a webhook.
func (r *SQLiteWebhookDeliveryRepository) GetByWebhookID(ctx context.Context, webhookID string, limit, offset int) ([]*models.WebhookDelivery, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, webhook_id, job_id, event_type, url, payload_json, request_headers_json,
			   status_code, response_body, response_time_ms, status, error_message,
			   attempt_number, max_attempts, next_retry_at, created_at, delivered_at
		FROM webhook_deliveries
		WHERE webhook_id = ?
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, webhookID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return r.scanDeliveries(rows)
}

// GetPendingRetries retrieves deliveries that are ready for retry.
func (r *SQLiteWebhookDeliveryRepository) GetPendingRetries(ctx context.Context, limit int) ([]*models.WebhookDelivery, error) {
	now := time.Now().Format(time.RFC3339)

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, webhook_id, job_id, event_type, url, payload_json, request_headers_json,
			   status_code, response_body, response_time_ms, status, error_message,
			   attempt_number, max_attempts, next_retry_at, created_at, delivered_at
		FROM webhook_deliveries
		WHERE status = 'retrying' AND next_retry_at <= ?
		ORDER BY next_retry_at
		LIMIT ?
	`, now, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return r.scanDeliveries(rows)
}

// DeleteByJobIDs deletes all deliveries for the specified job IDs.
func (r *SQLiteWebhookDeliveryRepository) DeleteByJobIDs(ctx context.Context, jobIDs []string) error {
	if len(jobIDs) == 0 {
		return nil
	}

	// Build placeholders for IN clause
	placeholders := ""
	args := make([]interface{}, len(jobIDs))
	for i, id := range jobIDs {
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
		args[i] = id
	}

	_, err := r.db.ExecContext(ctx, `DELETE FROM webhook_deliveries WHERE job_id IN (`+placeholders+`)`, args...)
	return err
}

// scanDelivery scans a single row into a WebhookDelivery.
func (r *SQLiteWebhookDeliveryRepository) scanDelivery(row *sql.Row) (*models.WebhookDelivery, error) {
	var delivery models.WebhookDelivery
	var webhookID sql.NullString
	var requestHeadersJSON sql.NullString
	var statusCode sql.NullInt64
	var responseBody sql.NullString
	var responseTimeMs sql.NullInt64
	var errorMessage sql.NullString
	var nextRetryAt sql.NullString
	var createdAt string
	var deliveredAt sql.NullString

	err := row.Scan(
		&delivery.ID,
		&webhookID,
		&delivery.JobID,
		&delivery.EventType,
		&delivery.URL,
		&delivery.PayloadJSON,
		&requestHeadersJSON,
		&statusCode,
		&responseBody,
		&responseTimeMs,
		&delivery.Status,
		&errorMessage,
		&delivery.AttemptNumber,
		&delivery.MaxAttempts,
		&nextRetryAt,
		&createdAt,
		&deliveredAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if webhookID.Valid {
		delivery.WebhookID = &webhookID.String
	}

	if requestHeadersJSON.Valid {
		if err := json.Unmarshal([]byte(requestHeadersJSON.String), &delivery.RequestHeaders); err != nil {
			return nil, err
		}
	}

	if statusCode.Valid {
		v := int(statusCode.Int64)
		delivery.StatusCode = &v
	}

	if responseTimeMs.Valid {
		v := int(responseTimeMs.Int64)
		delivery.ResponseTimeMs = &v
	}

	delivery.ResponseBody = responseBody.String
	delivery.ErrorMessage = errorMessage.String

	if nextRetryAt.Valid {
		t, _ := time.Parse(time.RFC3339, nextRetryAt.String)
		delivery.NextRetryAt = &t
	}

	delivery.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)

	if deliveredAt.Valid {
		t, _ := time.Parse(time.RFC3339, deliveredAt.String)
		delivery.DeliveredAt = &t
	}

	return &delivery, nil
}

// scanDeliveries scans multiple rows into WebhookDelivery slice.
func (r *SQLiteWebhookDeliveryRepository) scanDeliveries(rows *sql.Rows) ([]*models.WebhookDelivery, error) {
	var deliveries []*models.WebhookDelivery

	for rows.Next() {
		var delivery models.WebhookDelivery
		var webhookID sql.NullString
		var requestHeadersJSON sql.NullString
		var statusCode sql.NullInt64
		var responseBody sql.NullString
		var responseTimeMs sql.NullInt64
		var errorMessage sql.NullString
		var nextRetryAt sql.NullString
		var createdAt string
		var deliveredAt sql.NullString

		err := rows.Scan(
			&delivery.ID,
			&webhookID,
			&delivery.JobID,
			&delivery.EventType,
			&delivery.URL,
			&delivery.PayloadJSON,
			&requestHeadersJSON,
			&statusCode,
			&responseBody,
			&responseTimeMs,
			&delivery.Status,
			&errorMessage,
			&delivery.AttemptNumber,
			&delivery.MaxAttempts,
			&nextRetryAt,
			&createdAt,
			&deliveredAt,
		)
		if err != nil {
			return nil, err
		}

		if webhookID.Valid {
			delivery.WebhookID = &webhookID.String
		}

		if requestHeadersJSON.Valid {
			if err := json.Unmarshal([]byte(requestHeadersJSON.String), &delivery.RequestHeaders); err != nil {
				return nil, err
			}
		}

		if statusCode.Valid {
			v := int(statusCode.Int64)
			delivery.StatusCode = &v
		}

		if responseTimeMs.Valid {
			v := int(responseTimeMs.Int64)
			delivery.ResponseTimeMs = &v
		}

		delivery.ResponseBody = responseBody.String
		delivery.ErrorMessage = errorMessage.String

		if nextRetryAt.Valid {
			t, _ := time.Parse(time.RFC3339, nextRetryAt.String)
			delivery.NextRetryAt = &t
		}

		delivery.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)

		if deliveredAt.Valid {
			t, _ := time.Parse(time.RFC3339, deliveredAt.String)
			delivery.DeliveredAt = &t
		}

		deliveries = append(deliveries, &delivery)
	}

	return deliveries, rows.Err()
}
