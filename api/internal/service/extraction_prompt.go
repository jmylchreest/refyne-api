package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jmylchreest/refyne-api/internal/constants"
	"github.com/jmylchreest/refyne-api/internal/llm"
	"github.com/jmylchreest/refyne-api/internal/models"
)

// extractWithPrompt performs extraction using natural language instructions instead of a schema.
// This allows users to describe what they want extracted in plain text.
func (s *ExtractionService) extractWithPrompt(ctx context.Context, userID string, input ExtractInput, ectx *ExtractContext, startTime time.Time) (*ExtractOutput, error) {
	// Extract prompt text from Schema field (which contains the freeform text)
	promptText := strings.TrimSpace(string(input.Schema))

	s.logger.Info("starting prompt-based extraction",
		"user_id", userID,
		"url", input.URL,
		"prompt_length", len(promptText),
	)

	// Get LLM configs (same as schema-based extraction)
	llmConfigs, isBYOK := s.resolveLLMConfigsWithFallback(ctx, userID, input.LLMConfig, ectx.Tier, ectx.BYOKAllowed, ectx.ModelsCustomAllowed, ectx.LLMProvider, ectx.LLMModel)
	ectx.IsBYOK = isBYOK

	if len(llmConfigs) == 0 {
		return nil, fmt.Errorf("no LLM providers configured")
	}

	// Estimate cost and check balance for non-BYOK
	if s.billing != nil && len(llmConfigs) > 0 && !isBYOK {
		estimatedCost := s.billing.EstimateCost(1, llmConfigs[0].Model, llmConfigs[0].Provider)
		if err := s.billing.CheckSufficientBalance(ctx, userID, estimatedCost); err != nil {
			return nil, err
		}
	}

	// Fetch and clean page content
	fetchStart := time.Now()
	pageContent, fetchedURL, err := s.fetchAndCleanContent(ctx, input.URL, input.FetchMode, input.CleanerChain)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch page content: %w", err)
	}
	fetchDuration := time.Since(fetchStart)

	// Truncate content if too long (similar to analyzer service)
	maxContentLen := 100000 // ~25k tokens roughly
	if len(pageContent) > maxContentLen {
		pageContent = pageContent[:maxContentLen] + "\n\n[Content truncated...]"
	}

	// Build the extraction prompt
	extractPrompt := s.buildPromptExtractionPrompt(pageContent, promptText)

	// Try each LLM config until one succeeds
	var lastErr error
	var lastLLMErr *llm.LLMError
	var lastCfg *LLMConfigInput

	for providerIdx, llmCfg := range llmConfigs {
		lastCfg = llmCfg

		s.logger.Info("prompt extraction attempt",
			"user_id", userID,
			"url", input.URL,
			"provider", llmCfg.Provider,
			"model", llmCfg.Model,
			"provider_idx", providerIdx+1,
			"of_providers", len(llmConfigs),
		)

		// Call LLM using shared client
		llmClient := NewLLMClient(s.logger)
		result, err := llmClient.Call(ctx, llmCfg, extractPrompt, LLMCallOptions{
			Temperature: 0.1,  // Low temperature for extraction
			MaxTokens:   8192,
			Timeout:     180 * time.Second,
			JSONMode:    true, // Request JSON response
		})
		if err == nil && result != nil {
			// Success - parse response and return
			extractDuration := time.Since(startTime) - fetchDuration

			// Parse the LLM response as JSON
			var extractedData any
			if err := json.Unmarshal([]byte(result.Content), &extractedData); err != nil {
				// If JSON parsing fails, return the raw content wrapped
				extractedData = map[string]any{
					"raw_response": result.Content,
					"parse_error":  "Response was not valid JSON",
				}
			}

			// Calculate costs and record usage
			var costs CostResult
			if s.billing != nil {
				costs = s.billing.CalculateCosts(ctx, CostInput{
					TokensInput:  result.InputTokens,
					TokensOutput: result.OutputTokens,
					Model:        llmCfg.Model,
					Provider:     llmCfg.Provider,
					Tier:         ectx.Tier,
					IsBYOK:       isBYOK,
				})
				_ = s.billing.RecordUsage(ctx, &UsageRecord{
					UserID:          userID,
					JobType:         models.JobTypeExtract,
					Status:          "completed",
					TokensInput:     result.InputTokens,
					TokensOutput:    result.OutputTokens,
					TotalChargedUSD: costs.UserCostUSD,
					LLMCostUSD:      costs.LLMCostUSD,
					IsBYOK:          isBYOK,
					TargetURL:       input.URL,
					LLMProvider:     llmCfg.Provider,
					LLMModel:        llmCfg.Model,
				})
			}

			return &ExtractOutput{
				Data:        extractedData,
				URL:         fetchedURL,
				FetchedAt:   startTime,
				InputFormat: InputFormatPrompt,
				Usage: UsageInfo{
					InputTokens:  result.InputTokens,
					OutputTokens: result.OutputTokens,
					CostUSD:      costs.UserCostUSD,
					LLMCostUSD:   costs.LLMCostUSD,
					IsBYOK:       isBYOK,
				},
				Metadata: ExtractMeta{
					FetchDurationMs:   int(fetchDuration.Milliseconds()),
					ExtractDurationMs: int(extractDuration.Milliseconds()),
					Model:             llmCfg.Model,
					Provider:          llmCfg.Provider,
				},
				RawContent: pageContent,
			}, nil
		}

		// Failed - try next provider
		lastErr = err
		lastLLMErr = llm.WrapError(err, llmCfg.Provider, llmCfg.Model, isBYOK)

		s.logger.Warn("prompt extraction failed",
			"provider", llmCfg.Provider,
			"model", llmCfg.Model,
			"error", err,
		)

		if lastLLMErr != nil && !lastLLMErr.ShouldFallback {
			break
		}

		time.Sleep(constants.ProviderFallbackDelay)
	}

	// All attempts failed
	if lastErr != nil {
		return nil, s.handleLLMError(lastErr, lastCfg, isBYOK)
	}

	return nil, fmt.Errorf("prompt extraction failed: no LLM providers configured")
}

// buildPromptExtractionPrompt constructs the LLM prompt for freeform extraction.
func (s *ExtractionService) buildPromptExtractionPrompt(pageContent, userPrompt string) string {
	return fmt.Sprintf(`You are a data extraction assistant. Extract structured data from the provided web page content based on the user's instructions.

USER'S EXTRACTION INSTRUCTIONS:
%s

WEB PAGE CONTENT:
%s

IMPORTANT INSTRUCTIONS:
1. Extract ONLY the data requested by the user
2. Return your response as valid JSON
3. Use appropriate data types (strings, numbers, arrays, objects) based on the content
4. If requested data is not found, use null for that field
5. For lists/arrays, include all matching items found
6. Be thorough but only include what was explicitly requested

Respond with ONLY valid JSON, no markdown formatting or explanation.`, userPrompt, pageContent)
}

// fetchAndCleanContent fetches a URL and cleans the content using the configured cleaner chain.
func (s *ExtractionService) fetchAndCleanContent(ctx context.Context, targetURL, fetchMode string, cleanerChain []CleanerConfig) (string, string, error) {
	// Create cleaner chain
	factory := NewCleanerFactory()
	contentCleaner, err := factory.CreateChainWithDefault(cleanerChain, DefaultExtractionCleanerChain)
	if err != nil {
		return "", "", fmt.Errorf("invalid cleaner chain: %w", err)
	}

	// Fetch using HTTP client (similar to analyzer service approach)
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

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("failed to read response: %w", err)
	}

	// Clean the content
	cleanedContent, err := contentCleaner.Clean(string(body))
	if err != nil {
		// If cleaning fails, use raw content
		s.logger.Warn("content cleaning failed, using raw HTML", "error", err)
		cleanedContent = string(body)
	}

	// Get final URL (in case of redirects)
	finalURL := resp.Request.URL.String()

	return cleanedContent, finalURL, nil
}

