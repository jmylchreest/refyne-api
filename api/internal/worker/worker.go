package worker

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/jmylchreest/refyne-api/internal/models"
	"github.com/jmylchreest/refyne-api/internal/repository"
	"github.com/jmylchreest/refyne-api/internal/service"
)

// Worker processes background jobs.
type Worker struct {
	jobRepo       repository.JobRepository
	extractionSvc *service.ExtractionService
	webhookSvc    *service.WebhookService
	pollInterval  time.Duration
	concurrency   int
	stop          chan struct{}
	wg            sync.WaitGroup
	logger        *slog.Logger
}

// Config holds worker configuration.
type Config struct {
	PollInterval time.Duration
	Concurrency  int
}

// New creates a new worker.
func New(
	jobRepo repository.JobRepository,
	extractionSvc *service.ExtractionService,
	webhookSvc *service.WebhookService,
	cfg Config,
	logger *slog.Logger,
) *Worker {
	if cfg.PollInterval == 0 {
		cfg.PollInterval = 5 * time.Second
	}
	if cfg.Concurrency == 0 {
		cfg.Concurrency = 3
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Worker{
		jobRepo:       jobRepo,
		extractionSvc: extractionSvc,
		webhookSvc:    webhookSvc,
		pollInterval:  cfg.PollInterval,
		concurrency:   cfg.Concurrency,
		stop:          make(chan struct{}),
		logger:        logger.With("component", "worker"),
	}
}

// Start begins processing jobs.
func (w *Worker) Start(ctx context.Context) {
	w.logger.Info("starting", "concurrency", w.concurrency)

	// Start concurrent workers
	for i := 0; i < w.concurrency; i++ {
		w.wg.Add(1)
		go w.runWorker(ctx, i)
	}
}

// Stop gracefully stops the worker.
func (w *Worker) Stop() {
	w.logger.Info("stopping")
	close(w.stop)
	w.wg.Wait()
	w.logger.Info("stopped")
}

func (w *Worker) runWorker(ctx context.Context, workerID int) {
	defer w.wg.Done()

	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.stop:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.processNextJob(ctx, workerID)
		}
	}
}

func (w *Worker) processNextJob(ctx context.Context, workerID int) {
	// Claim a pending job
	job, err := w.jobRepo.ClaimPending(ctx)
	if err != nil {
		w.logger.Error("failed to claim job", "worker_id", workerID, "error", err)
		return
	}
	if job == nil {
		return // No pending jobs
	}

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
	resultData, _ := json.Marshal(result.Data)
	now := time.Now()
	job.Status = models.JobStatusCompleted
	job.PageCount = 1
	job.TokenUsageInput = result.Usage.InputTokens
	job.TokenUsageOutput = result.Usage.OutputTokens
	// Convert USD cost to credits (1 credit = $0.01) for backwards compatibility
	job.CostCredits = int(result.Usage.CostUSD * 100)
	job.ResultJSON = string(resultData)
	job.CompletedAt = &now

	if err := w.jobRepo.Update(ctx, job); err != nil {
		w.logger.Error("failed to update job", "job_id", job.ID, "error", err)
	}

	// Send webhook if configured
	if job.WebhookURL != "" {
		w.webhookSvc.Send(ctx, job.WebhookURL, map[string]any{
			"job_id": job.ID,
			"status": "completed",
			"data":   result.Data,
		})
	}

	w.logger.Info("completed job", "job_id", job.ID)
}

func (w *Worker) processCrawlJob(ctx context.Context, job *models.Job) {
	// Parse crawl options
	var options struct {
		FollowSelector   string `json:"follow_selector"`
		FollowPattern    string `json:"follow_pattern"`
		MaxDepth         int    `json:"max_depth"`
		NextSelector     string `json:"next_selector"`
		MaxPages         int    `json:"max_pages"`
		MaxURLs          int    `json:"max_urls"`
		Delay            string `json:"delay"`
		Concurrency      int    `json:"concurrency"`
		SameDomainOnly   bool   `json:"same_domain_only"`
		ExtractFromSeeds bool   `json:"extract_from_seeds"`
	}
	if job.CrawlOptionsJSON != "" {
		json.Unmarshal([]byte(job.CrawlOptionsJSON), &options)
	}

	// Set defaults
	if options.MaxDepth == 0 {
		options.MaxDepth = 1
	}
	if options.MaxPages == 0 {
		options.MaxPages = 10
	}
	if options.MaxURLs == 0 {
		options.MaxURLs = 50
	}
	if options.Concurrency == 0 {
		options.Concurrency = 3
	}

	result, err := w.extractionSvc.Crawl(ctx, job.UserID, service.CrawlInput{
		URL:    job.URL,
		Schema: json.RawMessage(job.SchemaJSON),
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
		},
	})
	if err != nil {
		w.failJob(ctx, job, err.Error())
		return
	}

	// Update job with results
	resultData, _ := json.Marshal(result.Results)
	now := time.Now()
	job.Status = models.JobStatusCompleted
	job.PageCount = result.PageCount
	job.TokenUsageInput = result.TotalTokensInput
	job.TokenUsageOutput = result.TotalTokensOutput
	job.CostCredits = result.TotalCredits
	job.ResultJSON = string(resultData)
	job.CompletedAt = &now

	if err := w.jobRepo.Update(ctx, job); err != nil {
		w.logger.Error("failed to update job", "job_id", job.ID, "error", err)
	}

	// Send webhook if configured
	if job.WebhookURL != "" {
		w.webhookSvc.Send(ctx, job.WebhookURL, map[string]any{
			"job_id":     job.ID,
			"status":     "completed",
			"page_count": result.PageCount,
			"results":    result.Results,
			"total_cost": result.TotalCredits,
		})
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
		w.webhookSvc.Send(ctx, job.WebhookURL, map[string]any{
			"job_id": job.ID,
			"status": "failed",
			"error":  errMsg,
		})
	}

	w.logger.Error("job failed", "job_id", job.ID, "error", errMsg)
}
