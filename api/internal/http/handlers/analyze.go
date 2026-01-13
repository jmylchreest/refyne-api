package handlers

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"github.com/jmylchreest/refyne-api/internal/http/mw"
	"github.com/jmylchreest/refyne-api/internal/models"
	"github.com/jmylchreest/refyne-api/internal/service"
)

// AnalyzeHandler handles URL analysis endpoints.
type AnalyzeHandler struct {
	analyzerSvc *service.AnalyzerService
}

// NewAnalyzeHandler creates a new analyze handler.
func NewAnalyzeHandler(analyzerSvc *service.AnalyzerService) *AnalyzeHandler {
	return &AnalyzeHandler{analyzerSvc: analyzerSvc}
}

// AnalyzeInput represents analyze request.
type AnalyzeInput struct {
	Body struct {
		URL       string `json:"url" minLength:"1" doc:"URL to analyze"`
		Depth     int    `json:"depth" minimum:"0" maximum:"1" default:"0" doc:"Crawl depth: 0=single page, 1=one level deep"`
		FetchMode string `json:"fetch_mode,omitempty" enum:"auto,static,dynamic" default:"auto" doc:"Fetch mode: auto, static, or dynamic"`
	}
}

// AnalyzeOutput represents analyze response.
type AnalyzeOutput struct {
	Body AnalyzeResponseBody
}

// AnalyzeResponseBody contains the analysis result.
type AnalyzeResponseBody struct {
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
	userID := getUserID(ctx)
	if userID == "" {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	// Get user's tier from claims
	tier := "free"
	if claims := mw.GetUserClaims(ctx); claims != nil && claims.Tier != "" {
		tier = claims.Tier
	}

	// Perform analysis
	result, err := h.analyzerSvc.Analyze(ctx, userID, service.AnalyzeInput{
		URL:       input.Body.URL,
		Depth:     input.Body.Depth,
		FetchMode: input.Body.FetchMode,
	}, tier)
	if err != nil {
		return nil, huma.Error500InternalServerError("analysis failed: " + err.Error())
	}

	// Convert result to output format
	output := &AnalyzeOutput{
		Body: AnalyzeResponseBody{
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
			Count:       elem.Count,
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

// pageTypeToString converts PageType to string.
func pageTypeToString(pt models.PageType) string {
	return string(pt)
}
