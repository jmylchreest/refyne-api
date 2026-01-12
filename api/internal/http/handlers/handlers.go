// Package handlers contains HTTP handlers for the API.
package handlers

import (
	"context"

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
