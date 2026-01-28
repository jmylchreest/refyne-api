package service

import (
	"context"
	"errors"

	"github.com/jmylchreest/refyne/pkg/refyne"
	"github.com/jmylchreest/refyne/pkg/schema"
)

// SchemaPageExtractor extracts data from a single page using a structured JSON schema.
// It wraps the refyne library's Extract() method with dynamic retry support for bot
// protection detection and insufficient content errors.
type SchemaPageExtractor struct {
	svc          *ExtractionService
	schema       schema.Schema
	llmCfg       *LLMConfigInput
	cleanerChain []CleanerConfig

	// Context for dynamic retry
	contentDynamicAllowed bool
	userID                string
	tier                  string
	jobID                 string
}

// NewSchemaPageExtractor creates a new schema-based page extractor.
func NewSchemaPageExtractor(svc *ExtractionService, sch schema.Schema, opts SchemaExtractorOptions) *SchemaPageExtractor {
	return &SchemaPageExtractor{
		svc:                   svc,
		schema:                sch,
		llmCfg:                opts.LLMConfig,
		cleanerChain:          opts.CleanerChain,
		contentDynamicAllowed: opts.ContentDynamicAllowed,
		userID:                opts.UserID,
		tier:                  opts.Tier,
		jobID:                 opts.JobID,
	}
}

// Extract fetches and extracts data from a single URL using the configured schema.
// Automatically retries with browser rendering if bot protection or insufficient
// content is detected (when ContentDynamicAllowed is true).
func (e *SchemaPageExtractor) Extract(ctx context.Context, pageURL string) (*PageExtractionResult, error) {
	result := &PageExtractionResult{URL: pageURL}

	// Start with auto mode to enable protection detection
	effectiveFetchMode := "auto"
	dynamicRetryAttempted := false

extractAttempt:
	// Create refyne instance with current fetch mode
	r, _, err := e.svc.createRefyneInstanceWithFetchMode(e.llmCfg, e.cleanerChain, FetchModeConfig{
		Mode:                  effectiveFetchMode,
		ContentDynamicAllowed: e.contentDynamicAllowed,
		UserID:                e.userID,
		Tier:                  e.tier,
		JobID:                 e.jobID,
	})
	if err != nil {
		// Check for permission/configuration errors that shouldn't be retried
		if errors.Is(err, ErrDynamicFetchNotAllowed) || errors.Is(err, ErrDynamicFetchNotConfigured) {
			result.Error = err
			result.ErrorCategory = "config_error"
			return result, err
		}
		result.Error = err
		result.ErrorCategory = "config_error"
		return result, err
	}
	defer func() { _ = r.Close() }()

	// Perform extraction
	refyneResult, err := r.Extract(ctx, pageURL, e.schema)

	// Check for success
	if err == nil && refyneResult != nil && refyneResult.Error == nil {
		// Success - populate result
		result.Data = e.svc.processExtractionResult(refyneResult.Data, refyneResult.URL)
		result.URL = refyneResult.URL
		result.RawContent = refyneResult.RawContent
		result.TokensInput = refyneResult.TokenUsage.InputTokens
		result.TokensOutput = refyneResult.TokenUsage.OutputTokens
		result.FetchDurationMs = int(refyneResult.FetchDuration.Milliseconds())
		result.ExtractDurationMs = int(refyneResult.ExtractDuration.Milliseconds())
		result.Provider = refyneResult.Provider
		result.Model = refyneResult.Model
		result.GenerationID = refyneResult.GenerationID
		result.UsedDynamicMode = effectiveFetchMode == "dynamic"
		return result, nil
	}

	// Handle error - check for dynamic retry conditions
	var lastErr error
	if err != nil {
		lastErr = err
	} else if refyneResult != nil && refyneResult.Error != nil {
		lastErr = refyneResult.Error
	}

	// Populate token usage even on error (for truncation detection)
	if refyneResult != nil {
		result.TokensInput = refyneResult.TokenUsage.InputTokens
		result.TokensOutput = refyneResult.TokenUsage.OutputTokens
		result.Provider = refyneResult.Provider
		result.Model = refyneResult.Model

		// Check for output truncation using refyne's FinishReason
		// "length" means the model hit max_tokens and the output was cut off
		if refyneResult.IsTruncated() {
			truncErr := &ErrOutputTruncated{
				OutputTokens: result.TokensOutput,
				MaxTokens:    e.llmCfg.MaxTokens,
				Model:        e.llmCfg.Model,
			}
			result.Error = truncErr
			result.ErrorCategory = "llm_truncation"
			e.svc.logger.Warn("schema extraction truncated, will fallback to next model",
				"url", pageURL,
				"model", e.llmCfg.Model,
				"output_tokens", result.TokensOutput,
				"max_tokens", e.llmCfg.MaxTokens,
				"finish_reason", refyneResult.FinishReason,
			)
			return result, truncErr
		}
	}

	// Check for bot protection - retry with dynamic if allowed
	var protectionErr *ErrBotProtectionDetected
	if errors.As(lastErr, &protectionErr) && !dynamicRetryAttempted && e.contentDynamicAllowed && e.svc.captchaSvc != nil {
		e.svc.logger.Info("auto-retrying schema extraction with browser rendering",
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
	if errors.As(lastErr, &insufficientErr) && !dynamicRetryAttempted && e.contentDynamicAllowed && e.svc.captchaSvc != nil {
		e.svc.logger.Info("auto-retrying schema extraction with browser rendering due to insufficient content",
			"url", pageURL,
			"content_size", insufficientErr.ContentSize,
		)
		effectiveFetchMode = "dynamic"
		dynamicRetryAttempted = true
		result.RetryCount++
		goto extractAttempt
	}

	result.Error = lastErr
	result.ErrorCategory = "extraction_error"
	result.UsedDynamicMode = effectiveFetchMode == "dynamic"
	return result, lastErr
}
