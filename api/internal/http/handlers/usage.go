package handlers

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"github.com/jmylchreest/refyne-api/internal/service"
)

// UsageHandler handles usage endpoints.
type UsageHandler struct {
	usageSvc *service.UsageService
}

// NewUsageHandler creates a new usage handler.
func NewUsageHandler(usageSvc *service.UsageService) *UsageHandler {
	return &UsageHandler{usageSvc: usageSvc}
}

// GetUsageInput represents usage request.
type GetUsageInput struct {
	Period string `query:"period" default:"month" enum:"day,week,month,year" doc:"Time period for usage summary"`
}

// GetUsageOutput represents usage response.
type GetUsageOutput struct {
	Body struct {
		TotalJobs       int     `json:"total_jobs" doc:"Total number of jobs"`
		TotalChargedUSD float64 `json:"total_charged_usd" doc:"Total USD charged for usage"`
		BYOKJobs        int     `json:"byok_jobs" doc:"Jobs using user's own API keys (not charged)"`
	}
}

// GetUsage handles getting usage summary.
func (h *UsageHandler) GetUsage(ctx context.Context, input *GetUsageInput) (*GetUsageOutput, error) {
	userID := getUserID(ctx)
	if userID == "" {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	summary, err := h.usageSvc.GetUsageSummary(ctx, userID, input.Period)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get usage")
	}

	return &GetUsageOutput{
		Body: struct {
			TotalJobs       int     `json:"total_jobs" doc:"Total number of jobs"`
			TotalChargedUSD float64 `json:"total_charged_usd" doc:"Total USD charged for usage"`
			BYOKJobs        int     `json:"byok_jobs" doc:"Jobs using user's own API keys (not charged)"`
		}{
			TotalJobs:       summary.TotalJobs,
			TotalChargedUSD: summary.TotalChargedUSD,
			BYOKJobs:        summary.BYOKJobs,
		},
	}, nil
}
