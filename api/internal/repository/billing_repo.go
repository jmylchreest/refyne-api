package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmylchreest/refyne-api/internal/models"
)

// ========================================
// Balance Repository
// ========================================

// SQLiteBalanceRepository implements BalanceRepository for SQLite.
type SQLiteBalanceRepository struct {
	db *sql.DB
}

// NewSQLiteBalanceRepository creates a new SQLite balance repository.
func NewSQLiteBalanceRepository(db *sql.DB) *SQLiteBalanceRepository {
	return &SQLiteBalanceRepository{db: db}
}

func (r *SQLiteBalanceRepository) Get(ctx context.Context, userID string) (*models.UserBalance, error) {
	query := `SELECT user_id, balance_usd, lifetime_added, lifetime_spent, period_start, period_end, updated_at FROM user_balances WHERE user_id = ?`
	var balance models.UserBalance
	var periodStart, periodEnd sql.NullString
	var updatedAt string
	err := r.db.QueryRowContext(ctx, query, userID).Scan(&balance.UserID, &balance.BalanceUSD, &balance.LifetimeAdded, &balance.LifetimeSpent, &periodStart, &periodEnd, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if periodStart.Valid {
		t, _ := time.Parse(time.RFC3339, periodStart.String)
		balance.PeriodStart = &t
	}
	if periodEnd.Valid {
		t, _ := time.Parse(time.RFC3339, periodEnd.String)
		balance.PeriodEnd = &t
	}
	balance.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return &balance, nil
}

func (r *SQLiteBalanceRepository) Upsert(ctx context.Context, balance *models.UserBalance) error {
	query := `INSERT INTO user_balances (user_id, balance_usd, lifetime_added, lifetime_spent, period_start, period_end, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			balance_usd = excluded.balance_usd,
			lifetime_added = excluded.lifetime_added,
			lifetime_spent = excluded.lifetime_spent,
			period_start = COALESCE(excluded.period_start, user_balances.period_start),
			period_end = COALESCE(excluded.period_end, user_balances.period_end),
			updated_at = excluded.updated_at`
	var periodStart, periodEnd *string
	if balance.PeriodStart != nil {
		s := balance.PeriodStart.Format(time.RFC3339)
		periodStart = &s
	}
	if balance.PeriodEnd != nil {
		s := balance.PeriodEnd.Format(time.RFC3339)
		periodEnd = &s
	}
	_, err := r.db.ExecContext(ctx, query, balance.UserID, balance.BalanceUSD, balance.LifetimeAdded, balance.LifetimeSpent, periodStart, periodEnd, balance.UpdatedAt.Format(time.RFC3339))
	return err
}

// UpdateSubscriptionPeriod updates only the subscription period dates for a user.
// This is called when we receive Clerk subscription webhooks.
func (r *SQLiteBalanceRepository) UpdateSubscriptionPeriod(ctx context.Context, userID string, periodStart, periodEnd time.Time) error {
	// First try to update existing record
	query := `UPDATE user_balances SET period_start = ?, period_end = ?, updated_at = ? WHERE user_id = ?`
	result, err := r.db.ExecContext(ctx, query, periodStart.Format(time.RFC3339), periodEnd.Format(time.RFC3339), time.Now().Format(time.RFC3339), userID)
	if err != nil {
		return err
	}

	// If no rows were updated, create a new record
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		insertQuery := `INSERT INTO user_balances (user_id, balance_usd, lifetime_added, lifetime_spent, period_start, period_end, updated_at)
			VALUES (?, 0, 0, 0, ?, ?, ?)`
		_, err = r.db.ExecContext(ctx, insertQuery, userID, periodStart.Format(time.RFC3339), periodEnd.Format(time.RFC3339), time.Now().Format(time.RFC3339))
	}
	return err
}

func (r *SQLiteBalanceRepository) GetAvailableBalance(ctx context.Context, userID string, now time.Time) (float64, error) {
	// Sum all non-expired credit transactions
	query := `SELECT COALESCE(SUM(amount_usd), 0) FROM credit_transactions
		WHERE user_id = ? AND is_expired = 0 AND (expires_at IS NULL OR expires_at > ?)`
	var balance float64
	err := r.db.QueryRowContext(ctx, query, userID, now.Format(time.RFC3339)).Scan(&balance)
	return balance, err
}

// ========================================
// Credit Transaction Repository
// ========================================

// SQLiteCreditTransactionRepository implements CreditTransactionRepository for SQLite.
type SQLiteCreditTransactionRepository struct {
	db *sql.DB
}

// NewSQLiteCreditTransactionRepository creates a new SQLite credit transaction repository.
func NewSQLiteCreditTransactionRepository(db *sql.DB) *SQLiteCreditTransactionRepository {
	return &SQLiteCreditTransactionRepository{db: db}
}

func (r *SQLiteCreditTransactionRepository) Create(ctx context.Context, tx *models.CreditTransaction) error {
	query := `INSERT INTO credit_transactions (id, user_id, type, amount_usd, balance_after, expires_at, is_expired, stripe_payment_id, job_id, description, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	var expiresAt, stripePaymentID, jobID *string
	if tx.ExpiresAt != nil {
		s := tx.ExpiresAt.Format(time.RFC3339)
		expiresAt = &s
	}
	if tx.StripePaymentID != nil {
		stripePaymentID = tx.StripePaymentID
	}
	if tx.JobID != nil {
		jobID = tx.JobID
	}

	_, err := r.db.ExecContext(ctx, query, tx.ID, tx.UserID, tx.Type, tx.AmountUSD, tx.BalanceAfter, expiresAt, tx.IsExpired, stripePaymentID, jobID, tx.Description, tx.CreatedAt.Format(time.RFC3339))
	return err
}

func (r *SQLiteCreditTransactionRepository) GetByUserID(ctx context.Context, userID string, limit, offset int) ([]*models.CreditTransaction, error) {
	query := `SELECT id, user_id, type, amount_usd, balance_after, expires_at, is_expired, stripe_payment_id, job_id, description, created_at
		FROM credit_transactions WHERE user_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?`

	rows, err := r.db.QueryContext(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var transactions []*models.CreditTransaction
	for rows.Next() {
		var tx models.CreditTransaction
		var expiresAt, stripePaymentID, jobID sql.NullString
		var createdAt string

		if err := rows.Scan(&tx.ID, &tx.UserID, &tx.Type, &tx.AmountUSD, &tx.BalanceAfter, &expiresAt, &tx.IsExpired, &stripePaymentID, &jobID, &tx.Description, &createdAt); err != nil {
			return nil, err
		}

		if expiresAt.Valid {
			t, _ := time.Parse(time.RFC3339, expiresAt.String)
			tx.ExpiresAt = &t
		}
		if stripePaymentID.Valid {
			tx.StripePaymentID = &stripePaymentID.String
		}
		if jobID.Valid {
			tx.JobID = &jobID.String
		}
		tx.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)

		transactions = append(transactions, &tx)
	}

	return transactions, rows.Err()
}

func (r *SQLiteCreditTransactionRepository) GetByStripePaymentID(ctx context.Context, stripePaymentID string) (*models.CreditTransaction, error) {
	query := `SELECT id, user_id, type, amount_usd, balance_after, expires_at, is_expired, stripe_payment_id, job_id, description, created_at
		FROM credit_transactions WHERE stripe_payment_id = ?`

	var tx models.CreditTransaction
	var expiresAt, spID, jobID sql.NullString
	var createdAt string

	err := r.db.QueryRowContext(ctx, query, stripePaymentID).Scan(&tx.ID, &tx.UserID, &tx.Type, &tx.AmountUSD, &tx.BalanceAfter, &expiresAt, &tx.IsExpired, &spID, &jobID, &tx.Description, &createdAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if expiresAt.Valid {
		t, _ := time.Parse(time.RFC3339, expiresAt.String)
		tx.ExpiresAt = &t
	}
	if spID.Valid {
		tx.StripePaymentID = &spID.String
	}
	if jobID.Valid {
		tx.JobID = &jobID.String
	}
	tx.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)

	return &tx, nil
}

func (r *SQLiteCreditTransactionRepository) GetNonExpiredSubscriptionCredits(ctx context.Context) ([]*models.CreditTransaction, error) {
	query := `SELECT id, user_id, type, amount_usd, balance_after, expires_at, is_expired, stripe_payment_id, job_id, description, created_at
		FROM credit_transactions WHERE type = 'subscription' AND is_expired = 0`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var transactions []*models.CreditTransaction
	for rows.Next() {
		var tx models.CreditTransaction
		var expiresAt, stripePaymentID, jobID sql.NullString
		var createdAt string

		if err := rows.Scan(&tx.ID, &tx.UserID, &tx.Type, &tx.AmountUSD, &tx.BalanceAfter, &expiresAt, &tx.IsExpired, &stripePaymentID, &jobID, &tx.Description, &createdAt); err != nil {
			return nil, err
		}

		if expiresAt.Valid {
			t, _ := time.Parse(time.RFC3339, expiresAt.String)
			tx.ExpiresAt = &t
		}
		if stripePaymentID.Valid {
			tx.StripePaymentID = &stripePaymentID.String
		}
		if jobID.Valid {
			tx.JobID = &jobID.String
		}
		tx.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)

		transactions = append(transactions, &tx)
	}

	return transactions, rows.Err()
}

func (r *SQLiteCreditTransactionRepository) UpdateExpiry(ctx context.Context, id string, expiresAt *time.Time) error {
	var expStr *string
	if expiresAt != nil {
		s := expiresAt.Format(time.RFC3339)
		expStr = &s
	}
	query := `UPDATE credit_transactions SET expires_at = ? WHERE id = ?`
	_, err := r.db.ExecContext(ctx, query, expStr, id)
	return err
}

func (r *SQLiteCreditTransactionRepository) MarkExpired(ctx context.Context, now time.Time) (int64, error) {
	query := `UPDATE credit_transactions SET is_expired = 1 WHERE expires_at < ? AND is_expired = 0`
	result, err := r.db.ExecContext(ctx, query, now.Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *SQLiteCreditTransactionRepository) GetAvailableBalance(ctx context.Context, userID string, now time.Time) (float64, error) {
	query := `SELECT COALESCE(SUM(amount_usd), 0) FROM credit_transactions
		WHERE user_id = ? AND is_expired = 0 AND (expires_at IS NULL OR expires_at > ?)`
	var balance float64
	err := r.db.QueryRowContext(ctx, query, userID, now.Format(time.RFC3339)).Scan(&balance)
	return balance, err
}

// ========================================
// Schema Snapshot Repository
// ========================================

// SQLiteSchemaSnapshotRepository implements SchemaSnapshotRepository for SQLite.
type SQLiteSchemaSnapshotRepository struct {
	db *sql.DB
}

// NewSQLiteSchemaSnapshotRepository creates a new SQLite schema snapshot repository.
func NewSQLiteSchemaSnapshotRepository(db *sql.DB) *SQLiteSchemaSnapshotRepository {
	return &SQLiteSchemaSnapshotRepository{db: db}
}

func (r *SQLiteSchemaSnapshotRepository) Create(ctx context.Context, snapshot *models.SchemaSnapshot) error {
	query := `INSERT INTO schema_snapshots (id, user_id, hash, schema_json, name, version, usage_count, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := r.db.ExecContext(ctx, query, snapshot.ID, snapshot.UserID, snapshot.Hash, snapshot.SchemaJSON, snapshot.Name, snapshot.Version, snapshot.UsageCount, snapshot.CreatedAt.Format(time.RFC3339))
	return err
}

func (r *SQLiteSchemaSnapshotRepository) GetByID(ctx context.Context, id string) (*models.SchemaSnapshot, error) {
	query := `SELECT id, user_id, hash, schema_json, name, version, usage_count, created_at FROM schema_snapshots WHERE id = ?`
	var snapshot models.SchemaSnapshot
	var name sql.NullString
	var createdAt string

	err := r.db.QueryRowContext(ctx, query, id).Scan(&snapshot.ID, &snapshot.UserID, &snapshot.Hash, &snapshot.SchemaJSON, &name, &snapshot.Version, &snapshot.UsageCount, &createdAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if name.Valid {
		snapshot.Name = name.String
	}
	snapshot.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)

	return &snapshot, nil
}

func (r *SQLiteSchemaSnapshotRepository) GetByUserAndHash(ctx context.Context, userID, hash string) (*models.SchemaSnapshot, error) {
	query := `SELECT id, user_id, hash, schema_json, name, version, usage_count, created_at FROM schema_snapshots WHERE user_id = ? AND hash = ?`
	var snapshot models.SchemaSnapshot
	var name sql.NullString
	var createdAt string

	err := r.db.QueryRowContext(ctx, query, userID, hash).Scan(&snapshot.ID, &snapshot.UserID, &snapshot.Hash, &snapshot.SchemaJSON, &name, &snapshot.Version, &snapshot.UsageCount, &createdAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if name.Valid {
		snapshot.Name = name.String
	}
	snapshot.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)

	return &snapshot, nil
}

func (r *SQLiteSchemaSnapshotRepository) GetByUserID(ctx context.Context, userID string, limit, offset int) ([]*models.SchemaSnapshot, error) {
	query := `SELECT id, user_id, hash, schema_json, name, version, usage_count, created_at
		FROM schema_snapshots WHERE user_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?`

	rows, err := r.db.QueryContext(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var snapshots []*models.SchemaSnapshot
	for rows.Next() {
		var snapshot models.SchemaSnapshot
		var name sql.NullString
		var createdAt string

		if err := rows.Scan(&snapshot.ID, &snapshot.UserID, &snapshot.Hash, &snapshot.SchemaJSON, &name, &snapshot.Version, &snapshot.UsageCount, &createdAt); err != nil {
			return nil, err
		}

		if name.Valid {
			snapshot.Name = name.String
		}
		snapshot.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)

		snapshots = append(snapshots, &snapshot)
	}

	return snapshots, rows.Err()
}

func (r *SQLiteSchemaSnapshotRepository) GetNextVersion(ctx context.Context, userID string) (int, error) {
	query := `SELECT COALESCE(MAX(version), 0) + 1 FROM schema_snapshots WHERE user_id = ?`
	var version int
	err := r.db.QueryRowContext(ctx, query, userID).Scan(&version)
	return version, err
}

func (r *SQLiteSchemaSnapshotRepository) IncrementUsageCount(ctx context.Context, id string) error {
	query := `UPDATE schema_snapshots SET usage_count = usage_count + 1 WHERE id = ?`
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}

// ========================================
// Usage Insight Repository
// ========================================

// SQLiteUsageInsightRepository implements UsageInsightRepository for SQLite.
type SQLiteUsageInsightRepository struct {
	db *sql.DB
}

// NewSQLiteUsageInsightRepository creates a new SQLite usage insight repository.
func NewSQLiteUsageInsightRepository(db *sql.DB) *SQLiteUsageInsightRepository {
	return &SQLiteUsageInsightRepository{db: db}
}

func (r *SQLiteUsageInsightRepository) Create(ctx context.Context, insight *models.UsageInsight) error {
	query := `INSERT INTO usage_insights (id, usage_id, target_url, schema_id, crawl_config_json, error_message, error_code,
		tokens_input, tokens_output, llm_cost_usd, markup_rate, markup_usd, llm_provider, llm_model, generation_id, byok_provider,
		pages_attempted, pages_successful, fetch_duration_ms, extract_duration_ms, total_duration_ms,
		request_id, user_agent, ip_country, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := r.db.ExecContext(ctx, query,
		insight.ID, insight.UsageID, insight.TargetURL, nullString(insight.SchemaID), nullString(insight.CrawlConfigJSON),
		nullString(insight.ErrorMessage), nullString(insight.ErrorCode),
		insight.TokensInput, insight.TokensOutput, insight.LLMCostUSD, insight.MarkupRate, insight.MarkupUSD,
		nullString(insight.LLMProvider), nullString(insight.LLMModel), nullString(insight.GenerationID), nullString(insight.BYOKProvider),
		insight.PagesAttempted, insight.PagesSuccessful, insight.FetchDurationMs, insight.ExtractDurationMs, insight.TotalDurationMs,
		nullString(insight.RequestID), nullString(insight.UserAgent), nullString(insight.IPCountry),
		insight.CreatedAt.Format(time.RFC3339))
	return err
}

func (r *SQLiteUsageInsightRepository) GetByUsageID(ctx context.Context, usageID string) (*models.UsageInsight, error) {
	query := `SELECT id, usage_id, target_url, schema_id, crawl_config_json, error_message, error_code,
		tokens_input, tokens_output, llm_cost_usd, markup_rate, markup_usd, llm_provider, llm_model, generation_id, byok_provider,
		pages_attempted, pages_successful, fetch_duration_ms, extract_duration_ms, total_duration_ms,
		request_id, user_agent, ip_country, created_at
		FROM usage_insights WHERE usage_id = ?`

	var insight models.UsageInsight
	var schemaID, crawlConfig, errorMsg, errorCode, provider, model, genID, byokProvider, reqID, userAgent, ipCountry sql.NullString
	var createdAt string

	err := r.db.QueryRowContext(ctx, query, usageID).Scan(
		&insight.ID, &insight.UsageID, &insight.TargetURL, &schemaID, &crawlConfig, &errorMsg, &errorCode,
		&insight.TokensInput, &insight.TokensOutput, &insight.LLMCostUSD, &insight.MarkupRate, &insight.MarkupUSD,
		&provider, &model, &genID, &byokProvider,
		&insight.PagesAttempted, &insight.PagesSuccessful, &insight.FetchDurationMs, &insight.ExtractDurationMs, &insight.TotalDurationMs,
		&reqID, &userAgent, &ipCountry, &createdAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	insight.SchemaID = schemaID.String
	insight.CrawlConfigJSON = crawlConfig.String
	insight.ErrorMessage = errorMsg.String
	insight.ErrorCode = errorCode.String
	insight.LLMProvider = provider.String
	insight.LLMModel = model.String
	insight.GenerationID = genID.String
	insight.BYOKProvider = byokProvider.String
	insight.RequestID = reqID.String
	insight.UserAgent = userAgent.String
	insight.IPCountry = ipCountry.String
	insight.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)

	return &insight, nil
}

func (r *SQLiteUsageInsightRepository) GetByUserID(ctx context.Context, userID string, limit, offset int) ([]*models.UsageInsight, error) {
	query := `SELECT i.id, i.usage_id, i.target_url, i.schema_id, i.crawl_config_json, i.error_message, i.error_code,
		i.tokens_input, i.tokens_output, i.llm_cost_usd, i.markup_rate, i.markup_usd, i.llm_provider, i.llm_model, i.generation_id, i.byok_provider,
		i.pages_attempted, i.pages_successful, i.fetch_duration_ms, i.extract_duration_ms, i.total_duration_ms,
		i.request_id, i.user_agent, i.ip_country, i.created_at
		FROM usage_insights i
		JOIN usage_records u ON i.usage_id = u.id
		WHERE u.user_id = ?
		ORDER BY i.created_at DESC LIMIT ? OFFSET ?`

	rows, err := r.db.QueryContext(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var insights []*models.UsageInsight
	for rows.Next() {
		var insight models.UsageInsight
		var schemaID, crawlConfig, errorMsg, errorCode, provider, model, genID, byokProvider, reqID, userAgent, ipCountry sql.NullString
		var createdAt string

		if err := rows.Scan(
			&insight.ID, &insight.UsageID, &insight.TargetURL, &schemaID, &crawlConfig, &errorMsg, &errorCode,
			&insight.TokensInput, &insight.TokensOutput, &insight.LLMCostUSD, &insight.MarkupRate, &insight.MarkupUSD,
			&provider, &model, &genID, &byokProvider,
			&insight.PagesAttempted, &insight.PagesSuccessful, &insight.FetchDurationMs, &insight.ExtractDurationMs, &insight.TotalDurationMs,
			&reqID, &userAgent, &ipCountry, &createdAt); err != nil {
			return nil, err
		}

		insight.SchemaID = schemaID.String
		insight.CrawlConfigJSON = crawlConfig.String
		insight.ErrorMessage = errorMsg.String
		insight.ErrorCode = errorCode.String
		insight.LLMProvider = provider.String
		insight.LLMModel = model.String
		insight.GenerationID = genID.String
		insight.BYOKProvider = byokProvider.String
		insight.RequestID = reqID.String
		insight.UserAgent = userAgent.String
		insight.IPCountry = ipCountry.String
		insight.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)

		insights = append(insights, &insight)
	}

	return insights, rows.Err()
}
