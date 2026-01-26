package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jmylchreest/refyne-api/internal/models"
)

// ExtractExecutor handles single-page extraction jobs.
// It implements the JobExecutor interface for synchronous extraction.
type ExtractExecutor struct {
	extractionSvc *ExtractionService
	input         ExtractInput
	ectx          *ExtractContext
	isByok        bool
	url           string
	schema        json.RawMessage
}

// NewExtractExecutor creates a new extraction executor.
func NewExtractExecutor(extractionSvc *ExtractionService, input ExtractInput, ectx *ExtractContext) *ExtractExecutor {
	return &ExtractExecutor{
		extractionSvc: extractionSvc,
		input:         input,
		ectx:          ectx,
		isByok:        ectx != nil && ectx.IsBYOK,
		url:           input.URL,
		schema:        input.Schema,
	}
}

// Execute runs the extraction and returns the result.
func (e *ExtractExecutor) Execute(ctx context.Context) (*JobExecutionResult, error) {
	startTime := time.Now()

	result, err := e.extractionSvc.ExtractWithContext(ctx, e.ectx.UserID, e.input, e.ectx)
	if err != nil {
		return nil, err
	}

	// Update BYOK status based on actual execution
	e.isByok = result.Usage.IsBYOK

	// Serialize result data for storage
	resultJSON, _ := json.Marshal(result.Data)

	// Build debug capture data if raw content is available
	var debugCapture *DebugCaptureData
	if result.RawContent != "" {
		debugCapture = &DebugCaptureData{
			URL:        result.URL,
			FetchMode:  e.input.FetchMode,
			RawContent: result.RawContent,
			DurationMs: time.Since(startTime).Milliseconds(),
		}
	}

	return &JobExecutionResult{
		// Store the full ExtractOutput so handlers can access all metadata
		Data:         result,
		TokensInput:  result.Usage.InputTokens,
		TokensOutput: result.Usage.OutputTokens,
		CostUSD:      result.Usage.CostUSD,
		LLMCostUSD:   result.Usage.LLMCostUSD,
		LLMProvider:  result.Metadata.Provider,
		LLMModel:     result.Metadata.Model,
		PageCount:    1,
		IsBYOK:       result.Usage.IsBYOK,
		ResultJSON:   string(resultJSON),
		DebugCapture: debugCapture,
		WebhookData: map[string]any{
			"data":         result.Data,
			"url":          result.URL,
			"fetched_at":   result.FetchedAt.Format(time.RFC3339),
			"input_format": string(result.InputFormat),
		},
	}, nil
}

// JobType returns the job type.
func (e *ExtractExecutor) JobType() models.JobType {
	return models.JobTypeExtract
}

// IsBYOK returns whether the job uses user's own API keys.
func (e *ExtractExecutor) IsBYOK() bool {
	return e.isByok
}

// GetURL returns the primary URL for this job.
func (e *ExtractExecutor) GetURL() string {
	return e.url
}

// GetSchema returns the schema for this job.
func (e *ExtractExecutor) GetSchema() json.RawMessage {
	return e.schema
}
