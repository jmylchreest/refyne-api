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
}

// CreateCrawlJobInput represents input for creating a crawl job.
type CreateCrawlJobInput struct {
	URL        string          `json:"url"`
	Schema     json.RawMessage `json:"schema"`
	Options    CrawlOptions    `json:"options,omitempty"`
	WebhookURL string          `json:"webhook_url,omitempty"`
	LLMConfig  *LLMConfigInput `json:"llm_config,omitempty"`
}

// CreateCrawlJobOutput represents output from creating a crawl job.
type CreateCrawlJobOutput struct {
	JobID     string `json:"job_id"`
	Status    string `json:"status"`
	StatusURL string `json:"status_url"`
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
		ID:               ulid.Make().String(),
		UserID:           userID,
		Type:             models.JobTypeCrawl,
		Status:           models.JobStatusPending,
		URL:              input.URL,
		SchemaJSON:       string(input.Schema),
		CrawlOptionsJSON: string(optionsJSON),
		WebhookURL:       input.WebhookURL,
		CreatedAt:        now,
		UpdatedAt:        now,
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

// GetJobResultsAfter retrieves results after a given ID (for SSE streaming).
func (s *JobService) GetJobResultsAfter(ctx context.Context, userID, jobID, afterID string) ([]*models.JobResult, error) {
	// Verify job belongs to user
	job, err := s.GetJob(ctx, userID, jobID)
	if err != nil || job == nil {
		return nil, err
	}

	return s.repos.JobResult.GetAfter(ctx, jobID, afterID)
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

// CountActiveJobsByUser counts jobs that are pending or running for a user.
func (s *JobService) CountActiveJobsByUser(ctx context.Context, userID string) (int, error) {
	return s.repos.Job.CountActiveByUserID(ctx, userID)
}
