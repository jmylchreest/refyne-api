package migrations

func init() {
	Register(Migration{
		Timestamp:   "20260118-150906",
		Description: "Add deleted_users table to track deleted accounts for audit purposes",
		Up: []string{
			`CREATE TABLE IF NOT EXISTS deleted_users (
				user_id TEXT PRIMARY KEY,
				deleted_at TEXT NOT NULL,
				reason TEXT
			)`,
			// Index for querying by deletion date
			`CREATE INDEX IF NOT EXISTS idx_deleted_users_deleted_at ON deleted_users(deleted_at)`,
		},
	})
}
