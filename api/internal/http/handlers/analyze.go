package handlers

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"github.com/jmylchreest/refyne-api/internal/service"
)

// AnalyzeHandler handles URL analysis endpoints.
type AnalyzeHandler struct {
	analyzerSvc *service.AnalyzerService
	jobSvc      *service.JobService
}

// NewAnalyzeHandler creates a new analyze handler.
func NewAnalyzeHandler(analyzerSvc *service.AnalyzerService, jobSvc *service.JobService) *AnalyzeHandler {
	return &AnalyzeHandler{
		analyzerSvc: analyzerSvc,
		jobSvc:      jobSvc,
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
// Uses the unified JobService.RunJob for consistent job lifecycle management including webhooks.
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

	// Determine depth with default of 0
	depth := 0
	if input.Body.Depth != nil {
		depth = *input.Body.Depth
	}

	// Create executor
	executor := service.NewAnalyzeExecutor(
		h.analyzerSvc,
		service.AnalyzeInput{
			URL:       input.Body.URL,
			Depth:     depth,
			FetchMode: input.Body.FetchMode,
		},
		uc.UserID,
		uc.Tier,
		uc.BYOKAllowed,
		uc.ModelsCustomAllowed,
		uc.ContentDynamicAllowed,
	)

	// Run job with full lifecycle management (creates job, executes, handles webhooks)
	var jobID string
	var result *service.AnalyzeOutput

	if h.jobSvc != nil {
		runResult, err := h.jobSvc.RunJob(ctx, executor, &service.RunJobOptions{
			UserID:       uc.UserID,
			Tier:         uc.Tier,
			CaptureDebug: captureDebug, // Default true for analyze
		})
		if err != nil {
			return nil, NewJobError(err, uc.BYOKAllowed)
		}

		// Get job ID from the run result
		jobID = runResult.JobID

		// Extract the full AnalyzeOutput from the execution result (stored in Data field)
		if runResult.Result != nil && runResult.Result.Data != nil {
			if analyzeResult, ok := runResult.Result.Data.(*service.AnalyzeOutput); ok {
				result = analyzeResult
			}
		}
	}

	// Fallback: direct analysis without job tracking (if jobSvc is nil or result extraction failed)
	if result == nil {
		directResult, directErr := h.analyzerSvc.Analyze(ctx, uc.UserID, service.AnalyzeInput{
			URL:       input.Body.URL,
			Depth:     depth,
			FetchMode: input.Body.FetchMode,
		}, uc.Tier, uc.BYOKAllowed, uc.ModelsCustomAllowed, uc.ContentDynamicAllowed)
		if directErr != nil {
			return nil, NewJobError(directErr, uc.BYOKAllowed)
		}
		result = directResult
	}

	// Convert result to output format
	output := &AnalyzeOutput{
		Body: AnalyzeResponseBody{
			JobID:                jobID,
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
