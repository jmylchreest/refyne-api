package config

// BillingConfig holds billing-related configuration.
type BillingConfig struct {
	// CreditRolloverMonths defines how many months unused credits remain valid.
	// 0 = no rollover (credits expire at end of billing period)
	// 1 = roll over for 1 additional month, etc.
	CreditRolloverMonths int

	// DefaultMarkup is the default markup applied to LLM costs (0.25 = 25%).
	DefaultMarkup float64

	// TierMarkup defines per-tier markup overrides.
	TierMarkup map[string]float64

	// TierAllocation defines the monthly USD allocation per tier.
	TierAllocation map[string]float64
}

// DefaultBillingConfig returns the default billing configuration.
func DefaultBillingConfig() BillingConfig {
	return BillingConfig{
		CreditRolloverMonths: 0, // No rollover by default

		DefaultMarkup: 0.25, // 25% default markup

		TierMarkup: map[string]float64{
			"free":       1.00, // 100% markup (we subsidize heavily, but limit usage)
			"starter":    0.25, // 25% markup
			"pro":        0.10, // 10% markup
			"selfhosted": 0.00, // No markup for self-hosted
		},

		TierAllocation: map[string]float64{
			"free":       0.50,  // $0.50 worth of API calls
			"starter":    15.00, // $15 of $19 subscription
			"pro":        45.00, // $45 of $49 subscription
			"selfhosted": 0.00,  // No allocation (BYOK only)
		},
	}
}

// GetMarkup returns the markup for a tier, falling back to default.
func (c *BillingConfig) GetMarkup(tier string) float64 {
	if markup, ok := c.TierMarkup[tier]; ok {
		return markup
	}
	return c.DefaultMarkup
}

// GetAllocation returns the monthly USD allocation for a tier.
func (c *BillingConfig) GetAllocation(tier string) float64 {
	if allocation, ok := c.TierAllocation[tier]; ok {
		return allocation
	}
	return 0
}
