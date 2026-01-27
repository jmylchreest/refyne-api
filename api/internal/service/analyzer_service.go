package service

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/gocolly/colly/v2"
	"github.com/oklog/ulid/v2"
	"github.com/jmylchreest/refyne/pkg/cleaner"

	"github.com/jmylchreest/refyne-api/internal/config"
	"github.com/jmylchreest/refyne-api/internal/crypto"
	"github.com/jmylchreest/refyne-api/internal/llm"
	"github.com/jmylchreest/refyne-api/internal/models"
	"github.com/jmylchreest/refyne-api/internal/preprocessor"
	"github.com/jmylchreest/refyne-api/internal/protection"
	"github.com/jmylchreest/refyne-api/internal/repository"
)

// AnalyzerService handles URL analysis and schema generation.
type AnalyzerService struct {
	cfg                 *config.Config
	repos               *repository.Repositories
	billing             *BillingService
	resolver            *LLMConfigResolver             // Shared LLM config resolver
	logger              *slog.Logger
	encryptor           *crypto.Encryptor
	cleaner             cleaner.Cleaner
	fallbackCleaner     cleaner.Cleaner                // Used when content is too large for context window
	preprocessor        *preprocessor.Chain            // Preprocessor chain for generating LLM hints
	captchaSvc          *CaptchaService                // For dynamic content fetching with browser rendering
	protectionDetector  *protection.Detector           // Detects bot protection signals in responses
}

// getStrictMode determines if a model supports strict JSON schema mode.
// Delegates to resolver which uses cached capabilities when available.
func (s *AnalyzerService) getStrictMode(ctx context.Context, provider, model string) bool {
	if s.resolver != nil {
		return s.resolver.GetStrictMode(ctx, provider, model, nil)
	}
	// Fall back to static defaults
	_, _, strictMode := llm.GetModelSettings(provider, model, nil, nil, nil)
	return strictMode
}

// NewAnalyzerService creates a new analyzer service (legacy constructor).
func NewAnalyzerService(cfg *config.Config, repos *repository.Repositories, logger *slog.Logger) *AnalyzerService {
	return NewAnalyzerServiceWithBilling(cfg, repos, nil, nil, logger)
}

// NewAnalyzerServiceWithBilling creates an analyzer service with billing and resolver integration.
func NewAnalyzerServiceWithBilling(cfg *config.Config, repos *repository.Repositories, billing *BillingService, resolver *LLMConfigResolver, logger *slog.Logger) *AnalyzerService {
	encryptor, err := crypto.NewEncryptor(cfg.EncryptionKey)
	if err != nil {
		logger.Error("failed to create encryptor for analyzer service", "error", err)
	}

	// Create cleaners using factory for consistency
	factory := NewCleanerFactory()

	// Primary cleaner chain: noop (raw HTML) for maximum analysis detail
	primaryCleaner, err := factory.CreateChain(DefaultAnalyzerCleanerChain)
	if err != nil {
		logger.Error("failed to create primary cleaner", "error", err)
		primaryCleaner = cleaner.NewNoop() // Fallback to direct creation
	}

	// Fallback cleaner chain: refyne (used when content exceeds context window)
	fallbackCleaner, err := factory.CreateChain(AnalyzerFallbackCleanerChain)
	if err != nil {
		logger.Error("failed to create fallback cleaner", "error", err)
		fallbackCleaner = cleaner.NewNoop()
	}

	// Create preprocessor chain for generating LLM hints
	// HintRepeats detects repeated HTML patterns to suggest array-based schemas
	// HintFeedback detects human feedback (reviews, comments) and adds sentiment analysis hints
	llmPreprocessor := preprocessor.NewChain(
		preprocessor.NewHintRepeats(),
		preprocessor.NewHintFeedback(),
	)

	return &AnalyzerService{
		cfg:                cfg,
		repos:              repos,
		billing:            billing,
		resolver:           resolver,
		logger:             logger,
		encryptor:          encryptor,
		cleaner:            primaryCleaner,  // noop by default - raw HTML for analysis
		fallbackCleaner:    fallbackCleaner, // refyne for context length fallback
		preprocessor:       llmPreprocessor,
		protectionDetector: protection.NewDetector(),
	}
}

// SetCaptchaService sets the captcha service for dynamic content fetching.
// When configured, users with the content_dynamic feature can use browser rendering.
func (s *AnalyzerService) SetCaptchaService(captchaSvc *CaptchaService) {
	s.captchaSvc = captchaSvc
}

// AnalyzeInput represents input for the analyze operation.
type AnalyzeInput struct {
	URL       string `json:"url"`
	Depth     int    `json:"depth"`      // 0 = single page, 1 = crawl one level
	FetchMode string `json:"fetch_mode"` // auto, static, dynamic
	JobID     string `json:"-"`          // Job ID for tracking (not serialized)
}

// AnalyzeTokenUsage represents token consumption and cost info for an analysis.
type AnalyzeTokenUsage struct {
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`      // Total cost charged to user
	LLMCostUSD   float64 `json:"llm_cost_usd"`  // Actual LLM cost from provider
	IsBYOK       bool    `json:"is_byok"`       // True if user's own API key was used
	LLMProvider  string  `json:"llm_provider"`  // Provider used
	LLMModel     string  `json:"llm_model"`     // Model used
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
	FetchModeUsed        models.FetchMode         `json:"fetch_mode_used,omitempty"` // Actual fetch mode used for this analysis (static or dynamic)
	SampleData           any                      `json:"sample_data,omitempty"`     // Preview extraction result
	TokenUsage           AnalyzeTokenUsage        `json:"token_usage"`
	// Debug capture data (only populated when debug capture is enabled)
	DebugCapture *AnalyzeDebugCapture `json:"-"` // Internal use only, not serialized in API response
}

// AnalyzeDebugCapture contains debug information for an analysis.
type AnalyzeDebugCapture struct {
	RawContent  string `json:"raw_content"`  // Raw HTML content fetched
	Prompt      string `json:"prompt"`       // Full prompt sent to LLM
	LLMResponse string `json:"llm_response"` // Raw LLM response
	FetchMode   string `json:"fetch_mode"`   // Fetch mode used
	DurationMs  int64  `json:"duration_ms"`  // Total duration
}


// Analyze fetches and analyzes a URL to generate schema suggestions.
// The byokAllowed parameter comes from JWT claims and indicates if user has the "provider_byok" feature.
// The modelsCustomAllowed parameter comes from JWT claims and indicates if user has the "models_custom" feature.
// The contentDynamicAllowed parameter comes from JWT claims and indicates if user has the "content_dynamic" feature.
// The skipCreditCheck parameter comes from JWT claims and indicates if user has the "skip_credit_check" feature.
func (s *AnalyzerService) Analyze(ctx context.Context, userID string, input AnalyzeInput, tier string, byokAllowed bool, modelsCustomAllowed bool, contentDynamicAllowed bool, skipCreditCheck bool) (*AnalyzeOutput, error) {
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
		"byok_allowed", byokAllowed,
		"models_custom_allowed", modelsCustomAllowed,
		"cleaner", "noop (fallback: refyne)",
	)

	// Get LLM config chain for analysis (fallback chain iterator)
	s.logger.Debug("resolving LLM config chain", "request_id", requestID, "user_id", userID, "tier", tier, "byok_allowed", byokAllowed, "models_custom_allowed", modelsCustomAllowed)
	llmChain := s.resolveLLMConfigChain(ctx, userID, tier, requestID, byokAllowed, modelsCustomAllowed)
	if llmChain.IsEmpty() {
		s.logger.Error("no valid LLM models configured",
			"request_id", requestID,
			"user_id", userID,
			"tier", tier,
			"byok_allowed", byokAllowed,
			"models_custom_allowed", modelsCustomAllowed,
		)
		return nil, llm.NewNoModelsConfiguredError("no models in fallback chain or missing API keys")
	}

	firstCfg := llmChain.First()
	s.logger.Debug("LLM config chain resolved",
		"request_id", requestID,
		"user_id", userID,
		"first_provider", firstCfg.Provider,
		"first_model", firstCfg.Model,
		"has_api_key", firstCfg.APIKey != "",
		"is_byok", llmChain.IsBYOK(),
		"chain_length", llmChain.Len(),
	)

	// Pre-flight cost estimate (triggers pricing cache refresh if needed) and balance check
	// Use first config in chain for estimate
	if s.billing != nil {
		estimatedCost := s.billing.EstimateCost(1, firstCfg.Model, firstCfg.Provider)
		s.logger.Debug("pre-flight cost estimate",
			"request_id", requestID,
			"user_id", userID,
			"provider", firstCfg.Provider,
			"model", firstCfg.Model,
			"estimated_cost_usd", estimatedCost,
			"is_byok", llmChain.IsBYOK(),
		)
		// Only check balance for non-BYOK users
		if !llmChain.IsBYOK() {
			if err := s.billing.CheckSufficientBalance(ctx, userID, skipCreditCheck, estimatedCost); err != nil {
				return nil, err
			}
		}
	}

	// Fetch main page content
	fetchStart := time.Now()
	s.logger.Debug("fetching main page content", "request_id", requestID, "user_id", userID, "url", targetURL, "fetch_mode", input.FetchMode, "job_id", input.JobID)
	mainContent, links, fetchMode, err := s.fetchContent(ctx, targetURL, input.FetchMode, userID, tier, contentDynamicAllowed, input.JobID)
	if err != nil {
		s.logger.Error("failed to fetch page content", "request_id", requestID, "user_id", userID, "url", targetURL, "error", err)
		s.recordAnalyzeUsage(ctx, userID, tier, targetURL, firstCfg, llmChain.IsBYOK(), 0, 0,
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
			content, _, _, err := s.fetchContent(ctx, detailURL, string(fetchMode), userID, tier, contentDynamicAllowed, input.JobID)
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
		maxSamples := min(2, len(links))
		for i := range maxSamples {
			content, _, _, err := s.fetchContent(ctx, links[i], string(fetchMode), userID, tier, contentDynamicAllowed, input.JobID)
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
	// Iterate through fallback chain if a model fails
	llmStart := time.Now()
	var result *analyzeResult
	var lastErr error
	var llmConfig *LLMConfigInput

	for cfg := llmChain.Next(); cfg != nil; cfg = llmChain.Next() {
		llmConfig = cfg // Track which config we're using
		pos, total := llmChain.Position()

		s.logger.Info("analysis attempt",
			"request_id", requestID,
			"user_id", userID,
			"provider", cfg.Provider,
			"model", cfg.Model,
			"attempt", pos,
			"of", total,
		)

		result, lastErr = s.analyzeWithLLM(ctx, targetURL, mainContent, detailContents, detailURLs, links, cfg)

		// If context length error, retry with cleaned content
		if lastErr != nil && isContextLengthError(lastErr) {
			s.logger.Info("context length error detected, retrying with cleaned content",
				"request_id", requestID,
				"user_id", userID,
				"original_content_length", len(mainContent),
				"error", lastErr.Error(),
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

			// Retry with cleaned content (same model)
			result, lastErr = s.analyzeWithLLM(ctx, targetURL, cleanedMain, cleanedDetails, detailURLs, links, cfg)
			if lastErr == nil {
				s.logger.Info("analysis succeeded after retry with cleaned content",
					"request_id", requestID,
					"user_id", userID,
					"provider", cfg.Provider,
					"model", cfg.Model,
				)
			}
		}

		// Success!
		if lastErr == nil {
			s.logger.Info("analysis succeeded",
				"request_id", requestID,
				"user_id", userID,
				"provider", cfg.Provider,
				"model", cfg.Model,
				"attempt", pos,
				"of", total,
			)
			break
		}

		// Log failure and try next model in the chain
		s.logger.Warn("analysis failed, trying next model in fallback chain",
			"request_id", requestID,
			"user_id", userID,
			"provider", cfg.Provider,
			"model", cfg.Model,
			"error", lastErr.Error(),
			"attempt", pos,
			"of", total,
		)
	}

	llmDuration := time.Since(llmStart)

	// If all models in the chain failed
	if lastErr != nil {
		s.logger.Error("LLM analysis failed (all models in fallback chain exhausted)",
			"request_id", requestID,
			"user_id", userID,
			"last_provider", llmConfig.Provider,
			"last_model", llmConfig.Model,
			"chain_length", llmChain.Len(),
			"error", lastErr,
		)
		s.recordAnalyzeUsage(ctx, userID, tier, targetURL, llmConfig, llmChain.IsBYOK(), 0, 0,
			int(fetchDuration.Milliseconds()), int(llmDuration.Milliseconds()), int(time.Since(startTime).Milliseconds()),
			"failed", lastErr.Error(), requestID)
		return nil, fmt.Errorf("LLM analysis failed: %w", lastErr)
	}

	// Record successful usage
	s.recordAnalyzeUsage(ctx, userID, tier, targetURL, llmConfig, llmChain.IsBYOK(),
		result.InputTokens, result.OutputTokens,
		int(fetchDuration.Milliseconds()), int(llmDuration.Milliseconds()), int(time.Since(startTime).Milliseconds()),
		"success", "", requestID)

	result.Output.RecommendedFetchMode = fetchMode
	result.Output.FetchModeUsed = fetchMode // Record what mode was actually used
	if len(links) > 0 {
		result.Output.SampleLinks = links[:min(10, len(links))] // Return up to 10 sample links
	}

	// Calculate costs
	var costs CostResult
	if s.billing != nil {
		costs = s.billing.CalculateCosts(ctx, CostInput{
			TokensInput:  result.InputTokens,
			TokensOutput: result.OutputTokens,
			Model:        llmConfig.Model,
			Provider:     llmConfig.Provider,
			Tier:         tier,
			IsBYOK:       llmChain.IsBYOK(),
		})
	}

	// Add token usage and cost info to output
	result.Output.TokenUsage = AnalyzeTokenUsage{
		InputTokens:  result.InputTokens,
		OutputTokens: result.OutputTokens,
		CostUSD:      costs.UserCostUSD,
		LLMCostUSD:   costs.LLMCostUSD,
		IsBYOK:       llmChain.IsBYOK(),
		LLMProvider:  llmConfig.Provider,
		LLMModel:     llmConfig.Model,
	}

	// Populate debug capture data (always - handler decides whether to store)
	result.Output.DebugCapture = &AnalyzeDebugCapture{
		RawContent:  mainContent,
		Prompt:      result.Prompt,
		LLMResponse: result.LLMResponse,
		FetchMode:   string(fetchMode),
		DurationMs:  time.Since(startTime).Milliseconds(),
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
		"llm_cost_usd", costs.LLMCostUSD,
		"user_cost_usd", costs.UserCostUSD,
		"is_byok", llmChain.IsBYOK(),
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
// Supports both static (Colly) and dynamic (browser rendering) fetch modes.
func (s *AnalyzerService) fetchContent(ctx context.Context, targetURL string, fetchMode string, userID string, tier string, contentDynamicAllowed bool, jobID string) (string, []string, models.FetchMode, error) {
	// Determine effective fetch mode
	effectiveFetchMode := fetchMode
	if effectiveFetchMode == "" || effectiveFetchMode == "auto" {
		effectiveFetchMode = "static"
	}

	// Handle dynamic fetch mode
	if effectiveFetchMode == "dynamic" {
		// Check if user has permission for dynamic content
		if !contentDynamicAllowed {
			return "", nil, models.FetchModeStatic, ErrDynamicFetchNotAllowed
		}
		// Check if captcha service is configured
		if s.captchaSvc == nil {
			return "", nil, models.FetchModeStatic, ErrDynamicFetchNotConfigured
		}

		s.logger.Info("using browser rendering for analysis",
			"url", targetURL,
			"user_id", userID,
			"job_id", jobID,
		)

		// Use captcha service for dynamic fetching
		result, err := s.captchaSvc.FetchDynamicContent(ctx, userID, tier, CaptchaSolveInput{
			URL:        targetURL,
			MaxTimeout: 60000,
			JobID:      jobID,
		})
		if err != nil {
			return "", nil, models.FetchModeDynamic, fmt.Errorf("browser rendering failed: %w", err)
		}
		if result.Status != "ok" || result.Solution == nil {
			return "", nil, models.FetchModeDynamic, fmt.Errorf("browser rendering returned non-ok status: %s", result.Message)
		}

		content := result.Solution.Response
		links := s.extractLinksFromHTML(targetURL, content)

		// Apply cleaner to content
		cleanedContent, err := s.cleaner.Clean(content)
		if err != nil {
			return "", nil, models.FetchModeDynamic, fmt.Errorf("failed to clean content: %w", err)
		}

		return cleanedContent, links, models.FetchModeDynamic, nil
	}

	// Static fetching with Colly
	var content string
	var rawBody []byte
	var statusCode int
	var links []string
	recommendedMode := models.FetchModeStatic
	isAutoMode := fetchMode == "" || fetchMode == "auto"

	c := colly.NewCollector(
		colly.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
		colly.AllowURLRevisit(),
	)

	// Set timeout
	c.SetRequestTimeout(30 * time.Second)

	// Capture full HTML and status code for protection detection
	c.OnResponse(func(r *colly.Response) {
		content = string(r.Body)
		rawBody = r.Body
		statusCode = r.StatusCode
	})

	// Also capture responses for 4xx/5xx errors (for bot protection detection)
	c.OnError(func(r *colly.Response, err error) {
		if r != nil {
			content = string(r.Body)
			rawBody = r.Body
			statusCode = r.StatusCode
		}
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

	var visitErr error
	if err := c.Visit(targetURL); err != nil {
		visitErr = err
		// If we didn't capture any response (true network error), return immediately
		if rawBody == nil {
			return "", nil, recommendedMode, fmt.Errorf("failed to fetch URL: %w", err)
		}
		// Otherwise, we have a response captured in OnError - continue to bot protection check
	}

	// Check for bot protection signals in the response
	if s.protectionDetector != nil {
		detection := s.protectionDetector.DetectFromResponse(statusCode, nil, rawBody)
		if detection.Detected {
			s.logger.Info("bot protection detected in static fetch",
				"url", targetURL,
				"user_id", userID,
				"signal", detection.Signal,
				"confidence", detection.Confidence,
				"description", detection.Description,
			)

			// If auto mode and protection is retryable, attempt dynamic fetch
			if isAutoMode && detection.IsRetryable() {
				// Check if user can use dynamic mode
				if contentDynamicAllowed && s.captchaSvc != nil {
					s.logger.Info("auto-retrying with browser rendering due to bot protection",
						"url", targetURL,
						"user_id", userID,
						"job_id", jobID,
						"protection_type", detection.Signal,
					)

					// Recursive call with explicit dynamic mode
					return s.fetchContent(ctx, targetURL, "dynamic", userID, tier, contentDynamicAllowed, jobID)
				}

				// Log why auto-retry didn't happen
				s.logger.Info("cannot auto-retry with dynamic mode",
					"url", targetURL,
					"user_id", userID,
					"content_dynamic_allowed", contentDynamicAllowed,
					"captcha_service_configured", s.captchaSvc != nil,
				)

				// User doesn't have dynamic feature or service not configured
				// Return error with suggestion to use browser rendering
				return "", nil, models.FetchModeStatic, NewErrBotProtectionDetected(
					string(detection.Signal),
					detection.UserMessage(),
				)
			}

			// If protection detected but not auto mode, just recommend dynamic
			recommendedMode = models.FetchModeDynamic
		}
	}

	// If we had an HTTP error but no bot protection was detected/handled, return the original error
	if visitErr != nil {
		return "", nil, recommendedMode, fmt.Errorf("failed to fetch URL: %w", visitErr)
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

// extractLinksFromHTML parses HTML content and extracts links.
// Used for dynamic fetch mode where we don't have Colly's built-in link extraction.
func (s *AnalyzerService) extractLinksFromHTML(baseURL string, htmlContent string) []string {
	var links []string

	// Simple regex-based link extraction
	// This is a fallback when Colly isn't available (dynamic mode)
	linkRegex := regexp.MustCompile(`href=["']([^"']+)["']`)
	matches := linkRegex.FindAllStringSubmatch(htmlContent, -1)

	parsedBase, err := url.Parse(baseURL)
	if err != nil {
		return links
	}

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		href := match[1]
		if href == "" || strings.HasPrefix(href, "#") || strings.HasPrefix(href, "javascript:") {
			continue
		}

		// Resolve relative URLs
		parsedHref, err := url.Parse(href)
		if err != nil {
			continue
		}
		absURL := parsedBase.ResolveReference(parsedHref).String()
		links = append(links, absURL)
	}

	return s.filterLinks(baseURL, links)
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
	slices.SortFunc(scored, func(a, b scoredLink) int {
		return cmp.Compare(b.score, a.score)
	})

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
	Prompt       string // Full prompt sent to LLM (for debug capture)
	LLMResponse  string // Raw LLM response (for debug capture)
}

// analyzeWithLLM calls the LLM to analyze page content and generate a schema.
func (s *AnalyzerService) analyzeWithLLM(ctx context.Context, mainURL string, mainContent string, detailContents []string, detailURLs []string, links []string, llmConfig *LLMConfigInput) (*analyzeResult, error) {
	// Run preprocessor to generate hints before truncation (needs full content)
	// Pass URL for URL-aware detection (e.g., /reviews triggers feedback hints)
	hints, err := s.preprocessor.ProcessWithURL(mainContent, mainURL)
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

	// Call LLM API using shared client
	llmClient := NewLLMClient(s.logger)
	opts := DefaultLLMCallOptions()
	opts.JSONMode = true // Request JSON response format for structured output
	llmResult, err := llmClient.Call(ctx, llmConfig, prompt, opts)
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
		Prompt:       prompt,
		LLMResponse:  llmResult.Content,
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
		for i := range maxLinks {
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

**Group content by its natural category using separate arrays.** Identify what types of content are on the page and create appropriately named arrays for each.

**IMPORTANT**: Do NOT use a generic items[] array with a content_type field. Instead, create separate arrays for each distinct content type you identify.

**Required fields**: title, url (always include these as required)
**Add relevant fields** based on what you observe in the content - don't limit yourself to a fixed template.

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

The suggested_schema MUST be a YAML string with this structure:

name: SchemaName
description: What this schema extracts
fields:
  - name: [category_name]
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
        # Add category-specific fields as needed
`)

	// Add contextual examples based on detected content types
	sb.WriteString(buildSchemaExamples(hints))

	sb.WriteString(`
Valid types: string, integer, number, boolean, array, object. Use "items" for arrays, "properties" for objects.

### Follow Patterns

Generate CSS selectors from the Links Found section. Use ONLY href-based selectors:
- GOOD: a[href*='/blog/'], a[href*='/products/'], a[href^='/jobs/']
- BAD: .blog-post a (invented class), main a[href*='/x/'] (assumes main exists)

Identify ALL distinct URL patterns. Large sites need 3-6 patterns for different sections.
`)

	return sb.String()
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

// resolveLLMConfigChain determines which LLM configurations to use for analysis.
// Returns an iterator over configs in the fallback chain so the caller can try each on failure.
// Implements 2x2 feature matrix:
// - BYOK + models_custom: user keys + user chain
// - BYOK only: user keys + system chain
// - models_custom only: system keys + user chain
// - Neither: system keys + system chain
func (s *AnalyzerService) resolveLLMConfigChain(ctx context.Context, userID, tier, requestID string, byokAllowed bool, modelsCustomAllowed bool) *LLMConfigChain {
	// Delegate to resolver for all standard cases
	if s.resolver != nil {
		return s.resolver.ResolveConfigChain(ctx, userID, nil, tier, byokAllowed, modelsCustomAllowed)
	}

	// Resolver not set - this is a configuration error
	s.logger.Error("LLM config resolver not set - this is a server configuration error")
	return NewLLMConfigChain(nil, false)
}

// ExtractDomain extracts the domain from a URL.
func ExtractDomain(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return parsed.Host
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
