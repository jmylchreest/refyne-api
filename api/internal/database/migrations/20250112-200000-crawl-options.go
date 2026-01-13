package migrations

func init() {
	Register(Migration{
		Timestamp:   "20250112-200000",
		Description: "Add crawl_options column to saved_sites",
		Up: []string{
			// Add crawl_options column to saved_sites table
			`ALTER TABLE saved_sites ADD COLUMN crawl_options TEXT`,
		},
	})
}
