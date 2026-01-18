package config

// BillingConfig holds billing-related configuration.
// Note: Per-tier billing settings (MarkupPercentage, CostPerTransaction,
// CreditAllocationUSD, CreditRolloverMonths) are now in constants.TierLimits.
// Use constants.GetTierLimits() to access them.
//
// This struct is retained for potential future non-tier-specific billing config
// and for backward compatibility with service constructors.
type BillingConfig struct {
	// Reserved for future non-tier-specific billing configuration
}

// DefaultBillingConfig returns the default billing configuration.
func DefaultBillingConfig() BillingConfig {
	return BillingConfig{}
}
