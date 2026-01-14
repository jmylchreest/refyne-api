package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/gocolly/colly/v2"
	"github.com/refyne/refyne/pkg/cleaner"

	"github.com/jmylchreest/refyne-api/internal/config"
	"github.com/jmylchreest/refyne-api/internal/crypto"
	"github.com/jmylchreest/refyne-api/internal/models"
	"github.com/jmylchreest/refyne-api/internal/repository"
)

// AnalyzerService handles URL analysis and schema generation.
type AnalyzerService struct {
	cfg             *config.Config
	repos           *repository.Repositories
	billing         *BillingService
	logger          *slog.Logger
	encryptor       *crypto.Encryptor
	cleaner         cleaner.Cleaner
	fallbackCleaner cleaner.Cleaner // Used when content is too large for context window
}

// NewAnalyzerService creates a new analyzer service (legacy constructor).
func NewAnalyzerService(cfg *config.Config, repos *repository.Repositories, logger *slog.Logger) *AnalyzerService {
	return NewAnalyzerServiceWithBilling(cfg, repos, nil, logger)
}

// NewAnalyzerServiceWithBilling creates an analyzer service with billing integration.
func NewAnalyzerServiceWithBilling(cfg *config.Config, repos *repository.Repositories, billing *BillingService, logger *slog.Logger) *AnalyzerService {
	encryptor, err := crypto.NewEncryptor(cfg.EncryptionKey)
	if err != nil {
		logger.Error("failed to create encryptor for analyzer service", "error", err)
	}

	// Create fallback cleaner: Trafilatura with HTML output preserving tables and links
	// Used when raw HTML exceeds context window limits
	fallbackCleaner := cleaner.NewTrafilatura(&cleaner.TrafilaturaConfig{
		Output: cleaner.OutputHTML, // HTML output preserves structure
		Tables: cleaner.Include,    // Explicitly preserve tables
		Links:  cleaner.Include,    // Explicitly preserve links
		Images: cleaner.Exclude,    // Exclude images to save tokens
	})

	return &AnalyzerService{
		cfg:             cfg,
		repos:           repos,
		billing:         billing,
		logger:          logger,
		encryptor:       encryptor,
		cleaner:         cleaner.NewNoop(), // Use noop cleaner - raw HTML for analysis
		fallbackCleaner: fallbackCleaner,
	}
}

// AnalyzeInput represents input for the analyze operation.
type AnalyzeInput struct {
	URL       string `json:"url"`
	Depth     int    `json:"depth"`     // 0 = single page, 1 = crawl one level
	FetchMode string `json:"fetch_mode"` // auto, static, dynamic
}

// AnalyzeTokenUsage represents token consumption for an analysis.
type AnalyzeTokenUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// AnalyzeOutput represents the output from analysis.
type AnalyzeOutput struct {
	SiteSummary          string                   `json:"site_summary"`
	PageType             models.PageType          `json:"page_type"`
	DetectedElements     []models.DetectedElement `json:"detected_elements"`
	SuggestedSchema      string                   `json:"suggested_schema"` // YAML
	FollowPatterns       []models.FollowPattern   `json:"follow_patterns"`
	SampleLinks          []string                 `json:"sample_links"`
	RecommendedFetchMode models.FetchMode         `json:"recommended_fetch_mode"`
	SampleData           any                      `json:"sample_data,omitempty"` // Preview extraction result
	TokenUsage           AnalyzeTokenUsage        `json:"token_usage"`
}

// llmCallResult holds the result of an LLM API call including token usage.
type llmCallResult struct {
	Content      string
	InputTokens  int
	OutputTokens int
}

// Analyze fetches and analyzes a URL to generate schema suggestions.
func (s *AnalyzerService) Analyze(ctx context.Context, userID string, input AnalyzeInput, tier string) (*AnalyzeOutput, error) {
	startTime := time.Now()

	// Normalize URL - add https:// if no scheme present
	targetURL := normalizeURL(input.URL)

	s.logger.Info("starting URL analysis",
		"user_id", userID,
		"url", targetURL,
		"depth", input.Depth,
		"tier", tier,
	)

	// Get LLM config for analysis early (needed for error recording)
	s.logger.Debug("resolving LLM config", "user_id", userID, "tier", tier)
	llmConfig, isBYOK := s.resolveLLMConfig(ctx, userID, tier)
	s.logger.Debug("LLM config resolved",
		"provider", llmConfig.Provider,
		"model", llmConfig.Model,
		"has_api_key", llmConfig.APIKey != "",
		"is_byok", isBYOK,
	)

	// Fetch main page content
	fetchStart := time.Now()
	s.logger.Debug("fetching main page content", "url", targetURL)
	mainContent, links, fetchMode, err := s.fetchContent(ctx, targetURL, input.FetchMode)
	if err != nil {
		s.logger.Error("failed to fetch page content", "url", targetURL, "error", err)
		s.recordAnalyzeUsage(ctx, userID, tier, targetURL, llmConfig, isBYOK, 0, 0,
			int(time.Since(fetchStart).Milliseconds()), 0, int(time.Since(startTime).Milliseconds()),
			"failed", err.Error())
		return nil, fmt.Errorf("failed to fetch page content: %w", err)
	}
	s.logger.Debug("page content fetched",
		"content_length", len(mainContent),
		"links_found", len(links),
		"fetch_mode", fetchMode,
	)

	// Fetch sample detail pages if depth > 0
	var detailContents []string
	if input.Depth > 0 && len(links) > 0 {
		// Fetch up to 3 sample detail pages
		maxSamples := 3
		if len(links) < maxSamples {
			maxSamples = len(links)
		}
		for i := 0; i < maxSamples; i++ {
			content, _, _, err := s.fetchContent(ctx, links[i], string(fetchMode))
			if err != nil {
				s.logger.Warn("failed to fetch detail page", "url", links[i], "error", err)
				continue
			}
			detailContents = append(detailContents, content)
		}
	}
	fetchDuration := time.Since(fetchStart)

	// Generate analysis prompt and call LLM
	// First try with raw content (noop cleaner), retry with fallback cleaner on context length errors
	llmStart := time.Now()
	s.logger.Debug("calling LLM for analysis", "provider", llmConfig.Provider, "model", llmConfig.Model)
	result, err := s.analyzeWithLLM(ctx, mainContent, detailContents, links, llmConfig)
	llmDuration := time.Since(llmStart)

	// If context length error, retry with cleaned content
	if err != nil && isContextLengthError(err) {
		s.logger.Info("context length error detected, retrying with cleaned content",
			"original_content_length", len(mainContent),
			"error", err.Error(),
		)

		// Clean main content with fallback cleaner (Trafilatura HTML with tables/links)
		cleanedMain, cleanErr := s.fallbackCleaner.Clean(mainContent)
		if cleanErr != nil {
			s.logger.Warn("fallback cleaner failed, using original content", "error", cleanErr)
			cleanedMain = mainContent
		} else {
			s.logger.Debug("content cleaned for retry",
				"original_length", len(mainContent),
				"cleaned_length", len(cleanedMain),
				"reduction_percent", 100-int(float64(len(cleanedMain))/float64(len(mainContent))*100),
			)
		}

		// Clean detail contents as well
		var cleanedDetails []string
		for _, detail := range detailContents {
			cleaned, err := s.fallbackCleaner.Clean(detail)
			if err != nil {
				cleaned = detail // Keep original on error
			}
			cleanedDetails = append(cleanedDetails, cleaned)
		}

		// Retry with cleaned content
		llmRetryStart := time.Now()
		result, err = s.analyzeWithLLM(ctx, cleanedMain, cleanedDetails, links, llmConfig)
		llmDuration = time.Since(llmStart) + time.Since(llmRetryStart) // Total LLM time including retry

		if err != nil {
			s.logger.Error("LLM analysis failed after retry with cleaned content",
				"provider", llmConfig.Provider,
				"model", llmConfig.Model,
				"error", err,
			)
			s.recordAnalyzeUsage(ctx, userID, tier, targetURL, llmConfig, isBYOK, 0, 0,
				int(fetchDuration.Milliseconds()), int(llmDuration.Milliseconds()), int(time.Since(startTime).Milliseconds()),
				"failed", "retry failed: "+err.Error())
			return nil, fmt.Errorf("LLM analysis failed after retry: %w", err)
		}
		s.logger.Info("analysis succeeded after retry with cleaned content")
	} else if err != nil {
		s.logger.Error("LLM analysis failed",
			"provider", llmConfig.Provider,
			"model", llmConfig.Model,
			"error", err,
		)
		s.recordAnalyzeUsage(ctx, userID, tier, targetURL, llmConfig, isBYOK, 0, 0,
			int(fetchDuration.Milliseconds()), int(llmDuration.Milliseconds()), int(time.Since(startTime).Milliseconds()),
			"failed", err.Error())
		return nil, fmt.Errorf("LLM analysis failed: %w", err)
	}

	// Record successful usage
	s.recordAnalyzeUsage(ctx, userID, tier, targetURL, llmConfig, isBYOK,
		result.InputTokens, result.OutputTokens,
		int(fetchDuration.Milliseconds()), int(llmDuration.Milliseconds()), int(time.Since(startTime).Milliseconds()),
		"success", "")

	result.Output.RecommendedFetchMode = fetchMode
	if len(links) > 0 {
		result.Output.SampleLinks = links[:min(10, len(links))] // Return up to 10 sample links
	}

	// Add token usage to output
	result.Output.TokenUsage = AnalyzeTokenUsage{
		InputTokens:  result.InputTokens,
		OutputTokens: result.OutputTokens,
	}

	s.logger.Info("URL analysis completed",
		"user_id", userID,
		"url", targetURL,
		"page_type", result.Output.PageType,
		"input_tokens", result.InputTokens,
		"output_tokens", result.OutputTokens,
	)

	return result.Output, nil
}

// recordAnalyzeUsage records usage for an analyze operation.
func (s *AnalyzerService) recordAnalyzeUsage(
	ctx context.Context,
	userID, tier, targetURL string,
	llmConfig *LLMConfigInput,
	isBYOK bool,
	inputTokens, outputTokens int,
	fetchDurationMs, extractDurationMs, totalDurationMs int,
	status, errorMessage string,
) {
	if s.billing == nil {
		return
	}

	var byokProvider string
	if isBYOK {
		byokProvider = llmConfig.Provider
	}

	usageRecord := &UsageRecord{
		UserID:            userID,
		JobType:           models.JobTypeAnalyze,
		Status:            status,
		TotalChargedUSD:   0, // Analyze operations are currently free
		IsBYOK:            isBYOK,
		TargetURL:         targetURL,
		ErrorMessage:      errorMessage,
		TokensInput:       inputTokens,
		TokensOutput:      outputTokens,
		LLMCostUSD:        0, // Could calculate from tokens if needed
		LLMProvider:       llmConfig.Provider,
		LLMModel:          llmConfig.Model,
		BYOKProvider:      byokProvider,
		PagesAttempted:    1,
		PagesSuccessful:   1,
		FetchDurationMs:   fetchDurationMs,
		ExtractDurationMs: extractDurationMs,
		TotalDurationMs:   totalDurationMs,
	}

	if status == "failed" {
		usageRecord.PagesSuccessful = 0
	}

	// Use detached context - we want to record usage even if request timed out
	if err := s.billing.RecordUsage(context.WithoutCancel(ctx), usageRecord); err != nil {
		s.logger.Warn("failed to record analyze usage", "error", err)
	}
}

// fetchContent fetches page content and extracts links.
func (s *AnalyzerService) fetchContent(ctx context.Context, targetURL string, fetchMode string) (string, []string, models.FetchMode, error) {
	// For now, use static fetching with Colly
	// Dynamic fetching (headless Chrome) would be added later for JS-heavy sites

	var content string
	var links []string
	recommendedMode := models.FetchModeStatic

	c := colly.NewCollector(
		colly.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
		colly.AllowURLRevisit(),
	)

	// Set timeout
	c.SetRequestTimeout(30 * time.Second)

	// Capture full HTML
	c.OnResponse(func(r *colly.Response) {
		content = string(r.Body)
	})

	// Extract links
	c.OnHTML("a[href]", func(e *colly.HTMLElement) {
		href := e.Attr("href")
		if href != "" && !strings.HasPrefix(href, "#") && !strings.HasPrefix(href, "javascript:") {
			absURL := e.Request.AbsoluteURL(href)
			if absURL != "" {
				links = append(links, absURL)
			}
		}
	})

	// Detect if page requires JavaScript
	c.OnHTML("script", func(e *colly.HTMLElement) {
		src := e.Attr("src")
		if strings.Contains(src, "react") || strings.Contains(src, "vue") || strings.Contains(src, "angular") {
			recommendedMode = models.FetchModeDynamic
		}
	})

	// Check for common SPA indicators in content
	c.OnResponse(func(r *colly.Response) {
		bodyStr := string(r.Body)
		if strings.Contains(bodyStr, "__NEXT_DATA__") ||
			strings.Contains(bodyStr, "window.__NUXT__") ||
			strings.Contains(bodyStr, "ng-app") ||
			strings.Contains(bodyStr, "id=\"app\"") && strings.Count(bodyStr, "<div") < 10 {
			recommendedMode = models.FetchModeDynamic
		}
	})

	if err := c.Visit(targetURL); err != nil {
		return "", nil, recommendedMode, fmt.Errorf("failed to fetch URL: %w", err)
	}

	// Apply cleaner to content (noop for analyzer - keeps raw HTML)
	cleanedContent, err := s.cleaner.Clean(content)
	if err != nil {
		return "", nil, recommendedMode, fmt.Errorf("failed to clean content: %w", err)
	}

	// Deduplicate and filter links
	links = s.filterLinks(targetURL, links)

	return cleanedContent, links, recommendedMode, nil
}

// filterLinks deduplicates and filters links to the same domain.
func (s *AnalyzerService) filterLinks(baseURL string, links []string) []string {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return links
	}
	baseDomain := parsed.Host

	seen := make(map[string]bool)
	var filtered []string

	for _, link := range links {
		linkParsed, err := url.Parse(link)
		if err != nil {
			continue
		}

		// Only include same-domain links
		if linkParsed.Host != baseDomain {
			continue
		}

		// Skip common non-content links
		path := strings.ToLower(linkParsed.Path)
		if strings.Contains(path, "login") ||
			strings.Contains(path, "signup") ||
			strings.Contains(path, "cart") ||
			strings.Contains(path, "checkout") ||
			strings.Contains(path, "account") ||
			strings.Contains(path, "privacy") ||
			strings.Contains(path, "terms") {
			continue
		}

		// Deduplicate
		if seen[link] {
			continue
		}
		seen[link] = true
		filtered = append(filtered, link)
	}

	return filtered
}

// analyzeResult holds the result of LLM analysis including token usage.
type analyzeResult struct {
	Output       *AnalyzeOutput
	InputTokens  int
	OutputTokens int
}

// analyzeWithLLM calls the LLM to analyze page content and generate a schema.
func (s *AnalyzerService) analyzeWithLLM(ctx context.Context, mainContent string, detailContents []string, links []string, llmConfig *LLMConfigInput) (*analyzeResult, error) {
	// Truncate content to avoid token limits
	mainContent = s.truncateContent(mainContent, 15000)
	for i := range detailContents {
		detailContents[i] = s.truncateContent(detailContents[i], 5000)
	}

	// Build the prompt
	prompt := s.buildAnalysisPrompt(mainContent, detailContents, links)

	// Call LLM API
	llmResult, err := s.callLLM(ctx, llmConfig, prompt)
	if err != nil {
		return nil, err
	}

	// Parse LLM response
	output, err := s.parseAnalysisResponse(llmResult.Content)
	if err != nil {
		return nil, err
	}

	return &analyzeResult{
		Output:       output,
		InputTokens:  llmResult.InputTokens,
		OutputTokens: llmResult.OutputTokens,
	}, nil
}

// truncateContent truncates HTML content to a maximum length while trying to preserve structure.
func (s *AnalyzerService) truncateContent(content string, maxLen int) string {
	if len(content) <= maxLen {
		return content
	}

	// Try to truncate at a tag boundary
	truncated := content[:maxLen]
	lastClose := strings.LastIndex(truncated, ">")
	if lastClose > maxLen/2 {
		return truncated[:lastClose+1]
	}
	return truncated
}

// buildAnalysisPrompt creates the prompt for the LLM to analyze the page.
func (s *AnalyzerService) buildAnalysisPrompt(mainContent string, detailContents []string, links []string) string {
	var sb strings.Builder

	sb.WriteString(`You are an expert data architect analyzing a website to generate a comprehensive extraction schema.

Your goal is to:
1. IDENTIFY what type of organization/business this website represents
2. DETERMINE what structured data would be most valuable to extract
3. GENERATE a schema that produces RICH, COMBINABLE data across multiple pages

## CRITICAL: Writing Effective Field Descriptions

The schema you generate will be used by OTHER AI models to extract data. Your field descriptions are INSTRUCTIONS to those models. Write them accordingly:

BAD description (vague, won't extract consistently):
  description: "The property price"

GOOD description (specific, actionable):
  description: "Property sale price as integer. Extract from prominent price display. If shown with currency symbol, omit symbol. If price is a range, use the lower value."

For EVERY field description, include:
1. **What to extract** - Be specific about the data type and format
2. **Where to find it** - Hint at typical page locations if relevant
3. **Edge cases** - What to do if missing, ambiguous, or in unexpected format
4. **Output format** - Exact format expected (e.g., "ISO date YYYY-MM-DD", "integer in cents", "array of strings")

Additional patterns that improve extraction reliability:
- For multilingual sites: specify which language to prefer, format for including originals: "English Name (原文)"
- For currency/units: specify whether to convert and to what
- For dates: specify format and calendar system handling
- For missing data: explicitly state "null if not found" vs required
- For arrays: specify ordering preference if relevant (e.g., "in order of appearance")

## Main Page Content (HTML)
`)
	sb.WriteString("```html\n")
	sb.WriteString(mainContent)
	sb.WriteString("\n```\n\n")

	if len(detailContents) > 0 {
		sb.WriteString("## Sample Detail Pages (for understanding the site structure)\n")
		for i, content := range detailContents {
			sb.WriteString(fmt.Sprintf("\n### Detail Page %d\n```html\n%s\n```\n", i+1, content))
		}
	}

	if len(links) > 0 {
		sb.WriteString("\n## Sample Links Found on Page\n")
		for i, link := range links[:min(20, len(links))] {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, link))
		}
	}

	sb.WriteString(`

## Schema Design Principles

When generating schemas, follow these principles:

1. **Schema-Level Description is Critical**: The top-level description field should contain:
   - What type of page/site this schema is for
   - CRITICAL OUTPUT REQUIREMENTS as a numbered list
   - Any data transformation rules (currency conversion, date format conversion, language handling)
   - Domain-specific knowledge the extractor needs

2. **Nested Objects for Complex Data**: Use objects within arrays when data has multiple related attributes
   - Good: services: [{name: "...", description: "...", pricing: "..."}]
   - Bad: service_names: ["..."], service_descriptions: ["..."]

3. **Consistent Keys Across Pages**: Schema should work across multiple pages of the same site
   - If crawling /about, /services, /team pages, each should extract into the same structure
   - Empty arrays/nulls are fine when data isn't present on a specific page

4. **Field Descriptions Are Extraction Instructions**: Write descriptions as if you're instructing another AI model how to extract that specific field. Include:
   - Expected format and data type
   - Edge case handling
   - What to return if not found

5. **Appropriate Nesting Depth**: Don't over-flatten. If clients have industries and descriptions, nest them.

6. **Add Source/Metadata Fields When Useful**: For prices, dates, or derived data, consider adding fields to track:
   - source: "page" vs "calculated/converted"
   - confidence indicators
   - original vs processed values

7. **Common Business Data Patterns**:
   - Company info: name, tagline, description, founded, team_size
   - Services/Products: array of objects with name, description, optional pricing/features
   - Clients/Testimonials: array of objects with name, industry, quote/description
   - Team members: array of objects with name, role, bio, image_url
   - Locations: array of strings or objects with city, address, phone
   - Contact: object with email, phone, address

## Example Schemas (Use as templates, adapt to actual site)

### Consultancy/Agency
` + "```yaml" + `
name: Consultancy
description: Extract company info, services, industries served, and clients
fields:
  - name: company_name
    type: string
    description: The name of the company
    required: true
  - name: tagline
    type: string
    description: Company tagline or value proposition
  - name: description
    type: string
    description: Brief overview of what the company does
  - name: founded
    type: string
    description: Year founded or establishment date
  - name: team_size
    type: string
    description: Number of employees or team size description
  - name: services
    type: array
    description: Services or capabilities offered
    items:
      type: object
      properties:
        - name: name
          type: string
          description: Service name
          required: true
        - name: description
          type: string
          description: Service description
  - name: industries
    type: array
    description: Industries or sectors served
    items:
      type: string
  - name: clients
    type: array
    description: Notable clients or case studies
    items:
      type: object
      properties:
        - name: name
          type: string
          description: Client company name
          required: true
        - name: industry
          type: string
          description: Client's industry
        - name: description
          type: string
          description: Work done or testimonial
  - name: locations
    type: array
    description: Office locations
    items:
      type: string
  - name: contact
    type: object
    description: Contact information
    properties:
      - name: email
        type: string
      - name: phone
        type: string
      - name: address
        type: string
` + "```" + `

### E-commerce Product
` + "```yaml" + `
name: Product
description: Product listing with full details
fields:
  - name: title
    type: string
    description: Product name
    required: true
  - name: price
    type: number
    description: Current price
  - name: original_price
    type: number
    description: Original price before discount
  - name: currency
    type: string
    description: Currency code (USD, EUR, etc.)
  - name: description
    type: string
    description: Full product description
  - name: brand
    type: string
    description: Brand or manufacturer
  - name: category
    type: string
    description: Product category
  - name: specifications
    type: array
    description: Product specifications
    items:
      type: object
      properties:
        - name: name
          type: string
        - name: value
          type: string
  - name: rating
    type: number
    description: Average rating out of 5
  - name: review_count
    type: number
    description: Number of reviews
  - name: images
    type: array
    items:
      type: string
    description: Product image URLs
  - name: availability
    type: string
    description: Stock status
  - name: url
    type: string
    description: Product detail page URL
` + "```" + `

### Real Estate Listing (Detailed Example)
` + "```yaml" + `
name: PropertyListing
description: |
  A real estate property listing page.

  CRITICAL OUTPUT REQUIREMENTS:
  1. All prices must be extracted as integers (no decimals, no currency symbols)
  2. If prices shown in multiple currencies, extract all with source tracking
  3. Areas should be in square meters (convert from sq ft if needed: divide by 10.764)
  4. Dates in ISO format YYYY-MM-DD
  5. For missing optional fields, return null (not empty string)

fields:
  - name: title
    type: string
    description: "Property listing headline. Extract the main title text, not taglines or subtitles."
    required: true
  - name: prices
    type: array
    description: "All prices shown for this property. Include each currency found on page."
    required: true
    items:
      type: object
      properties:
        currency:
          type: string
          description: "ISO currency code: USD, EUR, GBP, etc."
        value:
          type: integer
          description: "Price as integer. Remove decimals, commas, currency symbols."
        period:
          type: string
          description: "For rentals: 'month', 'week', 'year'. Null for sales."
        source:
          type: string
          description: "'page' if shown on page, 'converted' if you calculated it"
  - name: location
    type: object
    description: "Property location details"
    required: true
    properties:
      full_address:
        type: string
        description: "Complete address as shown, or null if only partial info available"
      city:
        type: string
        description: "City or town name"
      state_province:
        type: string
        description: "State, province, or region"
      postal_code:
        type: string
        description: "ZIP or postal code if shown"
      country:
        type: string
        description: "Country name or code"
  - name: bedrooms
    type: integer
    description: "Number of bedrooms as integer. Studio = 0. Null if not specified."
  - name: bathrooms
    type: number
    description: "Number of bathrooms. Can be decimal (e.g., 2.5 for 2 full + 1 half bath)."
  - name: area_sqm
    type: number
    description: "Living area in square meters. If shown in sq ft, convert by dividing by 10.764."
  - name: property_type
    type: string
    description: "Standardize to: House, Condo, Apartment, Townhouse, Villa, Land, Commercial, or Other"
  - name: listing_type
    type: string
    description: "'Sale' or 'Rent' - determine from context and price display"
  - name: features
    type: array
    description: "Property features and amenities. Use lowercase standard terms: pool, garage, garden, balcony, etc."
    items:
      type: string
  - name: description
    type: string
    description: "Main property description text. Limit to 2000 characters. Preserve key details."
  - name: images
    type: array
    description: "Property image URLs. First 10 only, in order of appearance."
    items:
      type: string
  - name: agent
    type: object
    description: "Listing agent or seller contact info"
    properties:
      name:
        type: string
        description: "Agent or agency name"
      phone:
        type: string
        description: "Phone number as shown (preserve formatting)"
      email:
        type: string
        description: "Email if displayed (may be hidden or require click)"
      type:
        type: string
        description: "'Agent', 'Agency', 'Owner', or 'Developer'"
  - name: listing_date
    type: string
    description: "Date listed in ISO format YYYY-MM-DD. Convert from relative ('3 days ago') if possible."
  - name: url
    type: string
    description: "Full URL of the listing page"
` + "```" + `

### Job Listing
` + "```yaml" + `
name: Job
description: Job posting details
fields:
  - name: title
    type: string
    description: Job title
    required: true
  - name: company
    type: string
    description: Hiring company name
  - name: location
    type: string
    description: Job location (city, remote, etc.)
  - name: job_type
    type: string
    description: Full-time, part-time, contract, etc.
  - name: salary
    type: object
    properties:
      - name: min
        type: number
      - name: max
        type: number
      - name: period
        type: string
        description: yearly, hourly, etc.
  - name: description
    type: string
    description: Full job description
  - name: requirements
    type: array
    items:
      type: string
    description: Job requirements and qualifications
  - name: benefits
    type: array
    items:
      type: string
  - name: posted_date
    type: string
  - name: apply_url
    type: string
` + "```" + `

### News Article
` + "```yaml" + `
name: Article
description: News or blog article
fields:
  - name: title
    type: string
    description: Article headline
    required: true
  - name: author
    type: string
  - name: published_date
    type: string
  - name: category
    type: string
  - name: tags
    type: array
    items:
      type: string
  - name: summary
    type: string
    description: Article summary or excerpt
  - name: content
    type: string
    description: Full article text
  - name: image_url
    type: string
    description: Featured image
  - name: source
    type: string
    description: Publication name
` + "```" + `

## Your Task

Analyze the HTML and:

1. **Identify the Site Type**: What kind of organization/business is this? (consultancy, e-commerce, news, job board, real estate, SaaS, agency, portfolio, etc.)

2. **site_summary**: Write a 1-2 sentence description of the site and what data would be valuable

3. **page_type**: Choose the best fit: listing, detail, article, product, recipe, company, service, team, contact, portfolio, unknown

4. **detected_elements**: List the data elements you found, with:
   - name: Field name (snake_case)
   - type: string, number, boolean, array, object, url, date
   - count: How many instances (if applicable)
   - description: What this represents

5. **suggested_schema**: Generate a YAML schema that:
   - Has a multi-line description with CRITICAL OUTPUT REQUIREMENTS
   - Is appropriate for the site type
   - Uses nested objects where it makes data cleaner
   - Will produce COMBINABLE results when crawling multiple pages
   - Has DETAILED field descriptions that serve as extraction instructions
   - Marks required fields appropriately

6. **follow_patterns**: Provide CSS selectors and/or URL patterns to find related pages:
   - pattern: CSS selector like "a[href*='/services']" or URL pattern like "/product/.*"
   - description: What pages this targets
   - sample_urls: 1-3 example URLs

## Quality Checklist (verify before outputting)

Before finalizing your schema, verify:
- [ ] Schema description includes numbered CRITICAL OUTPUT REQUIREMENTS
- [ ] Every field description explains WHAT to extract, not just what the field is
- [ ] Field descriptions specify output FORMAT (date format, number format, etc.)
- [ ] Field descriptions handle edge cases (what if missing? what if ambiguous?)
- [ ] Arrays specify item format clearly
- [ ] Nested objects have complete property descriptions
- [ ] Required fields are marked appropriately (only truly essential fields)

## Output Format

Return ONLY valid JSON (no markdown code blocks):
{
  "site_summary": "...",
  "page_type": "...",
  "detected_elements": [
    {"name": "...", "type": "...", "count": 0, "description": "..."}
  ],
  "suggested_schema": "name: ...\ndescription: ...\nfields:\n  ...",
  "follow_patterns": [
    {"pattern": "...", "description": "...", "sample_urls": ["..."]}
  ]
}
`)

	return sb.String()
}

// callLLM makes a direct call to the LLM API and returns the response with token usage.
func (s *AnalyzerService) callLLM(ctx context.Context, config *LLMConfigInput, prompt string) (*llmCallResult, error) {
	// Validate config
	if config.APIKey == "" && config.Provider != "ollama" {
		return nil, fmt.Errorf("no API key available for provider %s", config.Provider)
	}

	// Build request body
	reqBody := map[string]any{
		"model": config.Model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"temperature": 0.2,
		"max_tokens":  4096,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Determine API endpoint
	var apiURL string
	switch config.Provider {
	case "openrouter":
		apiURL = "https://openrouter.ai/api/v1/chat/completions"
	case "anthropic":
		apiURL = "https://api.anthropic.com/v1/messages"
	case "openai":
		apiURL = "https://api.openai.com/v1/chat/completions"
	case "ollama":
		baseURL := config.BaseURL
		if baseURL == "" {
			baseURL = "http://localhost:11434"
		}
		apiURL = baseURL + "/api/chat"
	default:
		apiURL = "https://openrouter.ai/api/v1/chat/completions"
	}

	s.logger.Debug("making LLM API request",
		"provider", config.Provider,
		"model", config.Model,
		"api_url", apiURL,
		"prompt_length", len(prompt),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Set auth header based on provider
	switch config.Provider {
	case "openrouter":
		req.Header.Set("Authorization", "Bearer "+config.APIKey)
		req.Header.Set("HTTP-Referer", "https://refyne.io")
		req.Header.Set("X-Title", "Refyne Analyzer")
	case "anthropic":
		req.Header.Set("x-api-key", config.APIKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	default:
		req.Header.Set("Authorization", "Bearer "+config.APIKey)
	}

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		s.logger.Error("LLM API request failed", "provider", config.Provider, "error", err)
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	s.logger.Debug("LLM API response received",
		"provider", config.Provider,
		"status_code", resp.StatusCode,
		"response_length", len(body),
	)

	if resp.StatusCode != http.StatusOK {
		s.logger.Error("LLM API error",
			"provider", config.Provider,
			"status_code", resp.StatusCode,
			"response", string(body),
		)
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	// Parse response based on provider
	return s.extractLLMResponse(config.Provider, body)
}

// extractLLMResponse extracts the text response and token usage from different LLM provider formats.
func (s *AnalyzerService) extractLLMResponse(provider string, body []byte) (*llmCallResult, error) {
	result := &llmCallResult{}

	switch provider {
	case "anthropic":
		var resp struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			Usage struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}
		if len(resp.Content) == 0 {
			return nil, fmt.Errorf("empty response from LLM")
		}
		result.Content = resp.Content[0].Text
		result.InputTokens = resp.Usage.InputTokens
		result.OutputTokens = resp.Usage.OutputTokens

	default: // OpenAI/OpenRouter format
		var resp struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
			Usage struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}
		if len(resp.Choices) == 0 {
			return nil, fmt.Errorf("empty response from LLM")
		}
		result.Content = resp.Choices[0].Message.Content
		result.InputTokens = resp.Usage.PromptTokens
		result.OutputTokens = resp.Usage.CompletionTokens
	}

	return result, nil
}

// parseAnalysisResponse parses the LLM response into an AnalyzeOutput.
func (s *AnalyzerService) parseAnalysisResponse(response string) (*AnalyzeOutput, error) {
	// Clean up response - sometimes LLMs wrap JSON in markdown code blocks
	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	// Parse JSON response
	var parsed struct {
		SiteSummary      string `json:"site_summary"`
		PageType         string `json:"page_type"`
		DetectedElements []struct {
			Name        string          `json:"name"`
			Type        string          `json:"type"`
			Count       models.FlexInt  `json:"count"`
			Description string          `json:"description"`
		} `json:"detected_elements"`
		SuggestedSchema string `json:"suggested_schema"`
		FollowPatterns  []struct {
			Pattern     string   `json:"pattern"`
			Description string   `json:"description"`
			SampleURLs  []string `json:"sample_urls"`
		} `json:"follow_patterns"`
	}

	if err := json.Unmarshal([]byte(response), &parsed); err != nil {
		// Try to extract JSON from the response if it contains other text
		jsonStart := strings.Index(response, "{")
		jsonEnd := strings.LastIndex(response, "}")
		if jsonStart >= 0 && jsonEnd > jsonStart {
			response = response[jsonStart : jsonEnd+1]
			if err := json.Unmarshal([]byte(response), &parsed); err != nil {
				return nil, fmt.Errorf("failed to parse LLM response as JSON: %w", err)
			}
		} else {
			return nil, fmt.Errorf("failed to parse LLM response as JSON: %w", err)
		}
	}

	// Convert to output format
	output := &AnalyzeOutput{
		SiteSummary:     parsed.SiteSummary,
		PageType:        s.parsePageType(parsed.PageType),
		SuggestedSchema: parsed.SuggestedSchema,
	}

	for _, elem := range parsed.DetectedElements {
		output.DetectedElements = append(output.DetectedElements, models.DetectedElement{
			Name:        elem.Name,
			Type:        elem.Type,
			Count:       elem.Count,
			Description: elem.Description,
		})
	}

	for _, pattern := range parsed.FollowPatterns {
		output.FollowPatterns = append(output.FollowPatterns, models.FollowPattern{
			Pattern:     pattern.Pattern,
			Description: pattern.Description,
			SampleURLs:  pattern.SampleURLs,
		})
	}

	return output, nil
}

// parsePageType converts a string to PageType.
func (s *AnalyzerService) parsePageType(pt string) models.PageType {
	switch strings.ToLower(pt) {
	case "listing":
		return models.PageTypeListing
	case "detail":
		return models.PageTypeDetail
	case "article":
		return models.PageTypeArticle
	case "product":
		return models.PageTypeProduct
	case "recipe":
		return models.PageTypeRecipe
	case "company":
		return models.PageTypeCompany
	case "service":
		return models.PageTypeService
	case "team":
		return models.PageTypeTeam
	case "contact":
		return models.PageTypeContact
	case "portfolio":
		return models.PageTypePortfolio
	default:
		return models.PageTypeUnknown
	}
}

// resolveLLMConfig determines which LLM configuration to use for analysis.
// Returns (config, isBYOK) where isBYOK is true if using user's own key.
func (s *AnalyzerService) resolveLLMConfig(ctx context.Context, userID string, tier string) (*LLMConfigInput, bool) {
	// 1. Check user's fallback chain first (new system)
	config := s.buildUserFallbackChainConfig(ctx, userID)
	if config != nil {
		s.logger.Info("using user fallback chain for analysis (BYOK)",
			"user_id", userID,
			"provider", config.Provider,
			"model", config.Model,
		)
		return config, true
	}

	// 2. Check user's legacy saved config (single provider)
	savedCfg, err := s.repos.LLMConfig.GetByUserID(ctx, userID)
	if err != nil {
		s.logger.Warn("failed to get user LLM config", "user_id", userID, "error", err)
	}

	if savedCfg != nil && savedCfg.Provider != "" {
		// Decrypt API key if present
		var apiKey string
		if savedCfg.APIKeyEncrypted != "" && s.encryptor != nil {
			decrypted, err := s.encryptor.Decrypt(savedCfg.APIKeyEncrypted)
			if err != nil {
				s.logger.Warn("failed to decrypt API key", "user_id", userID, "error", err)
			} else {
				apiKey = decrypted
			}
		}

		s.logger.Info("using legacy user config for analysis (BYOK)",
			"user_id", userID,
			"provider", savedCfg.Provider,
		)
		return &LLMConfigInput{
			Provider: savedCfg.Provider,
			APIKey:   apiKey,
			BaseURL:  savedCfg.BaseURL,
			Model:    savedCfg.Model,
		}, true
	}

	// 3. Use system fallback chain
	s.logger.Info("using system fallback chain for analysis",
		"user_id", userID,
		"tier", tier,
	)
	return s.getDefaultAnalysisConfig(tier), false
}

// buildUserFallbackChainConfig builds the first valid LLM config from user's fallback chain.
// Returns nil if user has no chain configured or no valid keys.
func (s *AnalyzerService) buildUserFallbackChainConfig(ctx context.Context, userID string) *LLMConfigInput {
	if s.repos.UserFallbackChain == nil || s.repos.UserServiceKey == nil {
		return nil
	}

	// Get user's enabled fallback chain entries
	chain, err := s.repos.UserFallbackChain.GetEnabledByUserID(ctx, userID)
	if err != nil {
		s.logger.Warn("failed to get user fallback chain", "user_id", userID, "error", err)
		return nil
	}
	if len(chain) == 0 {
		return nil
	}

	// Get user's service keys
	keys, err := s.repos.UserServiceKey.GetEnabledByUserID(ctx, userID)
	if err != nil {
		s.logger.Warn("failed to get user service keys", "user_id", userID, "error", err)
		return nil
	}

	// Build a map of provider -> key
	keyMap := make(map[string]*models.UserServiceKey)
	for _, k := range keys {
		keyMap[k.Provider] = k
	}

	// Return the first valid config from the chain
	for _, entry := range chain {
		key, ok := keyMap[entry.Provider]
		if !ok {
			continue // No key for this provider
		}

		// Ollama doesn't require an API key
		var apiKey string
		if entry.Provider != "ollama" {
			if key.APIKeyEncrypted == "" {
				continue // No API key configured
			}
			// Decrypt the API key
			if s.encryptor != nil {
				decrypted, err := s.encryptor.Decrypt(key.APIKeyEncrypted)
				if err != nil {
					s.logger.Warn("failed to decrypt user API key",
						"user_id", userID,
						"provider", entry.Provider,
						"error", err,
					)
					continue
				}
				apiKey = decrypted
			} else {
				apiKey = key.APIKeyEncrypted
			}
		}

		return &LLMConfigInput{
			Provider: entry.Provider,
			APIKey:   apiKey,
			BaseURL:  key.BaseURL,
			Model:    entry.Model,
		}
	}

	return nil
}

// getDefaultAnalysisConfig returns the default LLM config for analysis.
func (s *AnalyzerService) getDefaultAnalysisConfig(tier string) *LLMConfigInput {
	// First try to get from fallback chain
	if s.repos != nil && s.repos.FallbackChain != nil {
		var chain []*models.FallbackChainEntry
		var err error

		if tier != "" {
			chain, err = s.repos.FallbackChain.GetEnabledByTier(context.Background(), tier)
		} else {
			chain, err = s.repos.FallbackChain.GetEnabled(context.Background())
		}

		if err == nil && len(chain) > 0 {
			// Get service keys
			serviceKeys := s.getServiceKeys()

			for _, entry := range chain {
				config := &LLMConfigInput{
					Provider: entry.Provider,
					Model:    entry.Model,
				}

				switch entry.Provider {
				case "openrouter":
					config.APIKey = serviceKeys.OpenRouterKey
				case "anthropic":
					config.APIKey = serviceKeys.AnthropicKey
				case "openai":
					config.APIKey = serviceKeys.OpenAIKey
				}

				if config.APIKey != "" || entry.Provider == "ollama" {
					return config
				}
			}
		}
	}

	// Fallback to hardcoded defaults
	serviceKeys := s.getServiceKeys()

	if serviceKeys.OpenRouterKey != "" {
		return &LLMConfigInput{
			Provider: "openrouter",
			APIKey:   serviceKeys.OpenRouterKey,
			Model:    "google/gemini-2.0-flash-001",
		}
	}

	return &LLMConfigInput{
		Provider: "ollama",
		Model:    "llama3.2",
	}
}

// ServiceKeys holds the service API keys.
type analyzerServiceKeys struct {
	OpenRouterKey string
	AnthropicKey  string
	OpenAIKey     string
}

// getServiceKeys retrieves service keys, preferring DB over env vars.
func (s *AnalyzerService) getServiceKeys() analyzerServiceKeys {
	keys := analyzerServiceKeys{}

	// Try to load from database first
	if s.repos != nil && s.repos.ServiceKey != nil {
		dbKeys, err := s.repos.ServiceKey.GetAll(context.Background())
		if err == nil {
			for _, k := range dbKeys {
				apiKey := k.APIKeyEncrypted
				if s.encryptor != nil && k.APIKeyEncrypted != "" {
					if decrypted, err := s.encryptor.Decrypt(k.APIKeyEncrypted); err == nil {
						apiKey = decrypted
					}
				}

				switch k.Provider {
				case "openrouter":
					keys.OpenRouterKey = apiKey
				case "anthropic":
					keys.AnthropicKey = apiKey
				case "openai":
					keys.OpenAIKey = apiKey
				}
			}
		}
	}

	// Fall back to env vars
	if keys.OpenRouterKey == "" {
		keys.OpenRouterKey = s.cfg.ServiceOpenRouterKey
	}
	if keys.AnthropicKey == "" {
		keys.AnthropicKey = s.cfg.ServiceAnthropicKey
	}
	if keys.OpenAIKey == "" {
		keys.OpenAIKey = s.cfg.ServiceOpenAIKey
	}

	return keys
}

// ExtractDomain extracts the domain from a URL.
func ExtractDomain(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return parsed.Host
}

// min returns the smaller of two integers.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// isContextLengthError checks if an error is related to exceeding LLM context window limits.
func isContextLengthError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "context_length") ||
		strings.Contains(errStr, "context length") ||
		strings.Contains(errStr, "max_tokens") ||
		strings.Contains(errStr, "token limit") ||
		strings.Contains(errStr, "too long") ||
		strings.Contains(errStr, "maximum context") ||
		strings.Contains(errStr, "exceeds") && strings.Contains(errStr, "limit") ||
		strings.Contains(errStr, "input too large") ||
		strings.Contains(errStr, "content_too_large") ||
		strings.Contains(errStr, "request too large")
}

// CleanHTML removes scripts, styles, and excessive whitespace from HTML.
func CleanHTML(html string) string {
	// Remove script tags and content
	scriptRe := regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	html = scriptRe.ReplaceAllString(html, "")

	// Remove style tags and content
	styleRe := regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	html = styleRe.ReplaceAllString(html, "")

	// Remove comments
	commentRe := regexp.MustCompile(`<!--[\s\S]*?-->`)
	html = commentRe.ReplaceAllString(html, "")

	// Normalize whitespace
	whitespaceRe := regexp.MustCompile(`\s+`)
	html = whitespaceRe.ReplaceAllString(html, " ")

	return strings.TrimSpace(html)
}

// normalizeURL ensures a URL has a scheme (defaults to https://).
func normalizeURL(rawURL string) string {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return trimmed
	}
	// Check if URL already has a scheme
	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		return trimmed
	}
	// Add https:// by default
	return "https://" + trimmed
}
