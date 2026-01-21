package handlers

import (
	"context"
	"time"

	"github.com/jmylchreest/refyne-api/internal/models"
	"github.com/jmylchreest/refyne-api/internal/repository"
	"github.com/jmylchreest/refyne-api/internal/service"
)

// JobCompletionInput contains the common fields needed to complete a job.
type JobCompletionInput struct {
	TokensInput  int
	TokensOutput int
	CostUSD      float64
	LLMCostUSD   float64 // Actual LLM provider cost (always recorded)
	IsBYOK       bool
	LLMProvider  string
	LLMModel     string
	PageCount    int
	ResultJSON   string // Optional: for storing results
}

// JobFailureInput contains the common fields needed to record a job failure.
type JobFailureInput struct {
	ErrorMessage  string
	ErrorDetails  string
	ErrorCategory string
	LLMProvider   string
	LLMModel      string
}

// CompleteJobDirect updates a job record directly via repository.
// Used when the handler has direct access to the job object (e.g., analyze jobs).
func CompleteJobDirect(ctx context.Context, jobRepo repository.JobRepository, job *models.Job, input JobCompletionInput) error {
	now := time.Now()
	job.Status = models.JobStatusCompleted
	job.PageCount = input.PageCount
	if input.PageCount == 0 {
		job.PageCount = 1 // Default to 1 for single-page operations
	}
	job.TokenUsageInput = input.TokensInput
	job.TokenUsageOutput = input.TokensOutput
	job.CostUSD = input.CostUSD
	job.LLMCostUSD = input.LLMCostUSD
	job.IsBYOK = input.IsBYOK
	job.LLMProvider = input.LLMProvider
	job.LLMModel = input.LLMModel
	job.CompletedAt = &now
	job.UpdatedAt = now

	if input.ResultJSON != "" {
		job.ResultJSON = input.ResultJSON
	}

	return jobRepo.Update(ctx, job)
}

// FailJobDirect updates a job record with failure status directly via repository.
// Used when the handler has direct access to the job object (e.g., analyze jobs).
func FailJobDirect(ctx context.Context, jobRepo repository.JobRepository, job *models.Job, input JobFailureInput) error {
	now := time.Now()
	job.Status = models.JobStatusFailed
	job.ErrorMessage = input.ErrorMessage
	job.ErrorCategory = input.ErrorCategory
	job.LLMProvider = input.LLMProvider
	job.LLMModel = input.LLMModel
	job.CompletedAt = &now
	job.UpdatedAt = now

	return jobRepo.Update(ctx, job)
}

// CompleteExtractJobViaService completes an extract job via the JobService.
// Used when the handler uses the JobService abstraction (e.g., extract jobs).
func CompleteExtractJobViaService(ctx context.Context, jobSvc *service.JobService, jobID string, input JobCompletionInput) error {
	if jobSvc == nil || jobID == "" {
		return nil // No-op if service not available or no job ID
	}

	return jobSvc.CompleteExtractJob(ctx, jobID, service.CompleteExtractJobInput{
		ResultJSON:       input.ResultJSON,
		PageCount:        input.PageCount,
		TokenUsageInput:  input.TokensInput,
		TokenUsageOutput: input.TokensOutput,
		CostUSD:          input.CostUSD,
		LLMCostUSD:       input.LLMCostUSD,
		LLMProvider:      input.LLMProvider,
		LLMModel:         input.LLMModel,
		IsBYOK:           input.IsBYOK,
	})
}

// FailExtractJobViaService records an extract job failure via the JobService.
// Used when the handler uses the JobService abstraction (e.g., extract jobs).
func FailExtractJobViaService(ctx context.Context, jobSvc *service.JobService, jobID string, input JobFailureInput) error {
	if jobSvc == nil || jobID == "" {
		return nil // No-op if service not available or no job ID
	}

	return jobSvc.FailExtractJob(ctx, jobID, service.FailExtractJobInput{
		ErrorMessage:  input.ErrorMessage,
		ErrorDetails:  input.ErrorDetails,
		ErrorCategory: input.ErrorCategory,
		LLMProvider:   input.LLMProvider,
		LLMModel:      input.LLMModel,
	})
}

// JobCompletionFromErrorInfo creates a JobFailureInput from an ErrorInfo.
// This provides a convenient way to fail a job using extracted error information.
func JobFailureFromErrorInfo(info ErrorInfo) JobFailureInput {
	return JobFailureInput{
		ErrorMessage:  info.UserMessage,
		ErrorDetails:  info.Details,
		ErrorCategory: info.Category,
		LLMProvider:   info.Provider,
		LLMModel:      info.Model,
	}
}
