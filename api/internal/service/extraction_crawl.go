package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jmylchreest/refyne-api/internal/llm"
)

// CrawlInput represents crawl input.
type CrawlInput struct {
	JobID        string            `json:"job_id,omitempty"`        // Job ID for logging/tracking
	URL          string            `json:"url"`
	SeedURLs     []string          `json:"seed_urls,omitempty"`     // Additional seed URLs (from sitemap discovery)
	Schema       json.RawMessage   `json:"schema"`
	Options      CrawlOptions      `json:"options"`
	LLMConfigs   []*LLMConfigInput `json:"llm_configs,omitempty"`   // Pre-resolved LLM config chain
	Tier         string            `json:"tier,omitempty"`          // User's subscription tier at job creation time
	IsBYOK       bool              `json:"is_byok,omitempty"`       // Whether using user's own API keys
	CleanerChain []CleanerConfig   `json:"cleaner_chain,omitempty"` // Content cleaner chain
}

// Note: CrawlOptions is defined in job_service.go to avoid duplication

// PageResult represents an individual page result from a crawl.
type PageResult struct {
	URL               string  `json:"url"`
	ParentURL         *string `json:"parent_url,omitempty"` // URL that linked to this page (nil for seed)
	Depth             int     `json:"depth"`                // Distance from seed URL (0 for seed)
	Data              any     `json:"data,omitempty"`
	Error             string  `json:"error,omitempty"`           // User-visible error (sanitized)
	ErrorDetails      string  `json:"error_details,omitempty"`   // Full error (admin/BYOK only)
	ErrorCategory     string  `json:"error_category,omitempty"`  // Error classification
	LLMProvider       string  `json:"llm_provider,omitempty"`    // Provider used
	LLMModel          string  `json:"llm_model,omitempty"`       // Model used
	GenerationID      string  `json:"generation_id,omitempty"`   // Provider generation ID for cost tracking
	IsBYOK            bool    `json:"is_byok"`                   // True if user's own key
	RetryCount        int     `json:"retry_count"`               // Number of retries
	TokenUsageInput   int     `json:"token_usage_input"`
	TokenUsageOutput  int     `json:"token_usage_output"`
	FetchDurationMs   int     `json:"fetch_duration_ms,omitempty"`
	ExtractDurationMs int     `json:"extract_duration_ms,omitempty"`
	RawContent        string  `json:"-"` // Raw page content (not serialized, for debug capture only)
}

// CrawlResult represents the result of a crawl operation.
type CrawlResult struct {
	Results           []any        `json:"results"`      // Aggregated data (backward compat)
	PageResults       []PageResult `json:"page_results"` // Individual page results for SSE streaming
	PageCount         int          `json:"page_count"`
	TotalTokensInput  int          `json:"total_tokens_input"`
	TotalTokensOutput int          `json:"total_tokens_output"`
	TotalCostUSD      float64      `json:"total_cost_usd"`     // Actual USD cost charged to user
	TotalLLMCostUSD   float64      `json:"total_llm_cost_usd"` // Actual LLM provider cost
	LLMProvider       string       `json:"llm_provider"`       // LLM provider used
	LLMModel          string       `json:"llm_model"`          // LLM model used
	StoppedEarly      bool         `json:"stopped_early"`      // True if crawl terminated before completion
	StopReason        string       `json:"stop_reason"`        // Reason for early stop (e.g., "insufficient_balance")
}

// CrawlResultCallback is called for each page result during a crawl.
// Return an error to stop the crawl early.
type CrawlResultCallback func(result PageResult) error

// URLsQueuedCallback is called when URLs are discovered and queued for processing.
// The count parameter is the total number of URLs currently queued.
type URLsQueuedCallback func(queuedCount int)

// CrawlCallbacks holds callbacks for crawl events.
type CrawlCallbacks struct {
	// OnResult is called for each page result (success or failure).
	// Return an error to stop the crawl early.
	OnResult CrawlResultCallback

	// OnURLsQueued is called when URLs are discovered and queued.
	// This is useful for progress tracking when the total is not known upfront.
	OnURLsQueued URLsQueuedCallback
}

// Crawl performs a multi-page crawl extraction.
// Uses SchemaPageExtractor for each page, which handles dynamic retry for bot protection
// and insufficient content. This is a simplified version of CrawlWithCallback without
// callbacks or mid-crawl balance checking.
func (s *ExtractionService) Crawl(ctx context.Context, userID string, input CrawlInput) (*CrawlResult, error) {
	// Use pre-resolved LLM configs from input
	if len(input.LLMConfigs) == 0 {
		return nil, fmt.Errorf("no LLM configs provided")
	}
	llmCfg := input.LLMConfigs[0]
	isBYOK := input.IsBYOK

	// Build seed URLs list - use SeedURLs if provided (from sitemap), otherwise just the main URL
	seedURLs := input.SeedURLs
	if len(seedURLs) == 0 {
		seedURLs = []string{input.URL}
	}

	// Detect input format - prompts use a different extraction path
	inputFormat, sch, err := DetectInputFormat(input.Schema)
	if inputFormat == InputFormatPrompt {
		// Use prompt-based crawling
		return s.crawlWithPrompt(ctx, userID, input, nil)
	}
	if err != nil {
		return nil, fmt.Errorf("invalid schema: %w", err)
	}

	// Enrich cleaner chain with crawl selectors as keep selectors
	// This ensures elements matching FollowSelector/NextSelector are preserved during cleaning
	enrichedCleanerChain := EnrichCleanerChainWithCrawlSelectors(
		input.CleanerChain,
		input.Options.FollowSelector,
		input.Options.NextSelector,
	)

	s.logger.Info("crawl starting",
		"job_id", input.JobID,
		"user_id", userID,
		"url", input.URL,
		"seed_count", len(seedURLs),
		"provider", llmCfg.Provider,
		"model", llmCfg.Model,
	)

	// Phase 1: URL Discovery
	// If we have seed URLs (from sitemap), use those directly.
	// Otherwise, discover URLs using Colly-based URL discovery.
	var urlsToExtract []DiscoveredURL

	if len(seedURLs) > 1 {
		// Multiple seeds provided (from sitemap discovery) - use them directly
		for i, url := range seedURLs {
			urlsToExtract = append(urlsToExtract, DiscoveredURL{
				URL:       url,
				Depth:     0,
				ParentURL: "",
			})
			// Respect max pages limit
			if input.Options.MaxPages > 0 && i+1 >= input.Options.MaxPages {
				break
			}
		}
		s.logger.Info("using provided seed URLs",
			"job_id", input.JobID,
			"url_count", len(urlsToExtract),
		)
	} else if input.Options.FollowSelector != "" || input.Options.FollowPattern != "" || input.Options.NextSelector != "" {
		// Need to discover URLs - use URLDiscoverer
		discoverer := NewURLDiscoverer(s.logger)
		discovered, err := discoverer.Discover(ctx, seedURLs, URLDiscoveryOptions{
			FollowSelector: input.Options.FollowSelector,
			FollowPattern:  input.Options.FollowPattern,
			MaxPages:       input.Options.MaxPages,
			MaxDepth:       input.Options.MaxDepth,
			MaxURLs:        input.Options.MaxURLs,
			SameDomainOnly: input.Options.SameDomainOnly,
			NextSelector:   input.Options.NextSelector,
		})
		if err != nil {
			return nil, fmt.Errorf("URL discovery failed: %w", err)
		}
		urlsToExtract = discovered
		s.logger.Info("URL discovery completed",
			"job_id", input.JobID,
			"urls_discovered", len(urlsToExtract),
		)
	} else {
		// No follow selectors - just extract the seed URL
		urlsToExtract = []DiscoveredURL{{
			URL:       input.URL,
			Depth:     0,
			ParentURL: "",
		}}
	}

	// Phase 2: Per-page extraction using SchemaPageExtractor
	// This gives us dynamic retry for each page individually.

	// Create SchemaPageExtractor - handles dynamic retry for all pages
	extractor := NewSchemaPageExtractor(s, sch, SchemaExtractorOptions{
		LLMConfig:             llmCfg,
		CleanerChain:          enrichedCleanerChain,
		ContentDynamicAllowed: input.Options.ContentDynamicAllowed,
		UserID:                userID,
		Tier:                  input.Tier,
		JobID:                 input.JobID,
	})

	var (
		data              []any
		pageResults       []PageResult
		totalTokensInput  int
		totalTokensOutput int
		pageCount         int
		lastError         error
		cancelled         bool
	)

	// Process each URL using the extractor (gets dynamic retry for free!)
	for _, discoveredURL := range urlsToExtract {
		select {
		case <-ctx.Done():
			cancelled = true
		default:
		}

		if cancelled {
			break
		}

		var parentURL *string
		if discoveredURL.ParentURL != "" {
			parentURL = &discoveredURL.ParentURL
		}

		pageResult := PageResult{
			URL:         discoveredURL.URL,
			ParentURL:   parentURL,
			Depth:       discoveredURL.Depth,
			IsBYOK:      isBYOK,
			LLMProvider: llmCfg.Provider,
			LLMModel:    llmCfg.Model,
		}

		// Extract using SchemaPageExtractor (handles dynamic retry internally)
		extractResult, err := extractor.Extract(ctx, discoveredURL.URL)

		if err != nil || (extractResult != nil && extractResult.Error != nil) {
			// Handle error
			errToUse := err
			if errToUse == nil && extractResult != nil {
				errToUse = extractResult.Error
			}
			lastError = errToUse

			errInfo := llm.WrapError(errToUse, llmCfg.Provider, llmCfg.Model, isBYOK)
			pageResult.Error = errInfo.UserMessage
			pageResult.ErrorCategory = errInfo.Category
			if isBYOK {
				pageResult.ErrorDetails = errToUse.Error()
			}
			if extractResult != nil {
				pageResult.FetchDurationMs = extractResult.FetchDurationMs
				pageResult.ExtractDurationMs = extractResult.ExtractDurationMs
				pageResult.RetryCount = extractResult.RetryCount
			}
			s.logger.Warn("crawl page error",
				"job_id", input.JobID,
				"user_id", userID,
				"url", discoveredURL.URL,
				"error", errToUse,
				"used_dynamic", extractResult != nil && extractResult.UsedDynamicMode,
			)
		} else {
			// Success
			pageResult.URL = extractResult.URL
			pageResult.Data = extractResult.Data
			pageResult.TokenUsageInput = extractResult.TokensInput
			pageResult.TokenUsageOutput = extractResult.TokensOutput
			pageResult.FetchDurationMs = extractResult.FetchDurationMs
			pageResult.ExtractDurationMs = extractResult.ExtractDurationMs
			pageResult.GenerationID = extractResult.GenerationID
			pageResult.RetryCount = extractResult.RetryCount
			pageResult.RawContent = extractResult.RawContent

			data = append(data, extractResult.Data)
			totalTokensInput += extractResult.TokensInput
			totalTokensOutput += extractResult.TokensOutput
			pageCount++

			// Calculate costs for logging
			var pageCosts CostResult
			if s.billing != nil {
				pageCosts = s.billing.CalculateCosts(ctx, CostInput{
					TokensInput:  extractResult.TokensInput,
					TokensOutput: extractResult.TokensOutput,
					Model:        llmCfg.Model,
					Provider:     llmCfg.Provider,
					Tier:         input.Tier,
					IsBYOK:       isBYOK,
					GenerationID: extractResult.GenerationID,
					APIKey:       llmCfg.APIKey,
				})
			}

			s.logger.Info("extracted page",
				"job_id", input.JobID,
				"url", extractResult.URL,
				"input_tokens", extractResult.TokensInput,
				"output_tokens", extractResult.TokensOutput,
				"llm_cost_usd", pageCosts.LLMCostUSD,
				"user_cost_usd", pageCosts.UserCostUSD,
				"used_dynamic", extractResult.UsedDynamicMode,
				"retry_count", extractResult.RetryCount,
			)
		}

		pageResults = append(pageResults, pageResult)
	}

	// If no results and we have an error, return the error
	if pageCount == 0 && lastError != nil {
		return nil, s.handleLLMError(lastError, llmCfg, isBYOK)
	}

	// Calculate actual costs
	var totalCosts CostResult
	if s.billing != nil {
		totalCosts = s.billing.CalculateCosts(ctx, CostInput{
			TokensInput:  totalTokensInput,
			TokensOutput: totalTokensOutput,
			Model:        llmCfg.Model,
			Provider:     llmCfg.Provider,
			Tier:         input.Tier,
			IsBYOK:       isBYOK,
		})
	}

	s.logger.Info("crawl completed",
		"job_id", input.JobID,
		"user_id", userID,
		"url", input.URL,
		"provider", llmCfg.Provider,
		"model", llmCfg.Model,
		"page_count", pageCount,
		"total_input_tokens", totalTokensInput,
		"total_output_tokens", totalTokensOutput,
		"llm_cost_usd", totalCosts.LLMCostUSD,
		"user_cost_usd", totalCosts.UserCostUSD,
	)

	return &CrawlResult{
		Results:           data,
		PageResults:       pageResults,
		PageCount:         pageCount,
		TotalTokensInput:  totalTokensInput,
		TotalTokensOutput: totalTokensOutput,
		TotalCostUSD:      totalCosts.UserCostUSD,
		TotalLLMCostUSD:   totalCosts.LLMCostUSD,
		LLMProvider:       llmCfg.Provider,
		LLMModel:          llmCfg.Model,
		StoppedEarly:      false, // Simple Crawl doesn't have mid-crawl balance check
		StopReason:        "",
	}, nil
}

// CrawlWithCallback performs a multi-page crawl extraction with callbacks for events.
// This allows incremental processing of results (e.g., saving to database as they come in)
// and tracking of URL discovery for progress reporting.
// Uses SchemaPageExtractor for each page, which handles dynamic retry for bot protection
// and insufficient content.
func (s *ExtractionService) CrawlWithCallback(ctx context.Context, userID string, input CrawlInput, callbacks CrawlCallbacks) (*CrawlResult, error) {
	// Use pre-resolved LLM configs from input (supports fallback chain)
	if len(input.LLMConfigs) == 0 {
		return nil, fmt.Errorf("no LLM configs provided")
	}
	llmConfigs := input.LLMConfigs // Full fallback chain for per-page retries
	isBYOK := input.IsBYOK

	// Get available balance for non-BYOK users (for mid-crawl balance enforcement)
	// Skip balance check if user has skip_credit_check feature enabled
	var availableBalance float64
	var checkBalance bool
	var err error
	if !isBYOK && !input.Options.SkipCreditCheck && s.billing != nil {
		checkBalance = true
		availableBalance, err = s.billing.GetAvailableBalance(ctx, userID)
		if err != nil {
			s.logger.Warn("failed to get available balance, skipping mid-crawl balance check",
				"user_id", userID,
				"error", err,
			)
			checkBalance = false
		}
	}

	// Build seed URLs list - use SeedURLs if provided (from sitemap), otherwise just the main URL
	seedURLs := input.SeedURLs
	if len(seedURLs) == 0 {
		seedURLs = []string{input.URL}
	}

	// Detect input format - prompts use a different extraction path
	inputFormat, sch, err := DetectInputFormat(input.Schema)
	if inputFormat == InputFormatPrompt {
		// Use prompt-based crawling
		return s.crawlWithPrompt(ctx, userID, input, &callbacks)
	}
	if err != nil {
		return nil, fmt.Errorf("invalid schema: %w", err)
	}

	// Enrich cleaner chain with crawl selectors as keep selectors
	// This ensures elements matching FollowSelector/NextSelector are preserved during cleaning
	enrichedCleanerChain := EnrichCleanerChainWithCrawlSelectors(
		input.CleanerChain,
		input.Options.FollowSelector,
		input.Options.NextSelector,
	)

	s.logger.Info("crawl starting",
		"job_id", input.JobID,
		"user_id", userID,
		"url", input.URL,
		"seed_count", len(seedURLs),
		"provider", llmConfigs[0].Provider,
		"model", llmConfigs[0].Model,
		"fallback_chain_size", len(llmConfigs),
	)

	// Phase 1: URL Discovery
	// If we have seed URLs (from sitemap), use those directly.
	// Otherwise, discover URLs using Colly-based URL discovery.
	var urlsToExtract []DiscoveredURL

	if len(seedURLs) > 1 {
		// Multiple seeds provided (from sitemap discovery) - use them directly
		for i, url := range seedURLs {
			urlsToExtract = append(urlsToExtract, DiscoveredURL{
				URL:       url,
				Depth:     0,
				ParentURL: "",
			})
			// Respect max pages limit
			if input.Options.MaxPages > 0 && i+1 >= input.Options.MaxPages {
				break
			}
		}
		s.logger.Info("using provided seed URLs",
			"job_id", input.JobID,
			"url_count", len(urlsToExtract),
		)
	} else if input.Options.FollowSelector != "" || input.Options.FollowPattern != "" || input.Options.NextSelector != "" {
		// Need to discover URLs - use URLDiscoverer
		discoverer := NewURLDiscoverer(s.logger)
		discovered, err := discoverer.Discover(ctx, seedURLs, URLDiscoveryOptions{
			FollowSelector: input.Options.FollowSelector,
			FollowPattern:  input.Options.FollowPattern,
			MaxPages:       input.Options.MaxPages,
			MaxDepth:       input.Options.MaxDepth,
			MaxURLs:        input.Options.MaxURLs,
			SameDomainOnly: input.Options.SameDomainOnly,
			NextSelector:   input.Options.NextSelector,
		})
		if err != nil {
			return nil, fmt.Errorf("URL discovery failed: %w", err)
		}
		urlsToExtract = discovered
		s.logger.Info("URL discovery completed",
			"job_id", input.JobID,
			"urls_discovered", len(urlsToExtract),
		)
	} else {
		// No follow selectors - just extract the seed URL
		urlsToExtract = []DiscoveredURL{{
			URL:       input.URL,
			Depth:     0,
			ParentURL: "",
		}}
	}

	// Notify about queued URLs
	if callbacks.OnURLsQueued != nil {
		callbacks.OnURLsQueued(len(urlsToExtract))
	}

	// Phase 2: Per-page extraction with LLM fallback chain
	// Each page tries the full llmConfigs chain until one succeeds or all fail.

	var (
		data              []any
		pageResults       []PageResult
		totalTokensInput  int
		totalTokensOutput int
		pageCount         int
		cumulativeCostUSD float64
		stoppedEarly      bool
		stopReason        string
		lastError         error
		lastUsedConfig    *LLMConfigInput
	)

	// Process each URL with fallback chain support
	for _, discoveredURL := range urlsToExtract {
		select {
		case <-ctx.Done():
			stoppedEarly = true
			stopReason = "context_cancelled"
		default:
		}

		if stoppedEarly {
			break
		}

		var parentURL *string
		if discoveredURL.ParentURL != "" {
			parentURL = &discoveredURL.ParentURL
		}

		// Try each LLM config in the fallback chain for this page
		var pageResult PageResult
		var pageSuccess bool

		for cfgIdx, llmCfg := range llmConfigs {
			lastUsedConfig = llmCfg

			pageResult = PageResult{
				URL:         discoveredURL.URL,
				ParentURL:   parentURL,
				Depth:       discoveredURL.Depth,
				IsBYOK:      isBYOK,
				LLMProvider: llmCfg.Provider,
				LLMModel:    llmCfg.Model,
			}

			// Create extractor for this config
			extractor := NewSchemaPageExtractor(s, sch, SchemaExtractorOptions{
				LLMConfig:             llmCfg,
				CleanerChain:          enrichedCleanerChain,
				ContentDynamicAllowed: input.Options.ContentDynamicAllowed,
				UserID:                userID,
				Tier:                  input.Tier,
				JobID:                 input.JobID,
			})

			// Extract using SchemaPageExtractor (handles dynamic retry internally)
			extractResult, err := extractor.Extract(ctx, discoveredURL.URL)

			if err != nil || (extractResult != nil && extractResult.Error != nil) {
				// Handle error
				errToUse := err
				if errToUse == nil && extractResult != nil {
					errToUse = extractResult.Error
				}
				lastError = errToUse

				errInfo := llm.WrapError(errToUse, llmCfg.Provider, llmCfg.Model, isBYOK)
				pageResult.Error = errInfo.UserMessage
				pageResult.ErrorCategory = errInfo.Category
				if isBYOK {
					pageResult.ErrorDetails = errToUse.Error()
				}
				if extractResult != nil {
					pageResult.FetchDurationMs = extractResult.FetchDurationMs
					pageResult.ExtractDurationMs = extractResult.ExtractDurationMs
					pageResult.RetryCount = extractResult.RetryCount
				}

				// Check if we should try the next model in the chain
				if errInfo.ShouldFallback && cfgIdx < len(llmConfigs)-1 {
					s.logger.Info("crawl page error, trying fallback model",
						"job_id", input.JobID,
						"url", discoveredURL.URL,
						"failed_model", llmCfg.Model,
						"next_model", llmConfigs[cfgIdx+1].Model,
						"error_category", errInfo.Category,
					)
					continue // Try next model in chain
				}

				// No more fallbacks or error not retryable
				s.logger.Warn("crawl page error",
					"job_id", input.JobID,
					"user_id", userID,
					"url", discoveredURL.URL,
					"error", errToUse,
					"model", llmCfg.Model,
					"fallback_attempted", cfgIdx > 0,
					"used_dynamic", extractResult != nil && extractResult.UsedDynamicMode,
				)
				break // Stop trying for this page
			}

			// Success!
			pageSuccess = true
			pageResult.URL = extractResult.URL
			pageResult.Data = extractResult.Data
			pageResult.TokenUsageInput = extractResult.TokensInput
			pageResult.TokenUsageOutput = extractResult.TokensOutput
			pageResult.FetchDurationMs = extractResult.FetchDurationMs
			pageResult.ExtractDurationMs = extractResult.ExtractDurationMs
			pageResult.GenerationID = extractResult.GenerationID
			pageResult.RetryCount = extractResult.RetryCount
			pageResult.RawContent = extractResult.RawContent

			data = append(data, extractResult.Data)
			totalTokensInput += extractResult.TokensInput
			totalTokensOutput += extractResult.TokensOutput
			pageCount++

			// Calculate costs
			if s.billing != nil {
				pageCosts := s.billing.CalculateCosts(ctx, CostInput{
					TokensInput:  extractResult.TokensInput,
					TokensOutput: extractResult.TokensOutput,
					Model:        llmCfg.Model,
					Provider:     llmCfg.Provider,
					Tier:         input.Tier,
					IsBYOK:       isBYOK,
					GenerationID: extractResult.GenerationID,
					APIKey:       llmCfg.APIKey,
				})
				cumulativeCostUSD += pageCosts.UserCostUSD

				// Check balance for next page
				if checkBalance && pageCosts.UserCostUSD > 0 {
					remaining := availableBalance - cumulativeCostUSD
					if remaining < pageCosts.UserCostUSD {
						s.logger.Warn("insufficient balance for next page, stopping crawl",
							"job_id", input.JobID,
							"user_id", userID,
							"available_balance", availableBalance,
							"cumulative_cost", cumulativeCostUSD,
							"remaining_balance", remaining,
							"estimated_next_page_cost", pageCosts.UserCostUSD,
							"pages_completed", pageCount,
						)
						stoppedEarly = true
						stopReason = "insufficient_balance"
					}
				}
			}

			s.logger.Info("extracted page",
				"job_id", input.JobID,
				"url", extractResult.URL,
				"input_tokens", extractResult.TokensInput,
				"output_tokens", extractResult.TokensOutput,
				"model", llmCfg.Model,
				"fallback_used", cfgIdx > 0,
				"used_dynamic", extractResult.UsedDynamicMode,
				"retry_count", extractResult.RetryCount,
			)
			break // Success - don't try more models
		}

		pageResults = append(pageResults, pageResult)

		// Call result callback (even for failed pages)
		if callbacks.OnResult != nil {
			if cbErr := callbacks.OnResult(pageResult); cbErr != nil {
				s.logger.Error("callback error, stopping crawl", "error", cbErr)
				stoppedEarly = true
				stopReason = "callback_error"
				break
			}
		}

		// Don't continue if we had no success and there was an error (unless we want partial results)
		_ = pageSuccess // Currently we continue to next page even on failure
	}

	// If no results and we have an error, return the error
	if pageCount == 0 && lastError != nil {
		return nil, s.handleLLMError(lastError, lastUsedConfig, isBYOK)
	}

	// Calculate final costs (use primary model for estimation)
	primaryConfig := llmConfigs[0]
	var totalCosts CostResult
	if s.billing != nil {
		totalCosts = s.billing.CalculateCosts(ctx, CostInput{
			TokensInput:  totalTokensInput,
			TokensOutput: totalTokensOutput,
			Model:        primaryConfig.Model,
			Provider:     primaryConfig.Provider,
			Tier:         input.Tier,
			IsBYOK:       isBYOK,
		})
	}

	s.logger.Info("crawl completed",
		"job_id", input.JobID,
		"user_id", userID,
		"url", input.URL,
		"provider", primaryConfig.Provider,
		"model", primaryConfig.Model,
		"page_count", pageCount,
		"total_input_tokens", totalTokensInput,
		"total_output_tokens", totalTokensOutput,
		"llm_cost_usd", totalCosts.LLMCostUSD,
		"user_cost_usd", totalCosts.UserCostUSD,
		"stopped_early", stoppedEarly,
		"stop_reason", stopReason,
	)

	return &CrawlResult{
		Results:           data,
		PageResults:       pageResults,
		PageCount:         pageCount,
		TotalTokensInput:  totalTokensInput,
		TotalTokensOutput: totalTokensOutput,
		TotalCostUSD:      totalCosts.UserCostUSD,
		TotalLLMCostUSD:   totalCosts.LLMCostUSD,
		LLMProvider:       primaryConfig.Provider,
		LLMModel:          primaryConfig.Model,
		StoppedEarly:      stoppedEarly,
		StopReason:        stopReason,
	}, nil
}

// crawlWithPrompt performs a multi-page extraction using a freeform prompt instead of a schema.
// This iterates through URLs (from seeds, sitemap, or single URL) and extracts each page using the prompt.
// Uses PromptPageExtractor which handles dynamic retry for bot protection and insufficient content.
func (s *ExtractionService) crawlWithPrompt(ctx context.Context, userID string, input CrawlInput, callbacks *CrawlCallbacks) (*CrawlResult, error) {
	if len(input.LLMConfigs) == 0 {
		return nil, fmt.Errorf("no LLM configs provided")
	}
	llmCfg := input.LLMConfigs[0]
	isBYOK := input.IsBYOK

	// Extract prompt text from Schema field
	promptText := strings.TrimSpace(string(input.Schema))

	s.logger.Info("prompt-based crawl starting",
		"job_id", input.JobID,
		"user_id", userID,
		"url", input.URL,
		"prompt_length", len(promptText),
		"provider", llmCfg.Provider,
		"model", llmCfg.Model,
	)

	// Build URL list - use SeedURLs if provided, otherwise just the main URL
	urls := input.SeedURLs
	if len(urls) == 0 {
		urls = []string{input.URL}
	}

	// Limit to max pages
	maxPages := input.Options.MaxPages
	if maxPages > 0 && len(urls) > maxPages {
		urls = urls[:maxPages]
	}

	// Track results
	var pageResults []PageResult
	var allData []any
	var totalTokensInput, totalTokensOutput int
	var cumulativeCostUSD float64
	var stoppedEarly bool
	var stopReason string

	// Get available balance for non-BYOK users
	// Skip balance check if user has skip_credit_check feature enabled
	var availableBalance float64
	var checkBalance bool
	if !isBYOK && !input.Options.SkipCreditCheck && s.billing != nil {
		var err error
		availableBalance, err = s.billing.GetAvailableBalance(ctx, userID)
		if err != nil {
			s.logger.Warn("failed to get balance, skipping mid-crawl balance check", "error", err)
		} else {
			checkBalance = true
		}
	}

	// Report initial URL count
	if callbacks != nil && callbacks.OnURLsQueued != nil {
		callbacks.OnURLsQueued(len(urls))
	}

	// Create PromptPageExtractor - handles dynamic retry for all pages
	extractor := NewPromptPageExtractor(s, PromptExtractorOptions{
		PromptText:            promptText,
		LLMConfig:             llmCfg,
		CleanerChain:          input.CleanerChain,
		IsBYOK:                isBYOK,
		ContentDynamicAllowed: input.Options.ContentDynamicAllowed,
		UserID:                userID,
		Tier:                  input.Tier,
		JobID:                 input.JobID,
	})

	// Process each URL using the extractor (gets dynamic retry for free!)
	for i, pageURL := range urls {
		select {
		case <-ctx.Done():
			stoppedEarly = true
			stopReason = "context_cancelled"
		default:
		}

		if stoppedEarly {
			break
		}

		pageResult := PageResult{
			URL:         pageURL,
			Depth:       0, // All seed URLs are depth 0
			IsBYOK:      isBYOK,
			LLMProvider: llmCfg.Provider,
			LLMModel:    llmCfg.Model,
		}

		// Extract using PromptPageExtractor (handles dynamic retry internally)
		extractResult, err := extractor.Extract(ctx, pageURL)

		if err != nil || (extractResult != nil && extractResult.Error != nil) {
			// Handle error
			errToUse := err
			if errToUse == nil && extractResult != nil {
				errToUse = extractResult.Error
			}

			errInfo := llm.WrapError(errToUse, llmCfg.Provider, llmCfg.Model, isBYOK)
			pageResult.Error = errInfo.UserMessage
			pageResult.ErrorCategory = errInfo.Category
			if isBYOK {
				pageResult.ErrorDetails = errToUse.Error()
			}
			if extractResult != nil {
				pageResult.FetchDurationMs = extractResult.FetchDurationMs
				pageResult.ExtractDurationMs = extractResult.ExtractDurationMs
				pageResult.RetryCount = extractResult.RetryCount
			}
			s.logger.Warn("prompt crawl extraction error",
				"job_id", input.JobID,
				"url", pageURL,
				"error", errToUse,
				"used_dynamic", extractResult != nil && extractResult.UsedDynamicMode,
			)
		} else {
			// Success
			pageResult.URL = extractResult.URL
			pageResult.Data = extractResult.Data
			pageResult.TokenUsageInput = extractResult.TokensInput
			pageResult.TokenUsageOutput = extractResult.TokensOutput
			pageResult.FetchDurationMs = extractResult.FetchDurationMs
			pageResult.ExtractDurationMs = extractResult.ExtractDurationMs
			pageResult.RetryCount = extractResult.RetryCount
			pageResult.RawContent = extractResult.RawContent

			totalTokensInput += extractResult.TokensInput
			totalTokensOutput += extractResult.TokensOutput
			allData = append(allData, extractResult.Data)

			// Calculate costs
			if s.billing != nil {
				pageCosts := s.billing.CalculateCosts(ctx, CostInput{
					TokensInput:  extractResult.TokensInput,
					TokensOutput: extractResult.TokensOutput,
					Model:        llmCfg.Model,
					Provider:     llmCfg.Provider,
					Tier:         input.Tier,
					IsBYOK:       isBYOK,
				})
				cumulativeCostUSD += pageCosts.UserCostUSD

				// Check balance for next page
				if checkBalance && pageCosts.UserCostUSD > 0 {
					remaining := availableBalance - cumulativeCostUSD
					if remaining < pageCosts.UserCostUSD {
						s.logger.Warn("insufficient balance for next page",
							"job_id", input.JobID,
							"remaining", remaining,
							"pages_completed", i+1,
						)
						stoppedEarly = true
						stopReason = "insufficient_balance"
					}
				}
			}

			s.logger.Info("extracted page",
				"job_id", input.JobID,
				"url", extractResult.URL,
				"input_tokens", extractResult.TokensInput,
				"output_tokens", extractResult.TokensOutput,
				"used_dynamic", extractResult.UsedDynamicMode,
				"retry_count", extractResult.RetryCount,
			)
		}

		pageResults = append(pageResults, pageResult)

		// Call result callback
		if callbacks != nil && callbacks.OnResult != nil {
			if cbErr := callbacks.OnResult(pageResult); cbErr != nil {
				s.logger.Error("callback error, stopping crawl", "error", cbErr)
				stoppedEarly = true
				stopReason = "callback_error"
				break
			}
		}
	}

	// Calculate final costs
	var totalCosts CostResult
	if s.billing != nil {
		totalCosts = s.billing.CalculateCosts(ctx, CostInput{
			TokensInput:  totalTokensInput,
			TokensOutput: totalTokensOutput,
			Model:        llmCfg.Model,
			Provider:     llmCfg.Provider,
			Tier:         input.Tier,
			IsBYOK:       isBYOK,
		})
	}

	s.logger.Info("prompt crawl completed",
		"job_id", input.JobID,
		"user_id", userID,
		"page_count", len(pageResults),
		"total_input_tokens", totalTokensInput,
		"total_output_tokens", totalTokensOutput,
		"llm_cost_usd", totalCosts.LLMCostUSD,
		"user_cost_usd", totalCosts.UserCostUSD,
		"stopped_early", stoppedEarly,
		"stop_reason", stopReason,
	)

	return &CrawlResult{
		Results:           allData,
		PageResults:       pageResults,
		PageCount:         len(pageResults),
		TotalTokensInput:  totalTokensInput,
		TotalTokensOutput: totalTokensOutput,
		TotalCostUSD:      totalCosts.UserCostUSD,
		TotalLLMCostUSD:   totalCosts.LLMCostUSD,
		LLMProvider:       llmCfg.Provider,
		LLMModel:          llmCfg.Model,
		StoppedEarly:      stoppedEarly,
		StopReason:        stopReason,
	}, nil
}
