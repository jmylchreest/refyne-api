package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jmylchreest/refyne-api/internal/constants"
	"github.com/jmylchreest/refyne-api/internal/llm"
	"github.com/jmylchreest/refyne-api/internal/models"
)

// extractWithPrompt performs extraction using natural language instructions instead of a schema.
// This allows users to describe what they want extracted in plain text.
// Uses PromptPageExtractor which handles dynamic retry for bot protection and insufficient content.
func (s *ExtractionService) extractWithPrompt(ctx context.Context, userID string, input ExtractInput, ectx *ExtractContext, startTime time.Time) (*ExtractOutput, error) {
	// Extract prompt text from Schema field (which contains the freeform text)
	promptText := strings.TrimSpace(string(input.Schema))

	s.logger.Info("starting prompt-based extraction",
		"user_id", userID,
		"url", input.URL,
		"prompt_length", len(promptText),
	)

	// Get LLM config chain (same as schema-based extraction)
	llmChain := s.resolveLLMConfigChain(ctx, userID, input.LLMConfig, ectx.Tier, ectx.BYOKAllowed, ectx.ModelsCustomAllowed, ectx.LLMProvider, ectx.LLMModel, ectx.LLMConfigs)
	ectx.IsBYOK = llmChain.IsBYOK()

	if llmChain.IsEmpty() {
		return nil, llm.NewNoModelsConfiguredError("no models in fallback chain or missing API keys")
	}

	// For models_premium users, get available balance for per-model budget checking
	var availableBudget float64
	var budgetSkips []BudgetSkip
	useBudgetFallback := !llmChain.IsBYOK() && ectx.ModelsPremiumAllowed && s.billing != nil

	firstCfg := llmChain.First()
	if useBudgetFallback {
		var err error
		availableBudget, err = s.billing.GetAvailableBalance(ctx, userID)
		if err != nil {
			s.logger.Warn("failed to get available balance, disabling budget fallback",
				"user_id", userID,
				"error", err,
			)
			useBudgetFallback = false
		}
	} else if s.billing != nil && firstCfg != nil && !llmChain.IsBYOK() {
		// Standard pre-flight balance check for non-premium users
		estimatedCost := s.billing.EstimateCost(1, firstCfg.Model, firstCfg.Provider)
		if err := s.billing.CheckSufficientBalance(ctx, userID, ectx.SkipCreditCheckAllowed, estimatedCost); err != nil {
			return nil, err
		}
	}

	// Use JobID if available, otherwise fall back to SchemaID for tracking
	jobIDForTracking := ectx.JobID
	if jobIDForTracking == "" {
		jobIDForTracking = ectx.SchemaID
	}

	// Try each LLM config until one succeeds
	var lastErr error
	var lastLLMErr *llm.LLMError
	var lastCfg *LLMConfigInput
	modelsSkippedDueToBudget := 0

	for llmCfg := llmChain.Next(); llmCfg != nil; llmCfg = llmChain.Next() {
		lastCfg = llmCfg
		pos, total := llmChain.Position()

		// Budget-based fallback: check if this model is affordable
		if useBudgetFallback {
			estimatedCost := s.billing.EstimateCost(1, llmCfg.Model, llmCfg.Provider)

			if estimatedCost > availableBudget {
				// Skip this model - too expensive for remaining budget
				skip := BudgetSkip{
					Provider:        llmCfg.Provider,
					Model:           llmCfg.Model,
					EstimatedCost:   estimatedCost,
					AvailableBudget: availableBudget,
					Reason:          "estimated_cost_exceeds_budget",
				}
				budgetSkips = append(budgetSkips, skip)
				modelsSkippedDueToBudget++

				s.logger.Info("skipping model due to budget constraint",
					"user_id", userID,
					"provider", llmCfg.Provider,
					"model", llmCfg.Model,
					"estimated_cost_usd", estimatedCost,
					"available_budget_usd", availableBudget,
				)
				continue
			}
		}

		s.logger.Info("prompt extraction attempt",
			"user_id", userID,
			"url", input.URL,
			"provider", llmCfg.Provider,
			"model", llmCfg.Model,
			"attempt", pos,
			"of", total,
		)

		// Create page extractor - handles dynamic retry internally
		extractor := NewPromptPageExtractor(s, PromptExtractorOptions{
			PromptText:            promptText,
			LLMConfig:             llmCfg,
			CleanerChain:          input.CleanerChain,
			IsBYOK:                llmChain.IsBYOK(),
			ContentDynamicAllowed: ectx.ContentDynamicAllowed,
			UserID:                userID,
			Tier:                  ectx.Tier,
			JobID:                 jobIDForTracking,
		})

		// Perform extraction (dynamic retry happens inside Extract)
		pageResult, err := extractor.Extract(ctx, input.URL)

		if err == nil && pageResult != nil && pageResult.Error == nil {
			// Success - calculate costs and return

			// Calculate costs and record usage
			var costs CostResult
			if s.billing != nil {
				costs = s.billing.CalculateCosts(ctx, CostInput{
					TokensInput:  pageResult.TokensInput,
					TokensOutput: pageResult.TokensOutput,
					Model:        llmCfg.Model,
					Provider:     llmCfg.Provider,
					Tier:         ectx.Tier,
					IsBYOK:       llmChain.IsBYOK(),
				})
				_ = s.billing.RecordUsage(ctx, &UsageRecord{
					UserID:          userID,
					JobType:         models.JobTypeExtract,
					Status:          "completed",
					TokensInput:     pageResult.TokensInput,
					TokensOutput:    pageResult.TokensOutput,
					TotalChargedUSD: costs.UserCostUSD,
					LLMCostUSD:      costs.LLMCostUSD,
					IsBYOK:          llmChain.IsBYOK(),
					TargetURL:       input.URL,
					LLMProvider:     llmCfg.Provider,
					LLMModel:        llmCfg.Model,
				})
			}

			return &ExtractOutput{
				Data:        pageResult.Data,
				URL:         pageResult.URL,
				FetchedAt:   startTime,
				InputFormat: InputFormatPrompt,
				Usage: UsageInfo{
					InputTokens:  pageResult.TokensInput,
					OutputTokens: pageResult.TokensOutput,
					CostUSD:      costs.UserCostUSD,
					LLMCostUSD:   costs.LLMCostUSD,
					IsBYOK:       llmChain.IsBYOK(),
				},
				Metadata: ExtractMeta{
					FetchDurationMs:   pageResult.FetchDurationMs,
					ExtractDurationMs: pageResult.ExtractDurationMs,
					Model:             llmCfg.Model,
					Provider:          llmCfg.Provider,
					BudgetSkips:       budgetSkips,
				},
				RawContent: pageResult.RawContent,
			}, nil
		}

		// Failed - classify error
		if err != nil {
			lastErr = err
		} else if pageResult != nil && pageResult.Error != nil {
			lastErr = pageResult.Error
		}

		// Check for config errors that shouldn't trigger fallback
		if errors.Is(lastErr, ErrDynamicFetchNotAllowed) || errors.Is(lastErr, ErrDynamicFetchNotConfigured) {
			return nil, lastErr
		}

		// Check for bot protection (extractor already tried dynamic if allowed)
		var protectionErr *ErrBotProtectionDetected
		if errors.As(lastErr, &protectionErr) {
			return nil, protectionErr
		}

		lastLLMErr = llm.WrapError(lastErr, llmCfg.Provider, llmCfg.Model, llmChain.IsBYOK())

		s.logger.Warn("prompt extraction failed",
			"provider", llmCfg.Provider,
			"model", llmCfg.Model,
			"error", lastErr,
		)

		if lastLLMErr != nil && !lastLLMErr.ShouldFallback {
			break
		}

		time.Sleep(constants.ProviderFallbackDelay)
	}

	// All attempts failed
	if lastErr != nil {
		return nil, s.handleLLMError(lastErr, lastCfg, llmChain.IsBYOK())
	}

	// Check if all models were skipped due to budget constraints
	if modelsSkippedDueToBudget > 0 && modelsSkippedDueToBudget == llmChain.Len() {
		s.logger.Warn("all models skipped due to budget constraints",
			"user_id", userID,
			"models_skipped", modelsSkippedDueToBudget,
			"available_budget_usd", availableBudget,
		)
		s.recordBudgetExhaustedFailure(ctx, userID, input, ectx, startTime, budgetSkips)
		return nil, llm.NewInsufficientCreditsError("all models exceed available budget", 0, int(availableBudget*100))
	}

	return nil, llm.NewNoModelsConfiguredError("no models in fallback chain or missing API keys")
}

// buildPromptExtractionPrompt constructs the LLM prompt for freeform extraction.
// Uses strings.Builder for efficient memory allocation with large page content.
func (s *ExtractionService) buildPromptExtractionPrompt(pageContent, userPrompt string) string {
	const templateOverhead = 500 // Approximate size of template text
	var b strings.Builder
	b.Grow(len(pageContent) + len(userPrompt) + templateOverhead)

	b.WriteString(`You are a data extraction assistant. Extract structured data from the provided web page content based on the user's instructions.

USER'S EXTRACTION INSTRUCTIONS:
`)
	b.WriteString(userPrompt)
	b.WriteString(`

WEB PAGE CONTENT:
`)
	b.WriteString(pageContent)
	b.WriteString(`

IMPORTANT INSTRUCTIONS:
1. Extract ONLY the data requested by the user
2. Return your response as valid JSON
3. Use appropriate data types (strings, numbers, arrays, objects) based on the content
4. If requested data is not found, use null for that field
5. For lists/arrays, include all matching items found
6. Be thorough but only include what was explicitly requested

Respond with ONLY valid JSON, no markdown formatting or explanation.`)

	return b.String()
}

