package migrations

func init() {
	Register(Migration{
		Timestamp:   "20260114-134511",
		Description: "Add strict_mode to fallback chains",
		Up: []string{
			// Add strict_mode to system fallback_chain (NULL = use model default)
			`ALTER TABLE fallback_chain ADD COLUMN strict_mode INTEGER`,

			// Add strict_mode to user_fallback_chain
			`ALTER TABLE user_fallback_chain ADD COLUMN strict_mode INTEGER`,
		},
	})
}
