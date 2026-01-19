package mw

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/jmylchreest/refyne-api/internal/auth"
	"github.com/jmylchreest/refyne-api/internal/service"
)

// HumaAuthConfig holds dependencies for the Huma auth middleware.
type HumaAuthConfig struct {
	ClerkVerifier     *auth.ClerkVerifier
	AuthService       *service.AuthService
	SubscriptionCache *auth.SubscriptionCache
	UsageService      *service.UsageService
	JobService        JobService
}

// SecurityScheme is the name of the security scheme used in OpenAPI.
const SecurityScheme = "bearerAuth"

// OperationMetadataKey is the key for storing additional operation requirements.
type OperationMetadataKey string

const (
	// MetaKeyRequireFeature is metadata key for required features.
	MetaKeyRequireFeature OperationMetadataKey = "requireFeature"
	// MetaKeyRequireQuota is metadata key for quota check requirement.
	MetaKeyRequireQuota OperationMetadataKey = "requireQuota"
	// MetaKeyRequireConcurrencyLimit is metadata key for concurrent job limit check.
	MetaKeyRequireConcurrencyLimit OperationMetadataKey = "requireConcurrencyLimit"
	// MetaKeyRequireSuperadmin is metadata key for superadmin requirement.
	MetaKeyRequireSuperadmin OperationMetadataKey = "requireSuperadmin"
)

// HumaAuth returns a Huma middleware that handles authentication based on operation security.
// It checks ctx.Operation().Security to determine if authentication is required.
func HumaAuth(api huma.API, cfg HumaAuthConfig) func(ctx huma.Context, next func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		op := ctx.Operation()
		if op == nil {
			next(ctx)
			return
		}

		// Check if this operation requires bearer auth
		requiresAuth := operationRequiresAuth(op)
		if !requiresAuth {
			next(ctx)
			return
		}

		// Get token from Authorization header
		authHeader := ctx.Header("Authorization")
		if authHeader == "" {
			huma.WriteErr(api, ctx, http.StatusUnauthorized, "missing authorization header")
			return
		}

		// Extract token
		var token string
		if strings.HasPrefix(authHeader, "Bearer ") {
			token = strings.TrimPrefix(authHeader, "Bearer ")
		} else {
			token = authHeader
		}

		// Validate token
		var claims *UserClaims
		var err error

		stdCtx := ctx.Context()

		// Check if it's an API key (starts with rf_)
		if strings.HasPrefix(token, "rf_") {
			claims, err = validateAPIKey(stdCtx, cfg.AuthService, cfg.SubscriptionCache, token)
		} else {
			claims, err = validateClerkToken(cfg.ClerkVerifier, token)
		}

		if err != nil {
			slog.Debug("auth validation failed", "error", err)
			huma.WriteErr(api, ctx, http.StatusUnauthorized, "invalid token")
			return
		}

		// Check superadmin requirement
		if requiresSuperadmin(op) {
			if !claims.GlobalSuperadmin {
				huma.WriteErr(api, ctx, http.StatusForbidden, "superadmin access required")
				return
			}
		}

		// Check feature requirements
		if feature := getRequiredFeature(op); feature != "" {
			if !claims.HasFeature(feature) {
				slog.Debug("feature check failed",
					"user_id", claims.UserID,
					"tier", claims.Tier,
					"required_feature", feature,
					"user_features", claims.Features,
				)
				writeFeatureError(api, ctx, feature, claims.Tier)
				return
			}
		}

		// Add claims to context
		newCtx := context.WithValue(stdCtx, UserClaimsKey, claims)

		// Add tier limits to context
		limits := GetTierLimits(newCtx, claims.Tier)
		newCtx = context.WithValue(newCtx, TierLimitsKey, limits)

		// Check quota if required
		if requiresQuotaCheck(op) && cfg.UsageService != nil {
			if err := checkQuota(api, ctx, newCtx, cfg.UsageService, claims, limits); err != nil {
				return // Error already written
			}
		}

		// Check concurrent job limit if required
		if requiresConcurrencyCheck(op) && cfg.JobService != nil {
			if err := checkConcurrencyLimit(api, ctx, newCtx, cfg.JobService, claims, limits); err != nil {
				return // Error already written
			}
		}

		// Continue with enriched context
		next(huma.WithContext(ctx, newCtx))
	}
}

// operationRequiresAuth checks if the operation has bearerAuth in its security requirements.
func operationRequiresAuth(op *huma.Operation) bool {
	for _, secReq := range op.Security {
		if _, ok := secReq[SecurityScheme]; ok {
			return true
		}
	}
	return false
}

// requiresSuperadmin checks operation metadata for superadmin requirement.
func requiresSuperadmin(op *huma.Operation) bool {
	if op.Metadata == nil {
		return false
	}
	if val, ok := op.Metadata[string(MetaKeyRequireSuperadmin)]; ok {
		if b, ok := val.(bool); ok {
			return b
		}
	}
	return false
}

// getRequiredFeature returns the required feature from operation metadata.
func getRequiredFeature(op *huma.Operation) string {
	if op.Metadata == nil {
		return ""
	}
	if val, ok := op.Metadata[string(MetaKeyRequireFeature)]; ok {
		if s, ok := val.(string); ok {
			return s
		}
	}
	return ""
}

// requiresQuotaCheck checks operation metadata for quota check requirement.
func requiresQuotaCheck(op *huma.Operation) bool {
	if op.Metadata == nil {
		return false
	}
	if val, ok := op.Metadata[string(MetaKeyRequireQuota)]; ok {
		if b, ok := val.(bool); ok {
			return b
		}
	}
	return false
}

// requiresConcurrencyCheck checks operation metadata for concurrency limit check requirement.
func requiresConcurrencyCheck(op *huma.Operation) bool {
	if op.Metadata == nil {
		return false
	}
	if val, ok := op.Metadata[string(MetaKeyRequireConcurrencyLimit)]; ok {
		if b, ok := val.(bool); ok {
			return b
		}
	}
	return false
}

// checkQuota validates that the user has remaining quota.
func checkQuota(api huma.API, humaCtx huma.Context, stdCtx context.Context, usageSvc *service.UsageService, claims *UserClaims, limits TierLimits) error {
	// If unlimited (0), allow through
	if limits.MonthlyExtractions == 0 {
		return nil
	}

	// Check current billing period usage
	usage, err := usageSvc.GetBillingPeriodUsage(stdCtx, claims.UserID)
	if err != nil {
		slog.Error("failed to check usage quota",
			"user_id", claims.UserID,
			"error", err,
		)
		huma.WriteErr(api, humaCtx, http.StatusInternalServerError, "failed to check usage quota")
		return err
	}

	// Check if over quota
	if usage.TotalJobs >= limits.MonthlyExtractions {
		slog.Debug("billing period quota exceeded",
			"user_id", claims.UserID,
			"tier", claims.Tier,
			"limit", limits.MonthlyExtractions,
			"used", usage.TotalJobs,
		)
		writeQuotaExceededError(api, humaCtx, claims, limits, usage.TotalJobs)
		return err
	}

	// Add remaining quota to response headers
	remaining := limits.MonthlyExtractions - usage.TotalJobs
	humaCtx.SetHeader("X-RateLimit-Limit", intToString(limits.MonthlyExtractions))
	humaCtx.SetHeader("X-RateLimit-Remaining", intToString(remaining))

	return nil
}

// checkConcurrencyLimit validates that the user can start another job.
func checkConcurrencyLimit(api huma.API, humaCtx huma.Context, stdCtx context.Context, jobSvc JobService, claims *UserClaims, limits TierLimits) error {
	// If unlimited (0), allow through
	if limits.MaxConcurrentJobs == 0 {
		return nil
	}

	// Check current active jobs
	activeJobs, err := jobSvc.CountActiveJobsByUser(stdCtx, claims.UserID)
	if err != nil {
		slog.Error("failed to count active jobs",
			"user_id", claims.UserID,
			"error", err,
		)
		huma.WriteErr(api, humaCtx, http.StatusInternalServerError, "failed to check job limit")
		return err
	}

	// Check if at limit
	if activeJobs >= limits.MaxConcurrentJobs {
		slog.Debug("concurrent job limit exceeded",
			"user_id", claims.UserID,
			"tier", claims.Tier,
			"limit", limits.MaxConcurrentJobs,
			"active", activeJobs,
		)
		writeConcurrencyError(api, humaCtx, claims, limits, activeJobs)
		return err
	}

	humaCtx.SetHeader("X-Concurrent-Limit", intToString(limits.MaxConcurrentJobs))
	humaCtx.SetHeader("X-Concurrent-Active", intToString(activeJobs))

	return nil
}

// writeFeatureError writes a structured error for missing feature.
func writeFeatureError(api huma.API, ctx huma.Context, feature, tier string) {
	huma.WriteErr(api, ctx, http.StatusForbidden,
		"feature_not_available: "+getFeatureNotAvailableMessage(feature),
		fmt.Errorf("feature %s not available for tier %s", feature, tier))
}

// writeQuotaExceededError writes a structured error for quota exceeded.
func writeQuotaExceededError(api huma.API, ctx huma.Context, claims *UserClaims, limits TierLimits, used int) {
	ctx.SetHeader("X-RateLimit-Limit", intToString(limits.MonthlyExtractions))
	ctx.SetHeader("X-RateLimit-Remaining", "0")
	huma.WriteErr(api, ctx, http.StatusTooManyRequests,
		"monthly quota exceeded: "+getQuotaExceededMessage(claims.Tier, limits.MonthlyExtractions),
		fmt.Errorf("limit: %d, used: %d, tier: %s", limits.MonthlyExtractions, used, claims.Tier))
}

// writeConcurrencyError writes a structured error for concurrent job limit exceeded.
func writeConcurrencyError(api huma.API, ctx huma.Context, claims *UserClaims, limits TierLimits, active int) {
	ctx.SetHeader("X-Concurrent-Limit", intToString(limits.MaxConcurrentJobs))
	ctx.SetHeader("X-Concurrent-Active", intToString(active))
	huma.WriteErr(api, ctx, http.StatusTooManyRequests,
		"concurrent_job_limit_exceeded: Maximum concurrent jobs reached for your tier",
		fmt.Errorf("limit: %d, active: %d, tier: %s", limits.MaxConcurrentJobs, active, claims.Tier))
}

// HumaRateLimit returns a Huma middleware that applies user-based rate limiting.
// Rate limiting is only applied to authenticated requests with user claims.
// This is a passthrough that delegates to the Chi rate limiter via the
// underlying request since httprate works with Chi's http.Request.
func HumaRateLimit(_ RateLimitConfig) func(ctx huma.Context, next func(huma.Context)) {
	// Rate limiting is handled by the Chi middleware layer (RateLimitByUser)
	// for operations that need it. This Huma middleware is a passthrough
	// that could be extended for more granular Huma-specific rate limiting.
	return func(ctx huma.Context, next func(huma.Context)) {
		next(ctx)
	}
}
