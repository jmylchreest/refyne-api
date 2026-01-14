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
	jobSvc     *service.JobService
	storageSvc *service.StorageService
}

// NewJobHandler creates a new job handler.
func NewJobHandler(jobSvc *service.JobService, storageSvc *service.StorageService) *JobHandler {
	return &JobHandler{
		jobSvc:     jobSvc,
		storageSvc: storageSvc,
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
type CreateCrawlJobInput struct {
	Wait    bool `query:"wait" default:"false" doc:"Block until job completes and return results directly. Max wait time is 2 minutes. Returns 202 if timeout exceeded."`
	Timeout int  `query:"timeout" default:"120" minimum:"10" maximum:"120" doc:"Maximum seconds to wait when wait=true (default 120s, max 120s/2min). For longer jobs, use async mode."`
	Body    struct {
		URL        string          `json:"url" minLength:"1" example:"https://example.com/products" doc:"Seed URL to start crawling from"`
		Schema     json.RawMessage `json:"schema" doc:"JSON Schema defining the data structure to extract. Example: {\"name\":\"string\",\"price\":\"number\",\"description\":\"string\"}"`
		Options    CrawlOptions    `json:"options,omitempty" doc:"Crawl configuration options"`
		WebhookURL string          `json:"webhook_url,omitempty" format:"uri" example:"https://my-app.com/webhook/crawl-complete" doc:"URL to receive POST webhook on completion (async mode)"`
		LLMConfig  *LLMConfigInput `json:"llm_config,omitempty" doc:"Optional LLM configuration override (BYOK)"`
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
	CostCredits  int            `json:"cost_credits,omitempty" example:"150" doc:"Total credits consumed (sync mode)"`
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
	userID := getUserID(ctx)
	if userID == "" {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	result, err := h.jobSvc.CreateCrawlJob(ctx, userID, service.CreateCrawlJobInput{
		URL:    input.Body.URL,
		Schema: input.Body.Schema,
		Options: service.CrawlOptions{
			FollowSelector:   input.Body.Options.FollowSelector,
			FollowPattern:    input.Body.Options.FollowPattern,
			MaxDepth:         input.Body.Options.MaxDepth,
			NextSelector:     input.Body.Options.NextSelector,
			MaxPages:         input.Body.Options.MaxPages,
			MaxURLs:          input.Body.Options.MaxURLs,
			Delay:            input.Body.Options.Delay,
			Concurrency:      input.Body.Options.Concurrency,
			SameDomainOnly:   input.Body.Options.SameDomainOnly,
			ExtractFromSeeds: input.Body.Options.ExtractFromSeeds,
			UseSitemap:       input.Body.Options.UseSitemap,
		},
		WebhookURL: input.Body.WebhookURL,
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
		job, err = h.jobSvc.GetJob(ctx, userID, result.JobID)
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
			CostCredits:  job.CostCredits,
			DurationMs:   durationMs,
			ErrorMessage: job.ErrorMessage,
			TokenUsage: &TokenUsage{
				Input:  job.TokenUsageInput,
				Output: job.TokenUsageOutput,
			},
		},
	}

	// For completed jobs, get and merge results
	if job.Status == models.JobStatusCompleted {
		results, err := h.jobSvc.GetJobResults(ctx, userID, job.ID)
		if err == nil && len(results) > 0 {
			merged := make(map[string]any)
			for _, r := range results {
				if r.DataJSON == "" {
					continue
				}
				var data map[string]any
				if err := json.Unmarshal([]byte(r.DataJSON), &data); err != nil {
					continue
				}
				deepMergeResults(merged, data)
			}
			output.Body.Data = merged
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
	ID               string `json:"id"`
	Type             string `json:"type"`
	Status           string `json:"status"`
	URL              string `json:"url"`
	PageCount        int    `json:"page_count"`
	TokenUsageInput  int    `json:"token_usage_input"`
	TokenUsageOutput int    `json:"token_usage_output"`
	CostCredits      int    `json:"cost_credits"`
	ErrorMessage     string `json:"error_message,omitempty"`
	StartedAt        string `json:"started_at,omitempty"`
	CompletedAt      string `json:"completed_at,omitempty"`
	CreatedAt        string `json:"created_at"`
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
			PageCount:        job.PageCount,
			TokenUsageInput:  job.TokenUsageInput,
			TokenUsageOutput: job.TokenUsageOutput,
			CostCredits:      job.CostCredits,
			ErrorMessage:     job.ErrorMessage,
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
		PageCount:        job.PageCount,
		TokenUsageInput:  job.TokenUsageInput,
		TokenUsageOutput: job.TokenUsageOutput,
		CostCredits:      job.CostCredits,
		ErrorMessage:     job.ErrorMessage,
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

// StreamResults handles SSE streaming of job results.
// This is a raw HTTP handler (not Huma) to support SSE.
func (h *JobHandler) StreamResults(w http.ResponseWriter, r *http.Request) {
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

	// Verify job belongs to user
	job, err := h.jobSvc.GetJob(r.Context(), userID, jobID)
	if err != nil {
		http.Error(w, `{"error":"failed to get job"}`, http.StatusInternalServerError)
		return
	}
	if job == nil {
		http.Error(w, `{"error":"job not found"}`, http.StatusNotFound)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering

	// Ensure we can flush
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, `{"error":"streaming not supported"}`, http.StatusInternalServerError)
		return
	}

	// Disable write timeout for SSE - jobs can run for a long time
	// Use ResponseController (Go 1.20+) to extend the write deadline
	rc := http.NewResponseController(w)
	if err := rc.SetWriteDeadline(time.Time{}); err != nil {
		// If we can't disable the deadline, log but continue - some proxies may not support this
		// The connection will just close after the server's WriteTimeout
	}

	// Send initial status
	sendSSEEvent(w, flusher, "status", map[string]any{
		"job_id": job.ID,
		"status": string(job.Status),
	})

	// If job is already completed, send results and close
	if job.Status == models.JobStatusCompleted || job.Status == models.JobStatusFailed {
		if job.Status == models.JobStatusCompleted {
			// Send final results
			results, _ := h.jobSvc.GetJobResults(r.Context(), userID, jobID)
			for _, result := range results {
				sendSSEEvent(w, flusher, "result", map[string]any{
					"id":   result.ID,
					"url":  result.URL,
					"data": json.RawMessage(result.DataJSON),
				})
			}
		}
		sendSSEEvent(w, flusher, "complete", map[string]any{
			"job_id":     job.ID,
			"status":     string(job.Status),
			"page_count": job.PageCount,
		})
		return
	}

	// Poll for results with heartbeat to prevent proxy timeouts
	var lastResultID string
	pollTicker := time.NewTicker(1 * time.Second)
	defer pollTicker.Stop()

	// Heartbeat every 15 seconds to keep connection alive through proxies
	heartbeatTicker := time.NewTicker(15 * time.Second)
	defer heartbeatTicker.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeatTicker.C:
			// Send heartbeat to keep connection alive
			sendSSEHeartbeat(w, flusher)
		case <-pollTicker.C:
			// Check for new results
			results, err := h.jobSvc.GetJobResultsAfter(ctx, userID, jobID, lastResultID)
			if err != nil {
				sendSSEEvent(w, flusher, "error", map[string]any{
					"message": "failed to fetch results",
				})
				continue
			}

			// Send new results
			for _, result := range results {
				sendSSEEvent(w, flusher, "result", map[string]any{
					"id":   result.ID,
					"url":  result.URL,
					"data": json.RawMessage(result.DataJSON),
				})
				lastResultID = result.ID
			}

			// Check job status
			job, err = h.jobSvc.GetJob(ctx, userID, jobID)
			if err != nil {
				continue
			}

			// Send status update
			sendSSEEvent(w, flusher, "status", map[string]any{
				"job_id":     job.ID,
				"status":     string(job.Status),
				"page_count": job.PageCount,
			})

			// If job is done, send complete event and close
			if job.Status == models.JobStatusCompleted || job.Status == models.JobStatusFailed {
				sendSSEEvent(w, flusher, "complete", map[string]any{
					"job_id":       job.ID,
					"status":       string(job.Status),
					"page_count":   job.PageCount,
					"error":        job.ErrorMessage,
					"cost_credits": job.CostCredits,
				})
				return
			}
		}
	}
}

// sendSSEEvent sends a Server-Sent Event.
func sendSSEEvent(w http.ResponseWriter, flusher http.Flusher, event string, data any) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "event: %s\n", event)
	fmt.Fprintf(w, "data: %s\n\n", jsonData)
	flusher.Flush()
}

// sendSSEHeartbeat sends an SSE comment as a keepalive/heartbeat.
// SSE comments start with a colon and are ignored by the client EventSource API.
func sendSSEHeartbeat(w http.ResponseWriter, flusher http.Flusher) {
	fmt.Fprintf(w, ": heartbeat\n\n")
	flusher.Flush()
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
	TokenUsageInput   int     `json:"token_usage_input" doc:"Input tokens used"`
	TokenUsageOutput  int     `json:"token_usage_output" doc:"Output tokens used"`
	FetchDurationMs   int     `json:"fetch_duration_ms" doc:"Time to fetch page in ms"`
	ExtractDurationMs int     `json:"extract_duration_ms" doc:"Time to extract data in ms"`
	DiscoveredAt      string  `json:"discovered_at,omitempty" doc:"When URL was discovered"`
	CompletedAt       string  `json:"completed_at,omitempty" doc:"When processing completed"`
}

// GetCrawlMapOutput represents crawl map response.
type GetCrawlMapOutput struct {
	Body struct {
		JobID    string          `json:"job_id" doc:"Job ID"`
		SeedURL  string          `json:"seed_url" doc:"Initial seed URL"`
		Total    int             `json:"total" doc:"Total pages in crawl map"`
		MaxDepth int             `json:"max_depth" doc:"Maximum depth reached"`
		Entries  []CrawlMapEntry `json:"entries" doc:"Crawl map entries ordered by depth"`
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

	for _, r := range results {
		// Track seed URL and max depth
		if r.Depth == 0 {
			seedURL = r.URL
		}
		if r.Depth > maxDepth {
			maxDepth = r.Depth
		}

		entry := CrawlMapEntry{
			ID:                r.ID,
			URL:               r.URL,
			ParentURL:         r.ParentURL,
			Depth:             r.Depth,
			Status:            string(r.CrawlStatus),
			ErrorMessage:      r.ErrorMessage,
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

	return &GetCrawlMapOutput{
		Body: struct {
			JobID    string          `json:"job_id" doc:"Job ID"`
			SeedURL  string          `json:"seed_url" doc:"Initial seed URL"`
			Total    int             `json:"total" doc:"Total pages in crawl map"`
			MaxDepth int             `json:"max_depth" doc:"Maximum depth reached"`
			Entries  []CrawlMapEntry `json:"entries" doc:"Crawl map entries ordered by depth"`
		}{
			JobID:    input.ID,
			SeedURL:  seedURL,
			Total:    len(entries),
			MaxDepth: maxDepth,
			Entries:  entries,
		},
	}, nil
}

// GetJobResultsInput represents job results request.
type GetJobResultsInput struct {
	ID    string `path:"id" doc:"Job ID"`
	Merge bool   `query:"merge" default:"false" doc:"Merge all results into a single object"`
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
		// Merge all results into a single object
		merged := make(map[string]any)
		for _, r := range results {
			if r.DataJSON == "" {
				continue
			}
			var data map[string]any
			if err := json.Unmarshal([]byte(r.DataJSON), &data); err != nil {
				continue // Skip malformed data
			}
			deepMergeResults(merged, data)
		}
		output.Body.Merged = merged
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

// deepMergeResults merges src into dst.
// Arrays are concatenated and deduplicated, primitives use first non-nil value.
func deepMergeResults(dst, src map[string]any) {
	for key, srcVal := range src {
		if srcVal == nil {
			continue
		}

		dstVal, exists := dst[key]
		if !exists || dstVal == nil {
			dst[key] = srcVal
			continue
		}

		// Both exist and non-nil
		switch srcTyped := srcVal.(type) {
		case []any:
			if dstArr, ok := dstVal.([]any); ok {
				// Concatenate arrays and deduplicate
				dst[key] = dedupeArray(append(dstArr, srcTyped...))
			}
		case map[string]any:
			if dstMap, ok := dstVal.(map[string]any); ok {
				// Recursively merge nested objects
				deepMergeResults(dstMap, srcTyped)
			}
		// Primitives: keep first value (already set)
		}
	}
}

// dedupeArray removes duplicate elements from an array.
// Uses JSON serialization for comparison to handle complex objects.
func dedupeArray(arr []any) []any {
	if len(arr) == 0 {
		return arr
	}

	seen := make(map[string]bool)
	result := make([]any, 0, len(arr))

	for _, item := range arr {
		// Serialize to JSON for comparison
		key, err := json.Marshal(item)
		if err != nil {
			// If serialization fails, include the item anyway
			result = append(result, item)
			continue
		}

		keyStr := string(key)
		if !seen[keyStr] {
			seen[keyStr] = true
			result = append(result, item)
		}
	}

	return result
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
