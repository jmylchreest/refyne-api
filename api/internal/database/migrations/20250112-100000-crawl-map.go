package migrations

func init() {
	Register(Migration{
		Timestamp:   "20250112-100000",
		Description: "Add crawl map fields to job_results",
		Up: []string{
			// Add parent_url - which URL discovered this one (NULL for seed URL)
			`ALTER TABLE job_results ADD COLUMN parent_url TEXT`,

			// Add depth - level from seed URL (0 for seed, 1 for first level, etc.)
			`ALTER TABLE job_results ADD COLUMN depth INTEGER NOT NULL DEFAULT 0`,

			// Add crawl_status - tracking state: pending, crawling, completed, failed, skipped
			`ALTER TABLE job_results ADD COLUMN crawl_status TEXT NOT NULL DEFAULT 'completed'`,

			// Add discovered_at - when this URL was found (may differ from created_at for queued URLs)
			`ALTER TABLE job_results ADD COLUMN discovered_at TEXT`,

			// Add completed_at - when crawling/extraction finished for this URL
			`ALTER TABLE job_results ADD COLUMN completed_at TEXT`,

			// Index for crawl map queries - find all results for a job ordered by depth
			`CREATE INDEX IF NOT EXISTS idx_job_results_job_depth ON job_results(job_id, depth)`,

			// Index for finding pending URLs to crawl
			`CREATE INDEX IF NOT EXISTS idx_job_results_crawl_status ON job_results(job_id, crawl_status)`,
		},
	})
}
