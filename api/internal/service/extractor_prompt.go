package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/jmylchreest/refyne/pkg/refyne"

	"github.com/jmylchreest/refyne-api/internal/llm"
)

// PromptPageExtractor extracts data from a single page using a freeform prompt.
// It handles fetch + LLM call with dynamic retry support for bot protection
// detection and insufficient content errors.
type PromptPageExtractor struct {
	svc          *ExtractionService
	promptText   string
	llmCfg       *LLMConfigInput
	cleanerChain []CleanerConfig
	isBYOK       bool

	// Context for dynamic retry
	contentDynamicAllowed bool
	userID                string
	tier                  string
	jobID                 string
}

// NewPromptPageExtractor creates a new prompt-based page extractor.
func NewPromptPageExtractor(svc *ExtractionService, opts PromptExtractorOptions) *PromptPageExtractor {
	return &PromptPageExtractor{
		svc:                   svc,
		promptText:            opts.PromptText,
		llmCfg:                opts.LLMConfig,
		cleanerChain:          opts.CleanerChain,
		isBYOK:                opts.IsBYOK,
		contentDynamicAllowed: opts.ContentDynamicAllowed,
		userID:                opts.UserID,
		tier:                  opts.Tier,
		jobID:                 opts.JobID,
	}
}

// Extract fetches and extracts data from a single URL using the configured prompt.
// Automatically retries with browser rendering if bot protection or insufficient
// content is detected (when ContentDynamicAllowed is true).
func (e *PromptPageExtractor) Extract(ctx context.Context, pageURL string) (*PageExtractionResult, error) {
	result := &PageExtractionResult{URL: pageURL}
	startTime := time.Now()

	effectiveFetchMode := "auto"
	dynamicRetryAttempted := false

extractAttempt:
	// 1. Fetch and clean content (with fetch mode)
	fetchStart := time.Now()
	pageContent, fetchedURL, err := e.fetchAndCleanContentWithMode(ctx, pageURL, effectiveFetchMode)
	result.FetchDurationMs = int(time.Since(fetchStart).Milliseconds())
	result.URL = fetchedURL

	if err != nil {
		// Check for bot protection - retry with dynamic if allowed
		var protectionErr *ErrBotProtectionDetected
		if errors.As(err, &protectionErr) && !dynamicRetryAttempted && e.contentDynamicAllowed && e.svc.captchaSvc != nil {
			e.svc.logger.Info("auto-retrying prompt extraction with browser rendering",
				"url", pageURL,
				"protection_type", protectionErr.ProtectionType,
			)
			effectiveFetchMode = "dynamic"
			dynamicRetryAttempted = true
			result.RetryCount++
			goto extractAttempt
		}

		// Check for insufficient content - retry with dynamic if allowed
		var insufficientErr *refyne.InsufficientContentError
		if errors.As(err, &insufficientErr) && !dynamicRetryAttempted && e.contentDynamicAllowed && e.svc.captchaSvc != nil {
			e.svc.logger.Info("auto-retrying prompt extraction with browser rendering due to insufficient content",
				"url", pageURL,
				"content_size", insufficientErr.ContentSize,
			)
			effectiveFetchMode = "dynamic"
			dynamicRetryAttempted = true
			result.RetryCount++
			goto extractAttempt
		}

		result.Error = err
		result.ErrorCategory = "fetch_error"
		result.UsedDynamicMode = effectiveFetchMode == "dynamic"
		return result, err
	}

	result.RawContent = pageContent
	result.UsedDynamicMode = effectiveFetchMode == "dynamic"

	// 2. Truncate content if too long
	maxContentLen := 100000 // ~25k tokens roughly
	if len(pageContent) > maxContentLen {
		pageContent = pageContent[:maxContentLen] + "\n\n[Content truncated...]"
	}

	// 3. Check for insufficient content after cleaning
	minContentSize := 200
	if len(pageContent) < minContentSize {
		if !dynamicRetryAttempted && e.contentDynamicAllowed && e.svc.captchaSvc != nil {
			e.svc.logger.Info("auto-retrying prompt extraction with browser rendering due to insufficient content after cleaning",
				"url", pageURL,
				"content_size", len(pageContent),
			)
			effectiveFetchMode = "dynamic"
			dynamicRetryAttempted = true
			result.RetryCount++
			goto extractAttempt
		}
		err := fmt.Errorf("page has insufficient content (%d bytes) - likely requires JavaScript rendering", len(pageContent))
		result.Error = err
		result.ErrorCategory = "fetch_error"
		return result, err
	}

	// 4. Build prompt and call LLM
	extractPrompt := e.svc.buildPromptExtractionPrompt(pageContent, e.promptText)
	llmClient := NewLLMClient(e.svc.logger)
	llmResult, err := llmClient.Call(ctx, e.llmCfg, extractPrompt, LLMCallOptions{
		Temperature: 0.1,
		MaxTokens:   8192,
		Timeout:     180 * time.Second,
		JSONMode:    true,
	})

	result.ExtractDurationMs = int(time.Since(startTime).Milliseconds()) - result.FetchDurationMs
	result.Provider = e.llmCfg.Provider
	result.Model = e.llmCfg.Model

	if err != nil {
		errInfo := llm.WrapError(err, e.llmCfg.Provider, e.llmCfg.Model, e.isBYOK)
		result.Error = err
		result.ErrorCategory = errInfo.Category
		return result, err
	}

	// 5. Parse JSON response
	var extractedData any
	if jsonErr := json.Unmarshal([]byte(llmResult.Content), &extractedData); jsonErr != nil {
		extractedData = map[string]any{
			"raw_response": llmResult.Content,
			"parse_error":  "Response was not valid JSON",
		}
	}

	result.Data = extractedData
	result.TokensInput = llmResult.InputTokens
	result.TokensOutput = llmResult.OutputTokens

	return result, nil
}

// fetchAndCleanContentWithMode fetches a URL and cleans the content, supporting different fetch modes.
// When mode is "dynamic", uses browser rendering via the captcha service.
// When mode is "auto", uses protection-aware fetcher that detects bot protection.
func (e *PromptPageExtractor) fetchAndCleanContentWithMode(ctx context.Context, targetURL, fetchMode string) (string, string, error) {
	// Create cleaner chain
	factory := NewCleanerFactory()
	contentCleaner, err := factory.CreateChainWithDefault(e.cleanerChain, DefaultExtractionCleanerChain)
	if err != nil {
		return "", "", fmt.Errorf("invalid cleaner chain: %w", err)
	}

	var body []byte
	var finalURL string

	switch fetchMode {
	case "dynamic":
		// Use browser rendering via captcha service
		if !e.contentDynamicAllowed {
			return "", "", ErrDynamicFetchNotAllowed
		}
		if e.svc.captchaSvc == nil {
			return "", "", ErrDynamicFetchNotConfigured
		}

		e.svc.logger.Info("using browser rendering for prompt extraction",
			"url", targetURL,
			"user_id", e.userID,
			"job_id", e.jobID,
		)

		result, err := e.svc.captchaSvc.FetchDynamicContent(ctx, e.userID, e.tier, CaptchaSolveInput{
			URL:        targetURL,
			MaxTimeout: 60000,
			JobID:      e.jobID,
		})
		if err != nil {
			return "", "", fmt.Errorf("dynamic fetch failed: %w", err)
		}
		if result.Status != "ok" || result.Solution == nil {
			return "", "", fmt.Errorf("browser rendering returned non-ok status: %s", result.Message)
		}
		body = []byte(result.Solution.Response)
		finalURL = targetURL // Browser service doesn't track redirects

	case "auto", "":
		// Use protection-aware fetching
		var resp *http.Response
		body, resp, err = e.fetchWithProtectionDetection(ctx, targetURL)
		if err != nil {
			return "", "", err
		}
		finalURL = resp.Request.URL.String()

	default:
		// Static mode - simple HTTP fetch
		client := &http.Client{Timeout: 60 * time.Second}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
		if err != nil {
			return "", "", fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		req.Header.Set("Accept-Language", "en-US,en;q=0.5")

		resp, err := client.Do(req)
		if err != nil {
			return "", "", fmt.Errorf("failed to fetch page: %w", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			return "", "", fmt.Errorf("page returned status %d", resp.StatusCode)
		}

		body, err = io.ReadAll(resp.Body)
		if err != nil {
			return "", "", fmt.Errorf("failed to read response: %w", err)
		}
		finalURL = resp.Request.URL.String()
	}

	// Clean the content
	cleanedContent, err := contentCleaner.Clean(string(body))
	if err != nil {
		// If cleaning fails, use raw content
		e.svc.logger.Warn("content cleaning failed, using raw HTML", "error", err)
		cleanedContent = string(body)
	}

	return cleanedContent, finalURL, nil
}

// fetchWithProtectionDetection performs HTTP fetch with bot protection detection.
// Returns the body, response (for URL tracking), and any error.
func (e *PromptPageExtractor) fetchWithProtectionDetection(ctx context.Context, targetURL string) ([]byte, *http.Response, error) {
	client := &http.Client{Timeout: 60 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")

	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch page: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check for bot protection signals using the protection detector
	if e.svc.protectionDetector != nil {
		result := e.svc.protectionDetector.DetectFromResponse(resp.StatusCode, resp.Header, body)
		if result.Detected && result.SuggestDynamic {
			return nil, nil, NewErrBotProtectionDetected(string(result.Signal), result.UserMessage())
		}
	}

	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("page returned status %d", resp.StatusCode)
	}

	return body, resp, nil
}
