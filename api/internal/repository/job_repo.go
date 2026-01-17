package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jmylchreest/refyne-api/internal/constants"
	"github.com/jmylchreest/refyne-api/internal/models"
)

// SQLiteJobRepository implements JobRepository for SQLite.
type SQLiteJobRepository struct {
	db *sql.DB
}

// NewSQLiteJobRepository creates a new SQLite job repository.
func NewSQLiteJobRepository(db *sql.DB) *SQLiteJobRepository {
	return &SQLiteJobRepository{db: db}
}

func (r *SQLiteJobRepository) Create(ctx context.Context, job *models.Job) error {
	query := `
		INSERT INTO jobs (id, user_id, type, status, url, schema_json, crawl_options_json,
			result_json, error_message, urls_queued, page_count, token_usage_input, token_usage_output,
			cost_usd, webhook_url, webhook_status, webhook_attempts, started_at, completed_at,
			created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := r.db.ExecContext(ctx, query,
		job.ID,
		job.UserID,
		job.Type,
		job.Status,
		job.URL,
		job.SchemaJSON,
		nullString(job.CrawlOptionsJSON),
		nullString(job.ResultJSON),
		nullString(job.ErrorMessage),
		job.URLsQueued,
		job.PageCount,
		job.TokenUsageInput,
		job.TokenUsageOutput,
		job.CostUSD,
		nullString(job.WebhookURL),
		nullString(job.WebhookStatus),
		job.WebhookAttempts,
		nullTime(job.StartedAt),
		nullTime(job.CompletedAt),
		job.CreatedAt.Format(time.RFC3339),
		job.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("failed to create job: %w", err)
	}
	return nil
}

func (r *SQLiteJobRepository) GetByID(ctx context.Context, id string) (*models.Job, error) {
	query := `
		SELECT id, user_id, type, status, url, schema_json, crawl_options_json,
			result_json, error_message, urls_queued, page_count, token_usage_input, token_usage_output,
			cost_usd, webhook_url, webhook_status, webhook_attempts, started_at, completed_at,
			created_at, updated_at
		FROM jobs WHERE id = ?
	`
	return r.scanJob(r.db.QueryRowContext(ctx, query, id))
}

func (r *SQLiteJobRepository) GetByUserID(ctx context.Context, userID string, limit, offset int) ([]*models.Job, error) {
	query := `
		SELECT id, user_id, type, status, url, schema_json, crawl_options_json,
			result_json, error_message, urls_queued, page_count, token_usage_input, token_usage_output,
			cost_usd, webhook_url, webhook_status, webhook_attempts, started_at, completed_at,
			created_at, updated_at
		FROM jobs WHERE user_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?
	`
	rows, err := r.db.QueryContext(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query jobs: %w", err)
	}
	defer rows.Close()

	var jobs []*models.Job
	for rows.Next() {
		job, err := r.scanJobFromRows(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

func (r *SQLiteJobRepository) Update(ctx context.Context, job *models.Job) error {
	query := `
		UPDATE jobs SET status = ?, result_json = ?, error_message = ?, urls_queued = ?, page_count = ?,
			token_usage_input = ?, token_usage_output = ?, cost_usd = ?,
			webhook_status = ?, webhook_attempts = ?, started_at = ?, completed_at = ?, updated_at = ?
		WHERE id = ?
	`
	_, err := r.db.ExecContext(ctx, query,
		job.Status,
		nullString(job.ResultJSON),
		nullString(job.ErrorMessage),
		job.URLsQueued,
		job.PageCount,
		job.TokenUsageInput,
		job.TokenUsageOutput,
		job.CostUSD,
		nullString(job.WebhookStatus),
		job.WebhookAttempts,
		nullTime(job.StartedAt),
		nullTime(job.CompletedAt),
		time.Now().Format(time.RFC3339),
		job.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update job: %w", err)
	}
	return nil
}

func (r *SQLiteJobRepository) GetPending(ctx context.Context, limit int) ([]*models.Job, error) {
	query := `
		SELECT id, user_id, type, status, url, schema_json, crawl_options_json,
			result_json, error_message, urls_queued, page_count, token_usage_input, token_usage_output,
			cost_usd, webhook_url, webhook_status, webhook_attempts, started_at, completed_at,
			created_at, updated_at
		FROM jobs WHERE status = 'pending' ORDER BY created_at ASC LIMIT ?
	`
	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending jobs: %w", err)
	}
	defer rows.Close()

	var jobs []*models.Job
	for rows.Next() {
		job, err := r.scanJobFromRows(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

func (r *SQLiteJobRepository) ClaimJob(ctx context.Context, id string) (*models.Job, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx,
		"UPDATE jobs SET status = 'running', started_at = ?, updated_at = ? WHERE id = ? AND status = 'pending'",
		time.Now().Format(time.RFC3339),
		time.Now().Format(time.RFC3339),
		id,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to claim job: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to get affected rows: %w", err)
	}
	if affected == 0 {
		return nil, nil
	}

	query := `
		SELECT id, user_id, type, status, url, schema_json, crawl_options_json,
			result_json, error_message, urls_queued, page_count, token_usage_input, token_usage_output,
			cost_usd, webhook_url, webhook_status, webhook_attempts, started_at, completed_at,
			created_at, updated_at
		FROM jobs WHERE id = ?
	`
	job, err := r.scanJob(tx.QueryRowContext(ctx, query, id))
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return job, nil
}

func (r *SQLiteJobRepository) ClaimPending(ctx context.Context) (*models.Job, error) {
	// Begin transaction (SQLite/libsql doesn't support custom isolation levels)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}

	// Ensure transaction is always cleaned up
	committed := false
	defer func() {
		if !committed {
			tx.Rollback()
		}
	}()

	// Use UPDATE ... RETURNING to atomically claim and fetch in one statement
	// This reduces lock contention compared to SELECT then UPDATE
	now := time.Now().Format(time.RFC3339)
	query := `
		UPDATE jobs
		SET status = 'running', started_at = ?, updated_at = ?
		WHERE id = (
			SELECT id FROM jobs
			WHERE status = 'pending'
			ORDER BY created_at ASC
			LIMIT 1
		)
		RETURNING id, user_id, type, status, url, schema_json, crawl_options_json,
			result_json, error_message, urls_queued, page_count, token_usage_input, token_usage_output,
			cost_usd, webhook_url, webhook_status, webhook_attempts, started_at, completed_at,
			created_at, updated_at
	`

	job, err := r.scanJob(tx.QueryRowContext(ctx, query, now, now))
	if err == sql.ErrNoRows || job == nil {
		// No pending jobs - this is normal, not an error
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to claim job: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}
	committed = true

	return job, nil
}

func (r *SQLiteJobRepository) scanJob(row *sql.Row) (*models.Job, error) {
	var job models.Job
	var createdAt, updatedAt string
	var crawlOptionsJSON, resultJSON, errorMessage, webhookURL, webhookStatus sql.NullString
	var startedAt, completedAt sql.NullString

	err := row.Scan(
		&job.ID, &job.UserID, &job.Type, &job.Status, &job.URL, &job.SchemaJSON,
		&crawlOptionsJSON, &resultJSON, &errorMessage, &job.URLsQueued, &job.PageCount,
		&job.TokenUsageInput, &job.TokenUsageOutput, &job.CostUSD,
		&webhookURL, &webhookStatus, &job.WebhookAttempts,
		&startedAt, &completedAt, &createdAt, &updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan job: %w", err)
	}

	job.CrawlOptionsJSON = crawlOptionsJSON.String
	job.ResultJSON = resultJSON.String
	job.ErrorMessage = errorMessage.String
	job.WebhookURL = webhookURL.String
	job.WebhookStatus = webhookStatus.String
	job.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	job.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	if startedAt.Valid {
		t, _ := time.Parse(time.RFC3339, startedAt.String)
		job.StartedAt = &t
	}
	if completedAt.Valid {
		t, _ := time.Parse(time.RFC3339, completedAt.String)
		job.CompletedAt = &t
	}

	return &job, nil
}

func (r *SQLiteJobRepository) scanJobFromRows(rows *sql.Rows) (*models.Job, error) {
	var job models.Job
	var createdAt, updatedAt string
	var crawlOptionsJSON, resultJSON, errorMessage, webhookURL, webhookStatus sql.NullString
	var startedAt, completedAt sql.NullString

	err := rows.Scan(
		&job.ID, &job.UserID, &job.Type, &job.Status, &job.URL, &job.SchemaJSON,
		&crawlOptionsJSON, &resultJSON, &errorMessage, &job.URLsQueued, &job.PageCount,
		&job.TokenUsageInput, &job.TokenUsageOutput, &job.CostUSD,
		&webhookURL, &webhookStatus, &job.WebhookAttempts,
		&startedAt, &completedAt, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan job: %w", err)
	}

	job.CrawlOptionsJSON = crawlOptionsJSON.String
	job.ResultJSON = resultJSON.String
	job.ErrorMessage = errorMessage.String
	job.WebhookURL = webhookURL.String
	job.WebhookStatus = webhookStatus.String
	job.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	job.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	if startedAt.Valid {
		t, _ := time.Parse(time.RFC3339, startedAt.String)
		job.StartedAt = &t
	}
	if completedAt.Valid {
		t, _ := time.Parse(time.RFC3339, completedAt.String)
		job.CompletedAt = &t
	}

	return &job, nil
}

// DeleteOlderThan deletes jobs older than the specified time and returns the deleted job IDs.
func (r *SQLiteJobRepository) DeleteOlderThan(ctx context.Context, before time.Time) ([]string, error) {
	// First, get the IDs of jobs to be deleted
	query := `SELECT id FROM jobs WHERE created_at < ? AND status IN ('completed', 'failed')`
	rows, err := r.db.QueryContext(ctx, query, before.Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("failed to query old jobs: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan job id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating job ids: %w", err)
	}

	if len(ids) == 0 {
		return nil, nil
	}

	// Delete the jobs
	deleteQuery := `DELETE FROM jobs WHERE created_at < ? AND status IN ('completed', 'failed')`
	_, err = r.db.ExecContext(ctx, deleteQuery, before.Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("failed to delete old jobs: %w", err)
	}

	return ids, nil
}

// MarkStaleRunningJobsFailed marks jobs that have been running longer than maxAge as failed.
// This is used to clean up jobs that were left in "running" state due to server restart.
func (r *SQLiteJobRepository) MarkStaleRunningJobsFailed(ctx context.Context, maxAge time.Duration) (int64, error) {
	cutoff := time.Now().Add(-maxAge).Format(time.RFC3339)
	now := time.Now().Format(time.RFC3339)

	query := `
		UPDATE jobs
		SET status = ?, error_message = ?, completed_at = ?, updated_at = ?
		WHERE status = ? AND started_at < ?
	`
	result, err := r.db.ExecContext(ctx, query,
		models.JobStatusFailed,
		"Job terminated: server restart or timeout",
		now,
		now,
		models.JobStatusRunning,
		cutoff,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to mark stale jobs as failed: %w", err)
	}

	count, _ := result.RowsAffected()
	return count, nil
}

// CountActiveByUserID counts jobs that are pending or actively running for a user.
// Running jobs older than StaleJobAge are excluded to avoid blocking users when
// jobs get stuck (e.g., due to backend restarts during development).
func (r *SQLiteJobRepository) CountActiveByUserID(ctx context.Context, userID string) (int, error) {
	// Calculate the stale cutoff time
	staleCutoff := time.Now().Add(-constants.StaleJobAge).Format(time.RFC3339)

	query := `
		SELECT COUNT(*) FROM jobs
		WHERE user_id = ? AND (
			status = ?
			OR (status = ? AND started_at > ?)
		)
	`
	var count int
	err := r.db.QueryRowContext(ctx, query, userID, models.JobStatusPending, models.JobStatusRunning, staleCutoff).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count active jobs: %w", err)
	}
	return count, nil
}

// Helper functions
func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func nullStringPtr(s *string) sql.NullString {
	if s == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
}

func nullTime(t *time.Time) sql.NullString {
	if t == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: t.Format(time.RFC3339), Valid: true}
}

// SQLiteJobResultRepository implements JobResultRepository for SQLite.
type SQLiteJobResultRepository struct {
	db *sql.DB
}

func NewSQLiteJobResultRepository(db *sql.DB) *SQLiteJobResultRepository {
	return &SQLiteJobResultRepository{db: db}
}

func (r *SQLiteJobResultRepository) Create(ctx context.Context, result *models.JobResult) error {
	query := `
		INSERT INTO job_results (id, job_id, url, parent_url, depth, crawl_status,
			data_json, error_message, error_details, error_category,
			llm_provider, llm_model, is_byok, retry_count,
			token_usage_input, token_usage_output,
			fetch_duration_ms, extract_duration_ms, discovered_at, completed_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	isBYOK := 0
	if result.IsBYOK {
		isBYOK = 1
	}
	_, err := r.db.ExecContext(ctx, query,
		result.ID, result.JobID, result.URL, nullStringPtr(result.ParentURL),
		result.Depth, result.CrawlStatus, nullString(result.DataJSON),
		nullString(result.ErrorMessage), nullString(result.ErrorDetails), nullString(result.ErrorCategory),
		nullString(result.LLMProvider), nullString(result.LLMModel), isBYOK, result.RetryCount,
		result.TokenUsageInput, result.TokenUsageOutput,
		result.FetchDurationMs, result.ExtractDurationMs,
		nullTime(result.DiscoveredAt), nullTime(result.CompletedAt),
		result.CreatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("failed to create job result: %w", err)
	}
	return nil
}

func (r *SQLiteJobResultRepository) GetByJobID(ctx context.Context, jobID string) ([]*models.JobResult, error) {
	query := `
		SELECT id, job_id, url, parent_url, depth, crawl_status, data_json, error_message,
			error_details, error_category, llm_provider, llm_model, is_byok, retry_count,
			token_usage_input, token_usage_output, fetch_duration_ms, extract_duration_ms,
			discovered_at, completed_at, created_at
		FROM job_results WHERE job_id = ? ORDER BY created_at ASC
	`
	rows, err := r.db.QueryContext(ctx, query, jobID)
	if err != nil {
		return nil, fmt.Errorf("failed to query job results: %w", err)
	}
	defer rows.Close()

	return r.scanJobResults(rows)
}

// GetCrawlMap returns all results for a job ordered by depth then creation time.
// This is useful for visualizing the crawl structure.
func (r *SQLiteJobResultRepository) GetCrawlMap(ctx context.Context, jobID string) ([]*models.JobResult, error) {
	query := `
		SELECT id, job_id, url, parent_url, depth, crawl_status, data_json, error_message,
			error_details, error_category, llm_provider, llm_model, is_byok, retry_count,
			token_usage_input, token_usage_output, fetch_duration_ms, extract_duration_ms,
			discovered_at, completed_at, created_at
		FROM job_results WHERE job_id = ? ORDER BY depth ASC, created_at ASC
	`
	rows, err := r.db.QueryContext(ctx, query, jobID)
	if err != nil {
		return nil, fmt.Errorf("failed to query crawl map: %w", err)
	}
	defer rows.Close()

	return r.scanJobResults(rows)
}

// scanJobResults is a helper to scan multiple job result rows.
func (r *SQLiteJobResultRepository) scanJobResults(rows *sql.Rows) ([]*models.JobResult, error) {
	var results []*models.JobResult
	for rows.Next() {
		var result models.JobResult
		var parentURL, dataJSON, errorMessage, errorDetails, errorCategory, crawlStatus sql.NullString
		var llmProvider, llmModel sql.NullString
		var isBYOK int
		var discoveredAt, completedAt sql.NullString
		var createdAt string

		err := rows.Scan(
			&result.ID, &result.JobID, &result.URL, &parentURL, &result.Depth, &crawlStatus,
			&dataJSON, &errorMessage, &errorDetails, &errorCategory,
			&llmProvider, &llmModel, &isBYOK, &result.RetryCount,
			&result.TokenUsageInput, &result.TokenUsageOutput,
			&result.FetchDurationMs, &result.ExtractDurationMs,
			&discoveredAt, &completedAt, &createdAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan job result: %w", err)
		}

		if parentURL.Valid {
			result.ParentURL = &parentURL.String
		}
		if crawlStatus.Valid {
			result.CrawlStatus = models.CrawlStatus(crawlStatus.String)
		} else {
			result.CrawlStatus = models.CrawlStatusCompleted
		}
		result.DataJSON = dataJSON.String
		result.ErrorMessage = errorMessage.String
		result.ErrorDetails = errorDetails.String
		result.ErrorCategory = errorCategory.String
		result.LLMProvider = llmProvider.String
		result.LLMModel = llmModel.String
		result.IsBYOK = isBYOK == 1
		result.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		if discoveredAt.Valid {
			t, _ := time.Parse(time.RFC3339, discoveredAt.String)
			result.DiscoveredAt = &t
		}
		if completedAt.Valid {
			t, _ := time.Parse(time.RFC3339, completedAt.String)
			result.CompletedAt = &t
		}
		results = append(results, &result)
	}
	return results, nil
}

// GetAfterID returns results with ID greater than afterID.
// Pass empty string for afterID to get all results.
// This works correctly because IDs are ULIDs which are lexicographically time-ordered.
func (r *SQLiteJobResultRepository) GetAfterID(ctx context.Context, jobID, afterID string) ([]*models.JobResult, error) {
	query := `
		SELECT id, job_id, url, parent_url, depth, crawl_status, data_json, error_message,
			error_details, error_category, llm_provider, llm_model, is_byok, retry_count,
			token_usage_input, token_usage_output, fetch_duration_ms, extract_duration_ms,
			discovered_at, completed_at, created_at
		FROM job_results WHERE job_id = ? AND id > ? ORDER BY id ASC
	`
	rows, err := r.db.QueryContext(ctx, query, jobID, afterID)
	if err != nil {
		return nil, fmt.Errorf("failed to query job results: %w", err)
	}
	defer rows.Close()

	return r.scanJobResults(rows)
}

// DeleteByJobIDs deletes all results for the specified job IDs.
func (r *SQLiteJobResultRepository) DeleteByJobIDs(ctx context.Context, jobIDs []string) error {
	if len(jobIDs) == 0 {
		return nil
	}

	// Build placeholders for IN clause
	placeholders := make([]string, len(jobIDs))
	args := make([]interface{}, len(jobIDs))
	for i, id := range jobIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf("DELETE FROM job_results WHERE job_id IN (%s)",
		strings.Join(placeholders, ","))
	_, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to delete job results: %w", err)
	}
	return nil
}

// CountByJobID returns the total number of results for a job.
func (r *SQLiteJobResultRepository) CountByJobID(ctx context.Context, jobID string) (int, error) {
	query := `SELECT COUNT(*) FROM job_results WHERE job_id = ?`
	var count int
	err := r.db.QueryRowContext(ctx, query, jobID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count job results: %w", err)
	}
	return count, nil
}

// SQLiteAPIKeyRepository implements APIKeyRepository for SQLite.
type SQLiteAPIKeyRepository struct {
	db *sql.DB
}

func NewSQLiteAPIKeyRepository(db *sql.DB) *SQLiteAPIKeyRepository {
	return &SQLiteAPIKeyRepository{db: db}
}

func (r *SQLiteAPIKeyRepository) Create(ctx context.Context, key *models.APIKey) error {
	query := `INSERT INTO api_keys (id, user_id, name, key_hash, key_prefix, scopes, last_used_at, expires_at, created_at, revoked_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	scopesJSON, _ := json.Marshal(key.Scopes)
	_, err := r.db.ExecContext(ctx, query, key.ID, key.UserID, key.Name, key.KeyHash, key.KeyPrefix, string(scopesJSON), nullTime(key.LastUsedAt), nullTime(key.ExpiresAt), key.CreatedAt.Format(time.RFC3339), nullTime(key.RevokedAt))
	return err
}

func (r *SQLiteAPIKeyRepository) GetByID(ctx context.Context, id string) (*models.APIKey, error) {
	query := `SELECT id, user_id, name, key_hash, key_prefix, scopes, last_used_at, expires_at, created_at, revoked_at FROM api_keys WHERE id = ? AND revoked_at IS NULL`
	row := r.db.QueryRowContext(ctx, query, id)
	return r.scanAPIKey(row)
}

func (r *SQLiteAPIKeyRepository) GetByKeyHash(ctx context.Context, hash string) (*models.APIKey, error) {
	query := `SELECT id, user_id, name, key_hash, key_prefix, scopes, last_used_at, expires_at, created_at, revoked_at FROM api_keys WHERE key_hash = ? AND revoked_at IS NULL`
	row := r.db.QueryRowContext(ctx, query, hash)
	return r.scanAPIKey(row)
}

func (r *SQLiteAPIKeyRepository) GetByUserID(ctx context.Context, userID string) ([]*models.APIKey, error) {
	query := `SELECT id, user_id, name, key_hash, key_prefix, scopes, last_used_at, expires_at, created_at, revoked_at FROM api_keys WHERE user_id = ? AND revoked_at IS NULL ORDER BY created_at DESC`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []*models.APIKey
	for rows.Next() {
		key, err := r.scanAPIKeyFromRows(rows)
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, nil
}

func (r *SQLiteAPIKeyRepository) UpdateLastUsed(ctx context.Context, id string, lastUsed time.Time) error {
	_, err := r.db.ExecContext(ctx, "UPDATE api_keys SET last_used_at = ? WHERE id = ?", lastUsed.Format(time.RFC3339), id)
	return err
}

func (r *SQLiteAPIKeyRepository) Revoke(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE api_keys SET revoked_at = ? WHERE id = ?", time.Now().Format(time.RFC3339), id)
	return err
}

func (r *SQLiteAPIKeyRepository) scanAPIKey(row *sql.Row) (*models.APIKey, error) {
	var key models.APIKey
	var scopesJSON, lastUsedAt, expiresAt, revokedAt sql.NullString
	var createdAt string
	err := row.Scan(&key.ID, &key.UserID, &key.Name, &key.KeyHash, &key.KeyPrefix, &scopesJSON, &lastUsedAt, &expiresAt, &createdAt, &revokedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if scopesJSON.Valid {
		json.Unmarshal([]byte(scopesJSON.String), &key.Scopes)
	}
	key.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	if lastUsedAt.Valid {
		t, _ := time.Parse(time.RFC3339, lastUsedAt.String)
		key.LastUsedAt = &t
	}
	if expiresAt.Valid {
		t, _ := time.Parse(time.RFC3339, expiresAt.String)
		key.ExpiresAt = &t
	}
	return &key, nil
}

func (r *SQLiteAPIKeyRepository) scanAPIKeyFromRows(rows *sql.Rows) (*models.APIKey, error) {
	var key models.APIKey
	var scopesJSON, lastUsedAt, expiresAt, revokedAt sql.NullString
	var createdAt string
	if err := rows.Scan(&key.ID, &key.UserID, &key.Name, &key.KeyHash, &key.KeyPrefix, &scopesJSON, &lastUsedAt, &expiresAt, &createdAt, &revokedAt); err != nil {
		return nil, err
	}
	if scopesJSON.Valid {
		json.Unmarshal([]byte(scopesJSON.String), &key.Scopes)
	}
	key.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	if lastUsedAt.Valid {
		t, _ := time.Parse(time.RFC3339, lastUsedAt.String)
		key.LastUsedAt = &t
	}
	if expiresAt.Valid {
		t, _ := time.Parse(time.RFC3339, expiresAt.String)
		key.ExpiresAt = &t
	}
	return &key, nil
}

// SQLiteLLMConfigRepository implements LLMConfigRepository for SQLite.
type SQLiteLLMConfigRepository struct {
	db *sql.DB
}

func NewSQLiteLLMConfigRepository(db *sql.DB) *SQLiteLLMConfigRepository {
	return &SQLiteLLMConfigRepository{db: db}
}

func (r *SQLiteLLMConfigRepository) GetByUserID(ctx context.Context, userID string) (*models.LLMConfig, error) {
	query := `SELECT id, user_id, provider, api_key_encrypted, base_url, model, created_at, updated_at FROM llm_configs WHERE user_id = ?`
	row := r.db.QueryRowContext(ctx, query, userID)
	var cfg models.LLMConfig
	var apiKey, baseURL, model sql.NullString
	var createdAt, updatedAt string
	err := row.Scan(&cfg.ID, &cfg.UserID, &cfg.Provider, &apiKey, &baseURL, &model, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	cfg.APIKeyEncrypted = apiKey.String
	cfg.BaseURL = baseURL.String
	cfg.Model = model.String
	cfg.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	cfg.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return &cfg, nil
}

func (r *SQLiteLLMConfigRepository) Upsert(ctx context.Context, config *models.LLMConfig) error {
	query := `INSERT INTO llm_configs (id, user_id, provider, api_key_encrypted, base_url, model, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET provider = excluded.provider, api_key_encrypted = excluded.api_key_encrypted, base_url = excluded.base_url, model = excluded.model, updated_at = excluded.updated_at`
	_, err := r.db.ExecContext(ctx, query, config.ID, config.UserID, config.Provider, nullString(config.APIKeyEncrypted), nullString(config.BaseURL), nullString(config.Model), config.CreatedAt.Format(time.RFC3339), config.UpdatedAt.Format(time.RFC3339))
	return err
}

// SQLiteUsageRepository implements UsageRepository for SQLite.
// This is the LEAN billing table - detailed telemetry is in usage_insights.
type SQLiteUsageRepository struct {
	db *sql.DB
}

func NewSQLiteUsageRepository(db *sql.DB) *SQLiteUsageRepository {
	return &SQLiteUsageRepository{db: db}
}

func (r *SQLiteUsageRepository) Create(ctx context.Context, record *models.UsageRecord) error {
	query := `INSERT INTO usage_records (id, user_id, job_id, date, type, status, total_charged_usd, is_byok, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
	isBYOK := 0
	if record.IsBYOK {
		isBYOK = 1
	}
	_, err := r.db.ExecContext(ctx, query, record.ID, record.UserID, nullString(record.JobID), record.Date, record.Type, record.Status, record.TotalChargedUSD, isBYOK, record.CreatedAt.Format(time.RFC3339))
	return err
}

func (r *SQLiteUsageRepository) GetByUserID(ctx context.Context, userID string, startDate, endDate string) ([]*models.UsageRecord, error) {
	query := `SELECT id, user_id, job_id, date, type, status, total_charged_usd, is_byok, created_at
		FROM usage_records WHERE user_id = ? AND date >= ? AND date <= ? ORDER BY date DESC`
	rows, err := r.db.QueryContext(ctx, query, userID, startDate, endDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []*models.UsageRecord
	for rows.Next() {
		var record models.UsageRecord
		var jobID sql.NullString
		var isBYOK int
		var createdAt string
		if err := rows.Scan(&record.ID, &record.UserID, &jobID, &record.Date, &record.Type, &record.Status, &record.TotalChargedUSD, &isBYOK, &createdAt); err != nil {
			return nil, err
		}
		record.JobID = jobID.String
		record.IsBYOK = isBYOK == 1
		record.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		records = append(records, &record)
	}
	return records, nil
}

func (r *SQLiteUsageRepository) GetSummary(ctx context.Context, userID string, period string) (*UsageSummary, error) {
	// Calculate date range based on period
	var startDate, endDate string
	now := time.Now()

	switch period {
	case "day":
		startDate = now.Format("2006-01-02")
		endDate = now.AddDate(0, 0, 1).Format("2006-01-02")
	case "month":
		startDate = now.Format("2006-01") + "-01"
		endDate = now.AddDate(0, 1, 0).Format("2006-01") + "-01"
	case "year":
		startDate = now.Format("2006") + "-01-01"
		endDate = now.AddDate(1, 0, 0).Format("2006") + "-01-01"
	default:
		// All time
		startDate = "1970-01-01"
		endDate = "2100-01-01"
	}

	query := `SELECT COUNT(*), COALESCE(SUM(total_charged_usd), 0), COALESCE(SUM(CASE WHEN is_byok = 1 THEN 1 ELSE 0 END), 0)
		FROM usage_records WHERE user_id = ? AND date >= ? AND date < ?`
	var summary UsageSummary
	err := r.db.QueryRowContext(ctx, query, userID, startDate, endDate).Scan(&summary.TotalJobs, &summary.TotalChargedUSD, &summary.BYOKJobs)
	if err != nil {
		return nil, err
	}
	return &summary, nil
}

func (r *SQLiteUsageRepository) GetMonthlySpend(ctx context.Context, userID string, month time.Time) (float64, error) {
	startDate := month.Format("2006-01") + "-01"
	endDate := month.AddDate(0, 1, 0).Format("2006-01") + "-01"

	query := `SELECT COALESCE(SUM(total_charged_usd), 0) FROM usage_records WHERE user_id = ? AND date >= ? AND date < ?`
	var total float64
	err := r.db.QueryRowContext(ctx, query, userID, startDate, endDate).Scan(&total)
	return total, err
}

func (r *SQLiteUsageRepository) CountByUserAndDateRange(ctx context.Context, userID string, startDate, endDate string) (int, error) {
	query := `SELECT COUNT(*) FROM usage_records WHERE user_id = ? AND date >= ? AND date < ?`
	var count int
	err := r.db.QueryRowContext(ctx, query, userID, startDate, endDate).Scan(&count)
	return count, err
}

// SQLiteTelemetryRepository implements TelemetryRepository for SQLite.
type SQLiteTelemetryRepository struct {
	db *sql.DB
}

func NewSQLiteTelemetryRepository(db *sql.DB) *SQLiteTelemetryRepository {
	return &SQLiteTelemetryRepository{db: db}
}

func (r *SQLiteTelemetryRepository) Create(ctx context.Context, event *models.TelemetryEvent) error {
	query := `INSERT INTO telemetry_events (id, user_id, event_type, event_data, created_at) VALUES (?, ?, ?, ?, ?)`
	_, err := r.db.ExecContext(ctx, query, event.ID, nullString(event.UserID), event.EventType, nullString(event.EventData), event.CreatedAt.Format(time.RFC3339))
	return err
}

func (r *SQLiteTelemetryRepository) GetByType(ctx context.Context, eventType string, startTime, endTime time.Time, limit, offset int) ([]*models.TelemetryEvent, error) {
	query := `SELECT id, user_id, event_type, event_data, created_at FROM telemetry_events WHERE event_type = ? AND created_at >= ? AND created_at <= ? ORDER BY created_at DESC LIMIT ? OFFSET ?`
	rows, err := r.db.QueryContext(ctx, query, eventType, startTime.Format(time.RFC3339), endTime.Format(time.RFC3339), limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*models.TelemetryEvent
	for rows.Next() {
		var event models.TelemetryEvent
		var userID, eventData sql.NullString
		var createdAt string
		if err := rows.Scan(&event.ID, &userID, &event.EventType, &eventData, &createdAt); err != nil {
			return nil, err
		}
		event.UserID = userID.String
		event.EventData = eventData.String
		event.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		events = append(events, &event)
	}
	return events, nil
}

func (r *SQLiteTelemetryRepository) CountByTypeAndPeriod(ctx context.Context, eventType, period string) (int, error) {
	query := `SELECT COUNT(*) FROM telemetry_events WHERE event_type = ?`
	var count int
	err := r.db.QueryRowContext(ctx, query, eventType).Scan(&count)
	return count, err
}

func (r *SQLiteTelemetryRepository) CountUniqueUsersByPeriod(ctx context.Context, period string) (int, error) {
	query := `SELECT COUNT(DISTINCT user_id) FROM telemetry_events WHERE user_id IS NOT NULL`
	var count int
	err := r.db.QueryRowContext(ctx, query).Scan(&count)
	return count, err
}

// SQLiteLicenseRepository implements LicenseRepository for SQLite.
type SQLiteLicenseRepository struct {
	db *sql.DB
}

func NewSQLiteLicenseRepository(db *sql.DB) *SQLiteLicenseRepository {
	return &SQLiteLicenseRepository{db: db}
}

func (r *SQLiteLicenseRepository) Create(ctx context.Context, license *models.License) error {
	query := `INSERT INTO licenses (id, license_key, organization_name, email, tier, max_users, features, issued_at, expires_at, revoked_at, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	featuresJSON, _ := json.Marshal(license.Features)
	_, err := r.db.ExecContext(ctx, query, license.ID, license.LicenseKey, license.OrganizationName, license.Email, license.Tier, license.MaxUsers, string(featuresJSON), license.IssuedAt.Format(time.RFC3339), nullTime(license.ExpiresAt), nullTime(license.RevokedAt), license.CreatedAt.Format(time.RFC3339))
	return err
}

func (r *SQLiteLicenseRepository) GetByKey(ctx context.Context, key string) (*models.License, error) {
	query := `SELECT id, license_key, organization_name, email, tier, max_users, features, issued_at, expires_at, revoked_at, created_at FROM licenses WHERE license_key = ?`
	row := r.db.QueryRowContext(ctx, query, key)
	var license models.License
	var featuresJSON sql.NullString
	var expiresAt, revokedAt sql.NullString
	var issuedAt, createdAt string
	err := row.Scan(&license.ID, &license.LicenseKey, &license.OrganizationName, &license.Email, &license.Tier, &license.MaxUsers, &featuresJSON, &issuedAt, &expiresAt, &revokedAt, &createdAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if featuresJSON.Valid {
		json.Unmarshal([]byte(featuresJSON.String), &license.Features)
	}
	license.IssuedAt, _ = time.Parse(time.RFC3339, issuedAt)
	license.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	if expiresAt.Valid {
		t, _ := time.Parse(time.RFC3339, expiresAt.String)
		license.ExpiresAt = &t
	}
	if revokedAt.Valid {
		t, _ := time.Parse(time.RFC3339, revokedAt.String)
		license.RevokedAt = &t
	}
	return &license, nil
}

func (r *SQLiteLicenseRepository) Update(ctx context.Context, license *models.License) error {
	query := `UPDATE licenses SET organization_name = ?, email = ?, tier = ?, max_users = ?, features = ?, expires_at = ?, revoked_at = ? WHERE id = ?`
	featuresJSON, _ := json.Marshal(license.Features)
	_, err := r.db.ExecContext(ctx, query, license.OrganizationName, license.Email, license.Tier, license.MaxUsers, string(featuresJSON), nullTime(license.ExpiresAt), nullTime(license.RevokedAt), license.ID)
	return err
}

func (r *SQLiteLicenseRepository) List(ctx context.Context, limit, offset int) ([]*models.License, error) {
	query := `SELECT id, license_key, organization_name, email, tier, max_users, features, issued_at, expires_at, revoked_at, created_at FROM licenses ORDER BY created_at DESC LIMIT ? OFFSET ?`
	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var licenses []*models.License
	for rows.Next() {
		var license models.License
		var featuresJSON sql.NullString
		var expiresAt, revokedAt sql.NullString
		var issuedAt, createdAt string
		if err := rows.Scan(&license.ID, &license.LicenseKey, &license.OrganizationName, &license.Email, &license.Tier, &license.MaxUsers, &featuresJSON, &issuedAt, &expiresAt, &revokedAt, &createdAt); err != nil {
			return nil, err
		}
		if featuresJSON.Valid {
			json.Unmarshal([]byte(featuresJSON.String), &license.Features)
		}
		license.IssuedAt, _ = time.Parse(time.RFC3339, issuedAt)
		license.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		if expiresAt.Valid {
			t, _ := time.Parse(time.RFC3339, expiresAt.String)
			license.ExpiresAt = &t
		}
		if revokedAt.Valid {
			t, _ := time.Parse(time.RFC3339, revokedAt.String)
			license.RevokedAt = &t
		}
		licenses = append(licenses, &license)
	}
	return licenses, nil
}
