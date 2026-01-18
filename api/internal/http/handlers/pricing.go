package handlers

import (
	"context"
	"sort"

	"github.com/jmylchreest/refyne-api/internal/constants"
)

// TierLimitsResponse represents the enforced numeric limits for a single tier.
// Feature availability (webhooks, BYOK, etc.) is controlled by Clerk features, not tier limits.
// Only includes limits that are actually enforced by the API.
type TierLimitsResponse struct {
	Name                 string  `json:"name" doc:"Tier name (free, standard, pro, selfhosted)"`
	DisplayName          string  `json:"display_name" doc:"Human-readable tier name (from Clerk Commerce)"`
	MonthlyExtractions   int     `json:"monthly_extractions" doc:"Monthly extraction limit (0 = unlimited)"`
	MaxConcurrentJobs    int     `json:"max_concurrent_jobs" doc:"Max concurrent jobs (0 = unlimited)"`
	MaxPagesPerCrawl     int     `json:"max_pages_per_crawl" doc:"Max pages per crawl job (0 = unlimited)"`
	RequestsPerMinute    int     `json:"requests_per_minute" doc:"API requests per minute limit (0 = unlimited)"`
	CreditAllocationUSD  float64 `json:"credit_allocation_usd" doc:"Monthly USD credit for premium model calls (0 = none)"`
	CreditRolloverMonths int     `json:"credit_rollover_months" doc:"Credit expiry: -1 = never, 0 = current period, N = N additional periods"`
}

// ListTierLimitsOutput is the response for the tier limits endpoint.
type ListTierLimitsOutput struct {
	Body struct {
		Tiers []TierLimitsResponse `json:"tiers" doc:"List of visible tiers and their limits"`
	}
}

// allTierNames is the list of all known tier names.
// This is used to iterate over tiers when building the response.
var allTierNames = []string{
	constants.TierFree,
	constants.TierStandard,
	constants.TierPro,
	constants.TierSelfHosted,
}

// tierWithOrder holds a tier response with its sort order.
type tierWithOrder struct {
	response TierLimitsResponse
	order    int
}

// ListTierLimits returns the limits for all visible subscription tiers.
// Only tiers with Visible=true are returned, sorted by their Order field.
// DisplayName comes from TierLimits (synced from Clerk Commerce).
// This is a public endpoint for use in pricing pages.
func ListTierLimits(ctx context.Context, _ *struct{}) (*ListTierLimitsOutput, error) {
	var visibleTiers []tierWithOrder

	for _, name := range allTierNames {
		// Use S3-backed limits if available, otherwise hardcoded defaults
		limits := constants.GetTierLimitsWithS3(ctx, name)

		// Skip tiers that aren't visible
		// TODO: Sync Visible flag from Clerk Commerce public availability setting
		if !limits.Visible {
			continue
		}

		// Use DisplayName from TierLimits (can be synced from Clerk)
		displayName := limits.DisplayName
		if displayName == "" {
			// Fallback to tier name if not set
			displayName = name
		}

		visibleTiers = append(visibleTiers, tierWithOrder{
			response: TierLimitsResponse{
				Name:                 name,
				DisplayName:          displayName,
				MonthlyExtractions:   limits.MonthlyExtractions,
				MaxConcurrentJobs:    limits.MaxConcurrentJobs,
				MaxPagesPerCrawl:     limits.MaxPagesPerCrawl,
				RequestsPerMinute:    limits.RequestsPerMinute,
				CreditAllocationUSD:  limits.CreditAllocationUSD,
				CreditRolloverMonths: limits.CreditRolloverMonths,
			},
			order: limits.Order,
		})
	}

	// Sort by order
	sort.Slice(visibleTiers, func(i, j int) bool {
		return visibleTiers[i].order < visibleTiers[j].order
	})

	// Extract just the responses
	tiers := make([]TierLimitsResponse, len(visibleTiers))
	for i, t := range visibleTiers {
		tiers[i] = t.response
	}

	return &ListTierLimitsOutput{
		Body: struct {
			Tiers []TierLimitsResponse `json:"tiers" doc:"List of visible tiers and their limits"`
		}{Tiers: tiers},
	}, nil
}
