package handlers

import (
	"context"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/jmylchreest/refyne-api/internal/http/mw"
	"github.com/jmylchreest/refyne-api/internal/repository"
	"github.com/jmylchreest/refyne-api/internal/service"
)

// AdminAnalyticsHandler handles admin analytics endpoints.
type AdminAnalyticsHandler struct {
	analyticsRepo *repository.SQLiteAnalyticsRepository
	storageSvc    *service.StorageService
}

// NewAdminAnalyticsHandler creates a new admin analytics handler.
func NewAdminAnalyticsHandler(analyticsRepo *repository.SQLiteAnalyticsRepository, storageSvc *service.StorageService) *AdminAnalyticsHandler {
	return &AdminAnalyticsHandler{
		analyticsRepo: analyticsRepo,
		storageSvc:    storageSvc,
	}
}

// --- Overview Stats ---

// OverviewInput represents the overview request parameters.
type OverviewInput struct {
	StartDate string `query:"start_date" doc:"Start date (YYYY-MM-DD)"`
	EndDate   string `query:"end_date" doc:"End date (YYYY-MM-DD)"`
}

// OverviewResponse represents the overview statistics.
type OverviewResponse struct {
	TotalJobs         int     `json:"total_jobs" doc:"Total number of jobs"`
	CompletedJobs     int     `json:"completed_jobs" doc:"Number of completed jobs"`
	FailedJobs        int     `json:"failed_jobs" doc:"Number of failed jobs"`
	TotalCostUSD      float64 `json:"total_cost_usd" doc:"Total cost in USD"`
	TotalTokensInput  int     `json:"total_tokens_input" doc:"Total input tokens"`
	TotalTokensOutput int     `json:"total_tokens_output" doc:"Total output tokens"`
	ActiveUsers       int     `json:"active_users" doc:"Number of unique users with jobs"`
	BYOKJobs          int     `json:"byok_jobs" doc:"Jobs using user API keys"`
	PlatformJobs      int     `json:"platform_jobs" doc:"Jobs using platform API keys"`
	ErrorRate         float64 `json:"error_rate" doc:"Error rate percentage"`
}

// GetOverviewOutput represents the overview response.
type GetOverviewOutput struct {
	Body OverviewResponse
}

// GetOverview returns overview analytics for the given date range.
func (h *AdminAnalyticsHandler) GetOverview(ctx context.Context, input *OverviewInput) (*GetOverviewOutput, error) {
	claims := mw.GetUserClaims(ctx)
	if claims == nil || !claims.GlobalSuperadmin {
		return nil, huma.Error403Forbidden("superadmin access required")
	}

	startDate, endDate := parseDateRange(input.StartDate, input.EndDate)

	overview, err := h.analyticsRepo.GetOverview(ctx, startDate, endDate)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get overview: " + err.Error())
	}

	return &GetOverviewOutput{
		Body: OverviewResponse{
			TotalJobs:         overview.TotalJobs,
			CompletedJobs:     overview.CompletedJobs,
			FailedJobs:        overview.FailedJobs,
			TotalCostUSD:      overview.TotalCostUSD,
			TotalTokensInput:  overview.TotalTokensInput,
			TotalTokensOutput: overview.TotalTokensOutput,
			ActiveUsers:       overview.ActiveUsers,
			BYOKJobs:          overview.BYOKJobs,
			PlatformJobs:      overview.PlatformJobs,
			ErrorRate:         overview.ErrorRate,
		},
	}, nil
}

// --- Jobs List ---

// AnalyticsJobsInput represents the jobs list request parameters.
type AnalyticsJobsInput struct {
	StartDate string `query:"start_date" doc:"Start date (YYYY-MM-DD)"`
	EndDate   string `query:"end_date" doc:"End date (YYYY-MM-DD)"`
	Status    string `query:"status" enum:",pending,running,completed,failed,cancelled" doc:"Filter by status"`
	Type      string `query:"type" enum:",extract,crawl,analyze" doc:"Filter by job type"`
	UserID    string `query:"user_id" doc:"Filter by user ID"`
	Limit     int    `query:"limit" default:"50" minimum:"1" maximum:"100" doc:"Results per page"`
	Offset    int    `query:"offset" default:"0" minimum:"0" doc:"Offset for pagination"`
	Sort      string `query:"sort" default:"created_at" enum:"created_at,cost_usd,status,type" doc:"Sort field"`
	Order     string `query:"order" default:"desc" enum:"asc,desc" doc:"Sort order"`
}

// AnalyticsJobResponse represents a job in the analytics response.
type AnalyticsJobResponse struct {
	ID              string  `json:"id"`
	UserID          string  `json:"user_id"`
	Type            string  `json:"type"`
	Status          string  `json:"status"`
	URL             string  `json:"url"`
	CostUSD         float64 `json:"cost_usd"`
	LLMCostUSD      float64 `json:"llm_cost_usd"`
	TokensInput     int     `json:"tokens_input"`
	TokensOutput    int     `json:"tokens_output"`
	ErrorCategory   string  `json:"error_category,omitempty"`
	ErrorMessage    string  `json:"error_message,omitempty"`
	Provider        string  `json:"provider,omitempty"`
	Model           string  `json:"model,omitempty"`
	DiscoveryMethod string  `json:"discovery_method,omitempty"`
	IsBYOK          bool    `json:"is_byok"`
	CreatedAt       string  `json:"created_at"`
	CompletedAt     string  `json:"completed_at,omitempty"`
}

// GetAnalyticsJobsOutput represents the jobs list response.
type GetAnalyticsJobsOutput struct {
	Body struct {
		Jobs       []AnalyticsJobResponse `json:"jobs"`
		TotalCount int                    `json:"total_count"`
	}
}

// GetJobs returns a paginated list of jobs with analytics data.
func (h *AdminAnalyticsHandler) GetJobs(ctx context.Context, input *AnalyticsJobsInput) (*GetAnalyticsJobsOutput, error) {
	claims := mw.GetUserClaims(ctx)
	if claims == nil || !claims.GlobalSuperadmin {
		return nil, huma.Error403Forbidden("superadmin access required")
	}

	startDate, endDate := parseDateRange(input.StartDate, input.EndDate)

	result, err := h.analyticsRepo.GetJobs(ctx, repository.AnalyticsJobsParams{
		StartDate: startDate,
		EndDate:   endDate,
		Status:    input.Status,
		Type:      input.Type,
		UserID:    input.UserID,
		Limit:     input.Limit,
		Offset:    input.Offset,
		Sort:      input.Sort,
		Order:     input.Order,
	})
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get jobs: " + err.Error())
	}

	jobs := make([]AnalyticsJobResponse, 0, len(result.Jobs))
	for _, job := range result.Jobs {
		resp := AnalyticsJobResponse{
			ID:              job.ID,
			UserID:          job.UserID,
			Type:            job.Type,
			Status:          job.Status,
			URL:             job.URL,
			CostUSD:         job.CostUSD,
			LLMCostUSD:      job.LLMCostUSD,
			TokensInput:     job.TokensInput,
			TokensOutput:    job.TokensOutput,
			ErrorCategory:   job.ErrorCategory,
			ErrorMessage:    job.ErrorMessage,
			Provider:        job.Provider,
			Model:           job.Model,
			DiscoveryMethod: job.DiscoveryMethod,
			IsBYOK:          job.IsBYOK,
			CreatedAt:       job.CreatedAt.Format(time.RFC3339),
		}
		if job.CompletedAt != nil {
			resp.CompletedAt = job.CompletedAt.Format(time.RFC3339)
		}
		jobs = append(jobs, resp)
	}

	return &GetAnalyticsJobsOutput{
		Body: struct {
			Jobs       []AnalyticsJobResponse `json:"jobs"`
			TotalCount int                    `json:"total_count"`
		}{
			Jobs:       jobs,
			TotalCount: result.TotalCount,
		},
	}, nil
}

// --- Error Summary ---

// ErrorsInput represents the errors summary request parameters.
type ErrorsInput struct {
	StartDate string `query:"start_date" doc:"Start date (YYYY-MM-DD)"`
	EndDate   string `query:"end_date" doc:"End date (YYYY-MM-DD)"`
}

// ErrorCategoryResponse represents error statistics by category.
type ErrorCategoryResponse struct {
	Category       string   `json:"category"`
	Count          int      `json:"count"`
	Percentage     float64  `json:"percentage"`
	SampleMessages []string `json:"sample_messages,omitempty"`
}

// FailingURLResponse represents a failing URL with count.
type FailingURLResponse struct {
	URL   string `json:"url"`
	Count int    `json:"count"`
}

// ProviderErrorResponse represents errors by provider.
type ProviderErrorResponse struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Count    int    `json:"count"`
}

// GetErrorsOutput represents the errors summary response.
type GetErrorsOutput struct {
	Body struct {
		ByCategory     []ErrorCategoryResponse `json:"by_category"`
		TopFailingURLs []FailingURLResponse    `json:"top_failing_urls"`
		ByProvider     []ProviderErrorResponse `json:"by_provider"`
	}
}

// GetErrors returns error summary statistics for the given date range.
func (h *AdminAnalyticsHandler) GetErrors(ctx context.Context, input *ErrorsInput) (*GetErrorsOutput, error) {
	claims := mw.GetUserClaims(ctx)
	if claims == nil || !claims.GlobalSuperadmin {
		return nil, huma.Error403Forbidden("superadmin access required")
	}

	startDate, endDate := parseDateRange(input.StartDate, input.EndDate)

	summary, err := h.analyticsRepo.GetErrors(ctx, startDate, endDate)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get errors: " + err.Error())
	}

	categories := make([]ErrorCategoryResponse, 0, len(summary.ByCategory))
	for _, cat := range summary.ByCategory {
		categories = append(categories, ErrorCategoryResponse{
			Category:       cat.Category,
			Count:          cat.Count,
			Percentage:     cat.Percentage,
			SampleMessages: cat.SampleMessages,
		})
	}

	failingURLs := make([]FailingURLResponse, 0, len(summary.TopFailingURLs))
	for _, url := range summary.TopFailingURLs {
		failingURLs = append(failingURLs, FailingURLResponse{
			URL:   url.URL,
			Count: url.Count,
		})
	}

	providers := make([]ProviderErrorResponse, 0, len(summary.ByProvider))
	for _, p := range summary.ByProvider {
		providers = append(providers, ProviderErrorResponse{
			Provider: p.Provider,
			Model:    p.Model,
			Count:    p.Count,
		})
	}

	return &GetErrorsOutput{
		Body: struct {
			ByCategory     []ErrorCategoryResponse `json:"by_category"`
			TopFailingURLs []FailingURLResponse    `json:"top_failing_urls"`
			ByProvider     []ProviderErrorResponse `json:"by_provider"`
		}{
			ByCategory:     categories,
			TopFailingURLs: failingURLs,
			ByProvider:     providers,
		},
	}, nil
}

// --- Trends ---

// TrendsInput represents the trends request parameters.
type TrendsInput struct {
	StartDate string `query:"start_date" doc:"Start date (YYYY-MM-DD)"`
	EndDate   string `query:"end_date" doc:"End date (YYYY-MM-DD)"`
	Interval  string `query:"interval" default:"day" enum:"day,week,month" doc:"Aggregation interval"`
}

// TrendPointResponse represents a single trend data point.
type TrendPointResponse struct {
	Date       string  `json:"date"`
	JobCount   int     `json:"job_count"`
	CostUSD    float64 `json:"cost_usd"`
	ErrorCount int     `json:"error_count"`
	Tokens     int     `json:"tokens"`
}

// GetTrendsOutput represents the trends response.
type GetTrendsOutput struct {
	Body struct {
		Trends []TrendPointResponse `json:"trends"`
	}
}

// GetTrends returns trend data for charts.
func (h *AdminAnalyticsHandler) GetTrends(ctx context.Context, input *TrendsInput) (*GetTrendsOutput, error) {
	claims := mw.GetUserClaims(ctx)
	if claims == nil || !claims.GlobalSuperadmin {
		return nil, huma.Error403Forbidden("superadmin access required")
	}

	startDate, endDate := parseDateRange(input.StartDate, input.EndDate)

	trends, err := h.analyticsRepo.GetTrends(ctx, startDate, endDate, input.Interval)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get trends: " + err.Error())
	}

	trendPoints := make([]TrendPointResponse, 0, len(trends))
	for _, t := range trends {
		trendPoints = append(trendPoints, TrendPointResponse{
			Date:       t.Date,
			JobCount:   t.JobCount,
			CostUSD:    t.CostUSD,
			ErrorCount: t.ErrorCount,
			Tokens:     t.Tokens,
		})
	}

	return &GetTrendsOutput{
		Body: struct {
			Trends []TrendPointResponse `json:"trends"`
		}{
			Trends: trendPoints,
		},
	}, nil
}

// --- User Summary ---

// AnalyticsUsersInput represents the users summary request parameters.
type AnalyticsUsersInput struct {
	StartDate string `query:"start_date" doc:"Start date (YYYY-MM-DD)"`
	EndDate   string `query:"end_date" doc:"End date (YYYY-MM-DD)"`
	Limit     int    `query:"limit" default:"50" minimum:"1" maximum:"100" doc:"Results per page"`
	Offset    int    `query:"offset" default:"0" minimum:"0" doc:"Offset for pagination"`
	Sort      string `query:"sort" default:"total_cost" enum:"total_cost,total_jobs,last_active,failed_jobs,total_tokens" doc:"Sort field"`
	Order     string `query:"order" default:"desc" enum:"asc,desc" doc:"Sort order"`
}

// UserSummaryResponse represents per-user analytics.
type UserSummaryResponse struct {
	UserID        string `json:"user_id"`
	TotalJobs     int    `json:"total_jobs"`
	CompletedJobs int    `json:"completed_jobs"`
	FailedJobs    int    `json:"failed_jobs"`
	TotalCostUSD  float64 `json:"total_cost_usd"`
	TotalTokens   int    `json:"total_tokens"`
	LastActive    string `json:"last_active,omitempty"`
}

// GetAnalyticsUsersOutput represents the users summary response.
type GetAnalyticsUsersOutput struct {
	Body struct {
		Users      []UserSummaryResponse `json:"users"`
		TotalCount int                   `json:"total_count"`
	}
}

// GetUsers returns a paginated list of user summaries.
func (h *AdminAnalyticsHandler) GetUsers(ctx context.Context, input *AnalyticsUsersInput) (*GetAnalyticsUsersOutput, error) {
	claims := mw.GetUserClaims(ctx)
	if claims == nil || !claims.GlobalSuperadmin {
		return nil, huma.Error403Forbidden("superadmin access required")
	}

	startDate, endDate := parseDateRange(input.StartDate, input.EndDate)

	result, err := h.analyticsRepo.GetUsers(ctx, repository.AnalyticsUsersParams{
		StartDate: startDate,
		EndDate:   endDate,
		Limit:     input.Limit,
		Offset:    input.Offset,
		Sort:      input.Sort,
		Order:     input.Order,
	})
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get users: " + err.Error())
	}

	users := make([]UserSummaryResponse, 0, len(result.Users))
	for _, user := range result.Users {
		resp := UserSummaryResponse{
			UserID:        user.UserID,
			TotalJobs:     user.TotalJobs,
			CompletedJobs: user.CompletedJobs,
			FailedJobs:    user.FailedJobs,
			TotalCostUSD:  user.TotalCostUSD,
			TotalTokens:   user.TotalTokens,
		}
		if user.LastActive != nil {
			resp.LastActive = user.LastActive.Format(time.RFC3339)
		}
		users = append(users, resp)
	}

	return &GetAnalyticsUsersOutput{
		Body: struct {
			Users      []UserSummaryResponse `json:"users"`
			TotalCount int                   `json:"total_count"`
		}{
			Users:      users,
			TotalCount: result.TotalCount,
		},
	}, nil
}

// parseDateRange parses and validates date range, defaulting to last 7 days.
func parseDateRange(startDate, endDate string) (string, string) {
	now := time.Now()

	if endDate == "" {
		endDate = now.AddDate(0, 0, 1).Format("2006-01-02") // Tomorrow to include today
	} else {
		// Add one day to endDate to make it inclusive
		if t, err := time.Parse("2006-01-02", endDate); err == nil {
			endDate = t.AddDate(0, 0, 1).Format("2006-01-02")
		}
	}

	if startDate == "" {
		startDate = now.AddDate(0, 0, -7).Format("2006-01-02") // 7 days ago
	}

	return startDate, endDate
}

// --- Job Results (Admin) ---

// AdminJobResultsInput represents the job results request parameters.
type AdminJobResultsInput struct {
	ID string `path:"id" doc:"Job ID"`
}

// AdminJobResultsOutput represents the job results response.
type AdminJobResultsOutput struct {
	Body struct {
		JobID       string `json:"job_id" doc:"Job ID"`
		DownloadURL string `json:"download_url" doc:"Presigned URL to download results (valid for 1 hour)"`
		ExpiresAt   string `json:"expires_at" doc:"URL expiration time"`
	}
}

// GetJobResults returns a presigned URL for downloading job results (admin access).
func (h *AdminAnalyticsHandler) GetJobResults(ctx context.Context, input *AdminJobResultsInput) (*AdminJobResultsOutput, error) {
	claims := mw.GetUserClaims(ctx)
	if claims == nil || !claims.GlobalSuperadmin {
		return nil, huma.Error403Forbidden("superadmin access required")
	}

	// Check if storage is enabled
	if h.storageSvc == nil || !h.storageSvc.IsEnabled() {
		return nil, huma.Error503ServiceUnavailable("result storage is not configured")
	}

	// Check if results exist in storage
	exists, err := h.storageSvc.JobResultExists(ctx, input.ID)
	if err != nil || !exists {
		return nil, huma.Error404NotFound("results not found in storage")
	}

	// Generate presigned URL (valid for 1 hour)
	expiry := 1 * time.Hour
	downloadURL, err := h.storageSvc.GetJobResultsPresignedURL(ctx, input.ID, expiry)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to generate download URL: " + err.Error())
	}

	return &AdminJobResultsOutput{
		Body: struct {
			JobID       string `json:"job_id" doc:"Job ID"`
			DownloadURL string `json:"download_url" doc:"Presigned URL to download results (valid for 1 hour)"`
			ExpiresAt   string `json:"expires_at" doc:"URL expiration time"`
		}{
			JobID:       input.ID,
			DownloadURL: downloadURL,
			ExpiresAt:   time.Now().Add(expiry).Format(time.RFC3339),
		},
	}, nil
}
