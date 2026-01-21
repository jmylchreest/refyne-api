package migrations

func init() {
	Register(Migration{
		Timestamp:   "20260121-001736",
		Description: "Add discovery_method column to jobs for tracking URL discovery method",
		Up: []string{
			`ALTER TABLE jobs ADD COLUMN discovery_method TEXT NOT NULL DEFAULT ''`,
		},
	})
}
