package service

import (
	"context"
	"log/slog"

	"github.com/jmylchreest/refyne-api/internal/auth"
	"github.com/jmylchreest/refyne-api/internal/constants"
)

// TierSyncService syncs tier metadata (visibility, display names) from Clerk Commerce.
type TierSyncService struct {
	clerkClient *auth.ClerkBackendClient
	logger      *slog.Logger
}

// NewTierSyncService creates a new tier sync service.
func NewTierSyncService(clerkClient *auth.ClerkBackendClient, logger *slog.Logger) *TierSyncService {
	return &TierSyncService{
		clerkClient: clerkClient,
		logger:      logger,
	}
}

// SyncFromClerk fetches plan metadata from Clerk Commerce and updates tier visibility/display names.
// This should be called on application startup and when receiving plan update webhooks.
func (s *TierSyncService) SyncFromClerk(ctx context.Context) error {
	if s.clerkClient == nil {
		s.logger.Debug("clerk client not configured, skipping tier sync")
		return nil
	}

	plans, err := s.clerkClient.ListSubscriptionProducts(ctx)
	if err != nil {
		s.logger.Error("failed to fetch plans from Clerk", "error", err)
		return err
	}

	// Convert Clerk plans to tier metadata
	metadata := make([]constants.TierMetadata, 0, len(plans))
	for _, plan := range plans {
		// Normalize the slug to our internal tier name
		normalizedSlug := constants.NormalizeTierName(plan.Slug)

		metadata = append(metadata, constants.TierMetadata{
			Slug:        plan.Slug, // Keep original for logging, UpdateTierMetadata will normalize
			DisplayName: plan.Name,
			Visible:     plan.PubliclyVisible,
		})

		s.logger.Info("clerk plan details",
			"original_slug", plan.Slug,
			"normalized_slug", normalizedSlug,
			"name", plan.Name,
			"publicly_visible", plan.PubliclyVisible,
			"is_default", plan.IsDefault,
		)
	}

	// Update the tier configuration
	constants.UpdateTierMetadata(metadata)

	s.logger.Info("tier metadata synced from Clerk",
		"plan_count", len(plans),
	)

	return nil
}
