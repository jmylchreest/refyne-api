package migrations

func init() {
	Register(Migration{
		Timestamp:   "20260112-230137",
		Description: "User LLM provider keys and fallback chain",
		Up: []string{
			// User service keys - user-configured LLM provider API keys
			// Similar to admin service_keys but per-user, no model (models are in chain)
			`CREATE TABLE IF NOT EXISTS user_service_keys (
				id TEXT PRIMARY KEY,
				user_id TEXT NOT NULL,
				provider TEXT NOT NULL,
				api_key_encrypted TEXT,
				base_url TEXT,
				is_enabled INTEGER NOT NULL DEFAULT 1,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL,
				UNIQUE(user_id, provider)
			)`,
			`CREATE INDEX IF NOT EXISTS idx_user_service_keys_user_id ON user_service_keys(user_id)`,

			// User fallback chain - personal extraction provider/model order
			// Similar to admin fallback_chain but per-user instead of per-tier
			`CREATE TABLE IF NOT EXISTS user_fallback_chain (
				id TEXT PRIMARY KEY,
				user_id TEXT NOT NULL,
				position INTEGER NOT NULL,
				provider TEXT NOT NULL,
				model TEXT NOT NULL,
				is_enabled INTEGER NOT NULL DEFAULT 1,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL,
				UNIQUE(user_id, position)
			)`,
			`CREATE INDEX IF NOT EXISTS idx_user_fallback_chain_user_id ON user_fallback_chain(user_id)`,
		},
	})
}
