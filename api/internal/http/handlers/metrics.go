package handlers

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"github.com/jmylchreest/refyne-api/internal/http/mw"
	"github.com/jmylchreest/refyne-api/internal/repository"
)

// MetricsHandler handles internal metrics endpoints (superadmin only).
type MetricsHandler struct {
	repos *repository.Repositories
}

// NewMetricsHandler creates a new metrics handler.
func NewMetricsHandler(repos *repository.Repositories) *MetricsHandler {
	return &MetricsHandler{repos: repos}
}

// JobQueueStats represents job queue statistics.
type JobQueueStats struct {
	PendingTotal   int            `json:"pending_total" doc:"Total pending jobs"`
	RunningTotal   int            `json:"running_total" doc:"Total running jobs"`
	PendingByTier  map[string]int `json:"pending_by_tier" doc:"Pending jobs by tier"`
	RunningByTier  map[string]int `json:"running_by_tier" doc:"Running jobs by tier"`
	RunningByUser  map[string]int `json:"running_by_user" doc:"Running jobs by user ID"`
}

// RateLimitStats represents API key rate limiting statistics.
type RateLimitStats struct {
	ActiveSuspensions int `json:"active_suspensions" doc:"Number of currently suspended API keys"`
	TotalEntries      int `json:"total_entries" doc:"Total rate limit entries in database"`
}

// SystemMetrics represents overall system metrics.
type SystemMetrics struct {
	JobQueue   JobQueueStats  `json:"job_queue" doc:"Job queue statistics"`
	RateLimits RateLimitStats `json:"rate_limits" doc:"API key rate limit statistics"`
}

// GetMetricsOutput represents the metrics response.
type GetMetricsOutput struct {
	Body SystemMetrics
}

// GetMetrics returns system metrics (superadmin only).
func (h *MetricsHandler) GetMetrics(ctx context.Context, input *struct{}) (*GetMetricsOutput, error) {
	claims := mw.GetUserClaims(ctx)
	if claims == nil || !claims.GlobalSuperadmin {
		return nil, huma.Error403Forbidden("superadmin access required")
	}

	metrics := SystemMetrics{
		JobQueue: JobQueueStats{
			PendingByTier: make(map[string]int),
			RunningByTier: make(map[string]int),
			RunningByUser: make(map[string]int),
		},
	}

	// Get job queue stats from analytics repository
	if h.repos.Analytics != nil {
		stats, err := h.repos.Analytics.GetJobQueueStats(ctx)
		if err == nil {
			metrics.JobQueue.PendingTotal = stats.PendingTotal
			metrics.JobQueue.RunningTotal = stats.RunningTotal
			metrics.JobQueue.PendingByTier = stats.PendingByTier
			metrics.JobQueue.RunningByTier = stats.RunningByTier
			metrics.JobQueue.RunningByUser = stats.RunningByUser
		}
	}

	// Get rate limit stats
	if h.repos.RateLimit != nil {
		rlStats, err := h.repos.RateLimit.GetStats(ctx)
		if err == nil {
			metrics.RateLimits.ActiveSuspensions = rlStats.ActiveSuspensions
			metrics.RateLimits.TotalEntries = rlStats.TotalEntries
		}
	}

	return &GetMetricsOutput{Body: metrics}, nil
}
