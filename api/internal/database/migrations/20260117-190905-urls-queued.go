package migrations

func init() {
	Register(Migration{
		Timestamp:   "20260117-190905",
		Description: "Add urls_queued to jobs for progress tracking",
		Up: []string{
			`ALTER TABLE jobs ADD COLUMN urls_queued INTEGER NOT NULL DEFAULT 0`,
		},
	})
}
