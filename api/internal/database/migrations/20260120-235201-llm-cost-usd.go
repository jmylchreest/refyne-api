package migrations

func init() {
	Register(Migration{
		Timestamp:   "20260120-235201",
		Description: "Add llm_cost_usd column to jobs for tracking actual LLM provider cost",
		Up: []string{
			// Add llm_cost_usd column to track the actual cost from the LLM provider
			// This is distinct from cost_usd which is what we charge the user (0 for BYOK)
			`ALTER TABLE jobs ADD COLUMN llm_cost_usd REAL NOT NULL DEFAULT 0`,
		},
	})
}
