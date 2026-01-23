package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jmylchreest/refyne/pkg/refyne"

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
func (s *ExtractionService) Crawl(ctx context.Context, userID string, input CrawlInput) (*CrawlResult, error) {
	// Use pre-resolved LLM configs from input
	if len(input.LLMConfigs) == 0 {
		return nil, fmt.Errorf("no LLM configs provided")
	}
	llmCfg := input.LLMConfigs[0]
	isBYOK := input.IsBYOK

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

	// Create refyne instance with configured cleaner
	r, cleanerName, err := s.createRefyneInstance(llmCfg, enrichedCleanerChain)
	if err != nil {
		return nil, s.handleLLMError(err, llmCfg, isBYOK)
	}
	defer func() { _ = r.Close() }()

	s.logger.Info("crawl starting",
		"job_id", input.JobID,
		"user_id", userID,
		"url", input.URL,
		"cleaner", cleanerName,
		"provider", llmCfg.Provider,
		"model", llmCfg.Model,
	)

	// Build crawl options
	crawlOpts := s.buildCrawlOptions(input.Options)

	// Perform crawl - Crawl returns a channel of results
	results := r.Crawl(ctx, input.URL, sch, crawlOpts...)

	// Aggregate results
	// Track URLs we've seen to determine depth (seed URL = depth 0, others = depth 1+)
	seedURL := input.URL
	seenURLs := make(map[string]int) // URL -> depth
	seenURLs[seedURL] = 0

	var (
		data              []any
		pageResults       []PageResult
		totalTokensInput  int
		totalTokensOutput int
		pageCount         int
		lastError         error
	)

	for result := range results {
		// Determine depth and parent URL based on whether this is the seed
		var parentURL *string
		depth := 0
		if result.URL != seedURL {
			// Non-seed URL - set parent to seed and depth based on discovery order
			parentURL = &seedURL
			// If we haven't seen this URL yet, assign next depth level
			if existingDepth, seen := seenURLs[result.URL]; seen {
				depth = existingDepth
			} else {
				// URLs discovered during crawl get depth = 1 (directly linked from seed)
				// Note: refyne doesn't expose actual depth, so we approximate
				depth = 1
				seenURLs[result.URL] = depth
			}
		}

		if result.Error != nil {
			// Log with job/user/model/provider/cleaner info for debugging
			s.logger.Warn("crawl page error",
				"job_id", input.JobID,
				"user_id", userID,
				"url", result.URL,
				"cleaner", cleanerName,
				"provider", llmCfg.Provider,
				"model", llmCfg.Model,
				"error", result.Error,
			)
			lastError = result.Error

			// Wrap error for user-friendly messaging
			llmErr := llm.WrapError(result.Error, llmCfg.Provider, llmCfg.Model, isBYOK)
			userMsg := result.Error.Error()
			category := "unknown"
			if llmErr != nil {
				userMsg = llmErr.UserMessage
				category = llmErr.Category
			}
			// Still track failed pages for SSE streaming and crawl map
			pageResults = append(pageResults, PageResult{
				URL:           result.URL,
				ParentURL:     parentURL,
				Depth:         depth,
				Error:         userMsg,
				ErrorDetails:  result.Error.Error(), // Full error for BYOK/admin
				ErrorCategory: category,
				LLMProvider:   llmCfg.Provider,
				LLMModel:      llmCfg.Model,
				IsBYOK:        isBYOK,
			})
			continue
		}

		// Calculate per-page cost for logging - use actual cost from provider when available
		var pageCosts CostResult
		if s.billing != nil {
			pageCosts = s.billing.CalculateCosts(ctx, CostInput{
				TokensInput:  result.TokenUsage.InputTokens,
				TokensOutput: result.TokenUsage.OutputTokens,
				Model:        llmCfg.Model,
				Provider:     llmCfg.Provider,
				Tier:         input.Tier,
				IsBYOK:       isBYOK,
				GenerationID: result.GenerationID,
				APIKey:       llmCfg.APIKey,
			})
		}

		// Log successful extraction
		s.logger.Info("extracted",
			"job_id", input.JobID,
			"user_id", userID,
			"url", result.URL,
			"cleaner", cleanerName,
			"provider", llmCfg.Provider,
			"model", llmCfg.Model,
			"fetch_ms", result.FetchDuration.Milliseconds(),
			"extract_ms", result.ExtractDuration.Milliseconds(),
			"input_tokens", result.TokenUsage.InputTokens,
			"output_tokens", result.TokenUsage.OutputTokens,
			"llm_cost_usd", pageCosts.LLMCostUSD,
			"user_cost_usd", pageCosts.UserCostUSD,
			"is_byok", isBYOK,
		)

		// Apply shared post-extraction processing (URL resolution, etc.)
		processedData := s.processExtractionResult(result.Data, result.URL)

		data = append(data, processedData)
		pageResults = append(pageResults, PageResult{
			URL:               result.URL,
			ParentURL:         parentURL,
			Depth:             depth,
			Data:              processedData,
			TokenUsageInput:   result.TokenUsage.InputTokens,
			TokenUsageOutput:  result.TokenUsage.OutputTokens,
			FetchDurationMs:   int(result.FetchDuration.Milliseconds()),
			ExtractDurationMs: int(result.ExtractDuration.Milliseconds()),
			LLMProvider:       result.Provider,
			LLMModel:          result.Model,
			RawContent:        result.RawContent,
		})
		totalTokensInput += result.TokenUsage.InputTokens
		totalTokensOutput += result.TokenUsage.OutputTokens
		pageCount++
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
		"cleaner", cleanerName,
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
func (s *ExtractionService) CrawlWithCallback(ctx context.Context, userID string, input CrawlInput, callbacks CrawlCallbacks) (*CrawlResult, error) {
	// Use pre-resolved LLM configs from input
	if len(input.LLMConfigs) == 0 {
		return nil, fmt.Errorf("no LLM configs provided")
	}
	llmCfg := input.LLMConfigs[0]
	isBYOK := input.IsBYOK

	// Get available balance for non-BYOK users (for mid-crawl balance enforcement)
	var availableBalance float64
	var checkBalance bool
	var err error
	if !isBYOK && s.billing != nil {
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

	// Create a cancellable context so we can stop the crawler if balance is exhausted
	crawlCtx, cancelCrawl := context.WithCancel(ctx)
	defer cancelCrawl()

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

	// Create refyne instance with configured cleaner
	r, cleanerName, err := s.createRefyneInstance(llmCfg, enrichedCleanerChain)
	if err != nil {
		return nil, s.handleLLMError(err, llmCfg, isBYOK)
	}
	defer func() { _ = r.Close() }()

	s.logger.Info("crawl starting",
		"job_id", input.JobID,
		"user_id", userID,
		"url", input.URL,
		"seed_count", len(seedURLs),
		"cleaner", cleanerName,
		"provider", llmCfg.Provider,
		"model", llmCfg.Model,
	)

	// Build crawl options
	crawlOpts := s.buildCrawlOptions(input.Options)
	crawlOpts = s.addURLsQueuedCallback(crawlOpts, callbacks.OnURLsQueued)

	s.logger.Debug("initiating crawler",
		"job_id", input.JobID,
		"seed_count", len(seedURLs),
		"options", fmt.Sprintf("%+v", input.Options),
	)

	// Perform crawl - use CrawlMany for multiple seeds (from sitemap), single Crawl otherwise
	// Use crawlCtx so we can cancel if balance is exhausted
	var results <-chan *refyne.Result
	if len(seedURLs) > 1 {
		results = r.CrawlMany(crawlCtx, seedURLs, sch, crawlOpts...)
	} else {
		results = r.Crawl(crawlCtx, input.URL, sch, crawlOpts...)
	}

	s.logger.Debug("crawler channel ready, waiting for results",
		"job_id", input.JobID,
	)

	// Aggregate results while calling callback for each
	seedURL := input.URL
	seenURLs := make(map[string]int)
	for _, url := range seedURLs {
		seenURLs[url] = 0 // All seed URLs start at depth 0
	}

	var (
		data              []any
		pageResults       []PageResult
		totalTokensInput  int
		totalTokensOutput int
		pageCount         int
		lastError         error
		cumulativeCostUSD float64 // Running total of costs (for balance tracking)
		stoppedEarly      bool    // True if we stopped due to balance or other limit
		stopReason        string  // Reason for early stop
	)

	for result := range results {
		// Determine depth and parent URL based on whether this is the seed
		var parentURL *string
		depth := 0
		if result.URL != seedURL {
			parentURL = &seedURL
			if existingDepth, seen := seenURLs[result.URL]; seen {
				depth = existingDepth
			} else {
				depth = 1
				seenURLs[result.URL] = depth
			}
		}

		var pageResult PageResult
		if result.Error != nil {
			// Log with job/user/model/provider/cleaner info for debugging
			s.logger.Warn("crawl page error",
				"job_id", input.JobID,
				"user_id", userID,
				"url", result.URL,
				"cleaner", cleanerName,
				"provider", llmCfg.Provider,
				"model", llmCfg.Model,
				"error", result.Error,
			)
			lastError = result.Error

			// Wrap error for user-friendly messaging
			llmErr := llm.WrapError(result.Error, llmCfg.Provider, llmCfg.Model, isBYOK)
			userMsg := result.Error.Error()
			category := "unknown"
			if llmErr != nil {
				userMsg = llmErr.UserMessage
				category = llmErr.Category
			}
			pageResult = PageResult{
				URL:           result.URL,
				ParentURL:     parentURL,
				Depth:         depth,
				Error:         userMsg,
				ErrorDetails:  result.Error.Error(), // Full error for BYOK/admin
				ErrorCategory: category,
				LLMProvider:   llmCfg.Provider,
				LLMModel:      llmCfg.Model,
				IsBYOK:        isBYOK,
			}
		} else {
			// Calculate per-page cost for logging - use actual cost from provider when available
			var pageCosts CostResult
			if s.billing != nil {
				pageCosts = s.billing.CalculateCosts(ctx, CostInput{
					TokensInput:  result.TokenUsage.InputTokens,
					TokensOutput: result.TokenUsage.OutputTokens,
					Model:        llmCfg.Model,
					Provider:     llmCfg.Provider,
					Tier:         input.Tier,
					IsBYOK:       isBYOK,
					GenerationID: result.GenerationID,
					APIKey:       llmCfg.APIKey,
				})
			}

			// Track cumulative cost for balance enforcement
			cumulativeCostUSD += pageCosts.UserCostUSD

			// Log successful extraction
			s.logger.Info("extracted",
				"job_id", input.JobID,
				"user_id", userID,
				"url", result.URL,
				"cleaner", cleanerName,
				"provider", llmCfg.Provider,
				"model", llmCfg.Model,
				"fetch_ms", result.FetchDuration.Milliseconds(),
				"extract_ms", result.ExtractDuration.Milliseconds(),
				"input_tokens", result.TokenUsage.InputTokens,
				"output_tokens", result.TokenUsage.OutputTokens,
				"llm_cost_usd", pageCosts.LLMCostUSD,
				"user_cost_usd", pageCosts.UserCostUSD,
				"cumulative_cost_usd", cumulativeCostUSD,
				"is_byok", isBYOK,
			)

			// Apply shared post-extraction processing (URL resolution, etc.)
			processedData := s.processExtractionResult(result.Data, result.URL)

			data = append(data, processedData)
			pageResult = PageResult{
				URL:               result.URL,
				ParentURL:         parentURL,
				Depth:             depth,
				Data:              processedData,
				TokenUsageInput:   result.TokenUsage.InputTokens,
				TokenUsageOutput:  result.TokenUsage.OutputTokens,
				FetchDurationMs:   int(result.FetchDuration.Milliseconds()),
				ExtractDurationMs: int(result.ExtractDuration.Milliseconds()),
				LLMProvider:       result.Provider,
				LLMModel:          result.Model,
				GenerationID:      result.GenerationID,
				RawContent:        result.RawContent,
			}
			totalTokensInput += result.TokenUsage.InputTokens
			totalTokensOutput += result.TokenUsage.OutputTokens
			pageCount++

			// Check if we have enough balance for the next page (non-BYOK only)
			// Use the cost of this page as estimate for the next page
			if checkBalance && pageCosts.UserCostUSD > 0 {
				remainingBalance := availableBalance - cumulativeCostUSD
				if remainingBalance < pageCosts.UserCostUSD {
					s.logger.Warn("insufficient balance for next page, stopping crawl",
						"job_id", input.JobID,
						"user_id", userID,
						"available_balance", availableBalance,
						"cumulative_cost", cumulativeCostUSD,
						"remaining_balance", remainingBalance,
						"estimated_next_page_cost", pageCosts.UserCostUSD,
						"pages_completed", pageCount,
					)
					stoppedEarly = true
					stopReason = "insufficient_balance"
					cancelCrawl() // Stop the crawler
					// Continue processing to drain any remaining results from the channel
				}
			}
		}

		pageResults = append(pageResults, pageResult)

		// Call the result callback
		if callbacks.OnResult != nil {
			if err := callbacks.OnResult(pageResult); err != nil {
				s.logger.Error("callback error, stopping crawl", "error", err)
				stoppedEarly = true
				stopReason = "callback_error"
				break
			}
		}
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
		"cleaner", cleanerName,
		"provider", llmCfg.Provider,
		"model", llmCfg.Model,
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
		LLMProvider:       llmCfg.Provider,
		LLMModel:          llmCfg.Model,
		StoppedEarly:      stoppedEarly,
		StopReason:        stopReason,
	}, nil
}

// addURLsQueuedCallback adds the OnURLsQueued callback to crawl options.
func (s *ExtractionService) addURLsQueuedCallback(opts []refyne.CrawlOption, callback URLsQueuedCallback) []refyne.CrawlOption {
	if callback == nil {
		return opts
	}
	return append(opts, refyne.WithOnURLsQueued(callback))
}

// buildCrawlOptions converts CrawlOptions to refyne.CrawlOption slice.
func (s *ExtractionService) buildCrawlOptions(opts CrawlOptions) []refyne.CrawlOption {
	crawlOpts := []refyne.CrawlOption{}

	if opts.FollowSelector != "" {
		crawlOpts = append(crawlOpts, refyne.WithFollowSelector(opts.FollowSelector))
	}
	if opts.FollowPattern != "" {
		crawlOpts = append(crawlOpts, refyne.WithFollowPattern(opts.FollowPattern))
	}
	if opts.MaxDepth > 0 {
		crawlOpts = append(crawlOpts, refyne.WithMaxDepth(opts.MaxDepth))
	}
	if opts.NextSelector != "" {
		crawlOpts = append(crawlOpts, refyne.WithNextSelector(opts.NextSelector))
	}
	if opts.MaxPages > 0 {
		crawlOpts = append(crawlOpts, refyne.WithMaxPages(opts.MaxPages))
	}
	if opts.MaxURLs > 0 {
		crawlOpts = append(crawlOpts, refyne.WithMaxURLs(opts.MaxURLs))
	}
	if opts.Delay != "" {
		if d, err := time.ParseDuration(opts.Delay); err == nil {
			crawlOpts = append(crawlOpts, refyne.WithDelay(d))
		}
	}
	if opts.Concurrency > 0 {
		crawlOpts = append(crawlOpts, refyne.WithConcurrency(opts.Concurrency))
	}
	if opts.SameDomainOnly {
		crawlOpts = append(crawlOpts, refyne.WithSameDomainOnly(true))
	}
	if opts.ExtractFromSeeds {
		crawlOpts = append(crawlOpts, refyne.WithExtractFromSeeds(true))
	}

	return crawlOpts
}

// crawlWithPrompt performs a multi-page extraction using a freeform prompt instead of a schema.
// This iterates through URLs (from seeds, sitemap, or single URL) and extracts each page using the prompt.
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
	var availableBalance float64
	var checkBalance bool
	if !isBYOK && s.billing != nil {
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

	// Process each URL
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

		pageStart := time.Now()
		pageResult := PageResult{
			URL:         pageURL,
			Depth:       0, // All seed URLs are depth 0
			IsBYOK:      isBYOK,
			LLMProvider: llmCfg.Provider,
			LLMModel:    llmCfg.Model,
		}

		// Fetch and clean content
		fetchStart := time.Now()
		pageContent, fetchedURL, err := s.fetchAndCleanContent(ctx, pageURL, "", input.CleanerChain)
		fetchDuration := time.Since(fetchStart)
		pageResult.FetchDurationMs = int(fetchDuration.Milliseconds())

		if err != nil {
			pageResult.Error = fmt.Sprintf("failed to fetch page: %v", err)
			pageResult.ErrorCategory = "fetch_error"
			s.logger.Warn("prompt crawl fetch error",
				"job_id", input.JobID,
				"url", pageURL,
				"error", err,
			)
		} else {
			// Truncate content if too long
			maxContentLen := 100000
			if len(pageContent) > maxContentLen {
				pageContent = pageContent[:maxContentLen] + "\n\n[Content truncated...]"
			}

			// Build and call LLM with prompt
			extractPrompt := s.buildPromptExtractionPrompt(pageContent, promptText)
			llmClient := NewLLMClient(s.logger)
			result, llmErr := llmClient.Call(ctx, llmCfg, extractPrompt, LLMCallOptions{
				Temperature: 0.1,
				MaxTokens:   8192,
				Timeout:     180 * time.Second,
				JSONMode:    true,
			})

			extractDuration := time.Since(pageStart) - fetchDuration
			pageResult.ExtractDurationMs = int(extractDuration.Milliseconds())
			pageResult.URL = fetchedURL // Use final URL after redirects

			if llmErr != nil {
				errInfo := llm.WrapError(llmErr, llmCfg.Provider, llmCfg.Model, isBYOK)
				pageResult.Error = errInfo.UserMessage
				pageResult.ErrorCategory = errInfo.Category
				if isBYOK {
					pageResult.ErrorDetails = llmErr.Error()
				}
				s.logger.Warn("prompt crawl extraction error",
					"job_id", input.JobID,
					"url", pageURL,
					"error", llmErr,
				)
			} else {
				// Parse response
				var extractedData any
				if jsonErr := json.Unmarshal([]byte(result.Content), &extractedData); jsonErr != nil {
					extractedData = map[string]any{
						"raw_response": result.Content,
						"parse_error":  "Response was not valid JSON",
					}
				}

				pageResult.Data = extractedData
				pageResult.TokenUsageInput = result.InputTokens
				pageResult.TokenUsageOutput = result.OutputTokens

				totalTokensInput += result.InputTokens
				totalTokensOutput += result.OutputTokens
				allData = append(allData, extractedData)

				// Calculate costs
				if s.billing != nil {
					pageCosts := s.billing.CalculateCosts(ctx, CostInput{
						TokensInput:  result.InputTokens,
						TokensOutput: result.OutputTokens,
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
			}
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
