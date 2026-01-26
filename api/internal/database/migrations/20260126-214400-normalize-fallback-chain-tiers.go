package migrations

func init() {
	Register(Migration{
		Timestamp:   "20260126-214400",
		Description: "Normalize tier names in fallback_chain table (tier_v1_free -> free, etc.)",
		Up: []string{
			// Update Clerk Commerce tier names to internal tier names
			`UPDATE fallback_chain SET tier = 'free' WHERE tier = 'tier_v1_free'`,
			`UPDATE fallback_chain SET tier = 'standard' WHERE tier = 'tier_v1_standard'`,
			`UPDATE fallback_chain SET tier = 'pro' WHERE tier = 'tier_v1_pro'`,
			`UPDATE fallback_chain SET tier = 'selfhosted' WHERE tier = 'tier_v1_selfhosted'`,
		},
	})
}
