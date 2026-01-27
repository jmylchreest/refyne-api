package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"

	"github.com/jmylchreest/refyne-api/internal/constants"
	"github.com/jmylchreest/refyne-api/internal/http/mw"
	"github.com/jmylchreest/refyne-api/internal/models"
	"github.com/jmylchreest/refyne-api/internal/service"
)

// JobHandler handles job endpoints.
type JobHandler struct {
	jobSvc      *service.JobService
	storageSvc  *service.StorageService
	webhookSvc  *service.WebhookService
	resolver    *service.LLMConfigResolver
}

// NewJobHandler creates a new job handler.
func NewJobHandler(jobSvc *service.JobService, storageSvc *service.StorageService, resolver *service.LLMConfigResolver) *JobHandler {
	return &JobHandler{
		jobSvc:     jobSvc,
		storageSvc: storageSvc,
		resolver:   resolver,
	}
}

// NewJobHandlerWithWebhook creates a new job handler with webhook service.
// Note: This handler is for read operations (list/get jobs) and doesn't need the resolver.
// Use NewJobHandler for job creation which requires config resolution.
func NewJobHandlerWithWebhook(jobSvc *service.JobService, storageSvc *service.StorageService, webhookSvc *service.WebhookService) *JobHandler {
	return &JobHandler{
		jobSvc:     jobSvc,
		storageSvc: storageSvc,
		webhookSvc: webhookSvc,
	}
}

// CreateCrawlJobInput represents crawl job creation request.
//
// The crawl endpoint supports two modes:
//
// **Async Mode (default):** Returns immediately with a job_id. Use SSE streaming,
// polling, or webhooks to get results.
//
// **Sync Mode (wait=true):** Blocks until the job completes (up to 2 minutes max)
// and returns merged results directly. Ideal for SDK/CLI clients. For longer jobs,
// use async mode with webhooks.
//
// Example (async): POST /api/v1/crawl
// Example (sync):  POST /api/v1/crawl?wait=true
// CrawlWebhookHeaderInput represents a custom header in webhook requests.
type CrawlWebhookHeaderInput struct {
	Name  string `json:"name" minLength:"1" maxLength:"256" doc:"Header name"`
	Value string `json:"value" maxLength:"4096" doc:"Header value"`
}

// CrawlInlineWebhookInput represents an ephemeral webhook configuration for crawl jobs.
type CrawlInlineWebhookInput struct {
	URL     string                    `json:"url" format:"uri" minLength:"1" doc:"Webhook URL"`
	Secret  string                    `json:"secret,omitempty" maxLength:"256" doc:"Secret for HMAC-SHA256 signature"`
	Events  []string                  `json:"events,omitempty" doc:"Event types to subscribe to (empty for all)"`
	Headers []CrawlWebhookHeaderInput `json:"headers,omitempty" maxItems:"10" doc:"Custom headers"`
}

// JobCleanerConfigInput represents a cleaner in the chain (duplicated to avoid import cycle).
type JobCleanerConfigInput struct {
	Name    string                  `json:"name" minLength:"1" doc:"Cleaner name (noop, refyne)"`
	Options *JobCleanerOptionsInput `json:"options,omitempty" doc:"Cleaner-specific options"`
}

// JobCleanerOptionsInput represents cleaner configuration options for refyne cleaner.
type JobCleanerOptionsInput struct {
	// Output format
	Output  string `json:"output,omitempty" enum:"html,text,markdown" default:"html" doc:"Output format: html, text, or markdown"`
	BaseURL string `json:"base_url,omitempty" doc:"Base URL for resolving relative links"`

	// Preset and selectors
	Preset          string   `json:"preset,omitempty" enum:"default,minimal,aggressive" doc:"Preset: default, minimal, or aggressive"`
	RemoveSelectors []string `json:"remove_selectors,omitempty" doc:"CSS selectors for elements to remove"`
	KeepSelectors   []string `json:"keep_selectors,omitempty" doc:"CSS selectors for elements to always keep"`

	// Markdown output options
	IncludeFrontmatter bool `json:"include_frontmatter,omitempty" doc:"Prepend YAML frontmatter with metadata (markdown output)"`
	ExtractImages      bool `json:"extract_images,omitempty" doc:"Extract images to frontmatter with {{IMG_001}} placeholders (markdown)"`
	ExtractHeadings    bool `json:"extract_headings,omitempty" doc:"Extract heading structure to frontmatter (markdown)"`
	ResolveURLs        bool `json:"resolve_urls,omitempty" doc:"Resolve relative URLs to absolute using base_url"`
}

type CreateCrawlJobInput struct {
	Wait    bool `query:"wait" default:"false" doc:"Block until job completes and return results directly. Max wait time is 2 minutes. Returns 202 if timeout exceeded."`
	Timeout int  `query:"timeout" default:"120" minimum:"10" maximum:"120" doc:"Maximum seconds to wait when wait=true (default 120s, max 120s/2min). For longer jobs, use async mode."`
	Body    struct {
		URL          string                  `json:"url" minLength:"1" example:"https://example.com/products" doc:"Seed URL to start crawling from"`
		Schema       json.RawMessage         `json:"schema" minLength:"1" doc:"Extraction instructions - either a structured schema (YAML/JSON with 'name' and 'fields') or freeform natural language prompt. The API auto-detects the format."`
		Options      CrawlOptions            `json:"options,omitempty" doc:"Crawl configuration options"`
		CleanerChain []JobCleanerConfigInput `json:"cleaner_chain,omitempty" doc:"Content cleaner chain (default: [markdown])"`
		WebhookID    string                  `json:"webhook_id,omitempty" doc:"ID of a saved webhook to call on job events"`
		Webhook      *CrawlInlineWebhookInput `json:"webhook,omitempty" doc:"Inline ephemeral webhook configuration"`
		WebhookURL   string                  `json:"webhook_url,omitempty" format:"uri" example:"https://my-app.com/webhook/crawl-complete" doc:"Simple webhook URL (backward compatible)"`
		LLMConfig    *LLMConfigInput         `json:"llm_config,omitempty" doc:"Optional LLM configuration override (BYOK)"`
	}
}

// CrawlOptions represents crawl configuration options.
//
// The crawler discovers URLs by:
// 1. Fetching sitemap.xml if use_sitemap is enabled
// 2. Extracting links matching follow_selector CSS selectors
// 3. Filtering URLs by follow_pattern regex (if provided)
// 4. Respecting max_pages, max_depth, and same_domain_only limits
type CrawlOptions struct {
	FollowSelector   string `json:"follow_selector,omitempty" example:"a.product-link, a[href*='/product/']" doc:"CSS selector(s) for links to follow. Comma-separated or newline-separated."`
	FollowPattern    string `json:"follow_pattern,omitempty" example:"/product/.*|/item/.*" doc:"Regex pattern to filter URLs. Only matching URLs are crawled."`
	MaxDepth         int    `json:"max_depth,omitempty" default:"1" maximum:"5" example:"2" doc:"Maximum crawl depth from seed URL (1 = seed + direct links)"`
	NextSelector     string `json:"next_selector,omitempty" example:"a.pagination-next" doc:"CSS selector for pagination 'next' link"`
	MaxPages         int    `json:"max_pages,omitempty" default:"10" maximum:"100" example:"20" doc:"Maximum total pages to crawl (0 = no limit, up to tier max)"`
	MaxURLs          int    `json:"max_urls,omitempty" default:"50" maximum:"500" example:"100" doc:"Maximum URLs to discover and queue"`
	Delay            string `json:"delay,omitempty" default:"500ms" example:"1s" doc:"Delay between requests (e.g., 500ms, 1s, 2s)"`
	Concurrency      int    `json:"concurrency,omitempty" default:"3" maximum:"10" example:"5" doc:"Concurrent extraction requests"`
	SameDomainOnly   bool   `json:"same_domain_only,omitempty" default:"true" doc:"Only follow links on the same domain as seed URL"`
	ExtractFromSeeds bool   `json:"extract_from_seeds,omitempty" example:"true" doc:"Extract data from the seed URL (not just discovered pages)"`
	UseSitemap       bool   `json:"use_sitemap,omitempty" doc:"Discover URLs from sitemap.xml instead of CSS selectors"`
	FetchMode        string `json:"fetch_mode,omitempty" enum:"auto,static,dynamic" default:"auto" doc:"Page fetching mode: auto (detect and retry with browser if needed), static (fast, Colly-based), dynamic (browser rendering for JS-heavy sites, requires content_dynamic feature)"`
}

// TokenUsage represents LLM token consumption for a job.
type TokenUsage struct {
	Input  int `json:"input" example:"8500" doc:"Total input tokens consumed across all extractions"`
	Output int `json:"output" example:"2100" doc:"Total output tokens generated across all extractions"`
}

// CrawlJobResponseBody is the response body for crawl job creation.
// Fields are populated based on the request mode and job outcome.
type CrawlJobResponseBody struct {
	JobID        string         `json:"job_id" example:"01HXYZ123ABC456DEF789" doc:"Unique job identifier (ULID)"`
	Status       string         `json:"status" example:"completed" doc:"Job status: pending, running, completed, failed"`
	StatusURL    string         `json:"status_url,omitempty" example:"https://api.refyne.uk/api/v1/jobs/01HXYZ123ABC456DEF789" doc:"URL to poll for job status (async mode)"`
	PageCount    int            `json:"page_count,omitempty" example:"5" doc:"Number of pages successfully extracted (sync mode)"`
	Data         map[string]any `json:"data,omitempty" doc:"Merged extraction results from all pages (sync mode, completed only)"`
	TokenUsage   *TokenUsage    `json:"token_usage,omitempty" doc:"Token usage statistics (sync mode)"`
	CostUSD      float64        `json:"cost_usd,omitempty" example:"0.15" doc:"Total USD cost charged (sync mode)"`
	DurationMs   int64          `json:"duration_ms,omitempty" example:"12500" doc:"Total job duration in milliseconds (sync mode)"`
	ErrorMessage string         `json:"error_message,omitempty" doc:"Error message if job failed or timed out"`
}

// CreateCrawlJobOutput represents crawl job creation response.
//
// Response varies by mode and outcome:
//
// **Async Mode (201 Created):**
//
//	{"job_id": "01HXY...", "status": "pending", "status_url": "https://api.refyne.uk/api/v1/jobs/01HXY..."}
//
// **Sync Mode - Completed (200 OK):**
//
//	{"job_id": "01HXY...", "status": "completed", "page_count": 5, "data": {...merged results...}, "token_usage": {...}}
//
// **Sync Mode - Timeout (202 Accepted):**
//
//	{"job_id": "01HXY...", "status": "running", "status_url": "...", "error_message": "timeout - job continues in background"}
type CreateCrawlJobOutput struct {
	Status int                  `header:"Status-Code"`
	Body   CrawlJobResponseBody `json:"body"`
}

// CreateCrawlJob handles crawl job creation.
// Supports both async mode (default) and sync mode (wait=true).
func (h *JobHandler) CreateCrawlJob(ctx context.Context, input *CreateCrawlJobInput) (*CreateCrawlJobOutput, error) {
	// Extract user context from JWT claims
	uc := ExtractUserContext(ctx)
	if !uc.IsAuthenticated() {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	// Resolve LLM config chain at job creation time
	var llmChain *service.LLMConfigChain
	if h.resolver != nil {
		llmChain = h.resolver.ResolveConfigChain(ctx, uc.UserID, nil, uc.Tier, uc.BYOKAllowed, uc.ModelsCustomAllowed)
	}
	if llmChain == nil || llmChain.IsEmpty() {
		return nil, huma.Error500InternalServerError("failed to resolve LLM configuration")
	}

	// Convert cleaner chain from handler input to service input using shared utility
	cleanerChain := ConvertJobCleanerChain(input.Body.CleanerChain)

	result, err := h.jobSvc.CreateCrawlJob(ctx, uc.UserID, service.CreateCrawlJobInput{
		URL:    input.Body.URL,
		Schema: input.Body.Schema,
		Options: service.CrawlOptions{
			FollowSelector:        input.Body.Options.FollowSelector,
			FollowPattern:         input.Body.Options.FollowPattern,
			MaxDepth:              input.Body.Options.MaxDepth,
			NextSelector:          input.Body.Options.NextSelector,
			MaxPages:              input.Body.Options.MaxPages,
			MaxURLs:               input.Body.Options.MaxURLs,
			Delay:                 input.Body.Options.Delay,
			Concurrency:           input.Body.Options.Concurrency,
			SameDomainOnly:        input.Body.Options.SameDomainOnly,
			ExtractFromSeeds:      input.Body.Options.ExtractFromSeeds,
			UseSitemap:            input.Body.Options.UseSitemap,
			FetchMode:             input.Body.Options.FetchMode,
			ContentDynamicAllowed: uc.ContentDynamicAllowed,
			SkipCreditCheck:       uc.SkipCreditCheckAllowed,
		},
		CleanerChain: cleanerChain,
		WebhookURL:   input.Body.WebhookURL,
		LLMConfigs:   llmChain.All(),
		Tier:         uc.Tier,
		IsBYOK:       llmChain.IsBYOK(),
	})
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to create crawl job: " + err.Error())
	}

	// Async mode (default): return immediately with job ID
	if !input.Wait {
		return &CreateCrawlJobOutput{
			Status: http.StatusCreated,
			Body: CrawlJobResponseBody{
				JobID:     result.JobID,
				Status:    result.Status,
				StatusURL: result.StatusURL,
			},
		}, nil
	}

	// Sync mode: wait for job completion with timeout
	// Cap timeout to MaxSyncWaitTimeout to prevent resource exhaustion
	timeout := time.Duration(input.Timeout) * time.Second
	if timeout == 0 || timeout > constants.MaxSyncWaitTimeout {
		timeout = constants.MaxSyncWaitTimeout
	}

	startTime := time.Now()
	deadline := startTime.Add(timeout)
	pollInterval := 1 * time.Second

	var job *models.Job
	for time.Now().Before(deadline) {
		// Check if client disconnected
		select {
		case <-ctx.Done():
			// Client disconnected - job continues in background
			// Return 202 so they know the job_id for polling
			return &CreateCrawlJobOutput{
				Status: http.StatusAccepted,
				Body: CrawlJobResponseBody{
					JobID:     result.JobID,
					Status:    "running",
					StatusURL: result.StatusURL,
				},
			}, nil
		default:
		}

		// Poll job status
		job, err = h.jobSvc.GetJob(ctx, uc.UserID, result.JobID)
		if err != nil {
			time.Sleep(pollInterval)
			continue
		}

		// Check if job is done
		if job.Status == models.JobStatusCompleted || job.Status == models.JobStatusFailed {
			break
		}

		time.Sleep(pollInterval)
	}

	// If we timed out or job is still running, return 202 Accepted
	if job == nil || (job.Status != models.JobStatusCompleted && job.Status != models.JobStatusFailed) {
		return &CreateCrawlJobOutput{
			Status: http.StatusAccepted,
			Body: CrawlJobResponseBody{
				JobID:        result.JobID,
				Status:       "running",
				StatusURL:    result.StatusURL,
				ErrorMessage: "timeout waiting for job completion - job continues in background",
			},
		}, nil
	}

	// Job completed - build sync response with merged data
	durationMs := time.Since(startTime).Milliseconds()
	if job.StartedAt != nil && job.CompletedAt != nil {
		durationMs = job.CompletedAt.Sub(*job.StartedAt).Milliseconds()
	}

	output := &CreateCrawlJobOutput{
		Status: http.StatusOK,
		Body: CrawlJobResponseBody{
			JobID:        job.ID,
			Status:       string(job.Status),
			PageCount:    job.PageCount,
			CostUSD:      job.CostUSD,
			DurationMs:   durationMs,
			ErrorMessage: job.ErrorMessage,
			TokenUsage: &TokenUsage{
				Input:  job.TokenUsageInput,
				Output: job.TokenUsageOutput,
			},
		},
	}

	// For completed jobs, collect results into { items: [...] }
	if job.Status == models.JobStatusCompleted {
		results, err := h.jobSvc.GetJobResults(ctx, uc.UserID, job.ID)
		if err == nil && len(results) > 0 {
			output.Body.Data = collectAllResults(results)
		}
	}

	return output, nil
}

// ListJobsInput represents job listing request.
type ListJobsInput struct {
	Limit  int `query:"limit" default:"20" maximum:"100" doc:"Number of jobs to return"`
	Offset int `query:"offset" default:"0" doc:"Offset for pagination"`
}

// ListJobsOutput represents job listing response.
type ListJobsOutput struct {
	Body struct {
		Jobs []JobResponse `json:"jobs"`
	}
}

// JobResponse represents a job in API responses.
type JobResponse struct {
	ID               string  `json:"id"`
	Type             string  `json:"type"`
	Status           string  `json:"status"`
	URL              string  `json:"url"`
	URLsQueued       int     `json:"urls_queued"` // Total URLs queued for processing (for progress tracking)
	PageCount        int     `json:"page_count"`  // Pages processed so far
	TokenUsageInput  int     `json:"token_usage_input"`
	TokenUsageOutput int     `json:"token_usage_output"`
	CostUSD          float64 `json:"cost_usd"` // Actual USD cost charged (0 for BYOK)
	CaptureDebug     bool    `json:"capture_debug"` // Whether debug capture was enabled
	ErrorMessage     string  `json:"error_message,omitempty"`
	ErrorCategory    string  `json:"error_category,omitempty"`
	StartedAt        string  `json:"started_at,omitempty"`
	CompletedAt      string  `json:"completed_at,omitempty"`
	CreatedAt        string  `json:"created_at"`
}

// ListJobs handles job listing.
func (h *JobHandler) ListJobs(ctx context.Context, input *ListJobsInput) (*ListJobsOutput, error) {
	userID := getUserID(ctx)
	if userID == "" {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	jobs, err := h.jobSvc.ListJobs(ctx, userID, input.Limit, input.Offset)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list jobs: " + err.Error())
	}

	var responses []JobResponse
	for _, job := range jobs {
		resp := JobResponse{
			ID:               job.ID,
			Type:             string(job.Type),
			Status:           string(job.Status),
			URL:              job.URL,
			URLsQueued:       job.URLsQueued,
			PageCount:        job.PageCount,
			TokenUsageInput:  job.TokenUsageInput,
			TokenUsageOutput: job.TokenUsageOutput,
			CostUSD:          job.CostUSD,
			CaptureDebug:     job.CaptureDebug,
			ErrorMessage:     job.ErrorMessage,
			ErrorCategory:    job.ErrorCategory,
			CreatedAt:        job.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
		if job.StartedAt != nil {
			resp.StartedAt = job.StartedAt.Format("2006-01-02T15:04:05Z07:00")
		}
		if job.CompletedAt != nil {
			resp.CompletedAt = job.CompletedAt.Format("2006-01-02T15:04:05Z07:00")
		}
		responses = append(responses, resp)
	}

	return &ListJobsOutput{
		Body: struct {
			Jobs []JobResponse `json:"jobs"`
		}{
			Jobs: responses,
		},
	}, nil
}

// GetJobInput represents get job request.
type GetJobInput struct {
	ID string `path:"id" doc:"Job ID"`
}

// GetJobOutput represents get job response.
type GetJobOutput struct {
	Body JobResponse
}

// GetJob handles getting a single job.
func (h *JobHandler) GetJob(ctx context.Context, input *GetJobInput) (*GetJobOutput, error) {
	userID := getUserID(ctx)
	if userID == "" {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	job, err := h.jobSvc.GetJob(ctx, userID, input.ID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get job: " + err.Error())
	}
	if job == nil {
		return nil, huma.Error404NotFound("job not found")
	}

	resp := JobResponse{
		ID:               job.ID,
		Type:             string(job.Type),
		Status:           string(job.Status),
		URL:              job.URL,
		URLsQueued:       job.URLsQueued,
		PageCount:        job.PageCount,
		TokenUsageInput:  job.TokenUsageInput,
		TokenUsageOutput: job.TokenUsageOutput,
		CostUSD:          job.CostUSD,
		CaptureDebug:     job.CaptureDebug,
		ErrorMessage:     job.ErrorMessage,
		ErrorCategory:    job.ErrorCategory,
		CreatedAt:        job.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if job.StartedAt != nil {
		resp.StartedAt = job.StartedAt.Format("2006-01-02T15:04:05Z07:00")
	}
	if job.CompletedAt != nil {
		resp.CompletedAt = job.CompletedAt.Format("2006-01-02T15:04:05Z07:00")
	}

	return &GetJobOutput{Body: resp}, nil
}


// GetCrawlMapInput represents crawl map request.
type GetCrawlMapInput struct {
	ID string `path:"id" doc:"Job ID"`
}

// CrawlMapEntry represents a single entry in the crawl map.
type CrawlMapEntry struct {
	ID                string  `json:"id" doc:"Result ID"`
	URL               string  `json:"url" doc:"Page URL"`
	ParentURL         *string `json:"parent_url,omitempty" doc:"URL that discovered this page (null for seed)"`
	Depth             int     `json:"depth" doc:"Crawl depth (0 for seed URL)"`
	Status            string  `json:"status" doc:"Crawl status: pending, crawling, completed, failed, skipped"`
	ErrorMessage      string  `json:"error_message,omitempty" doc:"Error message if failed"`
	ErrorCategory     string  `json:"error_category,omitempty" doc:"Error classification: rate_limit, quota_exceeded, provider_error, invalid_key, context_length, invalid_response, network_error, unknown"`
	ErrorDetails      string  `json:"error_details,omitempty" doc:"Full error details (BYOK users only)"`
	LLMProvider       string  `json:"llm_provider,omitempty" doc:"LLM provider used (BYOK users only)"`
	LLMModel          string  `json:"llm_model,omitempty" doc:"LLM model used (BYOK users only)"`
	TokenUsageInput   int     `json:"token_usage_input" doc:"Input tokens used"`
	TokenUsageOutput  int     `json:"token_usage_output" doc:"Output tokens used"`
	FetchDurationMs   int     `json:"fetch_duration_ms" doc:"Time to fetch page in ms"`
	ExtractDurationMs int     `json:"extract_duration_ms" doc:"Time to extract data in ms"`
	DiscoveredAt      string  `json:"discovered_at,omitempty" doc:"When URL was discovered"`
	CompletedAt       string  `json:"completed_at,omitempty" doc:"When processing completed"`
}

// ErrorSummary provides a breakdown of errors by category.
type ErrorSummary struct {
	Total        int            `json:"total" doc:"Total number of failed pages"`
	ByCategory   map[string]int `json:"by_category" doc:"Count of errors by category (rate_limit, quota_exceeded, etc.)"`
	HasRateLimit bool           `json:"has_rate_limit" doc:"True if any rate_limit errors occurred"`
}

// GetCrawlMapOutput represents crawl map response.
type GetCrawlMapOutput struct {
	Body struct {
		JobID        string          `json:"job_id" doc:"Job ID"`
		SeedURL      string          `json:"seed_url" doc:"Initial seed URL"`
		Total        int             `json:"total" doc:"Total pages in crawl map"`
		MaxDepth     int             `json:"max_depth" doc:"Maximum depth reached"`
		Completed    int             `json:"completed" doc:"Number of successfully completed pages"`
		Failed       int             `json:"failed" doc:"Number of failed pages"`
		ErrorSummary *ErrorSummary   `json:"error_summary,omitempty" doc:"Summary of errors if any pages failed"`
		Entries      []CrawlMapEntry `json:"entries" doc:"Crawl map entries ordered by depth"`
	}
}

// GetCrawlMap handles getting the crawl map for a job.
func (h *JobHandler) GetCrawlMap(ctx context.Context, input *GetCrawlMapInput) (*GetCrawlMapOutput, error) {
	userID := getUserID(ctx)
	if userID == "" {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	results, err := h.jobSvc.GetCrawlMap(ctx, userID, input.ID)
	if err != nil {
		if err.Error() == "crawl map only available for crawl jobs" {
			return nil, huma.Error400BadRequest(err.Error())
		}
		return nil, huma.Error500InternalServerError("failed to get crawl map: " + err.Error())
	}
	if results == nil {
		return nil, huma.Error404NotFound("job not found")
	}

	// Build response
	var entries []CrawlMapEntry
	var seedURL string
	var maxDepth int
	var completed, failed int
	errorCounts := make(map[string]int)

	for _, r := range results {
		// Track seed URL and max depth
		if r.Depth == 0 {
			seedURL = r.URL
		}
		if r.Depth > maxDepth {
			maxDepth = r.Depth
		}

		// Track completion/error stats
		switch r.CrawlStatus {
		case models.CrawlStatusCompleted:
			completed++
		case models.CrawlStatusFailed:
			failed++
			if r.ErrorCategory != "" {
				errorCounts[r.ErrorCategory]++
			} else {
				errorCounts["unknown"]++
			}
		}

		// Build client-safe result representation (BYOK sees details, others don't)
		clientResult := BuildClientResult(ResultInfo{
			ErrorMessage:  r.ErrorMessage,
			ErrorCategory: r.ErrorCategory,
			ErrorDetails:  r.ErrorDetails,
			LLMProvider:   r.LLMProvider,
			LLMModel:      r.LLMModel,
			IsBYOK:        r.IsBYOK,
		})

		entry := CrawlMapEntry{
			ID:                r.ID,
			URL:               r.URL,
			ParentURL:         r.ParentURL,
			Depth:             r.Depth,
			Status:            string(r.CrawlStatus),
			ErrorMessage:      clientResult.ErrorMessage,
			ErrorCategory:     clientResult.ErrorCategory,
			ErrorDetails:      clientResult.ErrorDetails,
			LLMProvider:       clientResult.LLMProvider,
			LLMModel:          clientResult.LLMModel,
			TokenUsageInput:   r.TokenUsageInput,
			TokenUsageOutput:  r.TokenUsageOutput,
			FetchDurationMs:   r.FetchDurationMs,
			ExtractDurationMs: r.ExtractDurationMs,
		}
		if r.DiscoveredAt != nil {
			entry.DiscoveredAt = r.DiscoveredAt.Format(time.RFC3339)
		}
		if r.CompletedAt != nil {
			entry.CompletedAt = r.CompletedAt.Format(time.RFC3339)
		}
		entries = append(entries, entry)
	}

	// Build error summary if there were failures
	var errSummary *ErrorSummary
	if failed > 0 {
		errSummary = &ErrorSummary{
			Total:        failed,
			ByCategory:   errorCounts,
			HasRateLimit: errorCounts["rate_limit"] > 0,
		}
	}

	return &GetCrawlMapOutput{
		Body: struct {
			JobID        string          `json:"job_id" doc:"Job ID"`
			SeedURL      string          `json:"seed_url" doc:"Initial seed URL"`
			Total        int             `json:"total" doc:"Total pages in crawl map"`
			MaxDepth     int             `json:"max_depth" doc:"Maximum depth reached"`
			Completed    int             `json:"completed" doc:"Number of successfully completed pages"`
			Failed       int             `json:"failed" doc:"Number of failed pages"`
			ErrorSummary *ErrorSummary   `json:"error_summary,omitempty" doc:"Summary of errors if any pages failed"`
			Entries      []CrawlMapEntry `json:"entries" doc:"Crawl map entries ordered by depth"`
		}{
			JobID:        input.ID,
			SeedURL:      seedURL,
			Total:        len(entries),
			MaxDepth:     maxDepth,
			Completed:    completed,
			Failed:       failed,
			ErrorSummary: errSummary,
			Entries:      entries,
		},
	}, nil
}

// GetJobResultsInput represents job results request.
type GetJobResultsInput struct {
	ID     string `path:"id" doc:"Job ID"`
	Merge  bool   `query:"merge" default:"false" doc:"Merge all results into a single object"`
	Format string `query:"format" default:"json" enum:"json,jsonl,yaml" doc:"Output format: json (default), jsonl (one result per line), or yaml"`
}

// JobResultEntry represents a single result in the response.
type JobResultEntry struct {
	ID   string          `json:"id" doc:"Result ID"`
	URL  string          `json:"url" doc:"Page URL"`
	Data json.RawMessage `json:"data" doc:"Extracted data"`
}

// GetJobResultsOutput represents job results response.
type GetJobResultsOutput struct {
	Body struct {
		JobID     string           `json:"job_id" doc:"Job ID"`
		Status    string           `json:"status" doc:"Job status"`
		PageCount int              `json:"page_count" doc:"Number of pages processed"`
		Results   []JobResultEntry `json:"results,omitempty" doc:"Extraction results (when merge=false)"`
		Merged    map[string]any   `json:"merged,omitempty" doc:"Merged extraction result (when merge=true)"`
	}
}

// GetJobResults returns the extraction results for a job.
func (h *JobHandler) GetJobResults(ctx context.Context, input *GetJobResultsInput) (*GetJobResultsOutput, error) {
	userID := getUserID(ctx)
	if userID == "" {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	// Verify job exists and belongs to user
	job, err := h.jobSvc.GetJob(ctx, userID, input.ID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get job: " + err.Error())
	}
	if job == nil {
		return nil, huma.Error404NotFound("job not found")
	}

	// Get results
	results, err := h.jobSvc.GetJobResults(ctx, userID, input.ID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get results: " + err.Error())
	}

	output := &GetJobResultsOutput{
		Body: struct {
			JobID     string           `json:"job_id" doc:"Job ID"`
			Status    string           `json:"status" doc:"Job status"`
			PageCount int              `json:"page_count" doc:"Number of pages processed"`
			Results   []JobResultEntry `json:"results,omitempty" doc:"Extraction results (when merge=false)"`
			Merged    map[string]any   `json:"merged,omitempty" doc:"Merged extraction result (when merge=true)"`
		}{
			JobID:     job.ID,
			Status:    string(job.Status),
			PageCount: len(results),
		},
	}

	if input.Merge {
		// Collect all results into { items: [...] }
		output.Body.Merged = collectAllResults(results)
	} else {
		// Return individual results
		var entries []JobResultEntry
		for _, r := range results {
			entries = append(entries, JobResultEntry{
				ID:   r.ID,
				URL:  r.URL,
				Data: json.RawMessage(r.DataJSON),
			})
		}
		output.Body.Results = entries
	}

	return output, nil
}


// GetJobWebhookDeliveriesInput represents job webhook deliveries request.
type GetJobWebhookDeliveriesInput struct {
	ID string `path:"id" doc:"Job ID"`
}

// JobWebhookDeliveryResponse represents a webhook delivery in job responses.
type JobWebhookDeliveryResponse struct {
	ID             string  `json:"id" doc:"Delivery ID"`
	WebhookID      *string `json:"webhook_id,omitempty" doc:"Webhook ID (null for ephemeral)"`
	EventType      string  `json:"event_type" doc:"Event type that triggered this delivery"`
	URL            string  `json:"url" doc:"Destination URL"`
	StatusCode     *int    `json:"status_code,omitempty" doc:"HTTP status code received"`
	ResponseTimeMs *int    `json:"response_time_ms,omitempty" doc:"Response time in milliseconds"`
	Status         string  `json:"status" doc:"Delivery status (pending, success, failed, retrying)"`
	ErrorMessage   string  `json:"error_message,omitempty" doc:"Error message if failed"`
	AttemptNumber  int     `json:"attempt_number" doc:"Current attempt number"`
	MaxAttempts    int     `json:"max_attempts" doc:"Maximum retry attempts"`
	CreatedAt      string  `json:"created_at" doc:"Creation timestamp"`
	DeliveredAt    *string `json:"delivered_at,omitempty" doc:"Successful delivery timestamp"`
}

// GetJobWebhookDeliveriesOutput represents job webhook deliveries response.
type GetJobWebhookDeliveriesOutput struct {
	Body struct {
		JobID      string                       `json:"job_id" doc:"Job ID"`
		Deliveries []JobWebhookDeliveryResponse `json:"deliveries" doc:"Webhook deliveries for this job"`
	}
}

// GetJobWebhookDeliveries returns webhook deliveries for a job.
func (h *JobHandler) GetJobWebhookDeliveries(ctx context.Context, input *GetJobWebhookDeliveriesInput) (*GetJobWebhookDeliveriesOutput, error) {
	userID := getUserID(ctx)
	if userID == "" {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	// Verify job exists and belongs to user
	job, err := h.jobSvc.GetJob(ctx, userID, input.ID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get job: " + err.Error())
	}
	if job == nil {
		return nil, huma.Error404NotFound("job not found")
	}

	// Get webhook deliveries
	var deliveries []*models.WebhookDelivery
	if h.webhookSvc != nil {
		deliveries, err = h.webhookSvc.GetDeliveriesForJob(ctx, input.ID)
		if err != nil {
			return nil, huma.Error500InternalServerError("failed to get webhook deliveries: " + err.Error())
		}
	}

	responses := make([]JobWebhookDeliveryResponse, 0, len(deliveries))
	for _, d := range deliveries {
		var deliveredAt *string
		if d.DeliveredAt != nil {
			s := d.DeliveredAt.Format(time.RFC3339)
			deliveredAt = &s
		}

		responses = append(responses, JobWebhookDeliveryResponse{
			ID:             d.ID,
			WebhookID:      d.WebhookID,
			EventType:      d.EventType,
			URL:            d.URL,
			StatusCode:     d.StatusCode,
			ResponseTimeMs: d.ResponseTimeMs,
			Status:         string(d.Status),
			ErrorMessage:   d.ErrorMessage,
			AttemptNumber:  d.AttemptNumber,
			MaxAttempts:    d.MaxAttempts,
			CreatedAt:      d.CreatedAt.Format(time.RFC3339),
			DeliveredAt:    deliveredAt,
		})
	}

	return &GetJobWebhookDeliveriesOutput{
		Body: struct {
			JobID      string                       `json:"job_id" doc:"Job ID"`
			Deliveries []JobWebhookDeliveryResponse `json:"deliveries" doc:"Webhook deliveries for this job"`
		}{
			JobID:      input.ID,
			Deliveries: responses,
		},
	}, nil
}

// GetJobResultsRaw handles job results with format-aware responses.
// This is a raw HTTP handler (not Huma) to support different Content-Types.
func (h *JobHandler) GetJobResultsRaw(w http.ResponseWriter, r *http.Request) {
	// Get user from context (set by auth middleware)
	claims := mw.GetUserClaims(r.Context())
	if claims == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	userID := claims.UserID

	// Get job ID from URL
	jobID := chi.URLParam(r, "id")
	if jobID == "" {
		http.Error(w, `{"error":"job ID required"}`, http.StatusBadRequest)
		return
	}

	// Parse query parameters
	merge := r.URL.Query().Get("merge") == "true"
	format := ParseOutputFormat(r.URL.Query().Get("format"))

	// Verify job exists and belongs to user
	job, err := h.jobSvc.GetJob(r.Context(), userID, jobID)
	if err != nil {
		http.Error(w, `{"error":"failed to get job"}`, http.StatusInternalServerError)
		return
	}
	if job == nil {
		http.Error(w, `{"error":"job not found"}`, http.StatusNotFound)
		return
	}

	// Set cache headers based on job status
	// Completed jobs are immutable and can be cached for a long time
	if job.Status == models.JobStatusCompleted {
		immutableSecs := int(constants.CacheMaxAgeImmutable.Seconds())
		w.Header().Set("Cache-Control", fmt.Sprintf("private, max-age=%d, immutable", immutableSecs))
	}

	// Get results
	results, err := h.jobSvc.GetJobResults(r.Context(), userID, jobID)
	if err != nil {
		http.Error(w, `{"error":"failed to get results"}`, http.StatusInternalServerError)
		return
	}

	// For JSON format, return full structured response (same as before)
	if format == FormatJSON {
		w.Header().Set("Content-Type", format.ContentType())

		if merge {
			resp := struct {
				JobID     string         `json:"job_id"`
				Status    string         `json:"status"`
				PageCount int            `json:"page_count"`
				Merged    map[string]any `json:"merged"`
			}{
				JobID:     job.ID,
				Status:    string(job.Status),
				PageCount: len(results),
				Merged:    collectAllResults(results),
			}
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				http.Error(w, `{"error":"failed to encode response"}`, http.StatusInternalServerError)
			}
		} else {
			var entries []JobResultEntry
			for _, r := range results {
				entries = append(entries, JobResultEntry{
					ID:   r.ID,
					URL:  r.URL,
					Data: json.RawMessage(r.DataJSON),
				})
			}
			resp := struct {
				JobID     string           `json:"job_id"`
				Status    string           `json:"status"`
				PageCount int              `json:"page_count"`
				Results   []JobResultEntry `json:"results"`
			}{
				JobID:     job.ID,
				Status:    string(job.Status),
				PageCount: len(results),
				Results:   entries,
			}
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				http.Error(w, `{"error":"failed to encode response"}`, http.StatusInternalServerError)
			}
		}
		return
	}

	// For JSONL and YAML formats, output just the data
	w.Header().Set("Content-Type", format.ContentType())

	if merge {
		merged := collectAllResults(results)
		data, err := FormatMergedResults(merged, format)
		if err != nil {
			http.Error(w, `{"error":"failed to format results"}`, http.StatusInternalServerError)
			return
		}
		w.Write(data)
	} else {
		var entries []JobResultEntry
		for _, r := range results {
			entries = append(entries, JobResultEntry{
				ID:   r.ID,
				URL:  r.URL,
				Data: json.RawMessage(r.DataJSON),
			})
		}
		data, err := FormatResults(entries, format)
		if err != nil {
			http.Error(w, `{"error":"failed to format results"}`, http.StatusInternalServerError)
			return
		}
		w.Write(data)
	}
}

// GetJobResultsDownloadInput represents job results download request.
type GetJobResultsDownloadInput struct {
	ID string `path:"id" doc:"Job ID"`
}

// GetJobResultsDownloadOutput represents job results download response.
type GetJobResultsDownloadOutput struct {
	Body struct {
		JobID       string `json:"job_id" doc:"Job ID"`
		DownloadURL string `json:"download_url" doc:"Presigned URL to download results (valid for 1 hour)"`
		ExpiresAt   string `json:"expires_at" doc:"URL expiration time"`
	}
}

// GetJobResultsDownload returns a presigned URL for downloading job results.
func (h *JobHandler) GetJobResultsDownload(ctx context.Context, input *GetJobResultsDownloadInput) (*GetJobResultsDownloadOutput, error) {
	userID := getUserID(ctx)
	if userID == "" {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	// Verify job exists and belongs to user
	job, err := h.jobSvc.GetJob(ctx, userID, input.ID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get job: " + err.Error())
	}
	if job == nil {
		return nil, huma.Error404NotFound("job not found")
	}

	// Check if job is completed
	if job.Status != models.JobStatusCompleted {
		return nil, huma.Error400BadRequest("job results not available - status: " + string(job.Status))
	}

	// Check if storage is enabled
	if h.storageSvc == nil || !h.storageSvc.IsEnabled() {
		return nil, huma.Error503ServiceUnavailable("result storage is not configured")
	}

	// Check if results exist in storage
	exists, err := h.storageSvc.JobResultExists(ctx, input.ID)
	if err != nil || !exists {
		return nil, huma.Error404NotFound("results not found in storage - job may have been processed before storage was enabled")
	}

	// Generate presigned URL (valid for 1 hour)
	expiry := 1 * time.Hour
	downloadURL, err := h.storageSvc.GetJobResultsPresignedURL(ctx, input.ID, expiry)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to generate download URL: " + err.Error())
	}

	return &GetJobResultsDownloadOutput{
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

// GetJobDebugCaptureInput represents debug capture retrieval request.
type GetJobDebugCaptureInput struct {
	ID string `path:"id" doc:"Job ID"`
}

// DebugCaptureLLMRequest represents metadata about an LLM request.
type DebugCaptureLLMRequest struct {
	Provider    string `json:"provider" doc:"LLM provider used"`
	Model       string `json:"model" doc:"LLM model used"`
	FetchMode   string `json:"fetch_mode,omitempty" doc:"Content fetch mode"`
	ContentSize int    `json:"content_size" doc:"Size of content sent to LLM"`
	PromptSize  int    `json:"prompt_size" doc:"Total prompt size including system instructions"`
}

// DebugCaptureLLMResponse represents metadata about an LLM response.
type DebugCaptureLLMResponse struct {
	InputTokens  int    `json:"input_tokens" doc:"Input tokens consumed"`
	OutputTokens int    `json:"output_tokens" doc:"Output tokens generated"`
	DurationMs   int64  `json:"duration_ms" doc:"Request duration in milliseconds"`
	Success      bool   `json:"success" doc:"Whether the request succeeded"`
	Error        string `json:"error,omitempty" doc:"Error message if failed"`
}

// DebugCaptureEntry represents a single captured LLM request/response.
type DebugCaptureEntry struct {
	ID         string                  `json:"id" doc:"Capture ID"`
	URL        string                  `json:"url" doc:"Page URL being processed"`
	Timestamp  string                  `json:"timestamp" doc:"When the request was made"`
	JobType    string                  `json:"job_type" doc:"Job type (analyze, extract, crawl)"`
	Request    DebugCaptureLLMRequest  `json:"request" doc:"LLM request metadata"`
	Response   DebugCaptureLLMResponse `json:"response" doc:"LLM response metadata"`
	Prompt     string                  `json:"prompt,omitempty" doc:"Full prompt sent to LLM (for analyze jobs)"`
	RawContent string                  `json:"raw_content,omitempty" doc:"Page content (for extract/crawl jobs)"`
	Schema     string                  `json:"schema,omitempty" doc:"Schema used for extraction"`
	Hints      map[string]string       `json:"hints_applied,omitempty" doc:"Preprocessing hints applied"`
}

// GetJobDebugCaptureOutput represents debug capture response.
type GetJobDebugCaptureOutput struct {
	Body struct {
		JobID    string              `json:"job_id" doc:"Job ID"`
		Enabled  bool                `json:"enabled" doc:"Whether debug capture was enabled for this job"`
		Captures []DebugCaptureEntry `json:"captures" doc:"Captured LLM requests"`
	}
}

// GetJobDebugCapture returns the debug captures for a job.
// This includes LLM prompts, metadata, and responses for debugging extraction issues.
func (h *JobHandler) GetJobDebugCapture(ctx context.Context, input *GetJobDebugCaptureInput) (*GetJobDebugCaptureOutput, error) {
	userID := getUserID(ctx)
	if userID == "" {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	// Verify job exists and belongs to user
	job, err := h.jobSvc.GetJob(ctx, userID, input.ID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get job: " + err.Error())
	}
	if job == nil {
		return nil, huma.Error404NotFound("job not found")
	}

	// Check if storage is enabled
	if h.storageSvc == nil || !h.storageSvc.IsEnabled() {
		return nil, huma.Error503ServiceUnavailable("debug capture storage is not configured")
	}

	// Get debug captures from storage
	capture, err := h.storageSvc.GetDebugCapture(ctx, input.ID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get debug capture: " + err.Error())
	}

	// Convert to response format
	entries := make([]DebugCaptureEntry, 0, len(capture.Captures))
	for _, c := range capture.Captures {
		entries = append(entries, DebugCaptureEntry{
			ID:        c.ID,
			URL:       c.URL,
			Timestamp: c.Timestamp.Format(time.RFC3339),
			JobType:   c.JobType,
			Request: DebugCaptureLLMRequest{
				Provider:    c.Request.Provider,
				Model:       c.Request.Model,
				FetchMode:   c.Request.FetchMode,
				ContentSize: c.Request.ContentSize,
				PromptSize:  c.Request.PromptSize,
			},
			Response: DebugCaptureLLMResponse{
				InputTokens:  c.Response.InputTokens,
				OutputTokens: c.Response.OutputTokens,
				DurationMs:   c.Response.DurationMs,
				Success:      c.Response.Success,
				Error:        c.Response.Error,
			},
			Prompt:     c.Prompt,
			RawContent: c.RawContent,
			Schema:     c.Schema,
			Hints:      c.Hints,
		})
	}

	return &GetJobDebugCaptureOutput{
		Body: struct {
			JobID    string              `json:"job_id" doc:"Job ID"`
			Enabled  bool                `json:"enabled" doc:"Whether debug capture was enabled for this job"`
			Captures []DebugCaptureEntry `json:"captures" doc:"Captured LLM requests"`
		}{
			JobID:    input.ID,
			Enabled:  capture.Enabled,
			Captures: entries,
		},
	}, nil
}

