package migrations

func init() {
	Register(Migration{
		Timestamp:   "20260114-001003",
		Description: "Add error tracking fields to jobs and job_results",
		Up: []string{
			// Job table: add detailed error tracking
			`ALTER TABLE jobs ADD COLUMN error_details TEXT`,
			`ALTER TABLE jobs ADD COLUMN error_category TEXT`,
			`ALTER TABLE jobs ADD COLUMN is_byok INTEGER NOT NULL DEFAULT 0`,
			`ALTER TABLE jobs ADD COLUMN llm_provider TEXT`,
			`ALTER TABLE jobs ADD COLUMN llm_model TEXT`,

			// JobResult table: add detailed error tracking
			`ALTER TABLE job_results ADD COLUMN error_details TEXT`,
			`ALTER TABLE job_results ADD COLUMN error_category TEXT`,
			`ALTER TABLE job_results ADD COLUMN llm_provider TEXT`,
			`ALTER TABLE job_results ADD COLUMN llm_model TEXT`,
			`ALTER TABLE job_results ADD COLUMN is_byok INTEGER NOT NULL DEFAULT 0`,
			`ALTER TABLE job_results ADD COLUMN retry_count INTEGER NOT NULL DEFAULT 0`,

			// Index for querying errors by category (useful for admin dashboard)
			`CREATE INDEX IF NOT EXISTS idx_jobs_error_category ON jobs(error_category) WHERE error_category IS NOT NULL`,
			`CREATE INDEX IF NOT EXISTS idx_job_results_error_category ON job_results(error_category) WHERE error_category IS NOT NULL`,
		},
	})
}
