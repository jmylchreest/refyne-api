package handlers

import (
	"context"

	"github.com/jmylchreest/refyne-api/captcha/internal/browser"
	"github.com/jmylchreest/refyne-api/captcha/internal/version"
)

// HealthResponse represents the health check response.
type HealthResponse struct {
	Status  string          `json:"status"`
	Version string          `json:"version"`
	Pool    *browser.PoolStats `json:"pool,omitempty"`
}

// HealthHandler handles health check requests.
type HealthHandler struct {
	pool *browser.Pool
}

// NewHealthHandler creates a new health handler.
func NewHealthHandler(pool *browser.Pool) *HealthHandler {
	return &HealthHandler{pool: pool}
}

// HealthOutput is the output wrapper for Huma.
type HealthOutput struct {
	Body HealthResponse
}

// Handle returns the health status.
func (h *HealthHandler) Handle(ctx context.Context) *HealthResponse {
	stats := h.pool.Stats()

	return &HealthResponse{
		Status:  "healthy",
		Version: version.Get().Version,
		Pool:    &stats,
	}
}
