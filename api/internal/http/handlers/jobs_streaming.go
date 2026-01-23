package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/sse"
	"github.com/go-chi/chi/v5"

	"github.com/jmylchreest/refyne-api/internal/http/mw"
	"github.com/jmylchreest/refyne-api/internal/models"
)

// =============================================================================
// SSE Event Types for OpenAPI Schema Generation
// =============================================================================

// SSEStatusEvent is sent as the initial event and when job status changes.
type SSEStatusEvent struct {
	JobID      string `json:"job_id" doc:"Job ID"`
	Status     string `json:"status" doc:"Job status (pending, running, completed, failed)"`
	URLsQueued int    `json:"urls_queued" doc:"Number of URLs queued for processing"`
	PageCount  int    `json:"page_count" doc:"Number of pages processed so far"`
}

// SSEResultEvent is sent for each extracted result.
// Note: Full extracted data is NOT included in SSE events to reduce bandwidth.
// Clients should fetch full results from the results_url after the complete event.
type SSEResultEvent struct {
	ID            string `json:"id" doc:"Result ID"`
	URL           string `json:"url" doc:"Source URL"`
	Status        string `json:"status" doc:"Crawl status (pending, completed, failed)"`
	ErrorMessage  string `json:"error_message,omitempty" doc:"Error message if failed"`
	ErrorCategory string `json:"error_category,omitempty" doc:"Error category if failed"`
	LLMProvider   string `json:"llm_provider,omitempty" doc:"LLM provider used"`
	LLMModel      string `json:"llm_model,omitempty" doc:"LLM model used"`
}

// SSECompleteEvent is sent when the job completes.
type SSECompleteEvent struct {
	JobID         string  `json:"job_id" doc:"Job ID"`
	Status        string  `json:"status" doc:"Final job status"`
	PageCount     int     `json:"page_count" doc:"Total pages processed"`
	ErrorMessage  string  `json:"error_message,omitempty" doc:"Error message if job failed"`
	ErrorCategory string  `json:"error_category,omitempty" doc:"Error category if job failed"`
	CostUSD       float64 `json:"cost_usd,omitempty" doc:"Total cost in USD"`
	ResultsURL    string  `json:"results_url" doc:"URL to fetch full results"`
}

// SSEErrorEvent is sent when an error occurs during streaming.
type SSEErrorEvent struct {
	Message string `json:"message" doc:"Error message"`
}

// SSEStreamInput is the input for the SSE stream endpoint.
type SSEStreamInput struct {
	ID string `path:"id" doc:"Job ID to stream results from"`
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
	// Best effort: disable write deadline for long-running SSE. Some proxies may not support this.
	_ = rc.SetWriteDeadline(time.Time{})

	// Send initial status with urls_queued for progress tracking
	sendSSEEvent(w, flusher, "status", map[string]any{
		"job_id":      job.ID,
		"status":      string(job.Status),
		"urls_queued": job.URLsQueued,
		"page_count":  job.PageCount,
	})

	// If job is already completed, send result metadata and close
	// Note: Full extracted data is NOT included in SSE events to reduce bandwidth.
	// Clients should fetch full results from /jobs/{id}/results after completion.
	if job.Status == models.JobStatusCompleted || job.Status == models.JobStatusFailed {
		// Send final results metadata (status/errors only, no extracted data)
		results, _ := h.jobSvc.GetJobResultsAfterID(r.Context(), userID, jobID, "")
		for _, result := range results {
			event := map[string]any{
				"id":     result.ID,
				"url":    result.URL,
				"status": string(result.CrawlStatus),
				// data is NOT included - fetch from /results endpoint
			}
			// Add result info with BYOK-aware sanitization
			ResultInfo{
				ErrorMessage:  result.ErrorMessage,
				ErrorCategory: result.ErrorCategory,
				ErrorDetails:  result.ErrorDetails,
				LLMProvider:   result.LLMProvider,
				LLMModel:      result.LLMModel,
				IsBYOK:        result.IsBYOK,
			}.ApplyToMap(event)
			sendSSEEvent(w, flusher, "result", event)
		}
		sendSSEEvent(w, flusher, "complete", map[string]any{
			"job_id":         job.ID,
			"status":         string(job.Status),
			"page_count":     job.PageCount,
			"error_message":  job.ErrorMessage,
			"error_category": job.ErrorCategory,
			"results_url":    fmt.Sprintf("/api/v1/jobs/%s/results", job.ID), // Where to fetch full results
		})
		return
	}

	// Poll for results with heartbeat to prevent proxy timeouts
	// Track by ULID which is lexicographically time-ordered
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
			// Check for new results (ULIDs are time-ordered so id > lastID works correctly)
			results, err := h.jobSvc.GetJobResultsAfterID(ctx, userID, jobID, lastResultID)
			if err != nil {
				sendSSEEvent(w, flusher, "error", map[string]any{
					"message": "failed to fetch results",
				})
				continue
			}

			// Send new results metadata (no extracted data - fetch from /results endpoint)
			for _, result := range results {
				event := map[string]any{
					"id":     result.ID,
					"url":    result.URL,
					"status": string(result.CrawlStatus),
					// data is NOT included - fetch from /results endpoint
				}
				// Add result info with BYOK-aware sanitization
				ResultInfo{
					ErrorMessage:  result.ErrorMessage,
					ErrorCategory: result.ErrorCategory,
					ErrorDetails:  result.ErrorDetails,
					LLMProvider:   result.LLMProvider,
					LLMModel:      result.LLMModel,
					IsBYOK:        result.IsBYOK,
				}.ApplyToMap(event)
				sendSSEEvent(w, flusher, "result", event)
				lastResultID = result.ID
			}

			// Check job status
			job, err = h.jobSvc.GetJob(ctx, userID, jobID)
			if err != nil {
				continue
			}

			// Send status update with urls_queued for progress tracking
			sendSSEEvent(w, flusher, "status", map[string]any{
				"job_id":      job.ID,
				"status":      string(job.Status),
				"urls_queued": job.URLsQueued,
				"page_count":  job.PageCount,
			})

			// If job is done, send complete event and close
			if job.Status == models.JobStatusCompleted || job.Status == models.JobStatusFailed {
				sendSSEEvent(w, flusher, "complete", map[string]any{
					"job_id":         job.ID,
					"status":         string(job.Status),
					"page_count":     job.PageCount,
					"error_message":  job.ErrorMessage,
					"error_category": job.ErrorCategory,
					"cost_usd":       job.CostUSD,
					"results_url":    fmt.Sprintf("/api/v1/jobs/%s/results", job.ID), // Where to fetch full results
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
	_, _ = fmt.Fprintf(w, "event: %s\n", event)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", jsonData)
	flusher.Flush()
}

// sendSSEHeartbeat sends an SSE comment as a keepalive/heartbeat.
// SSE comments start with a colon and are ignored by the client EventSource API.
func sendSSEHeartbeat(w http.ResponseWriter, flusher http.Flusher) {
	_, _ = fmt.Fprintf(w, ": heartbeat\n\n")
	flusher.Flush()
}

// =============================================================================
// Raw Endpoint OpenAPI Registration
// =============================================================================

// RegisterRawEndpoints registers raw HTTP endpoints (SSE, multi-format) with Huma for OpenAPI documentation.
// The actual handlers are registered separately via chi middleware for authentication.
// This ensures these endpoints appear in the OpenAPI spec with proper security requirements.
func (h *JobHandler) RegisterRawEndpoints(api huma.API) {
	// Register SSE stream endpoint for OpenAPI documentation
	sse.Register(api, huma.Operation{
		OperationID: "streamJobResults",
		Method:      http.MethodGet,
		Path:        "/api/v1/jobs/{id}/stream",
		Summary:     "Stream job results via SSE",
		Description: `Server-Sent Events stream for real-time job results.

Events sent:
- **status**: Initial job status and progress updates
- **result**: Each extracted result as it completes
- **complete**: Final status when job finishes
- **error**: Error notifications

The stream sends heartbeat comments every 15 seconds to keep connections alive through proxies.

Example usage with curl:
` + "```" + `bash
curl -H "Authorization: Bearer rf_your_key" \
     -H "Accept: text/event-stream" \
     https://api.refyne.dev/api/v1/jobs/{id}/stream
` + "```" + `
`,
		Tags:     []string{"Jobs"},
		Security: []map[string][]string{{mw.SecurityScheme: {}}},
	}, map[string]any{
		"status":   SSEStatusEvent{},
		"result":   SSEResultEvent{},
		"complete": SSECompleteEvent{},
		"error":    SSEErrorEvent{},
	}, func(ctx context.Context, input *SSEStreamInput, send sse.Sender) {
		// Placeholder handler - actual SSE is handled by chi router.
		// This registration is only for OpenAPI schema generation.
		<-ctx.Done()
	})

	// Register results endpoint for OpenAPI documentation
	// This endpoint returns different content types based on Accept header
	huma.Register(api, huma.Operation{
		OperationID: "getJobResultsRaw",
		Method:      http.MethodGet,
		Path:        "/api/v1/jobs/{id}/results",
		Summary:     "Get job results in various formats",
		Description: `Returns job results in the requested format.

Supported formats via Accept header or ?format= query parameter:
- **application/json** (default): JSON array of results
- **application/x-ndjson**: Newline-delimited JSON (one result per line)
- **application/yaml**: YAML formatted results

Query parameters:
- **merge=true**: Merge all page results into a single array
- **format=json|jsonl|yaml**: Override content type

Example:
` + "```" + `bash
# JSON format (default)
curl -H "Authorization: Bearer rf_your_key" \
     https://api.refyne.dev/api/v1/jobs/{id}/results

# JSONL format with merge
curl -H "Authorization: Bearer rf_your_key" \
     "https://api.refyne.dev/api/v1/jobs/{id}/results?format=jsonl&merge=true"
` + "```" + `
`,
		Tags:     []string{"Jobs"},
		Security: []map[string][]string{{mw.SecurityScheme: {}}},
		Responses: map[string]*huma.Response{
			"200": {
				Description: "Job results in requested format",
				Content: map[string]*huma.MediaType{
					"application/json": {
						Schema: &huma.Schema{
							Type:        "array",
							Description: "Array of job results",
							Items: &huma.Schema{
								Type: "object",
								Properties: map[string]*huma.Schema{
									"id":     {Type: "string", Description: "Result ID"},
									"url":    {Type: "string", Description: "Source URL"},
									"status": {Type: "string", Description: "Crawl status"},
									"data":   {Type: "object", Description: "Extracted data"},
								},
							},
						},
					},
					"application/x-ndjson": {
						Schema: &huma.Schema{
							Type:        "string",
							Description: "Newline-delimited JSON results",
						},
					},
					"application/yaml": {
						Schema: &huma.Schema{
							Type:        "string",
							Description: "YAML formatted results",
						},
					},
				},
			},
			"401": {Description: "Unauthorized - missing or invalid token"},
			"404": {Description: "Job not found"},
		},
	}, func(ctx context.Context, input *struct {
		ID     string `path:"id" doc:"Job ID"`
		Merge  bool   `query:"merge" default:"false" doc:"Merge all page results into single array"`
		Format string `query:"format" enum:"json,jsonl,yaml" doc:"Output format override"`
	}) (*struct{ Body []byte }, error) {
		// Placeholder handler - actual handling is done by chi router.
		// This registration is only for OpenAPI schema generation.
		return nil, huma.Error501NotImplemented("Use chi router handler")
	})
}
