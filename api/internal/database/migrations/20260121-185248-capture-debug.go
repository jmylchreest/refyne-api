package migrations

func init() {
	Register(Migration{
		Timestamp:   "20260121-185248",
		Description: "Add capture_debug column to jobs for LLM request debugging",
		Up: []string{
			`ALTER TABLE jobs ADD COLUMN capture_debug INTEGER NOT NULL DEFAULT 0`,
		},
	})
}
