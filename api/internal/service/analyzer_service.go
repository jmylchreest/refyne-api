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
	"github.com/oklog/ulid/v2"
	"github.com/refyne/refyne/pkg/cleaner"

	"github.com/jmylchreest/refyne-api/internal/config"
	"github.com/jmylchreest/refyne-api/internal/crypto"
	"github.com/jmylchreest/refyne-api/internal/models"
	"github.com/jmylchreest/refyne-api/internal/preprocessor"
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
	fallbackCleaner cleaner.Cleaner                // Used when content is too large for context window
	preprocessor    preprocessor.LLMPreProcessor   // Preprocessor for generating LLM hints
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

	// Create preprocessor chain for generating LLM hints
	// HintRepeats detects repeated HTML patterns to suggest array-based schemas
	llmPreprocessor := preprocessor.NewChain(
		preprocessor.NewHintRepeats(),
	)

	return &AnalyzerService{
		cfg:             cfg,
		repos:           repos,
		billing:         billing,
		logger:          logger,
		encryptor:       encryptor,
		cleaner:         cleaner.NewNoop(), // Use noop cleaner - raw HTML for analysis
		fallbackCleaner: fallbackCleaner,
		preprocessor:    llmPreprocessor,
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
	requestID := ulid.Make().String()

	// Normalize URL - add https:// if no scheme present
	targetURL := normalizeURL(input.URL)

	s.logger.Info("starting URL analysis",
		"request_id", requestID,
		"user_id", userID,
		"url", targetURL,
		"depth", input.Depth,
		"tier", tier,
		"cleaner", "noop (fallback: trafilatura->html)",
	)

	// Get LLM config for analysis early (needed for error recording)
	s.logger.Debug("resolving LLM config", "request_id", requestID, "user_id", userID, "tier", tier)
	llmConfig, isBYOK := s.resolveLLMConfig(ctx, userID, tier, requestID)
	s.logger.Debug("LLM config resolved",
		"request_id", requestID,
		"user_id", userID,
		"provider", llmConfig.Provider,
		"model", llmConfig.Model,
		"has_api_key", llmConfig.APIKey != "",
		"is_byok", isBYOK,
	)

	// Pre-flight cost estimate (triggers pricing cache refresh if needed)
	if s.billing != nil {
		estimatedCost := s.billing.EstimateCost(1, llmConfig.Model, llmConfig.Provider)
		s.logger.Debug("pre-flight cost estimate",
			"request_id", requestID,
			"user_id", userID,
			"provider", llmConfig.Provider,
			"model", llmConfig.Model,
			"estimated_cost_usd", estimatedCost,
			"is_byok", isBYOK,
		)
	}

	// Fetch main page content
	fetchStart := time.Now()
	s.logger.Debug("fetching main page content", "request_id", requestID, "user_id", userID, "url", targetURL)
	mainContent, links, fetchMode, err := s.fetchContent(ctx, targetURL, input.FetchMode)
	if err != nil {
		s.logger.Error("failed to fetch page content", "request_id", requestID, "user_id", userID, "url", targetURL, "error", err)
		s.recordAnalyzeUsage(ctx, userID, tier, targetURL, llmConfig, isBYOK, 0, 0,
			int(time.Since(fetchStart).Milliseconds()), 0, int(time.Since(startTime).Milliseconds()),
			"failed", err.Error(), requestID)
		return nil, fmt.Errorf("failed to fetch page content: %w", err)
	}
	s.logger.Debug("page content fetched",
		"request_id", requestID,
		"user_id", userID,
		"content_length", len(mainContent),
		"links_found", len(links),
		"fetch_mode", fetchMode,
	)

	// Auto mini-crawl: Fetch sample detail/product pages to understand site structure
	// This helps generate better combined schemas that work across listing and detail pages
	var detailContents []string
	var detailURLs []string

	// Identify promising detail page links
	detailLinks := s.identifyDetailLinks(targetURL, links)

	if len(detailLinks) > 0 {
		s.logger.Debug("auto mini-crawl: identified detail page candidates",
			"request_id", requestID,
			"user_id", userID,
			"detail_links", detailLinks,
		)

		for _, detailURL := range detailLinks {
			content, _, _, err := s.fetchContent(ctx, detailURL, string(fetchMode))
			if err != nil {
				s.logger.Warn("failed to fetch detail page for mini-crawl",
					"request_id", requestID,
					"user_id", userID,
					"url", detailURL,
					"error", err,
				)
				continue
			}
			detailContents = append(detailContents, content)
			detailURLs = append(detailURLs, detailURL)
		}

		s.logger.Info("auto mini-crawl completed",
			"request_id", requestID,
			"user_id", userID,
			"pages_fetched", len(detailContents),
		)
	} else if input.Depth > 0 && len(links) > 0 {
		// Fallback: if no detail links identified but depth > 0, use first few links
		maxSamples := 2
		if len(links) < maxSamples {
			maxSamples = len(links)
		}
		for i := 0; i < maxSamples; i++ {
			content, _, _, err := s.fetchContent(ctx, links[i], string(fetchMode))
			if err != nil {
				s.logger.Warn("failed to fetch detail page",
					"request_id", requestID,
					"user_id", userID,
					"url", links[i],
					"error", err,
				)
				continue
			}
			detailContents = append(detailContents, content)
			detailURLs = append(detailURLs, links[i])
		}
	}
	fetchDuration := time.Since(fetchStart)

	// Generate analysis prompt and call LLM
	// First try with raw content (noop cleaner), retry with fallback cleaner on context length errors
	llmStart := time.Now()
	s.logger.Debug("calling LLM for analysis", "request_id", requestID, "user_id", userID, "provider", llmConfig.Provider, "model", llmConfig.Model)
	result, err := s.analyzeWithLLM(ctx, mainContent, detailContents, detailURLs, links, llmConfig)
	llmDuration := time.Since(llmStart)

	// If context length error, retry with cleaned content
	if err != nil && isContextLengthError(err) {
		s.logger.Info("context length error detected, retrying with cleaned content",
			"request_id", requestID,
			"user_id", userID,
			"original_content_length", len(mainContent),
			"error", err.Error(),
		)

		// Clean main content with fallback cleaner (Trafilatura HTML with tables/links)
		cleanedMain, cleanErr := s.fallbackCleaner.Clean(mainContent)
		if cleanErr != nil {
			s.logger.Warn("fallback cleaner failed, using original content", "request_id", requestID, "user_id", userID, "error", cleanErr)
			cleanedMain = mainContent
		} else {
			s.logger.Debug("content cleaned for retry",
				"request_id", requestID,
				"user_id", userID,
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
		result, err = s.analyzeWithLLM(ctx, cleanedMain, cleanedDetails, detailURLs, links, llmConfig)
		llmDuration = time.Since(llmStart) + time.Since(llmRetryStart) // Total LLM time including retry

		if err != nil {
			s.logger.Error("LLM analysis failed after retry with cleaned content",
				"request_id", requestID,
				"user_id", userID,
				"provider", llmConfig.Provider,
				"model", llmConfig.Model,
				"error", err,
			)
			s.recordAnalyzeUsage(ctx, userID, tier, targetURL, llmConfig, isBYOK, 0, 0,
				int(fetchDuration.Milliseconds()), int(llmDuration.Milliseconds()), int(time.Since(startTime).Milliseconds()),
				"failed", "retry failed: "+err.Error(), requestID)
			return nil, fmt.Errorf("LLM analysis failed after retry: %w", err)
		}
		s.logger.Info("analysis succeeded after retry with cleaned content", "request_id", requestID, "user_id", userID)
	} else if err != nil {
		s.logger.Error("LLM analysis failed",
			"request_id", requestID,
			"user_id", userID,
			"provider", llmConfig.Provider,
			"model", llmConfig.Model,
			"error", err,
		)
		s.recordAnalyzeUsage(ctx, userID, tier, targetURL, llmConfig, isBYOK, 0, 0,
			int(fetchDuration.Milliseconds()), int(llmDuration.Milliseconds()), int(time.Since(startTime).Milliseconds()),
			"failed", err.Error(), requestID)
		return nil, fmt.Errorf("LLM analysis failed: %w", err)
	}

	// Record successful usage
	s.recordAnalyzeUsage(ctx, userID, tier, targetURL, llmConfig, isBYOK,
		result.InputTokens, result.OutputTokens,
		int(fetchDuration.Milliseconds()), int(llmDuration.Milliseconds()), int(time.Since(startTime).Milliseconds()),
		"success", "", requestID)

	result.Output.RecommendedFetchMode = fetchMode
	if len(links) > 0 {
		result.Output.SampleLinks = links[:min(10, len(links))] // Return up to 10 sample links
	}

	// Add token usage to output
	result.Output.TokenUsage = AnalyzeTokenUsage{
		InputTokens:  result.InputTokens,
		OutputTokens: result.OutputTokens,
	}

	// Calculate costs for logging
	var llmCostUSD, userCostUSD float64
	if s.billing != nil {
		llmCostUSD = s.billing.GetActualCost(ctx, result.InputTokens, result.OutputTokens, llmConfig.Model, llmConfig.Provider)
		userCostUSD, _, _ = s.billing.CalculateTotalCost(llmCostUSD, tier)
	}

	s.logger.Info("URL analysis completed",
		"request_id", requestID,
		"user_id", userID,
		"url", targetURL,
		"provider", llmConfig.Provider,
		"model", llmConfig.Model,
		"page_type", result.Output.PageType,
		"input_tokens", result.InputTokens,
		"output_tokens", result.OutputTokens,
		"llm_cost_usd", llmCostUSD,
		"user_cost_usd", userCostUSD,
		"is_byok", isBYOK,
		"duration_ms", time.Since(startTime).Milliseconds(),
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
	requestID string,
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
		JobID:             "", // Analyze operations don't have job records
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
		RequestID:         requestID,
	}

	if status == "failed" {
		usageRecord.PagesSuccessful = 0
	}

	// Use detached context - we want to record usage even if request timed out
	if err := s.billing.RecordUsage(context.WithoutCancel(ctx), usageRecord); err != nil {
		s.logger.Warn("failed to record analyze usage", "request_id", requestID, "user_id", userID, "error", err)
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

// identifyDetailLinks finds links that are likely product/item detail pages.
// These are used for auto mini-crawl to understand detail page structure.
func (s *AnalyzerService) identifyDetailLinks(baseURL string, links []string) []string {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil
	}
	basePath := parsed.Path

	// Patterns that suggest a detail/product page
	detailPatterns := []string{
		"/product/", "/products/", "/item/", "/items/",
		"/p/", "/pd/", "/dp/", // Common short product paths
		"/listing/", "/listings/",
		"/article/", "/articles/", "/post/", "/posts/",
		"/property/", "/properties/",
		"/job/", "/jobs/", "/career/", "/careers/",
		"/service/", "/services/",
		"/collection/", "/collections/", "/category/", "/categories/",
	}

	// Score links by how likely they are detail pages
	type scoredLink struct {
		url   string
		score int
	}
	var scored []scoredLink

	for _, link := range links {
		linkParsed, err := url.Parse(link)
		if err != nil {
			continue
		}

		// Skip if it's the same as base URL
		if linkParsed.Path == basePath {
			continue
		}

		path := strings.ToLower(linkParsed.Path)
		score := 0

		// Check for detail patterns
		for _, pattern := range detailPatterns {
			if strings.Contains(path, pattern) {
				score += 10
				break
			}
		}

		// Paths with slugs (contain hyphens) are often detail pages
		segments := strings.Split(strings.Trim(path, "/"), "/")
		if len(segments) >= 2 {
			lastSegment := segments[len(segments)-1]
			if strings.Contains(lastSegment, "-") && len(lastSegment) > 10 {
				score += 5 // Slug-like path (e.g., /products/raspberry-pi-5-8gb)
			}
		}

		// Deeper paths are more likely to be detail pages
		if len(segments) >= 2 {
			score += 2
		}

		// Paths with IDs (numbers) are often detail pages
		if regexp.MustCompile(`/\d+`).MatchString(path) {
			score += 3
		}

		if score > 0 {
			scored = append(scored, scoredLink{url: link, score: score})
		}
	}

	// Sort by score (highest first) and return top candidates
	for i := 0; i < len(scored)-1; i++ {
		for j := i + 1; j < len(scored); j++ {
			if scored[j].score > scored[i].score {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}

	// Return top 2 unique patterns (avoid duplicates like /products/a and /products/b)
	var result []string
	seenPatterns := make(map[string]bool)

	for _, sl := range scored {
		if len(result) >= 2 {
			break
		}

		// Extract pattern from URL (e.g., "/products/" from "/products/item-name")
		linkParsed, _ := url.Parse(sl.url)
		segments := strings.Split(strings.Trim(linkParsed.Path, "/"), "/")
		if len(segments) >= 2 {
			pattern := "/" + segments[0] + "/"
			if seenPatterns[pattern] {
				continue // Already have a link from this pattern
			}
			seenPatterns[pattern] = true
		}

		result = append(result, sl.url)
	}

	return result
}

// analyzeResult holds the result of LLM analysis including token usage.
type analyzeResult struct {
	Output       *AnalyzeOutput
	InputTokens  int
	OutputTokens int
}

// analyzeWithLLM calls the LLM to analyze page content and generate a schema.
func (s *AnalyzerService) analyzeWithLLM(ctx context.Context, mainContent string, detailContents []string, detailURLs []string, links []string, llmConfig *LLMConfigInput) (*analyzeResult, error) {
	// Run preprocessor to generate hints before truncation (needs full content)
	hints, err := s.preprocessor.Process(mainContent)
	if err != nil {
		s.logger.Warn("preprocessor failed", "error", err)
		hints = preprocessor.NewHints() // Continue with empty hints
	}

	// Log what the preprocessor detected
	if len(hints.DetectedTypes) > 0 {
		typeNames := make([]string, len(hints.DetectedTypes))
		for i, dt := range hints.DetectedTypes {
			typeNames[i] = fmt.Sprintf("%s(%d)", dt.Name, dt.Count)
		}
		s.logger.Info("preprocessor detected content types",
			"types", typeNames,
			"preprocessor", s.preprocessor.Name(),
		)
	} else {
		s.logger.Debug("preprocessor detected no repeated patterns",
			"preprocessor", s.preprocessor.Name(),
			"content_length", len(mainContent),
		)
	}

	// Truncate content to avoid token limits
	mainContent = s.truncateContent(mainContent, 15000)
	for i := range detailContents {
		detailContents[i] = s.truncateContent(detailContents[i], 5000)
	}

	// Build the prompt with preprocessing hints
	prompt := s.buildAnalysisPrompt(mainContent, detailContents, detailURLs, links, hints)

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
func (s *AnalyzerService) buildAnalysisPrompt(mainContent string, detailContents []string, detailURLs []string, links []string, hints *preprocessor.Hints) string {
	var sb strings.Builder

	sb.WriteString(`You are analyzing a website to generate an extraction schema.

## Page Content

### Main Page (listing/collection/homepage)
`)
	sb.WriteString("```html\n")
	sb.WriteString(mainContent)
	sb.WriteString("\n```\n")

	if len(detailContents) > 0 {
		sb.WriteString("\n### Sample Detail Pages\n")
		sb.WriteString("I've also fetched sample detail/product pages to help you understand the full data structure:\n\n")
		for i, content := range detailContents {
			url := ""
			if i < len(detailURLs) {
				url = detailURLs[i]
			}
			sb.WriteString(fmt.Sprintf("**Detail Page %d** (%s)\n```html\n%s\n```\n\n", i+1, url, content))
		}
	}

	if len(links) > 0 {
		sb.WriteString("\n### Links Found\n")
		maxLinks := min(15, len(links))
		for i := 0; i < maxLinks; i++ {
			sb.WriteString(fmt.Sprintf("- %s\n", links[i]))
		}
	}

	// Add preprocessor hints if available
	if hints != nil {
		hintsSection := hints.ToPromptSection()
		if hintsSection != "" {
			sb.WriteString(hintsSection)
		}
	}

	sb.WriteString(`

## Your Task

Generate a schema that works across listing AND detail pages. Fields should be optional for partial extraction.

### Schema Approach

Use a descriptive array name matching the content: products[], jobs[], articles[], episodes[], case_studies[], services[], etc.

**Required fields**: title, url (always include these as required)
**Common fields**: description, image_url, date, category, price, currency
**Add relevant fields** based on what you observe in the content - don't limit yourself to a fixed template.

**Mixed content sites** (e.g., /blog/ AND /solutions/ AND /podcasts/): Use items[] with a content_type field, plus a metadata object for type-specific fields the LLM can populate dynamically.

### Schema Rules

- Prices: integer in smallest unit (£15.99 = 1599) + currency field (ISO: GBP, USD)
- URLs: absolute paths only
- Only title + url are required; everything else optional

### Output Format (JSON only, no markdown)

{
  "site_summary": "Brief description",
  "page_type": "listing|detail|article|product|unknown",
  "detected_elements": [{"name": "...", "type": "string|number|array", "count": N}],
  "suggested_schema": "<YAML schema - see structure below>",
  "follow_patterns": [{"pattern": "a[href*='/path/']", "description": "..."}]
}

The suggested_schema MUST be a YAML string with this exact structure:

name: SchemaName
description: What this schema extracts
fields:
  - name: items
    type: array
    items:
      type: object
      properties:
        - name: title
          type: string
          required: true
        - name: url
          type: string
          required: true
        - name: description
          type: string
        - name: category
          type: string
        - name: metadata
          type: object
          description: Additional type-specific fields

Valid types: string, integer, number, boolean, array, object. Use "items" for arrays, "properties" for objects.

### Follow Patterns

Generate CSS selectors from the Links Found section. Use ONLY href-based selectors:
- GOOD: a[href*='/blog/'], a[href*='/products/'], a[href^='/jobs/']
- BAD: .blog-post a (invented class), main a[href*='/x/'] (assumes main exists)

Identify ALL distinct URL patterns. Large sites need 3-6 patterns for different sections.
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
func (s *AnalyzerService) resolveLLMConfig(ctx context.Context, userID, tier, requestID string) (*LLMConfigInput, bool) {
	// 1. Check user's fallback chain first (new system)
	config := s.buildUserFallbackChainConfig(ctx, userID)
	if config != nil {
		s.logger.Info("using user fallback chain for analysis (BYOK)",
			"request_id", requestID,
			"user_id", userID,
			"provider", config.Provider,
			"model", config.Model,
		)
		return config, true
	}

	// 2. Check user's legacy saved config (single provider)
	savedCfg, err := s.repos.LLMConfig.GetByUserID(ctx, userID)
	if err != nil {
		s.logger.Warn("failed to get user LLM config", "request_id", requestID, "user_id", userID, "error", err)
	}

	if savedCfg != nil && savedCfg.Provider != "" {
		// Decrypt API key if present
		var apiKey string
		if savedCfg.APIKeyEncrypted != "" && s.encryptor != nil {
			decrypted, err := s.encryptor.Decrypt(savedCfg.APIKeyEncrypted)
			if err != nil {
				s.logger.Warn("failed to decrypt API key", "request_id", requestID, "user_id", userID, "error", err)
			} else {
				apiKey = decrypted
			}
		}

		s.logger.Info("using legacy user config for analysis (BYOK)",
			"request_id", requestID,
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
		"request_id", requestID,
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
