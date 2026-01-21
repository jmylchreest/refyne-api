package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// AnalyticsOverview represents aggregated overview statistics.
type AnalyticsOverview struct {
	TotalJobs         int     `json:"total_jobs"`
	CompletedJobs     int     `json:"completed_jobs"`
	FailedJobs        int     `json:"failed_jobs"`
	TotalCostUSD      float64 `json:"total_cost_usd"`       // Total charged to users
	TotalLLMCostUSD   float64 `json:"total_llm_cost_usd"`   // Total actual LLM cost
	TotalTokensInput  int     `json:"total_tokens_input"`
	TotalTokensOutput int     `json:"total_tokens_output"`
	ActiveUsers       int     `json:"active_users"`
	BYOKJobs          int     `json:"byok_jobs"`
	PlatformJobs      int     `json:"platform_jobs"`
	ErrorRate         float64 `json:"error_rate"`
}

// AnalyticsJob represents a job in the analytics view.
type AnalyticsJob struct {
	ID              string     `json:"id"`
	UserID          string     `json:"user_id"`
	Type            string     `json:"type"`
	Status          string     `json:"status"`
	URL             string     `json:"url"`
	CostUSD         float64    `json:"cost_usd"`       // Cost charged to user (0 for BYOK)
	LLMCostUSD      float64    `json:"llm_cost_usd"`   // Actual LLM provider cost
	TokensInput     int        `json:"tokens_input"`
	TokensOutput    int        `json:"tokens_output"`
	ErrorCategory   string     `json:"error_category,omitempty"`
	ErrorMessage    string     `json:"error_message,omitempty"`
	Provider        string     `json:"provider,omitempty"`
	Model           string     `json:"model,omitempty"`
	DiscoveryMethod string     `json:"discovery_method,omitempty"` // How URLs were discovered: "sitemap", "links", or ""
	IsBYOK          bool       `json:"is_byok"`
	CreatedAt       time.Time  `json:"created_at"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
}

// AnalyticsJobsResult represents a paginated list of analytics jobs.
type AnalyticsJobsResult struct {
	Jobs       []*AnalyticsJob `json:"jobs"`
	TotalCount int             `json:"total_count"`
}

// ErrorCategorySummary represents error statistics grouped by category.
type ErrorCategorySummary struct {
	Category       string   `json:"category"`
	Count          int      `json:"count"`
	Percentage     float64  `json:"percentage"`
	SampleMessages []string `json:"sample_messages"`
}

// ErrorSummary represents comprehensive error statistics.
type ErrorSummary struct {
	ByCategory     []*ErrorCategorySummary `json:"by_category"`
	TopFailingURLs []struct {
		URL   string `json:"url"`
		Count int    `json:"count"`
	} `json:"top_failing_urls"`
	ByProvider []struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
		Count    int    `json:"count"`
	} `json:"by_provider"`
}

// TrendDataPoint represents a single data point in a trend chart.
type TrendDataPoint struct {
	Date       string  `json:"date"`
	JobCount   int     `json:"job_count"`
	CostUSD    float64 `json:"cost_usd"`     // User cost
	LLMCostUSD float64 `json:"llm_cost_usd"` // LLM provider cost
	ErrorCount int     `json:"error_count"`
	Tokens     int     `json:"tokens"`
}

// UserSummary represents per-user analytics.
type UserSummary struct {
	UserID        string     `json:"user_id"`
	TotalJobs     int        `json:"total_jobs"`
	CompletedJobs int        `json:"completed_jobs"`
	FailedJobs    int        `json:"failed_jobs"`
	TotalCostUSD  float64    `json:"total_cost_usd"`
	TotalTokens   int        `json:"total_tokens"`
	LastActive    *time.Time `json:"last_active,omitempty"`
}

// UserSummaryResult represents a paginated list of user summaries.
type UserSummaryResult struct {
	Users      []*UserSummary `json:"users"`
	TotalCount int            `json:"total_count"`
}

// AnalyticsJobsParams represents parameters for querying jobs.
type AnalyticsJobsParams struct {
	StartDate string
	EndDate   string
	Status    string
	Type      string
	UserID    string
	Limit     int
	Offset    int
	Sort      string
	Order     string
}

// AnalyticsUsersParams represents parameters for querying user summaries.
type AnalyticsUsersParams struct {
	StartDate string
	EndDate   string
	Limit     int
	Offset    int
	Sort      string
	Order     string
}

// SQLiteAnalyticsRepository implements analytics queries for SQLite.
type SQLiteAnalyticsRepository struct {
	db *sql.DB
}

// NewSQLiteAnalyticsRepository creates a new analytics repository.
func NewSQLiteAnalyticsRepository(db *sql.DB) *SQLiteAnalyticsRepository {
	return &SQLiteAnalyticsRepository{db: db}
}

// GetOverview returns aggregated analytics overview for the given date range.
func (r *SQLiteAnalyticsRepository) GetOverview(ctx context.Context, startDate, endDate string) (*AnalyticsOverview, error) {
	query := `
		SELECT
			COUNT(*) as total_jobs,
			SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) as completed_jobs,
			SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) as failed_jobs,
			COALESCE(SUM(cost_usd), 0) as total_cost_usd,
			COALESCE(SUM(llm_cost_usd), 0) as total_llm_cost_usd,
			COALESCE(SUM(token_usage_input), 0) as total_tokens_input,
			COALESCE(SUM(token_usage_output), 0) as total_tokens_output,
			COUNT(DISTINCT user_id) as active_users,
			SUM(CASE WHEN is_byok = 1 THEN 1 ELSE 0 END) as byok_jobs,
			SUM(CASE WHEN is_byok = 0 THEN 1 ELSE 0 END) as platform_jobs
		FROM jobs
		WHERE created_at >= ? AND created_at < ?
	`

	var overview AnalyticsOverview
	err := r.db.QueryRowContext(ctx, query, startDate, endDate).Scan(
		&overview.TotalJobs,
		&overview.CompletedJobs,
		&overview.FailedJobs,
		&overview.TotalCostUSD,
		&overview.TotalLLMCostUSD,
		&overview.TotalTokensInput,
		&overview.TotalTokensOutput,
		&overview.ActiveUsers,
		&overview.BYOKJobs,
		&overview.PlatformJobs,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get analytics overview: %w", err)
	}

	if overview.TotalJobs > 0 {
		overview.ErrorRate = float64(overview.FailedJobs) / float64(overview.TotalJobs) * 100
	}

	return &overview, nil
}

// GetJobs returns a paginated list of jobs for the given parameters.
func (r *SQLiteAnalyticsRepository) GetJobs(ctx context.Context, params AnalyticsJobsParams) (*AnalyticsJobsResult, error) {
	// Build WHERE clause
	var conditions []string
	var args []interface{}

	conditions = append(conditions, "created_at >= ? AND created_at < ?")
	args = append(args, params.StartDate, params.EndDate)

	if params.Status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, params.Status)
	}

	if params.Type != "" {
		conditions = append(conditions, "type = ?")
		args = append(args, params.Type)
	}

	if params.UserID != "" {
		conditions = append(conditions, "user_id = ?")
		args = append(args, params.UserID)
	}

	whereClause := strings.Join(conditions, " AND ")

	// Get total count
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM jobs WHERE %s", whereClause)
	var totalCount int
	err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&totalCount)
	if err != nil {
		return nil, fmt.Errorf("failed to count jobs: %w", err)
	}

	// Validate and set sort parameters
	validSortColumns := map[string]bool{
		"created_at": true,
		"cost_usd":   true,
		"status":     true,
		"type":       true,
	}
	sortColumn := "created_at"
	if validSortColumns[params.Sort] {
		sortColumn = params.Sort
	}

	sortOrder := "DESC"
	if strings.ToUpper(params.Order) == "ASC" {
		sortOrder = "ASC"
	}

	// Get paginated jobs
	query := fmt.Sprintf(`
		SELECT id, user_id, type, status, url, cost_usd, llm_cost_usd,
			token_usage_input, token_usage_output,
			COALESCE(error_category, '') as error_category,
			COALESCE(error_message, '') as error_message,
			COALESCE(llm_provider, '') as llm_provider,
			COALESCE(llm_model, '') as llm_model,
			COALESCE(discovery_method, '') as discovery_method,
			is_byok, created_at, completed_at
		FROM jobs
		WHERE %s
		ORDER BY %s %s
		LIMIT ? OFFSET ?
	`, whereClause, sortColumn, sortOrder)

	args = append(args, params.Limit, params.Offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query jobs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var jobs []*AnalyticsJob
	for rows.Next() {
		var job AnalyticsJob
		var isBYOK int
		var createdAt string
		var completedAt sql.NullString

		err := rows.Scan(
			&job.ID, &job.UserID, &job.Type, &job.Status, &job.URL, &job.CostUSD, &job.LLMCostUSD,
			&job.TokensInput, &job.TokensOutput,
			&job.ErrorCategory, &job.ErrorMessage,
			&job.Provider, &job.Model, &job.DiscoveryMethod,
			&isBYOK, &createdAt, &completedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan job: %w", err)
		}

		job.IsBYOK = isBYOK == 1
		job.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		if completedAt.Valid {
			t, _ := time.Parse(time.RFC3339, completedAt.String)
			job.CompletedAt = &t
		}

		jobs = append(jobs, &job)
	}

	return &AnalyticsJobsResult{
		Jobs:       jobs,
		TotalCount: totalCount,
	}, nil
}

// GetErrors returns error summary statistics for the given date range.
func (r *SQLiteAnalyticsRepository) GetErrors(ctx context.Context, startDate, endDate string) (*ErrorSummary, error) {
	summary := &ErrorSummary{
		ByCategory:     make([]*ErrorCategorySummary, 0),
		TopFailingURLs: make([]struct {
			URL   string `json:"url"`
			Count int    `json:"count"`
		}, 0),
		ByProvider: make([]struct {
			Provider string `json:"provider"`
			Model    string `json:"model"`
			Count    int    `json:"count"`
		}, 0),
	}

	// Get total failed count for percentage calculation
	var totalFailed int
	err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM jobs WHERE status = 'failed' AND created_at >= ? AND created_at < ?",
		startDate, endDate).Scan(&totalFailed)
	if err != nil {
		return nil, fmt.Errorf("failed to count failed jobs: %w", err)
	}

	if totalFailed == 0 {
		return summary, nil
	}

	// Get errors by category from job_results table (which has error_category)
	categoryQuery := `
		SELECT
			COALESCE(jr.error_category, 'unknown') as category,
			COUNT(*) as count,
			GROUP_CONCAT(DISTINCT SUBSTR(jr.error_message, 1, 100)) as sample_messages
		FROM job_results jr
		INNER JOIN jobs j ON jr.job_id = j.id
		WHERE j.status = 'failed'
			AND j.created_at >= ? AND j.created_at < ?
			AND jr.error_message != ''
		GROUP BY COALESCE(jr.error_category, 'unknown')
		ORDER BY count DESC
		LIMIT 10
	`

	categoryRows, err := r.db.QueryContext(ctx, categoryQuery, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to query error categories: %w", err)
	}
	defer func() { _ = categoryRows.Close() }()

	for categoryRows.Next() {
		var cat ErrorCategorySummary
		var sampleMessages sql.NullString
		err := categoryRows.Scan(&cat.Category, &cat.Count, &sampleMessages)
		if err != nil {
			return nil, fmt.Errorf("failed to scan error category: %w", err)
		}
		cat.Percentage = float64(cat.Count) / float64(totalFailed) * 100
		if sampleMessages.Valid && sampleMessages.String != "" {
			// Split by comma and take first 3
			msgs := strings.Split(sampleMessages.String, ",")
			if len(msgs) > 3 {
				msgs = msgs[:3]
			}
			cat.SampleMessages = msgs
		}
		summary.ByCategory = append(summary.ByCategory, &cat)
	}

	// Get top failing URLs
	urlQuery := `
		SELECT url, COUNT(*) as count
		FROM jobs
		WHERE status = 'failed' AND created_at >= ? AND created_at < ?
		GROUP BY url
		ORDER BY count DESC
		LIMIT 10
	`

	urlRows, err := r.db.QueryContext(ctx, urlQuery, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to query failing URLs: %w", err)
	}
	defer func() { _ = urlRows.Close() }()

	for urlRows.Next() {
		var urlStat struct {
			URL   string `json:"url"`
			Count int    `json:"count"`
		}
		err := urlRows.Scan(&urlStat.URL, &urlStat.Count)
		if err != nil {
			return nil, fmt.Errorf("failed to scan URL stat: %w", err)
		}
		summary.TopFailingURLs = append(summary.TopFailingURLs, urlStat)
	}

	// Get errors by provider from job_results
	providerQuery := `
		SELECT
			COALESCE(jr.llm_provider, 'unknown') as provider,
			COALESCE(jr.llm_model, 'unknown') as model,
			COUNT(*) as count
		FROM job_results jr
		INNER JOIN jobs j ON jr.job_id = j.id
		WHERE j.status = 'failed'
			AND j.created_at >= ? AND j.created_at < ?
			AND jr.error_message != ''
		GROUP BY COALESCE(jr.llm_provider, 'unknown'), COALESCE(jr.llm_model, 'unknown')
		ORDER BY count DESC
		LIMIT 10
	`

	providerRows, err := r.db.QueryContext(ctx, providerQuery, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to query provider errors: %w", err)
	}
	defer func() { _ = providerRows.Close() }()

	for providerRows.Next() {
		var providerStat struct {
			Provider string `json:"provider"`
			Model    string `json:"model"`
			Count    int    `json:"count"`
		}
		err := providerRows.Scan(&providerStat.Provider, &providerStat.Model, &providerStat.Count)
		if err != nil {
			return nil, fmt.Errorf("failed to scan provider stat: %w", err)
		}
		summary.ByProvider = append(summary.ByProvider, providerStat)
	}

	return summary, nil
}

// GetTrends returns trend data for the given date range and interval.
func (r *SQLiteAnalyticsRepository) GetTrends(ctx context.Context, startDate, endDate, interval string) ([]*TrendDataPoint, error) {
	// Determine date format based on interval
	var dateFormat string
	switch interval {
	case "week":
		// SQLite doesn't have native week functions, use strftime
		dateFormat = "strftime('%Y-W%W', created_at)"
	case "month":
		dateFormat = "strftime('%Y-%m', created_at)"
	default: // day
		dateFormat = "date(created_at)"
	}

	query := fmt.Sprintf(`
		SELECT
			%s as date_bucket,
			COUNT(*) as job_count,
			COALESCE(SUM(cost_usd), 0) as cost_usd,
			COALESCE(SUM(llm_cost_usd), 0) as llm_cost_usd,
			SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) as error_count,
			COALESCE(SUM(token_usage_input + token_usage_output), 0) as tokens
		FROM jobs
		WHERE created_at >= ? AND created_at < ?
		GROUP BY date_bucket
		ORDER BY date_bucket ASC
	`, dateFormat)

	rows, err := r.db.QueryContext(ctx, query, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to query trends: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var trends []*TrendDataPoint
	for rows.Next() {
		var point TrendDataPoint
		err := rows.Scan(&point.Date, &point.JobCount, &point.CostUSD, &point.LLMCostUSD, &point.ErrorCount, &point.Tokens)
		if err != nil {
			return nil, fmt.Errorf("failed to scan trend point: %w", err)
		}
		trends = append(trends, &point)
	}

	return trends, nil
}

// GetUsers returns a paginated list of user summaries for the given parameters.
func (r *SQLiteAnalyticsRepository) GetUsers(ctx context.Context, params AnalyticsUsersParams) (*UserSummaryResult, error) {
	// Get total count
	countQuery := `
		SELECT COUNT(DISTINCT user_id)
		FROM jobs
		WHERE created_at >= ? AND created_at < ?
	`
	var totalCount int
	err := r.db.QueryRowContext(ctx, countQuery, params.StartDate, params.EndDate).Scan(&totalCount)
	if err != nil {
		return nil, fmt.Errorf("failed to count users: %w", err)
	}

	// Validate and set sort parameters
	validSortColumns := map[string]string{
		"total_cost":   "total_cost_usd",
		"total_jobs":   "total_jobs",
		"last_active":  "last_active",
		"failed_jobs":  "failed_jobs",
		"total_tokens": "total_tokens",
	}
	sortColumn := "total_cost_usd"
	if col, ok := validSortColumns[params.Sort]; ok {
		sortColumn = col
	}

	sortOrder := "DESC"
	if strings.ToUpper(params.Order) == "ASC" {
		sortOrder = "ASC"
	}

	query := fmt.Sprintf(`
		SELECT
			user_id,
			COUNT(*) as total_jobs,
			SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) as completed_jobs,
			SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) as failed_jobs,
			COALESCE(SUM(cost_usd), 0) as total_cost_usd,
			COALESCE(SUM(token_usage_input + token_usage_output), 0) as total_tokens,
			MAX(created_at) as last_active
		FROM jobs
		WHERE created_at >= ? AND created_at < ?
		GROUP BY user_id
		ORDER BY %s %s
		LIMIT ? OFFSET ?
	`, sortColumn, sortOrder)

	rows, err := r.db.QueryContext(ctx, query, params.StartDate, params.EndDate, params.Limit, params.Offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query users: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var users []*UserSummary
	for rows.Next() {
		var user UserSummary
		var lastActive sql.NullString

		err := rows.Scan(
			&user.UserID,
			&user.TotalJobs,
			&user.CompletedJobs,
			&user.FailedJobs,
			&user.TotalCostUSD,
			&user.TotalTokens,
			&lastActive,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user: %w", err)
		}

		if lastActive.Valid {
			t, _ := time.Parse(time.RFC3339, lastActive.String)
			user.LastActive = &t
		}

		users = append(users, &user)
	}

	return &UserSummaryResult{
		Users:      users,
		TotalCount: totalCount,
	}, nil
}
