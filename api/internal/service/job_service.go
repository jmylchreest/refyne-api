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
	cfg        *config.Config
	repos      *repository.Repositories
	storageSvc *StorageService
	webhookSvc *WebhookService
	logger     *slog.Logger
}

// NewJobService creates a new job service.
func NewJobService(cfg *config.Config, repos *repository.Repositories, storageSvc *StorageService, logger *slog.Logger) *JobService {
	return &JobService{
		cfg:        cfg,
		repos:      repos,
		storageSvc: storageSvc,
		logger:     logger,
	}
}

// SetWebhookService sets the webhook service for job completion notifications.
// This allows for late binding to avoid circular dependencies.
func (s *JobService) SetWebhookService(webhookSvc *WebhookService) {
	s.webhookSvc = webhookSvc
}

// RunJobResult contains the result of running a job via RunJob.
type RunJobResult struct {
	JobID  string              // The job ID (ULID)
	Result *JobExecutionResult // The execution result
}

// RunJob executes a job with full lifecycle management.
// This is the single entry point for synchronous job types (extract, analyze).
// It handles: job record creation, execution, completion/failure, webhooks, and storage.
// Returns the job ID and result on success, or an error on failure.
func (s *JobService) RunJob(ctx context.Context, executor JobExecutor, opts *RunJobOptions) (*RunJobResult, error) {
	// 1. Create job record
	job, err := s.createJobRecord(ctx, executor, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create job record: %w", err)
	}

	// 2. Set the job ID on the executor for tracking in downstream services (e.g., captcha)
	executor.SetJobID(job.ID)

	// 3. Mark job as running
	if err := s.markJobRunning(ctx, job); err != nil {
		s.logger.Error("failed to mark job as running", "job_id", job.ID, "error", err)
		// Continue anyway - the job record exists
	}

	// 4. Execute the job
	result, execErr := executor.Execute(ctx)

	// 5. Handle completion or failure
	if execErr != nil {
		s.handleJobFailure(ctx, job, executor, execErr, opts)
		return &RunJobResult{JobID: job.ID}, execErr
	}

	// 6. Handle success
	s.handleJobSuccess(ctx, job, executor, result, opts)

	return &RunJobResult{
		JobID:  job.ID,
		Result: result,
	}, nil
}

// createJobRecord creates a job record for tracking.
func (s *JobService) createJobRecord(ctx context.Context, executor JobExecutor, opts *RunJobOptions) (*models.Job, error) {
	// If an existing job ID is provided, retrieve it
	if opts.JobID != "" {
		job, err := s.repos.Job.GetByID(ctx, opts.JobID)
		if err != nil {
			return nil, fmt.Errorf("failed to get existing job: %w", err)
		}
		if job == nil {
			return nil, fmt.Errorf("job not found: %s", opts.JobID)
		}
		return job, nil
	}

	// Create new job record
	now := time.Now()
	job := &models.Job{
		ID:           ulid.Make().String(),
		UserID:       opts.UserID,
		Type:         executor.JobType(),
		Status:       models.JobStatusPending,
		URL:          executor.GetURL(),
		SchemaJSON:   string(executor.GetSchema()),
		Tier:         opts.Tier,
		IsBYOK:       executor.IsBYOK(),
		CaptureDebug: opts.CaptureDebug,
		PageCount:    1, // Default for single-page jobs
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.repos.Job.Create(ctx, job); err != nil {
		return nil, fmt.Errorf("failed to create job: %w", err)
	}

	s.logger.Info("created job record",
		"job_id", job.ID,
		"job_type", job.Type,
		"user_id", opts.UserID,
		"url", job.URL,
	)

	return job, nil
}

// markJobRunning marks a job as running.
func (s *JobService) markJobRunning(ctx context.Context, job *models.Job) error {
	now := time.Now()
	job.Status = models.JobStatusRunning
	job.StartedAt = &now
	job.UpdatedAt = now
	return s.repos.Job.Update(ctx, job)
}

// handleJobSuccess handles successful job completion including webhooks and storage.
func (s *JobService) handleJobSuccess(ctx context.Context, job *models.Job, executor JobExecutor, result *JobExecutionResult, opts *RunJobOptions) {
	// Update job record with results
	now := time.Now()
	job.Status = models.JobStatusCompleted
	job.PageCount = result.PageCount
	job.TokenUsageInput = result.TokensInput
	job.TokenUsageOutput = result.TokensOutput
	job.CostUSD = result.CostUSD
	job.LLMCostUSD = result.LLMCostUSD
	job.LLMProvider = result.LLMProvider
	job.LLMModel = result.LLMModel
	job.IsBYOK = result.IsBYOK
	job.CompletedAt = &now
	job.UpdatedAt = now

	if err := s.repos.Job.Update(ctx, job); err != nil {
		s.logger.Error("failed to update job record", "job_id", job.ID, "error", err)
	}

	// Store results to S3/Tigris
	if s.storageSvc != nil && s.storageSvc.IsEnabled() && result.ResultJSON != "" {
		jobResults := &JobResults{
			JobID:       job.ID,
			UserID:      job.UserID,
			Status:      string(job.Status),
			TotalPages:  result.PageCount,
			CompletedAt: now,
			Results: []JobResultData{
				{
					ID:        job.URL,
					URL:       job.URL,
					Data:      json.RawMessage(result.ResultJSON),
					CreatedAt: now,
				},
			},
		}

		if err := s.storageSvc.StoreJobResults(ctx, jobResults); err != nil {
			s.logger.Error("failed to store job results to S3",
				"job_id", job.ID,
				"error", err,
			)
			// Don't fail - results are still tracked in the job record
		}
	}

	// Store debug capture if enabled
	if opts.CaptureDebug && result.DebugCapture != nil && s.storageSvc != nil && s.storageSvc.IsEnabled() {
		// Marshal parsed output if available
		var parsedOutputJSON json.RawMessage
		if result.DebugCapture.ParsedOutput != nil {
			if data, err := json.Marshal(result.DebugCapture.ParsedOutput); err == nil {
				parsedOutputJSON = data
			}
		}

		capture := &JobDebugCapture{
			JobID:          job.ID,
			JobType:        string(executor.JobType()),
			APIVersion:     result.DebugCapture.APIVersion,
			IsBYOK:         result.IsBYOK,
			Enabled:        true,
			TotalRequests:  1,
			TotalTokensIn:  result.TokensInput,
			TotalTokensOut: result.TokensOutput,
			TotalCostUSD:   result.DebugCapture.CostUSD,
			TotalDurationMs: result.DebugCapture.DurationMs,
			Captures: []LLMRequestCapture{
				{
					ID:         ulid.Make().String(),
					URL:        result.DebugCapture.URL,
					Timestamp:  now,
					JobType:    string(executor.JobType()),
					APIVersion: result.DebugCapture.APIVersion,
					Sequence:   result.DebugCapture.Sequence,
					IsBYOK:     result.DebugCapture.IsBYOK,
					Request: LLMRequestSection{
						Metadata: LLMRequestMeta{
							Provider:         result.DebugCapture.Provider,
							Model:            result.DebugCapture.Model,
							FetchMode:        result.DebugCapture.FetchMode,
							ContentSize:      len(result.DebugCapture.RawContent),
							PromptSize:       len(result.DebugCapture.Prompt) + len(result.DebugCapture.UserPrompt),
							Temperature:      result.DebugCapture.Temperature,
							MaxTokens:        result.DebugCapture.MaxTokens,
							JSONMode:         result.DebugCapture.JSONMode,
							FallbackPosition: result.DebugCapture.FallbackPosition,
							IsRetry:          result.DebugCapture.IsRetry,
						},
						Payload: LLMRequestPayload{
							SystemPrompt: result.DebugCapture.SystemPrompt,
							UserPrompt:   result.DebugCapture.UserPrompt,
							Schema:       result.DebugCapture.Schema,
							Prompt:       result.DebugCapture.Prompt,
							PageContent:  result.DebugCapture.RawContent,
							Hints:        result.DebugCapture.Hints,
						},
					},
					Response: LLMResponseSection{
						Metadata: LLMResponseMeta{
							InputTokens:  result.TokensInput,
							OutputTokens: result.TokensOutput,
							DurationMs:   result.DebugCapture.DurationMs,
							Success:      true,
							CostUSD:      result.DebugCapture.CostUSD,
						},
						Payload: LLMResponsePayload{
							RawOutput:    result.DebugCapture.RawLLMResponse,
							ParsedOutput: parsedOutputJSON,
							ParseError:   result.DebugCapture.ParseError,
						},
					},
				},
			},
		}

		if err := s.storageSvc.StoreDebugCapture(ctx, capture); err != nil {
			s.logger.Error("failed to store debug capture",
				"job_id", job.ID,
				"error", err,
			)
		}
	}

	// Send webhooks - ALWAYS for all job types
	s.sendWebhooksForJob(ctx, job, string(models.WebhookEventJobCompleted), map[string]any{
		"job_id":     job.ID,
		"job_type":   string(executor.JobType()),
		"status":     "completed",
		"url":        job.URL,
		"page_count": result.PageCount,
		"cost_usd":   result.CostUSD,
		"data":       result.WebhookData,
	}, opts.EphemeralWebhook)

	s.logger.Info("job completed successfully",
		"job_id", job.ID,
		"job_type", executor.JobType(),
		"provider", result.LLMProvider,
		"model", result.LLMModel,
		"cost_usd", result.CostUSD,
	)
}

// handleJobFailure handles job failure including webhooks.
func (s *JobService) handleJobFailure(ctx context.Context, job *models.Job, executor JobExecutor, err error, opts *RunJobOptions) {
	// Extract and classify error
	errInfo := ClassifyError(err, executor.IsBYOK())

	// Update job record
	now := time.Now()
	job.Status = models.JobStatusFailed
	job.ErrorMessage = errInfo.UserMessage
	job.ErrorDetails = errInfo.Details
	job.ErrorCategory = errInfo.Category
	job.LLMProvider = errInfo.Provider
	job.LLMModel = errInfo.Model
	job.CompletedAt = &now
	job.UpdatedAt = now

	if updateErr := s.repos.Job.Update(ctx, job); updateErr != nil {
		s.logger.Error("failed to update job record for failure",
			"job_id", job.ID,
			"error", updateErr,
		)
	}

	// Send webhooks - ALWAYS for all job types
	s.sendWebhooksForJob(ctx, job, string(models.WebhookEventJobFailed), map[string]any{
		"job_id":   job.ID,
		"job_type": string(executor.JobType()),
		"status":   "failed",
		"url":      job.URL,
		"error":    errInfo.UserMessage,
		"category": errInfo.Category,
	}, opts.EphemeralWebhook)

	s.logger.Warn("job failed",
		"job_id", job.ID,
		"job_type", executor.JobType(),
		"error_category", errInfo.Category,
		"error_message", errInfo.UserMessage,
	)
}

// sendWebhooksForJob sends webhooks for a job event.
// Handles both ephemeral (per-request) and persistent (saved) webhooks.
func (s *JobService) sendWebhooksForJob(ctx context.Context, job *models.Job, eventType string, data map[string]any, ephemeral *WebhookConfig) {
	if s.webhookSvc == nil {
		return
	}

	s.webhookSvc.SendForJob(ctx, job.UserID, eventType, job.ID, data, ephemeral)
}

// GetJobID returns the job ID from a RunJobOptions, or empty string if not set.
func (opts *RunJobOptions) GetJobID() string {
	if opts == nil {
		return ""
	}
	return opts.JobID
}

// CrawlOptions represents crawl job options.
type CrawlOptions struct {
	FollowSelector        string          `json:"follow_selector,omitempty"`
	FollowPattern         string          `json:"follow_pattern,omitempty"`
	MaxDepth              int             `json:"max_depth,omitempty"`
	NextSelector          string          `json:"next_selector,omitempty"`
	MaxPages              int             `json:"max_pages,omitempty"`
	MaxURLs               int             `json:"max_urls,omitempty"`
	Delay                 string          `json:"delay,omitempty"`
	Concurrency           int             `json:"concurrency,omitempty"`
	SameDomainOnly        bool            `json:"same_domain_only,omitempty"`
	ExtractFromSeeds      bool            `json:"extract_from_seeds,omitempty"`
	UseSitemap            bool            `json:"use_sitemap,omitempty"`
	FetchMode             string          `json:"fetch_mode,omitempty"`              // auto, static, or dynamic
	ContentDynamicAllowed bool            `json:"content_dynamic_allowed,omitempty"` // Whether user has content_dynamic feature (set at job creation)
	SkipCreditCheck       bool            `json:"skip_credit_check,omitempty"`       // Whether user has skip_credit_check feature (disables mid-crawl balance check)
	CleanerChain          []CleanerConfig `json:"cleaner_chain,omitempty"`
}

// CreateCrawlJobInput represents input for creating a crawl job.
type CreateCrawlJobInput struct {
	URL          string            `json:"url"`
	Schema       json.RawMessage   `json:"schema"`
	Options      CrawlOptions      `json:"options,omitempty"`
	CleanerChain []CleanerConfig   `json:"cleaner_chain,omitempty"` // Content cleaner chain
	WebhookURL   string            `json:"webhook_url,omitempty"`
	LLMConfigs   []*LLMConfigInput `json:"llm_configs"`        // Pre-resolved LLM config chain
	Tier         string            `json:"tier"`               // User's subscription tier at job creation time
	IsBYOK       bool              `json:"is_byok"`            // Whether using user's own API keys
	CaptureDebug *bool             `json:"capture_debug,omitempty"` // Whether to capture LLM requests for debugging
}

// CreateCrawlJobOutput represents output from creating a crawl job.
type CreateCrawlJobOutput struct {
	JobID     string `json:"job_id"`
	Status    string `json:"status"`
	StatusURL string `json:"status_url"`
}

// CreateCrawlJob creates a new crawl job.
func (s *JobService) CreateCrawlJob(ctx context.Context, userID string, input CreateCrawlJobInput) (*CreateCrawlJobOutput, error) {
	// Include cleaner chain in options for storage
	options := input.Options
	if len(input.CleanerChain) > 0 {
		options.CleanerChain = input.CleanerChain
	}

	// Serialize options (includes cleaner chain)
	optionsJSON, err := json.Marshal(options)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize options: %w", err)
	}

	// Serialize LLM configs
	llmConfigsJSON, err := json.Marshal(input.LLMConfigs)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize llm configs: %w", err)
	}

	// Determine capture_debug setting (default: false for crawl jobs)
	captureDebug := false
	if input.CaptureDebug != nil {
		captureDebug = *input.CaptureDebug
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
		LLMConfigsJSON:   string(llmConfigsJSON),
		Tier:             input.Tier,
		IsBYOK:           input.IsBYOK,
		CaptureDebug:     captureDebug,
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

// GetJobAdmin retrieves a job by ID without ownership check (for superadmin use).
func (s *JobService) GetJobAdmin(ctx context.Context, jobID string) (*models.Job, error) {
	job, err := s.repos.Job.GetByID(ctx, jobID)
	if err != nil {
		return nil, fmt.Errorf("failed to get job: %w", err)
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
// Results are fetched from S3/Tigris storage for both extract and crawl jobs.
// The job_results table contains only metadata for SSE progress tracking.
func (s *JobService) GetJobResults(ctx context.Context, userID, jobID string) ([]*models.JobResult, error) {
	// Verify job belongs to user
	job, err := s.GetJob(ctx, userID, jobID)
	if err != nil || job == nil {
		return nil, err
	}

	// All job types now use S3 storage for full results
	return s.getJobResultsFromStorage(ctx, job)
}

// GetJobResultsAdmin retrieves results for a job without ownership check (for superadmin use).
func (s *JobService) GetJobResultsAdmin(ctx context.Context, jobID string) ([]*models.JobResult, error) {
	job, err := s.GetJobAdmin(ctx, jobID)
	if err != nil || job == nil {
		return nil, err
	}
	return s.getJobResultsFromStorage(ctx, job)
}

// getJobResultsFromStorage retrieves job results from S3/Tigris.
// Converts the S3 storage format to JobResult models for API consistency.
// For crawl jobs, this also fetches metadata from job_results table and merges
// the extracted data from S3 with the metadata (status, tokens, timing, etc.).
func (s *JobService) getJobResultsFromStorage(ctx context.Context, job *models.Job) ([]*models.JobResult, error) {
	if s.storageSvc == nil || !s.storageSvc.IsEnabled() {
		s.logger.Debug("storage not enabled, returning metadata-only results",
			"job_id", job.ID,
			"job_type", job.Type,
		)
		// Fall back to database metadata (without extracted data)
		if job.Type == models.JobTypeCrawl {
			return s.repos.JobResult.GetByJobID(ctx, job.ID)
		}
		return []*models.JobResult{}, nil
	}

	// Only completed jobs have results in storage
	if job.Status != models.JobStatusCompleted {
		// For incomplete jobs, return metadata from database
		if job.Type == models.JobTypeCrawl {
			return s.repos.JobResult.GetByJobID(ctx, job.ID)
		}
		return []*models.JobResult{}, nil
	}

	// Fetch full results from S3
	storageResults, err := s.storageSvc.GetJobResults(ctx, job.ID)
	if err != nil {
		s.logger.Warn("failed to get job results from storage, falling back to metadata",
			"job_id", job.ID,
			"job_type", job.Type,
			"error", err,
		)
		// Fall back to database metadata
		if job.Type == models.JobTypeCrawl {
			return s.repos.JobResult.GetByJobID(ctx, job.ID)
		}
		return []*models.JobResult{}, nil
	}

	// For extract jobs, convert S3 format directly to JobResult
	if job.Type == models.JobTypeExtract {
		results := make([]*models.JobResult, 0, len(storageResults.Results))
		for _, r := range storageResults.Results {
			results = append(results, &models.JobResult{
				ID:          r.ID,
				JobID:       job.ID,
				URL:         r.URL,
				DataJSON:    string(r.Data),
				CrawlStatus: models.CrawlStatusCompleted,
				CreatedAt:   r.CreatedAt,
			})
		}
		return results, nil
	}

	// For crawl jobs, fetch metadata from database and merge with S3 data
	metadataResults, err := s.repos.JobResult.GetByJobID(ctx, job.ID)
	if err != nil {
		s.logger.Warn("failed to get job metadata from database",
			"job_id", job.ID,
			"error", err,
		)
		// Return S3 data without metadata
		results := make([]*models.JobResult, 0, len(storageResults.Results))
		for _, r := range storageResults.Results {
			results = append(results, &models.JobResult{
				ID:          r.ID,
				JobID:       job.ID,
				URL:         r.URL,
				DataJSON:    string(r.Data),
				CrawlStatus: models.CrawlStatusCompleted,
				CreatedAt:   r.CreatedAt,
			})
		}
		return results, nil
	}

	// Build a map of URL -> S3 data for efficient lookup
	s3DataByURL := make(map[string]json.RawMessage, len(storageResults.Results))
	for _, r := range storageResults.Results {
		s3DataByURL[r.URL] = r.Data
	}

	// Merge S3 data into metadata results
	for _, result := range metadataResults {
		if data, ok := s3DataByURL[result.URL]; ok {
			result.DataJSON = string(data)
		}
	}

	return metadataResults, nil
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
