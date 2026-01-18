package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/jmylchreest/refyne-api/internal/config"
	"github.com/jmylchreest/refyne-api/internal/models"
	"github.com/jmylchreest/refyne-api/internal/repository"
)

// JobService handles async job operations.
type JobService struct {
	cfg    *config.Config
	repos  *repository.Repositories
	logger *slog.Logger
}

// NewJobService creates a new job service.
func NewJobService(cfg *config.Config, repos *repository.Repositories, logger *slog.Logger) *JobService {
	return &JobService{
		cfg:    cfg,
		repos:  repos,
		logger: logger,
	}
}

// CrawlOptions represents crawl job options.
type CrawlOptions struct {
	FollowSelector   string `json:"follow_selector,omitempty"`
	FollowPattern    string `json:"follow_pattern,omitempty"`
	MaxDepth         int    `json:"max_depth,omitempty"`
	NextSelector     string `json:"next_selector,omitempty"`
	MaxPages         int    `json:"max_pages,omitempty"`
	MaxURLs          int    `json:"max_urls,omitempty"`
	Delay            string `json:"delay,omitempty"`
	Concurrency      int    `json:"concurrency,omitempty"`
	SameDomainOnly   bool   `json:"same_domain_only,omitempty"`
	ExtractFromSeeds bool   `json:"extract_from_seeds,omitempty"`
	UseSitemap       bool   `json:"use_sitemap,omitempty"`
}

// CreateCrawlJobInput represents input for creating a crawl job.
type CreateCrawlJobInput struct {
	URL                 string          `json:"url"`
	Schema              json.RawMessage `json:"schema"`
	Options             CrawlOptions    `json:"options,omitempty"`
	WebhookURL          string          `json:"webhook_url,omitempty"`
	LLMConfig           *LLMConfigInput `json:"llm_config,omitempty"`
	Tier                string          `json:"tier,omitempty"`                  // User's tier at job creation time
	BYOKAllowed         bool            `json:"byok_allowed,omitempty"`          // Whether user has the "provider_byok" feature
	ModelsCustomAllowed bool            `json:"models_custom_allowed,omitempty"` // Whether user has the "models_custom" feature
}

// CreateCrawlJobOutput represents output from creating a crawl job.
type CreateCrawlJobOutput struct {
	JobID     string `json:"job_id"`
	Status    string `json:"status"`
	StatusURL string `json:"status_url"`
}

// CreateExtractJobInput represents input for creating a single-page extract job.
type CreateExtractJobInput struct {
	URL       string          `json:"url"`
	Schema    json.RawMessage `json:"schema"`
	FetchMode string          `json:"fetch_mode,omitempty"`
	IsBYOK    bool            `json:"is_byok"`
}

// CreateExtractJobOutput represents output from creating an extract job.
type CreateExtractJobOutput struct {
	JobID  string `json:"job_id"`
	Status string `json:"status"`
}

// CompleteExtractJobInput represents input for completing a successful extract job.
type CompleteExtractJobInput struct {
	ResultJSON       string  `json:"result_json"`
	PageCount        int     `json:"page_count"`
	TokenUsageInput  int     `json:"token_usage_input"`
	TokenUsageOutput int     `json:"token_usage_output"`
	CostUSD          float64 `json:"cost_usd"`
	LLMProvider      string  `json:"llm_provider"`
	LLMModel         string  `json:"llm_model"`
}

// FailExtractJobInput represents input for failing an extract job.
type FailExtractJobInput struct {
	ErrorMessage  string `json:"error_message"`   // User-visible error (sanitized for non-BYOK)
	ErrorDetails  string `json:"error_details"`   // Full error details (admin/BYOK only)
	ErrorCategory string `json:"error_category"`  // Error classification (provider_error, rate_limit, invalid_key, parsing_error, fetch_error, etc.)
	LLMProvider   string `json:"llm_provider,omitempty"`
	LLMModel      string `json:"llm_model,omitempty"`
}

// CreateCrawlJob creates a new crawl job.
func (s *JobService) CreateCrawlJob(ctx context.Context, userID string, input CreateCrawlJobInput) (*CreateCrawlJobOutput, error) {
	// Serialize options
	optionsJSON, err := json.Marshal(input.Options)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize options: %w", err)
	}

	now := time.Now()
	job := &models.Job{
		ID:                  ulid.Make().String(),
		UserID:              userID,
		Tier:                input.Tier,                // User's tier at job creation time
		BYOKAllowed:         input.BYOKAllowed,         // Whether user had provider_byok feature at job creation
		ModelsCustomAllowed: input.ModelsCustomAllowed, // Whether user had models_custom feature at job creation
		Type:                models.JobTypeCrawl,
		Status:              models.JobStatusPending,
		URL:                 input.URL,
		SchemaJSON:          string(input.Schema),
		CrawlOptionsJSON:    string(optionsJSON),
		WebhookURL:          input.WebhookURL,
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	if err := s.repos.Job.Create(ctx, job); err != nil {
		return nil, fmt.Errorf("failed to create job: %w", err)
	}

	return &CreateCrawlJobOutput{
		JobID:     job.ID,
		Status:    string(job.Status),
		StatusURL: fmt.Sprintf("%s/api/v1/jobs/%s", s.cfg.BaseURL, job.ID),
	}, nil
}

// GetJob retrieves a job by ID.
func (s *JobService) GetJob(ctx context.Context, userID, jobID string) (*models.Job, error) {
	job, err := s.repos.Job.GetByID(ctx, jobID)
	if err != nil {
		return nil, fmt.Errorf("failed to get job: %w", err)
	}
	if job == nil {
		return nil, nil
	}

	// Ensure the job belongs to the user
	if job.UserID != userID {
		return nil, nil
	}

	return job, nil
}

// ListJobs retrieves jobs for a user.
func (s *JobService) ListJobs(ctx context.Context, userID string, limit, offset int) ([]*models.Job, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	return s.repos.Job.GetByUserID(ctx, userID, limit, offset)
}

// GetJobResults retrieves results for a job.
func (s *JobService) GetJobResults(ctx context.Context, userID, jobID string) ([]*models.JobResult, error) {
	// Verify job belongs to user
	job, err := s.GetJob(ctx, userID, jobID)
	if err != nil || job == nil {
		return nil, err
	}

	return s.repos.JobResult.GetByJobID(ctx, jobID)
}

// GetJobResultsAfterID retrieves results with ID greater than afterID (for SSE streaming).
// Pass empty string for afterID to get all results.
// This works correctly because IDs are ULIDs which are lexicographically time-ordered.
func (s *JobService) GetJobResultsAfterID(ctx context.Context, userID, jobID, afterID string) ([]*models.JobResult, error) {
	// Verify job belongs to user
	job, err := s.GetJob(ctx, userID, jobID)
	if err != nil || job == nil {
		return nil, err
	}

	return s.repos.JobResult.GetAfterID(ctx, jobID, afterID)
}

// GetCrawlMap retrieves the crawl map for a job (results ordered by depth).
// This shows the relationship between pages discovered during the crawl.
func (s *JobService) GetCrawlMap(ctx context.Context, userID, jobID string) ([]*models.JobResult, error) {
	// Verify job belongs to user
	job, err := s.GetJob(ctx, userID, jobID)
	if err != nil || job == nil {
		return nil, err
	}

	// Only crawl jobs have meaningful crawl maps
	if job.Type != models.JobTypeCrawl {
		return nil, fmt.Errorf("crawl map only available for crawl jobs")
	}

	return s.repos.JobResult.GetCrawlMap(ctx, jobID)
}

// CountJobResults returns the total number of results for a job.
// This is useful for progress tracking (total discovered URLs).
func (s *JobService) CountJobResults(ctx context.Context, userID, jobID string) (int, error) {
	// Verify job belongs to user
	job, err := s.GetJob(ctx, userID, jobID)
	if err != nil || job == nil {
		return 0, err
	}

	return s.repos.JobResult.CountByJobID(ctx, jobID)
}

// CountActiveJobsByUser counts jobs that are pending or running for a user.
func (s *JobService) CountActiveJobsByUser(ctx context.Context, userID string) (int, error) {
	return s.repos.Job.CountActiveByUserID(ctx, userID)
}

// CreateExtractJob creates a new single-page extract job with running status.
// This should be called before starting extraction so we have a job record for history.
func (s *JobService) CreateExtractJob(ctx context.Context, userID string, input CreateExtractJobInput) (*CreateExtractJobOutput, error) {
	now := time.Now()
	job := &models.Job{
		ID:         ulid.Make().String(),
		UserID:     userID,
		Type:       models.JobTypeExtract,
		Status:     models.JobStatusRunning,
		URL:        input.URL,
		SchemaJSON: string(input.Schema),
		IsBYOK:     input.IsBYOK,
		PageCount:  1, // Single-page extract always processes 1 page
		StartedAt:  &now,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if err := s.repos.Job.Create(ctx, job); err != nil {
		return nil, fmt.Errorf("failed to create extract job: %w", err)
	}

	s.logger.Info("created extract job",
		"job_id", job.ID,
		"user_id", userID,
		"url", input.URL,
		"is_byok", input.IsBYOK,
	)

	return &CreateExtractJobOutput{
		JobID:  job.ID,
		Status: string(job.Status),
	}, nil
}

// CompleteExtractJob marks an extract job as completed with results.
func (s *JobService) CompleteExtractJob(ctx context.Context, jobID string, input CompleteExtractJobInput) error {
	job, err := s.repos.Job.GetByID(ctx, jobID)
	if err != nil {
		return fmt.Errorf("failed to get job: %w", err)
	}
	if job == nil {
		return fmt.Errorf("job not found: %s", jobID)
	}

	now := time.Now()
	job.Status = models.JobStatusCompleted
	job.ResultJSON = input.ResultJSON
	job.PageCount = input.PageCount
	job.TokenUsageInput = input.TokenUsageInput
	job.TokenUsageOutput = input.TokenUsageOutput
	job.CostUSD = input.CostUSD
	job.LLMProvider = input.LLMProvider
	job.LLMModel = input.LLMModel
	job.CompletedAt = &now
	job.UpdatedAt = now

	if err := s.repos.Job.Update(ctx, job); err != nil {
		return fmt.Errorf("failed to update job: %w", err)
	}

	s.logger.Info("completed extract job",
		"job_id", jobID,
		"provider", input.LLMProvider,
		"model", input.LLMModel,
		"input_tokens", input.TokenUsageInput,
		"output_tokens", input.TokenUsageOutput,
	)

	return nil
}

// FailExtractJob marks an extract job as failed with error details.
func (s *JobService) FailExtractJob(ctx context.Context, jobID string, input FailExtractJobInput) error {
	job, err := s.repos.Job.GetByID(ctx, jobID)
	if err != nil {
		return fmt.Errorf("failed to get job: %w", err)
	}
	if job == nil {
		return fmt.Errorf("job not found: %s", jobID)
	}

	now := time.Now()
	job.Status = models.JobStatusFailed
	job.ErrorMessage = input.ErrorMessage
	job.ErrorDetails = input.ErrorDetails
	job.ErrorCategory = input.ErrorCategory
	job.LLMProvider = input.LLMProvider
	job.LLMModel = input.LLMModel
	job.CompletedAt = &now
	job.UpdatedAt = now

	if err := s.repos.Job.Update(ctx, job); err != nil {
		return fmt.Errorf("failed to update job: %w", err)
	}

	s.logger.Warn("extract job failed",
		"job_id", jobID,
		"error_category", input.ErrorCategory,
		"error_message", input.ErrorMessage,
		"provider", input.LLMProvider,
		"model", input.LLMModel,
	)

	return nil
}
