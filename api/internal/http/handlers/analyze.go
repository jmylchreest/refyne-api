package handlers

import (
	"context"
	"encoding/json"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/oklog/ulid/v2"

	"github.com/jmylchreest/refyne-api/internal/models"
	"github.com/jmylchreest/refyne-api/internal/repository"
	"github.com/jmylchreest/refyne-api/internal/service"
)

// AnalyzeHandler handles URL analysis endpoints.
type AnalyzeHandler struct {
	analyzerSvc *service.AnalyzerService
	jobRepo     repository.JobRepository
	storageSvc  *service.StorageService
}

// NewAnalyzeHandler creates a new analyze handler.
func NewAnalyzeHandler(analyzerSvc *service.AnalyzerService, jobRepo repository.JobRepository) *AnalyzeHandler {
	return &AnalyzeHandler{
		analyzerSvc: analyzerSvc,
		jobRepo:     jobRepo,
	}
}

// NewAnalyzeHandlerWithStorage creates a new analyze handler with storage service for debug capture.
func NewAnalyzeHandlerWithStorage(analyzerSvc *service.AnalyzerService, jobRepo repository.JobRepository, storageSvc *service.StorageService) *AnalyzeHandler {
	return &AnalyzeHandler{
		analyzerSvc: analyzerSvc,
		jobRepo:     jobRepo,
		storageSvc:  storageSvc,
	}
}

// AnalyzeInput represents analyze request.
type AnalyzeInput struct {
	Body struct {
		URL       string `json:"url" minLength:"1" doc:"URL to analyze"`
		Depth     *int   `json:"depth,omitempty" minimum:"0" maximum:"1" default:"0" doc:"Crawl depth: 0=single page, 1=one level deep"`
		FetchMode string `json:"fetch_mode,omitempty" enum:"auto,static,dynamic" default:"auto" doc:"Fetch mode: auto, static, or dynamic"`
		Debug     *bool  `json:"debug,omitempty" doc:"Capture debug information (LLM prompts/responses). Defaults to true for analyze jobs."`
	}
}

// AnalyzeOutput represents analyze response.
type AnalyzeOutput struct {
	Body AnalyzeResponseBody
}

// AnalyzeResponseBody contains the analysis result.
type AnalyzeResponseBody struct {
	JobID                string                  `json:"job_id" doc:"Unique job ID for this analysis (for tracking/history)"`
	SiteSummary          string                  `json:"site_summary" doc:"Brief description of what the site/page is about"`
	PageType             string                  `json:"page_type" doc:"Detected page type: listing, detail, article, product, recipe, unknown"`
	DetectedElements     []DetectedElementOutput `json:"detected_elements" doc:"Data elements detected on the page"`
	SuggestedSchema      string                  `json:"suggested_schema" doc:"YAML schema suggestion for extraction"`
	FollowPatterns       []FollowPatternOutput   `json:"follow_patterns" doc:"URL/selector patterns for crawling"`
	SampleLinks          []string                `json:"sample_links" doc:"Sample links found on the page"`
	RecommendedFetchMode string                  `json:"recommended_fetch_mode" doc:"Recommended fetch mode: static or dynamic"`
	SampleData           any                     `json:"sample_data,omitempty" doc:"Optional preview extraction result"`
}

// DetectedElementOutput represents a detected data element.
type DetectedElementOutput struct {
	Name        string `json:"name" doc:"Suggested field name"`
	Type        string `json:"type" doc:"Data type: string, number, boolean, array, url, date"`
	Count       int    `json:"count,omitempty" doc:"Number of occurrences"`
	Description string `json:"description" doc:"What this element represents"`
}

// FollowPatternOutput represents a follow pattern for crawling.
type FollowPatternOutput struct {
	Pattern     string   `json:"pattern" doc:"CSS selector or URL pattern"`
	Description string   `json:"description" doc:"What this pattern targets"`
	SampleURLs  []string `json:"sample_urls,omitempty" doc:"Example URLs matching this pattern"`
}

// Analyze handles URL analysis requests.
func (h *AnalyzeHandler) Analyze(ctx context.Context, input *AnalyzeInput) (*AnalyzeOutput, error) {
	// Extract user context from JWT claims
	uc := ExtractUserContext(ctx)
	if !uc.IsAuthenticated() {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	// Debug capture defaults to true for analyze jobs (unlike crawl/extract which default to false)
	captureDebug := true
	if input.Body.Debug != nil {
		captureDebug = *input.Body.Debug
	}

	// Create job record to track this analysis
	now := time.Now()
	startedAt := now
	job := &models.Job{
		ID:           ulid.Make().String(),
		UserID:       uc.UserID,
		Type:         models.JobTypeAnalyze,
		Status:       models.JobStatusRunning,
		URL:          input.Body.URL,
		CaptureDebug: captureDebug,
		StartedAt:    &startedAt,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	// Best effort: create job record. Analysis proceeds regardless.
	_ = h.jobRepo.Create(ctx, job)

	// Determine depth with default of 0
	depth := 0
	if input.Body.Depth != nil {
		depth = *input.Body.Depth
	}

	// Perform analysis
	result, err := h.analyzerSvc.Analyze(ctx, uc.UserID, service.AnalyzeInput{
		URL:       input.Body.URL,
		Depth:     depth,
		FetchMode: input.Body.FetchMode,
	}, uc.Tier, uc.BYOKAllowed, uc.ModelsCustomAllowed)

	if err != nil {
		// Update job with failure using shared utilities
		_ = FailJobDirect(ctx, h.jobRepo, job, JobFailureInput{
			ErrorMessage: err.Error(),
		})

		return nil, huma.Error500InternalServerError("analysis failed: " + err.Error())
	}

	// Update job with success using shared utilities
	resultJSON, _ := json.Marshal(result)
	_ = CompleteJobDirect(ctx, h.jobRepo, job, JobCompletionInput{
		TokensInput:  result.TokenUsage.InputTokens,
		TokensOutput: result.TokenUsage.OutputTokens,
		CostUSD:      result.TokenUsage.CostUSD,
		LLMCostUSD:   result.TokenUsage.LLMCostUSD,
		IsBYOK:       result.TokenUsage.IsBYOK,
		LLMProvider:  result.TokenUsage.LLMProvider,
		LLMModel:     result.TokenUsage.LLMModel,
		PageCount:    1,
		ResultJSON:   string(resultJSON),
	})

	// Store debug capture if enabled and storage is available
	if captureDebug && h.storageSvc != nil && h.storageSvc.IsEnabled() && result.DebugCapture != nil {
		capture := service.LLMRequestCapture{
			ID:        ulid.Make().String(),
			URL:       input.Body.URL,
			Timestamp: now,
			Request: service.LLMRequestMeta{
				Provider:    result.TokenUsage.LLMProvider,
				Model:       result.TokenUsage.LLMModel,
				FetchMode:   result.DebugCapture.FetchMode,
				ContentSize: len(result.DebugCapture.RawContent),
				PromptSize:  len(result.DebugCapture.Prompt),
			},
			Response: service.LLMResponseMeta{
				InputTokens:  result.TokenUsage.InputTokens,
				OutputTokens: result.TokenUsage.OutputTokens,
				DurationMs:   result.DebugCapture.DurationMs,
				Success:      true,
			},
			Prompt:     result.DebugCapture.Prompt,
			RawContent: result.DebugCapture.RawContent,
		}

		jobDebugCapture := &service.JobDebugCapture{
			JobID:    job.ID,
			Enabled:  true,
			Captures: []service.LLMRequestCapture{capture},
		}
		// Best effort - don't fail the request if debug storage fails
		_ = h.storageSvc.StoreDebugCapture(ctx, jobDebugCapture)
	}

	// Convert result to output format
	output := &AnalyzeOutput{
		Body: AnalyzeResponseBody{
			JobID:                job.ID,
			SiteSummary:          result.SiteSummary,
			PageType:             string(result.PageType),
			SuggestedSchema:      result.SuggestedSchema,
			SampleLinks:          result.SampleLinks,
			RecommendedFetchMode: string(result.RecommendedFetchMode),
			SampleData:           result.SampleData,
		},
	}

	// Convert detected elements
	for _, elem := range result.DetectedElements {
		output.Body.DetectedElements = append(output.Body.DetectedElements, DetectedElementOutput{
			Name:        elem.Name,
			Type:        elem.Type,
			Count:       elem.Count.Int(),
			Description: elem.Description,
		})
	}

	// Convert follow patterns
	for _, pattern := range result.FollowPatterns {
		output.Body.FollowPatterns = append(output.Body.FollowPatterns, FollowPatternOutput{
			Pattern:     pattern.Pattern,
			Description: pattern.Description,
			SampleURLs:  pattern.SampleURLs,
		})
	}

	return output, nil
}
