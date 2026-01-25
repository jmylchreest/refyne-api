package migrations

func init() {
	Register(Migration{
		Timestamp:   "20260125-134815",
		Description: "Add api_key_rate_limits table for distributed rate limit tracking",
		Up: []string{
			`CREATE TABLE IF NOT EXISTS api_key_rate_limits (
				key_hash TEXT PRIMARY KEY,
				suspended_until TEXT NOT NULL,
				backoff_count INTEGER NOT NULL DEFAULT 1,
				updated_at TEXT NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS idx_rate_limits_suspended ON api_key_rate_limits(suspended_until)`,
		},
	})
}
