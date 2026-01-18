package migrations

func init() {
	Register(Migration{
		Timestamp:   "20260118-210215",
		Description: "Add LLM configs to jobs table for resolved config storage",
		Up: []string{
			// Add llm_configs_json column to store resolved LLM config chain at job creation time.
			// This captures the exact provider/model/key configuration to use, eliminating the
			// need to re-resolve at processing time and decoupling from user's current settings.
			`ALTER TABLE jobs ADD COLUMN llm_configs_json TEXT NOT NULL DEFAULT '[]'`,
			// Add tier column to store user's subscription tier at job creation time.
			// This is needed for billing calculations and ensures the tier snapshot is preserved.
			`ALTER TABLE jobs ADD COLUMN tier TEXT NOT NULL DEFAULT 'free'`,
		},
	})
}
