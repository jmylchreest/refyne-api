package migrations

func init() {
	Register(Migration{
		Timestamp:   "20260117-200000",
		Description: "Add cost_usd column to jobs (replacing cost_credits)",
		Up: []string{
			// Add the new cost_usd column (REAL for float64 in SQLite)
			`ALTER TABLE jobs ADD COLUMN cost_usd REAL NOT NULL DEFAULT 0`,
			// Migrate extract job data: cost_credits was stored as cents, convert to USD
			// Crawl jobs had token-based "credits" which are not meaningful, so leave as 0
			`UPDATE jobs SET cost_usd = cost_credits / 100.0 WHERE type = 'extract' AND cost_credits > 0`,
		},
	})
}
