package migrations

func init() {
	Register(Migration{
		Timestamp:   "20260114-002655",
		Description: "Add temperature and max_tokens to fallback chains",
		Up: []string{
			// Add temperature and max_tokens to system fallback_chain
			`ALTER TABLE fallback_chain ADD COLUMN temperature REAL`,
			`ALTER TABLE fallback_chain ADD COLUMN max_tokens INTEGER`,

			// Add temperature and max_tokens to user_fallback_chain
			`ALTER TABLE user_fallback_chain ADD COLUMN temperature REAL`,
			`ALTER TABLE user_fallback_chain ADD COLUMN max_tokens INTEGER`,
		},
	})
}
