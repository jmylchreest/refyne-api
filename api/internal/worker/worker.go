package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/jmylchreest/refyne-api/internal/constants"
	"github.com/jmylchreest/refyne-api/internal/models"
	"github.com/jmylchreest/refyne-api/internal/repository"
	"github.com/jmylchreest/refyne-api/internal/service"
)

// Worker processes background jobs.
type Worker struct {
	jobRepo             repository.JobRepository
	jobResultRepo       repository.JobResultRepository
	extractionSvc       *service.ExtractionService
	webhookSvc          *service.WebhookService
	storageSvc          *service.StorageService
	sitemapSvc          *service.SitemapService
	basePollInterval    time.Duration // Base poll interval (reset to this after finding a job)
	maxPollInterval     time.Duration // Maximum backoff interval
	concurrency         int
	shutdownGracePeriod time.Duration
	stop                chan struct{}
	wg                  sync.WaitGroup
	activeJobs          int64 // Number of jobs currently being processed
	activeJobsMu        sync.Mutex
	logger              *slog.Logger
}

// Config holds worker configuration.
type Config struct {
	PollInterval        time.Duration // Base poll interval (minimum, reset after finding a job)
	MaxPollInterval     time.Duration // Maximum poll interval for backoff (default 30s)
	Concurrency         int
	ShutdownGracePeriod time.Duration // Max time to wait for running jobs during shutdown
}

// New creates a new worker.
func New(
	jobRepo repository.JobRepository,
	jobResultRepo repository.JobResultRepository,
	extractionSvc *service.ExtractionService,
	webhookSvc *service.WebhookService,
	storageSvc *service.StorageService,
	sitemapSvc *service.SitemapService,
	cfg Config,
	logger *slog.Logger,
) *Worker {
	if cfg.PollInterval == 0 {
		cfg.PollInterval = 1 * time.Second // Reduced base for faster response when jobs exist
	}
	if cfg.MaxPollInterval == 0 {
		cfg.MaxPollInterval = 30 * time.Second // Max backoff when idle
	}
	if cfg.Concurrency == 0 {
		cfg.Concurrency = 3
	}
	if cfg.ShutdownGracePeriod == 0 {
		cfg.ShutdownGracePeriod = 5 * time.Minute
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Worker{
		jobRepo:             jobRepo,
		jobResultRepo:       jobResultRepo,
		extractionSvc:       extractionSvc,
		webhookSvc:          webhookSvc,
		storageSvc:          storageSvc,
		sitemapSvc:          sitemapSvc,
		basePollInterval:    cfg.PollInterval,
		maxPollInterval:     cfg.MaxPollInterval,
		concurrency:         cfg.Concurrency,
		shutdownGracePeriod: cfg.ShutdownGracePeriod,
		stop:                make(chan struct{}),
		logger:              logger.With("component", "worker"),
	}
}

// Start begins processing jobs.
func (w *Worker) Start(ctx context.Context) {
	w.logger.Info("starting",
		"concurrency", w.concurrency,
		"base_poll_interval", w.basePollInterval,
		"max_poll_interval", w.maxPollInterval,
		"shutdown_grace_period", w.shutdownGracePeriod,
	)

	// Start concurrent workers
	for i := 0; i < w.concurrency; i++ {
		w.wg.Add(1)
		go w.runWorker(ctx, i)
	}
}

// ActiveJobs returns the number of jobs currently being processed.
func (w *Worker) ActiveJobs() int64 {
	w.activeJobsMu.Lock()
	defer w.activeJobsMu.Unlock()
	return w.activeJobs
}

// Stop gracefully stops the worker, waiting for active jobs to complete.
func (w *Worker) Stop() {
	w.logger.Info("stopping, waiting for active jobs to complete", "grace_period", w.shutdownGracePeriod)
	close(w.stop)

	// Wait for active jobs to complete (up to grace period)
	deadline := time.Now().Add(w.shutdownGracePeriod)
	pollInterval := 500 * time.Millisecond

	for time.Now().Before(deadline) {
		w.activeJobsMu.Lock()
		active := w.activeJobs
		w.activeJobsMu.Unlock()

		if active == 0 {
			w.logger.Info("all active jobs completed")
			break
		}

		w.logger.Info("waiting for active jobs", "active_jobs", active, "remaining", time.Until(deadline).Round(time.Second))
		time.Sleep(pollInterval)
	}

	// Final check - warn if jobs still running after grace period
	w.activeJobsMu.Lock()
	remaining := w.activeJobs
	w.activeJobsMu.Unlock()
	if remaining > 0 {
		w.logger.Warn("shutdown grace period exceeded, some jobs may be interrupted", "remaining_jobs", remaining)
	}

	w.wg.Wait()
	w.logger.Info("stopped")
}

func (w *Worker) runWorker(ctx context.Context, workerID int) {
	defer w.wg.Done()

	// Adaptive polling: start at base interval, back off when idle, reset on job found
	currentInterval := w.basePollInterval
	timer := time.NewTimer(currentInterval)
	defer timer.Stop()

	for {
		select {
		case <-w.stop:
			return
		case <-ctx.Done():
			return
		case <-timer.C:
			found := w.processNextJob(ctx, workerID)
			if found {
				// Job found - reset to base interval for quick pickup of next job
				currentInterval = w.basePollInterval
			} else {
				// No job found - exponential backoff (double interval, up to max)
				currentInterval = currentInterval * 2
				if currentInterval > w.maxPollInterval {
					currentInterval = w.maxPollInterval
				}
			}
			timer.Reset(currentInterval)
		}
	}
}

// processNextJob claims and processes the next available job.
// Returns true if a job was found and processed, false otherwise.
func (w *Worker) processNextJob(ctx context.Context, workerID int) bool {
	// Build tier limits from constants for priority scheduling
	tierLimits := repository.TierJobLimits{
		constants.TierFree:       constants.GetTierLimits(constants.TierFree).MaxConcurrentJobs,
		constants.TierStandard:   constants.GetTierLimits(constants.TierStandard).MaxConcurrentJobs,
		constants.TierPro:        constants.GetTierLimits(constants.TierPro).MaxConcurrentJobs,
		constants.TierSelfHosted: constants.GetTierLimits(constants.TierSelfHosted).MaxConcurrentJobs,
	}

	// Claim a pending job with priority scheduling and per-user limits
	job, err := w.jobRepo.ClaimPendingWithLimits(ctx, tierLimits)
	if err != nil {
		w.logger.Error("failed to claim job", "worker_id", workerID, "error", err)
		return false
	}
	if job == nil {
		return false // No pending jobs or all eligible users at limit
	}

	// Track active job
	w.activeJobsMu.Lock()
	w.activeJobs++
	w.activeJobsMu.Unlock()
	defer func() {
		w.activeJobsMu.Lock()
		w.activeJobs--
		w.activeJobsMu.Unlock()
	}()

	w.logger.Info("processing job", "worker_id", workerID, "job_id", job.ID, "type", job.Type)

	// Process based on job type
	switch job.Type {
	case models.JobTypeExtract:
		w.processExtractJob(ctx, job)
	case models.JobTypeCrawl:
		w.processCrawlJob(ctx, job)
	default:
		w.failJob(ctx, job, "unknown job type")
	}

	return true
}

func (w *Worker) processExtractJob(ctx context.Context, job *models.Job) {
	result, err := w.extractionSvc.Extract(ctx, job.UserID, service.ExtractInput{
		URL:       job.URL,
		Schema:    json.RawMessage(job.SchemaJSON),
		FetchMode: "auto",
	})
	if err != nil {
		w.failJob(ctx, job, err.Error())
		return
	}

	// Update job with results
	resultData, err := json.Marshal(result.Data)
	if err != nil {
		w.logger.Error("failed to marshal job result data", "job_id", job.ID, "error", err)
		w.failJob(ctx, job, "internal error: failed to marshal result")
		return
	}
	now := time.Now()
	job.Status = models.JobStatusCompleted
	job.PageCount = 1
	job.TokenUsageInput = result.Usage.InputTokens
	job.TokenUsageOutput = result.Usage.OutputTokens
	job.CostUSD = result.Usage.CostUSD
	job.ResultJSON = string(resultData)
	job.CompletedAt = &now

	if err := w.jobRepo.Update(ctx, job); err != nil {
		w.logger.Error("failed to update job", "job_id", job.ID, "error", err)
	}

	// Store debug capture if enabled
	if job.CaptureDebug && result.RawContent != "" && w.storageSvc != nil && w.storageSvc.IsEnabled() {
		capture := service.LLMRequestCapture{
			ID:        ulid.Make().String(),
			URL:       result.URL,
			Timestamp: now,
			JobType:   string(job.Type),
			Request: service.LLMRequestMeta{
				Provider:    result.Metadata.Provider,
				Model:       result.Metadata.Model,
				ContentSize: len(result.RawContent),
			},
			Response: service.LLMResponseMeta{
				InputTokens:  result.Usage.InputTokens,
				OutputTokens: result.Usage.OutputTokens,
				DurationMs:   int64(result.Metadata.ExtractDurationMs),
				Success:      true,
			},
			RawContent: result.RawContent,
			Schema:     job.SchemaJSON,
		}
		jobDebugCapture := &service.JobDebugCapture{
			JobID:    job.ID,
			Enabled:  true,
			Captures: []service.LLMRequestCapture{capture},
		}
		if err := w.storageSvc.StoreDebugCapture(ctx, jobDebugCapture); err != nil {
			w.logger.Error("failed to store debug capture", "job_id", job.ID, "error", err)
		} else {
			w.logger.Info("stored debug capture", "job_id", job.ID)
		}
	}

	// Send webhook if configured
	if job.WebhookURL != "" {
		ephemeralConfig := &service.WebhookConfig{
			URL:    job.WebhookURL,
			Events: []string{"*"},
		}
		w.webhookSvc.SendForJob(ctx, job.UserID, string(models.WebhookEventJobCompleted), job.ID, map[string]any{
			"job_id": job.ID,
			"status": "completed",
			"data":   result.Data,
		}, ephemeralConfig)
	}

	w.logger.Info("completed job", "job_id", job.ID)
}

func (w *Worker) processCrawlJob(ctx context.Context, job *models.Job) {
	// Parse crawl options (including cleaner chain)
	var options service.CrawlOptions
	if job.CrawlOptionsJSON != "" {
		_ = json.Unmarshal([]byte(job.CrawlOptionsJSON), &options)
	}

	// If using sitemap, discover URLs from sitemap.xml
	var sitemapURLs []string
	if options.UseSitemap && w.sitemapSvc != nil {
		w.logger.Info("discovering URLs from sitemap", "job_id", job.ID, "url", job.URL)
		urls, found := w.sitemapSvc.TrySitemapDiscovery(ctx, job.URL, options.FollowPattern)
		if found && len(urls) > 0 {
			sitemapURLs = urls
			w.logger.Info("discovered URLs from sitemap",
				"job_id", job.ID,
				"url_count", len(urls),
			)

			// Update job with total URLs queued for progress tracking
			job.URLsQueued = len(urls)
			if err := w.jobRepo.Update(ctx, job); err != nil {
				w.logger.Error("failed to update job with urls_queued", "job_id", job.ID, "error", err)
			}

			// Sitemap mode is "batch single-page extraction" - disable link following
			// We only extract from sitemap URLs, not from discovered links
			options.MaxDepth = 0
			options.FollowSelector = ""
			options.ExtractFromSeeds = true // Extract from all sitemap URLs
			w.logger.Debug("sitemap mode: disabled link following",
				"job_id", job.ID,
				"max_depth", options.MaxDepth,
			)
		} else {
			w.logger.Warn("sitemap discovery returned no URLs, falling back to CSS selector discovery",
				"job_id", job.ID,
			)
		}
	}

	// Set discovery method based on how URLs were found
	if len(sitemapURLs) > 0 {
		job.DiscoveryMethod = "sitemap"
	} else {
		job.DiscoveryMethod = "links"
	}
	// Persist the discovery method early (it's useful for tracking)
	if err := w.jobRepo.Update(ctx, job); err != nil {
		w.logger.Error("failed to update job discovery_method", "job_id", job.ID, "error", err)
	}

	// Set defaults for crawl mode (only if not using sitemap)
	if options.MaxDepth == 0 && len(sitemapURLs) == 0 {
		options.MaxDepth = 2 // Default to following links one level from seed
	}
	// Note: MaxPages == 0 means no limit, don't override it
	if options.MaxURLs == 0 {
		options.MaxURLs = 50
	}
	if options.Concurrency == 0 {
		options.Concurrency = 3
	}
	// SameDomainOnly defaults to true (only follow links on same domain)
	// Note: Go zero value for bool is false, so we check if it wasn't explicitly set
	// by checking if the JSON had any content at all
	if job.CrawlOptionsJSON == "" || !strings.Contains(job.CrawlOptionsJSON, "same_domain_only") {
		options.SameDomainOnly = true
	}

	// Track page count incrementally for SSE updates
	var pageCountMu sync.Mutex
	pageCount := 0

	// Collect debug captures if enabled
	var capturesMu sync.Mutex
	var debugCaptures []service.LLMRequestCapture

	// Callback to save each result incrementally for SSE streaming
	// Note: We only save metadata to job_results for progress tracking.
	// Full extracted data is accumulated and saved to S3 on completion.
	resultCallback := func(pageResult service.PageResult) error {
		now := time.Now()

		// Determine crawl status based on error
		crawlStatus := models.CrawlStatusCompleted
		if pageResult.Error != "" {
			crawlStatus = models.CrawlStatusFailed
		}

		// Count ALL processed pages (successful + failed) for total visibility
		pageCountMu.Lock()
		pageCount++
		currentCount := pageCount
		pageCountMu.Unlock()

		// Update job's page count in database for SSE polling
		job.PageCount = currentCount
		if err := w.jobRepo.Update(ctx, job); err != nil {
			w.logger.Error("failed to update job page count", "job_id", job.ID, "error", err)
		}

		// Save metadata only to job_results (no DataJSON - that goes to S3)
		jobResult := &models.JobResult{
			ID:                ulid.Make().String(),
			JobID:             job.ID,
			URL:               pageResult.URL,
			ParentURL:         pageResult.ParentURL,
			Depth:             pageResult.Depth,
			CrawlStatus:       crawlStatus,
			// DataJSON removed - full results are stored in S3 on completion
			ErrorMessage:      pageResult.Error,
			ErrorDetails:      pageResult.ErrorDetails,
			ErrorCategory:     pageResult.ErrorCategory,
			LLMProvider:       pageResult.LLMProvider,
			LLMModel:          pageResult.LLMModel,
			IsBYOK:            pageResult.IsBYOK,
			RetryCount:        pageResult.RetryCount,
			TokenUsageInput:   pageResult.TokenUsageInput,
			TokenUsageOutput:  pageResult.TokenUsageOutput,
			FetchDurationMs:   pageResult.FetchDurationMs,
			ExtractDurationMs: pageResult.ExtractDurationMs,
			DiscoveredAt:      &now,
			CompletedAt:       &now,
			CreatedAt:         now,
		}

		if err := w.jobResultRepo.Create(ctx, jobResult); err != nil {
			w.logger.Error("failed to save job result", "job_id", job.ID, "url", pageResult.URL, "error", err)
		}

		// Collect debug capture if enabled
		if job.CaptureDebug && pageResult.RawContent != "" {
			capture := service.LLMRequestCapture{
				ID:        ulid.Make().String(),
				URL:       pageResult.URL,
				Timestamp: now,
				JobType:   string(job.Type),
				Request: service.LLMRequestMeta{
					Provider:    pageResult.LLMProvider,
					Model:       pageResult.LLMModel,
					ContentSize: len(pageResult.RawContent),
				},
				Response: service.LLMResponseMeta{
					InputTokens:  pageResult.TokenUsageInput,
					OutputTokens: pageResult.TokenUsageOutput,
					DurationMs:   int64(pageResult.ExtractDurationMs),
					Success:      pageResult.Error == "",
					Error:        pageResult.Error,
				},
				RawContent: pageResult.RawContent,
				Schema:     job.SchemaJSON,
			}
			capturesMu.Lock()
			debugCaptures = append(debugCaptures, capture)
			capturesMu.Unlock()
		}

		return nil
	}

	// Callback for when URLs are queued (for progress tracking)
	urlsQueuedCallback := func(queuedCount int) {
		job.URLsQueued = queuedCount
		if err := w.jobRepo.Update(ctx, job); err != nil {
			w.logger.Error("failed to update urls_queued", "job_id", job.ID, "error", err)
		}
		w.logger.Debug("urls queued updated", "job_id", job.ID, "urls_queued", queuedCount)
	}

	// Deserialize LLM configs from job
	var llmConfigs []*service.LLMConfigInput
	if job.LLMConfigsJSON != "" {
		if err := json.Unmarshal([]byte(job.LLMConfigsJSON), &llmConfigs); err != nil {
			w.logger.Error("failed to parse LLM configs", "job_id", job.ID, "error", err)
			w.failJob(ctx, job, "invalid LLM configuration")
			return
		}
	}
	if len(llmConfigs) == 0 {
		w.failJob(ctx, job, "no LLM configuration found")
		return
	}

	result, err := w.extractionSvc.CrawlWithCallback(ctx, job.UserID, service.CrawlInput{
		JobID:        job.ID,
		URL:          job.URL,
		SeedURLs:     sitemapURLs, // URLs from sitemap discovery (empty if not using sitemap)
		Schema:       json.RawMessage(job.SchemaJSON),
		LLMConfigs:   llmConfigs,  // Pre-resolved config chain from job creation
		Tier:         job.Tier,    // User's tier at job creation time
		IsBYOK:       job.IsBYOK,  // Whether using user's own API keys
		CleanerChain: options.CleanerChain, // Content cleaner chain from job creation
		Options: service.CrawlOptions{
			FollowSelector:   options.FollowSelector,
			FollowPattern:    options.FollowPattern,
			MaxDepth:         options.MaxDepth,
			NextSelector:     options.NextSelector,
			MaxPages:         options.MaxPages,
			MaxURLs:          options.MaxURLs,
			Delay:            options.Delay,
			Concurrency:      options.Concurrency,
			SameDomainOnly:   options.SameDomainOnly,
			ExtractFromSeeds: options.ExtractFromSeeds,
			UseSitemap:       options.UseSitemap,
		},
	}, service.CrawlCallbacks{
		OnResult:     resultCallback,
		OnURLsQueued: urlsQueuedCallback,
	})
	if err != nil {
		w.failJob(ctx, job, err.Error())
		return
	}

	// Update job with final results
	resultData, _ := json.Marshal(result.Results)
	completedAt := time.Now()
	job.Status = models.JobStatusCompleted
	job.PageCount = result.PageCount
	job.TokenUsageInput = result.TotalTokensInput
	job.TokenUsageOutput = result.TotalTokensOutput
	job.CostUSD = result.TotalCostUSD
	job.LLMCostUSD = result.TotalLLMCostUSD
	job.LLMProvider = result.LLMProvider
	job.LLMModel = result.LLMModel
	job.ResultJSON = string(resultData)
	job.CompletedAt = &completedAt

	// Handle early stop scenarios (e.g., insufficient balance)
	if result.StoppedEarly {
		switch result.StopReason {
		case "insufficient_balance":
			job.ErrorMessage = fmt.Sprintf("Crawl stopped early: insufficient balance after %d pages. Partial results are available.", result.PageCount)
			job.ErrorCategory = "insufficient_balance"
		case "callback_error":
			job.ErrorMessage = fmt.Sprintf("Crawl stopped early after %d pages due to processing error.", result.PageCount)
			job.ErrorCategory = "processing_error"
		default:
			job.ErrorMessage = fmt.Sprintf("Crawl stopped early after %d pages.", result.PageCount)
			job.ErrorCategory = "early_stop"
		}
		w.logger.Warn("crawl stopped early",
			"job_id", job.ID,
			"reason", result.StopReason,
			"pages_completed", result.PageCount,
		)
	}

	if err := w.jobRepo.Update(ctx, job); err != nil {
		w.logger.Error("failed to update job", "job_id", job.ID, "error", err)
	}

	// Store results to object storage (Tigris) for later retrieval
	if w.storageSvc != nil && w.storageSvc.IsEnabled() {
		jobResults := &service.JobResults{
			JobID:       job.ID,
			UserID:      job.UserID,
			Status:      string(job.Status),
			TotalPages:  result.PageCount,
			CompletedAt: completedAt,
			Results:     make([]service.JobResultData, 0, len(result.PageResults)),
		}

		for _, pageResult := range result.PageResults {
			// Data is already processed by the service layer (URLs resolved, etc.)
			dataJSON, _ := json.Marshal(pageResult.Data)
			jobResults.Results = append(jobResults.Results, service.JobResultData{
				ID:        pageResult.URL, // Use URL as ID for now
				URL:       pageResult.URL,
				Data:      dataJSON,
				CreatedAt: completedAt,
			})
		}

		if err := w.storageSvc.StoreJobResults(ctx, jobResults); err != nil {
			w.logger.Error("failed to store job results to object storage", "job_id", job.ID, "error", err)
		}

		// Store debug captures if enabled and we have any
		if job.CaptureDebug && len(debugCaptures) > 0 {
			jobDebugCapture := &service.JobDebugCapture{
				JobID:    job.ID,
				Enabled:  true,
				Captures: debugCaptures,
			}
			if err := w.storageSvc.StoreDebugCapture(ctx, jobDebugCapture); err != nil {
				w.logger.Error("failed to store debug captures", "job_id", job.ID, "error", err)
			} else {
				w.logger.Info("stored debug captures",
					"job_id", job.ID,
					"capture_count", len(debugCaptures),
				)
			}
		}
	}

	// Send webhook if configured
	if job.WebhookURL != "" {
		ephemeralConfig := &service.WebhookConfig{
			URL:    job.WebhookURL,
			Events: []string{"*"},
		}
		w.webhookSvc.SendForJob(ctx, job.UserID, string(models.WebhookEventJobCompleted), job.ID, map[string]any{
			"job_id":       job.ID,
			"status":       "completed",
			"page_count":   result.PageCount,
			"results":      result.Results,
			"cost_usd":     result.TotalCostUSD,
		}, ephemeralConfig)
	}

	w.logger.Info("completed crawl job", "job_id", job.ID, "page_count", result.PageCount)
}

func (w *Worker) failJob(ctx context.Context, job *models.Job, errMsg string) {
	now := time.Now()
	job.Status = models.JobStatusFailed
	job.ErrorMessage = errMsg
	job.CompletedAt = &now

	if err := w.jobRepo.Update(ctx, job); err != nil {
		w.logger.Error("failed to update job", "job_id", job.ID, "error", err)
	}

	// Send webhook if configured
	if job.WebhookURL != "" {
		ephemeralConfig := &service.WebhookConfig{
			URL:    job.WebhookURL,
			Events: []string{"*"},
		}
		w.webhookSvc.SendForJob(ctx, job.UserID, string(models.WebhookEventJobFailed), job.ID, map[string]any{
			"job_id": job.ID,
			"status": "failed",
			"error":  errMsg,
		}, ephemeralConfig)
	}

	w.logger.Error("job failed", "job_id", job.ID, "error", errMsg)
}
