package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"

	"github.com/jmylchreest/refyne-api/internal/http/mw"
	"github.com/jmylchreest/refyne-api/internal/models"
	"github.com/jmylchreest/refyne-api/internal/service"
)

// JobHandler handles job endpoints.
type JobHandler struct {
	jobSvc *service.JobService
}

// NewJobHandler creates a new job handler.
func NewJobHandler(jobSvc *service.JobService) *JobHandler {
	return &JobHandler{jobSvc: jobSvc}
}

// CreateCrawlJobInput represents crawl job creation request.
type CreateCrawlJobInput struct {
	Body struct {
		URL        string          `json:"url" minLength:"1" doc:"Seed URL to start crawling from"`
		Schema     json.RawMessage `json:"schema" doc:"JSON Schema defining the data structure to extract"`
		Options    CrawlOptions    `json:"options,omitempty" doc:"Crawl configuration options"`
		WebhookURL string          `json:"webhook_url,omitempty" format:"uri" doc:"URL to receive webhook on completion"`
		LLMConfig  *LLMConfigInput `json:"llm_config,omitempty" doc:"Optional LLM configuration override"`
	}
}

// CrawlOptions represents crawl options.
type CrawlOptions struct {
	FollowSelector   string `json:"follow_selector,omitempty" doc:"CSS selector for links to follow"`
	FollowPattern    string `json:"follow_pattern,omitempty" doc:"Regex pattern for URLs to follow"`
	MaxDepth         int    `json:"max_depth,omitempty" default:"1" maximum:"5" doc:"Maximum crawl depth"`
	NextSelector     string `json:"next_selector,omitempty" doc:"CSS selector for pagination next link"`
	MaxPages         int    `json:"max_pages,omitempty" default:"10" maximum:"100" doc:"Maximum pages to crawl"`
	MaxURLs          int    `json:"max_urls,omitempty" default:"50" maximum:"500" doc:"Maximum total URLs to process"`
	Delay            string `json:"delay,omitempty" default:"500ms" doc:"Delay between requests"`
	Concurrency      int    `json:"concurrency,omitempty" default:"3" maximum:"10" doc:"Concurrent requests"`
	SameDomainOnly   bool   `json:"same_domain_only,omitempty" default:"true" doc:"Only follow same-domain links"`
	ExtractFromSeeds bool   `json:"extract_from_seeds,omitempty" doc:"Extract data from seed URLs"`
}

// CreateCrawlJobOutput represents crawl job creation response.
type CreateCrawlJobOutput struct {
	Body struct {
		JobID     string `json:"job_id" doc:"Unique job identifier"`
		Status    string `json:"status" doc:"Initial job status"`
		StatusURL string `json:"status_url" doc:"URL to check job status"`
	}
}

// CreateCrawlJob handles crawl job creation.
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
		},
		WebhookURL: input.Body.WebhookURL,
	})
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to create crawl job: " + err.Error())
	}

	return &CreateCrawlJobOutput{
		Body: struct {
			JobID     string `json:"job_id" doc:"Unique job identifier"`
			Status    string `json:"status" doc:"Initial job status"`
			StatusURL string `json:"status_url" doc:"URL to check job status"`
		}{
			JobID:     result.JobID,
			Status:    result.Status,
			StatusURL: result.StatusURL,
		},
	}, nil
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

	// Poll for results
	var lastResultID string
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
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
