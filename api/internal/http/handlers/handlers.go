// Package handlers contains HTTP handlers for the API.
package handlers

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"github.com/jmylchreest/refyne-api/internal/http/mw"
)

// HealthCheckOutput represents health check response.
type HealthCheckOutput struct {
	Body struct {
		Status  string `json:"status"`
		Version string `json:"version"`
	}
}

// HealthCheck returns the health status of the API.
func HealthCheck(ctx context.Context, input *struct{}) (*HealthCheckOutput, error) {
	return &HealthCheckOutput{
		Body: struct {
			Status  string `json:"status"`
			Version string `json:"version"`
		}{
			Status:  "healthy",
			Version: "1.0.0",
		},
	}, nil
}

// LivezOutput represents Kubernetes liveness probe response.
type LivezOutput struct {
	Body struct {
		Status string `json:"status" doc:"Liveness status"`
	}
}

// Livez is the Kubernetes liveness probe endpoint.
// Returns 200 if the process is running.
func Livez(ctx context.Context, input *struct{}) (*LivezOutput, error) {
	return &LivezOutput{
		Body: struct {
			Status string `json:"status" doc:"Liveness status"`
		}{
			Status: "ok",
		},
	}, nil
}

// ReadyzOutput represents Kubernetes readiness probe response.
type ReadyzOutput struct {
	Body struct {
		Status string `json:"status" doc:"Readiness status"`
	}
}

// ReadyzInput holds the database reference for readiness checks.
type ReadyzInput struct {
	// DB check will be done via the handler's db reference
}

// ReadyzHandler handles readiness probe with DB connectivity check.
type ReadyzHandler struct {
	db DBPinger
}

// DBPinger interface for database ping check.
type DBPinger interface {
	Ping() error
}

// NewReadyzHandler creates a new readiness handler.
func NewReadyzHandler(db DBPinger) *ReadyzHandler {
	return &ReadyzHandler{db: db}
}

// Readyz is the Kubernetes readiness probe endpoint.
// Returns 200 if the service is ready to accept traffic.
func (h *ReadyzHandler) Readyz(ctx context.Context, input *struct{}) (*ReadyzOutput, error) {
	// Check database connectivity
	if h.db != nil {
		if err := h.db.Ping(); err != nil {
			return nil, huma.Error503ServiceUnavailable("database unavailable: " + err.Error())
		}
	}

	return &ReadyzOutput{
		Body: struct {
			Status string `json:"status" doc:"Readiness status"`
		}{
			Status: "ok",
		},
	}, nil
}

// getUserID extracts user ID from context.
func getUserID(ctx context.Context) string {
	claims := mw.GetUserClaims(ctx)
	if claims == nil {
		return ""
	}
	return claims.UserID
}

// getUserClaims extracts user claims from context.
func getUserClaims(ctx context.Context) *mw.UserClaims {
	return mw.GetUserClaims(ctx)
}

// getUserTier extracts the user's subscription tier from context.
func getUserTier(ctx context.Context) string {
	claims := mw.GetUserClaims(ctx)
	if claims == nil {
		return "free"
	}
	return claims.Tier
}
