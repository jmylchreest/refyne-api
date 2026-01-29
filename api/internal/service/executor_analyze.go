package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jmylchreest/refyne-api/internal/models"
)

// AnalyzeExecutor handles URL analysis jobs.
// It implements the JobExecutor interface for synchronous analysis.
type AnalyzeExecutor struct {
	analyzerSvc            *AnalyzerService
	input                  AnalyzeInput
	userID                 string
	tier                   string
	byokAllowed            bool
	modelsCustomAllowed    bool
	contentDynamicAllowed  bool
	skipCreditCheckAllowed bool
	isByok                 bool
	jobID                  string // Job ID for tracking in downstream services
}

// NewAnalyzeExecutor creates a new analyze executor.
func NewAnalyzeExecutor(
	analyzerSvc *AnalyzerService,
	input AnalyzeInput,
	userID string,
	tier string,
	byokAllowed bool,
	modelsCustomAllowed bool,
	contentDynamicAllowed bool,
	skipCreditCheckAllowed bool,
) *AnalyzeExecutor {
	return &AnalyzeExecutor{
		analyzerSvc:            analyzerSvc,
		input:                  input,
		userID:                 userID,
		tier:                   tier,
		byokAllowed:            byokAllowed,
		modelsCustomAllowed:    modelsCustomAllowed,
		contentDynamicAllowed:  contentDynamicAllowed,
		skipCreditCheckAllowed: skipCreditCheckAllowed,
		isByok:                 false,
	}
}

// Execute runs the analysis and returns the result.
func (e *AnalyzeExecutor) Execute(ctx context.Context) (*JobExecutionResult, error) {
	startTime := time.Now()

	// Set the job ID on the input for tracking in downstream services
	input := e.input
	input.JobID = e.jobID

	result, err := e.analyzerSvc.Analyze(
		ctx,
		e.userID,
		input,
		e.tier,
		e.byokAllowed,
		e.modelsCustomAllowed,
		e.contentDynamicAllowed,
		e.skipCreditCheckAllowed,
	)
	if err != nil {
		return nil, err
	}

	// Update BYOK status based on actual execution
	e.isByok = result.TokenUsage.IsBYOK

	// Serialize result for storage
	resultJSON, _ := json.Marshal(result)

	// Build debug capture data if available
	var debugCapture *DebugCaptureData
	if result.DebugCapture != nil {
		debugCapture = &DebugCaptureData{
			URL:            e.input.URL,
			FetchMode:      result.DebugCapture.FetchMode,
			RawContent:     result.DebugCapture.RawContent,
			RawLLMResponse: result.DebugCapture.LLMResponse,
			Prompt:         result.DebugCapture.Prompt,
			DurationMs:     time.Since(startTime).Milliseconds(),
		}
	}

	// Build webhook data
	webhookData := map[string]any{
		"url":                    e.input.URL,
		"site_summary":           result.SiteSummary,
		"page_type":              string(result.PageType),
		"suggested_schema":       result.SuggestedSchema,
		"recommended_fetch_mode": string(result.RecommendedFetchMode),
	}

	// Add detected elements
	if len(result.DetectedElements) > 0 {
		elements := make([]map[string]any, len(result.DetectedElements))
		for i, elem := range result.DetectedElements {
			elements[i] = map[string]any{
				"name":        elem.Name,
				"type":        elem.Type,
				"description": elem.Description,
			}
		}
		webhookData["detected_elements"] = elements
	}

	return &JobExecutionResult{
		// Store the full AnalyzeOutput so handlers can access all metadata
		Data:         result,
		TokensInput:  result.TokenUsage.InputTokens,
		TokensOutput: result.TokenUsage.OutputTokens,
		CostUSD:      result.TokenUsage.CostUSD,
		LLMCostUSD:   result.TokenUsage.LLMCostUSD,
		LLMProvider:  result.TokenUsage.LLMProvider,
		LLMModel:     result.TokenUsage.LLMModel,
		PageCount:    1,
		IsBYOK:       result.TokenUsage.IsBYOK,
		ResultJSON:   string(resultJSON),
		DebugCapture: debugCapture,
		WebhookData:  webhookData,
	}, nil
}

// JobType returns the job type.
func (e *AnalyzeExecutor) JobType() models.JobType {
	return models.JobTypeAnalyze
}

// IsBYOK returns whether the job uses user's own API keys.
func (e *AnalyzeExecutor) IsBYOK() bool {
	return e.isByok
}

// GetURL returns the primary URL for this job.
func (e *AnalyzeExecutor) GetURL() string {
	return e.input.URL
}

// GetSchema returns the schema for this job (nil for analyze jobs).
func (e *AnalyzeExecutor) GetSchema() json.RawMessage {
	return nil
}

// SetJobID sets the job ID on the executor for tracking in downstream services.
func (e *AnalyzeExecutor) SetJobID(jobID string) {
	e.jobID = jobID
}
