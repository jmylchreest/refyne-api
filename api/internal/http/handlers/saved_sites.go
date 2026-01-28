package handlers

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"github.com/jmylchreest/refyne-api/internal/models"
	"github.com/jmylchreest/refyne-api/internal/repository"
	"github.com/jmylchreest/refyne-api/internal/service"
)

// SavedSitesHandler handles saved sites endpoints.
type SavedSitesHandler struct {
	repo repository.SavedSitesRepository
}

// NewSavedSitesHandler creates a new saved sites handler.
func NewSavedSitesHandler(repo repository.SavedSitesRepository) *SavedSitesHandler {
	return &SavedSitesHandler{repo: repo}
}

// CrawlOptionsOutput represents crawl options in API responses.
type CrawlOptionsOutput struct {
	FollowSelector string `json:"follow_selector,omitempty" doc:"CSS selector for links to follow"`
	FollowPattern  string `json:"follow_pattern,omitempty" doc:"Regex pattern for URLs to filter"`
	MaxPages       int    `json:"max_pages,omitempty" doc:"Max pages (0 = no limit)"`
	MaxDepth       int    `json:"max_depth,omitempty" doc:"Max crawl depth"`
}

// SavedSiteOutput represents a saved site in API responses.
type SavedSiteOutput struct {
	ID              string               `json:"id" doc:"Site ID"`
	UserID          string               `json:"user_id" doc:"Owner user ID"`
	OrganizationID  *string              `json:"organization_id,omitempty" doc:"Organization ID for sharing"`
	URL             string               `json:"url" doc:"Site URL"`
	Domain          string               `json:"domain" doc:"Extracted domain"`
	Name            string               `json:"name,omitempty" doc:"User-friendly name"`
	AnalysisResult  *AnalysisResultOutput `json:"analysis_result,omitempty" doc:"Analysis result"`
	DefaultSchemaID *string              `json:"default_schema_id,omitempty" doc:"Default schema to use"`
	CrawlOptions    *CrawlOptionsOutput  `json:"crawl_options,omitempty" doc:"Saved crawl options"`
	FetchMode       string               `json:"fetch_mode" doc:"Fetch mode: auto, static, dynamic"`
	CreatedAt       string               `json:"created_at" doc:"Creation timestamp"`
	UpdatedAt       string               `json:"updated_at" doc:"Last update timestamp"`
}

// AnalysisResultOutput represents analysis result in API responses.
type AnalysisResultOutput struct {
	SiteSummary          string                  `json:"site_summary" doc:"Brief site description"`
	PageType             string                  `json:"page_type" doc:"Detected page type"`
	DetectedElements     []DetectedElementOutput `json:"detected_elements" doc:"Detected data elements"`
	SuggestedSchema      string                  `json:"suggested_schema" doc:"Schema suggestion (JSON format)"`
	FollowPatterns       []FollowPatternOutput   `json:"follow_patterns" doc:"Follow patterns"`
	SampleLinks          []string                `json:"sample_links" doc:"Sample links found"`
	RecommendedFetchMode string                  `json:"recommended_fetch_mode" doc:"Recommended fetch mode"`
}

// ListSavedSitesOutput represents list sites response.
type ListSavedSitesOutput struct {
	Body struct {
		Sites []SavedSiteOutput `json:"sites" doc:"List of saved sites"`
	}
}

// ListSavedSites returns saved sites for the user.
func (h *SavedSitesHandler) ListSavedSites(ctx context.Context, input *struct{}) (*ListSavedSitesOutput, error) {
	userID := getUserID(ctx)
	if userID == "" {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	sites, err := h.repo.ListByUserID(ctx, userID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list sites: " + err.Error())
	}

	output := &ListSavedSitesOutput{}
	for _, s := range sites {
		output.Body.Sites = append(output.Body.Sites, siteToOutput(s))
	}

	return output, nil
}

// GetSavedSiteInput represents get site request.
type GetSavedSiteInput struct {
	ID string `path:"id" doc:"Site ID"`
}

// GetSavedSiteOutput represents get site response.
type GetSavedSiteOutput struct {
	Body SavedSiteOutput
}

// GetSavedSite retrieves a single saved site.
func (h *SavedSitesHandler) GetSavedSite(ctx context.Context, input *GetSavedSiteInput) (*GetSavedSiteOutput, error) {
	userID := getUserID(ctx)
	if userID == "" {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	site, err := h.repo.GetByID(ctx, input.ID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get site: " + err.Error())
	}
	if site == nil {
		return nil, huma.Error404NotFound("site not found")
	}

	// Check ownership
	if site.UserID != userID {
		return nil, huma.Error403Forbidden("access denied")
	}

	return &GetSavedSiteOutput{Body: siteToOutput(site)}, nil
}

// AnalysisResultInput represents analysis result in request body.
type AnalysisResultInput struct {
	SiteSummary          string                 `json:"site_summary,omitempty" doc:"Brief site description"`
	PageType             string                 `json:"page_type,omitempty" doc:"Detected page type"`
	DetectedElements     []DetectedElementInput `json:"detected_elements,omitempty" doc:"Detected data elements"`
	SuggestedSchema      string                 `json:"suggested_schema,omitempty" doc:"Schema (JSON or YAML format)"`
	FollowPatterns       []FollowPatternInput   `json:"follow_patterns,omitempty" doc:"Follow patterns"`
	SampleLinks          []string               `json:"sample_links,omitempty" doc:"Sample links found"`
	RecommendedFetchMode string                 `json:"recommended_fetch_mode,omitempty" doc:"Recommended fetch mode"`
}

// DetectedElementInput represents a detected element in request body.
type DetectedElementInput struct {
	Name        string `json:"name,omitempty" doc:"Element name"`
	Type        string `json:"type,omitempty" doc:"Element type"`
	Count       int    `json:"count,omitempty" doc:"Element count"`
	Description string `json:"description,omitempty" doc:"Element description"`
}

// FollowPatternInput represents a follow pattern in request body.
type FollowPatternInput struct {
	Pattern     string   `json:"pattern,omitempty" doc:"URL pattern"`
	Description string   `json:"description,omitempty" doc:"Pattern description"`
	SampleURLs  []string `json:"sample_urls,omitempty" doc:"Sample matching URLs"`
}

// CrawlOptionsInput represents crawl options in request body.
type CrawlOptionsInput struct {
	FollowSelector string `json:"follow_selector,omitempty" doc:"CSS selector for links to follow"`
	FollowPattern  string `json:"follow_pattern,omitempty" doc:"Regex pattern for URLs to filter"`
	MaxPages       int    `json:"max_pages,omitempty" doc:"Max pages (0 = no limit)"`
	MaxDepth       int    `json:"max_depth,omitempty" doc:"Max crawl depth"`
}

// CreateSavedSiteInput represents create site request.
type CreateSavedSiteInput struct {
	Body struct {
		URL             string               `json:"url" minLength:"1" doc:"Site URL"`
		Name            string               `json:"name,omitempty" doc:"User-friendly name"`
		AnalysisResult  *AnalysisResultInput `json:"analysis_result,omitempty" doc:"Analysis result to save"`
		DefaultSchemaID string               `json:"default_schema_id,omitempty" doc:"Default schema ID"`
		CrawlOptions    *CrawlOptionsInput   `json:"crawl_options,omitempty" doc:"Crawl options"`
		FetchMode       string               `json:"fetch_mode,omitempty" enum:"auto,static,dynamic" default:"auto" doc:"Fetch mode"`
	}
}

// CreateSavedSiteOutput represents create site response.
type CreateSavedSiteOutput struct {
	Body SavedSiteOutput
}

// CreateSavedSite creates a new saved site.
func (h *SavedSitesHandler) CreateSavedSite(ctx context.Context, input *CreateSavedSiteInput) (*CreateSavedSiteOutput, error) {
	userID := getUserID(ctx)
	if userID == "" {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	// Extract domain from URL
	domain := service.ExtractDomain(input.Body.URL)

	fetchMode := input.Body.FetchMode
	if fetchMode == "" {
		fetchMode = "auto"
	}

	site := &models.SavedSite{
		UserID:    userID,
		URL:       input.Body.URL,
		Domain:    domain,
		Name:      input.Body.Name,
		FetchMode: models.FetchMode(fetchMode),
	}

	// Set DefaultSchemaID if provided
	if input.Body.DefaultSchemaID != "" {
		site.DefaultSchemaID = &input.Body.DefaultSchemaID
	}

	// Convert analysis result if provided
	if input.Body.AnalysisResult != nil {
		site.AnalysisResult = inputToAnalysisResult(input.Body.AnalysisResult)
	}

	// Convert crawl options if provided
	if input.Body.CrawlOptions != nil {
		site.CrawlOptions = &models.SavedSiteCrawlOptions{
			FollowSelector: input.Body.CrawlOptions.FollowSelector,
			FollowPattern:  input.Body.CrawlOptions.FollowPattern,
			MaxPages:       input.Body.CrawlOptions.MaxPages,
			MaxDepth:       input.Body.CrawlOptions.MaxDepth,
		}
	}

	if err := h.repo.Create(ctx, site); err != nil {
		return nil, huma.Error500InternalServerError("failed to create site: " + err.Error())
	}

	return &CreateSavedSiteOutput{Body: siteToOutput(site)}, nil
}

// UpdateSavedSiteInput represents update site request.
type UpdateSavedSiteInput struct {
	ID   string `path:"id" doc:"Site ID"`
	Body struct {
		URL             string               `json:"url,omitempty" doc:"Site URL (ignored on update)"`
		Name            string               `json:"name,omitempty" doc:"User-friendly name"`
		AnalysisResult  *AnalysisResultInput `json:"analysis_result,omitempty" doc:"Analysis result to update"`
		DefaultSchemaID string               `json:"default_schema_id,omitempty" doc:"Default schema ID"`
		CrawlOptions    *CrawlOptionsInput   `json:"crawl_options,omitempty" doc:"Crawl options"`
		FetchMode       string               `json:"fetch_mode,omitempty" enum:"auto,static,dynamic" doc:"Fetch mode"`
	}
}

// UpdateSavedSiteOutput represents update site response.
type UpdateSavedSiteOutput struct {
	Body SavedSiteOutput
}

// UpdateSavedSite updates an existing saved site.
func (h *SavedSitesHandler) UpdateSavedSite(ctx context.Context, input *UpdateSavedSiteInput) (*UpdateSavedSiteOutput, error) {
	userID := getUserID(ctx)
	if userID == "" {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	site, err := h.repo.GetByID(ctx, input.ID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get site: " + err.Error())
	}
	if site == nil {
		return nil, huma.Error404NotFound("site not found")
	}

	// Check ownership
	if site.UserID != userID {
		return nil, huma.Error403Forbidden("access denied")
	}

	// Update fields (URL is ignored - cannot change site URL after creation)
	if input.Body.Name != "" {
		site.Name = input.Body.Name
	}
	if input.Body.DefaultSchemaID != "" {
		site.DefaultSchemaID = &input.Body.DefaultSchemaID
	}
	// Update analysis result if provided
	if input.Body.AnalysisResult != nil {
		site.AnalysisResult = inputToAnalysisResult(input.Body.AnalysisResult)
	}
	// Update crawl options if provided
	if input.Body.CrawlOptions != nil {
		site.CrawlOptions = &models.SavedSiteCrawlOptions{
			FollowSelector: input.Body.CrawlOptions.FollowSelector,
			FollowPattern:  input.Body.CrawlOptions.FollowPattern,
			MaxPages:       input.Body.CrawlOptions.MaxPages,
			MaxDepth:       input.Body.CrawlOptions.MaxDepth,
		}
	}
	if input.Body.FetchMode != "" {
		site.FetchMode = models.FetchMode(input.Body.FetchMode)
	}

	if err := h.repo.Update(ctx, site); err != nil {
		return nil, huma.Error500InternalServerError("failed to update site: " + err.Error())
	}

	return &UpdateSavedSiteOutput{Body: siteToOutput(site)}, nil
}

// DeleteSavedSiteInput represents delete site request.
type DeleteSavedSiteInput struct {
	ID string `path:"id" doc:"Site ID"`
}

// DeleteSavedSiteOutput represents delete site response.
type DeleteSavedSiteOutput struct {
	Body struct {
		Success bool `json:"success" doc:"Whether deletion was successful"`
	}
}

// DeleteSavedSite deletes a saved site.
func (h *SavedSitesHandler) DeleteSavedSite(ctx context.Context, input *DeleteSavedSiteInput) (*DeleteSavedSiteOutput, error) {
	userID := getUserID(ctx)
	if userID == "" {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	site, err := h.repo.GetByID(ctx, input.ID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get site: " + err.Error())
	}
	if site == nil {
		return nil, huma.Error404NotFound("site not found")
	}

	// Check ownership
	if site.UserID != userID {
		return nil, huma.Error403Forbidden("access denied")
	}

	if err := h.repo.Delete(ctx, input.ID); err != nil {
		return nil, huma.Error500InternalServerError("failed to delete site: " + err.Error())
	}

	return &DeleteSavedSiteOutput{Body: struct {
		Success bool `json:"success" doc:"Whether deletion was successful"`
	}{Success: true}}, nil
}

// Helper functions

func siteToOutput(s *models.SavedSite) SavedSiteOutput {
	output := SavedSiteOutput{
		ID:              s.ID,
		UserID:          s.UserID,
		OrganizationID:  s.OrganizationID,
		URL:             s.URL,
		Domain:          s.Domain,
		Name:            s.Name,
		DefaultSchemaID: s.DefaultSchemaID,
		FetchMode:       string(s.FetchMode),
		CreatedAt:       s.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:       s.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	if s.AnalysisResult != nil {
		output.AnalysisResult = analysisResultToOutput(s.AnalysisResult)
	}

	if s.CrawlOptions != nil {
		output.CrawlOptions = &CrawlOptionsOutput{
			FollowSelector: s.CrawlOptions.FollowSelector,
			FollowPattern:  s.CrawlOptions.FollowPattern,
			MaxPages:       s.CrawlOptions.MaxPages,
			MaxDepth:       s.CrawlOptions.MaxDepth,
		}
	}

	return output
}

func analysisResultToOutput(r *models.AnalysisResult) *AnalysisResultOutput {
	output := &AnalysisResultOutput{
		SiteSummary:          r.SiteSummary,
		PageType:             string(r.PageType),
		SuggestedSchema:      r.SuggestedSchema,
		SampleLinks:          r.SampleLinks,
		RecommendedFetchMode: string(r.RecommendedFetchMode),
	}

	for _, elem := range r.DetectedElements {
		output.DetectedElements = append(output.DetectedElements, DetectedElementOutput{
			Name:        elem.Name,
			Type:        elem.Type,
			Count:       elem.Count.Int(),
			Description: elem.Description,
		})
	}

	for _, pattern := range r.FollowPatterns {
		output.FollowPatterns = append(output.FollowPatterns, FollowPatternOutput{
			Pattern:     pattern.Pattern,
			Description: pattern.Description,
			SampleURLs:  pattern.SampleURLs,
		})
	}

	return output
}

func inputToAnalysisResult(input *AnalysisResultInput) *models.AnalysisResult {
	result := &models.AnalysisResult{
		SiteSummary:          input.SiteSummary,
		PageType:             models.PageType(input.PageType),
		SuggestedSchema:      input.SuggestedSchema,
		SampleLinks:          input.SampleLinks,
		RecommendedFetchMode: models.FetchMode(input.RecommendedFetchMode),
	}

	for _, elem := range input.DetectedElements {
		result.DetectedElements = append(result.DetectedElements, models.DetectedElement{
			Name:        elem.Name,
			Type:        elem.Type,
			Count:       models.FlexInt(elem.Count),
			Description: elem.Description,
		})
	}

	for _, pattern := range input.FollowPatterns {
		result.FollowPatterns = append(result.FollowPatterns, models.FollowPattern{
			Pattern:     pattern.Pattern,
			Description: pattern.Description,
			SampleURLs:  pattern.SampleURLs,
		})
	}

	return result
}
