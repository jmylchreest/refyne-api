package migrations

func init() {
	Register(Migration{
		Timestamp:   "20250101-000000",
		Description: "Initial schema",
		Up: []string{
			// API keys - for programmatic API access
			// user_id is a Clerk user ID (no FK constraint since users are in Clerk)
			`CREATE TABLE IF NOT EXISTS api_keys (
				id TEXT PRIMARY KEY,
				user_id TEXT NOT NULL,
				name TEXT NOT NULL,
				key_hash TEXT UNIQUE NOT NULL,
				key_prefix TEXT NOT NULL,
				scopes TEXT,
				last_used_at TEXT,
				expires_at TEXT,
				created_at TEXT NOT NULL,
				revoked_at TEXT
			)`,
			`CREATE INDEX IF NOT EXISTS idx_api_keys_user_id ON api_keys(user_id)`,
			`CREATE INDEX IF NOT EXISTS idx_api_keys_key_hash ON api_keys(key_hash)`,

			// LLM configs - user's LLM provider settings
			`CREATE TABLE IF NOT EXISTS llm_configs (
				id TEXT PRIMARY KEY,
				user_id TEXT UNIQUE NOT NULL,
				provider TEXT NOT NULL,
				api_key_encrypted TEXT,
				base_url TEXT,
				model TEXT,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,

			// Jobs - extraction and crawl jobs
			`CREATE TABLE IF NOT EXISTS jobs (
				id TEXT PRIMARY KEY,
				user_id TEXT NOT NULL,
				type TEXT NOT NULL,
				status TEXT NOT NULL DEFAULT 'pending',
				url TEXT NOT NULL,
				schema_json TEXT NOT NULL,
				crawl_options_json TEXT,
				result_json TEXT,
				error_message TEXT,
				page_count INTEGER DEFAULT 0,
				token_usage_input INTEGER DEFAULT 0,
				token_usage_output INTEGER DEFAULT 0,
				cost_credits INTEGER DEFAULT 0,
				webhook_url TEXT,
				webhook_status TEXT,
				webhook_attempts INTEGER DEFAULT 0,
				started_at TEXT,
				completed_at TEXT,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS idx_jobs_user_id ON jobs(user_id)`,
			`CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status)`,
			`CREATE INDEX IF NOT EXISTS idx_jobs_created_at ON jobs(created_at)`,

			// Job results - individual page results for crawl jobs
			`CREATE TABLE IF NOT EXISTS job_results (
				id TEXT PRIMARY KEY,
				job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
				url TEXT NOT NULL,
				data_json TEXT,
				error_message TEXT,
				token_usage_input INTEGER DEFAULT 0,
				token_usage_output INTEGER DEFAULT 0,
				fetch_duration_ms INTEGER,
				extract_duration_ms INTEGER,
				created_at TEXT NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS idx_job_results_job_id ON job_results(job_id)`,

			// Schema snapshots - versioned schemas for deduplication and audit
			// Must be created before usage_insights which references it
			`CREATE TABLE IF NOT EXISTS schema_snapshots (
				id TEXT PRIMARY KEY,
				user_id TEXT NOT NULL,
				hash TEXT NOT NULL,
				schema_json TEXT NOT NULL,
				name TEXT,
				version INTEGER NOT NULL DEFAULT 1,
				usage_count INTEGER NOT NULL DEFAULT 1,
				created_at TEXT NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS idx_schema_user_hash ON schema_snapshots(user_id, hash)`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_schema_user_hash_unique ON schema_snapshots(user_id, hash)`,

			// Usage records - LEAN table for fast billing queries
			`CREATE TABLE IF NOT EXISTS usage_records (
				id TEXT PRIMARY KEY,
				user_id TEXT NOT NULL,
				job_id TEXT REFERENCES jobs(id) ON DELETE SET NULL,
				date TEXT NOT NULL,
				type TEXT NOT NULL,
				status TEXT NOT NULL DEFAULT 'success',
				total_charged_usd REAL NOT NULL DEFAULT 0,
				is_byok INTEGER NOT NULL DEFAULT 0,
				created_at TEXT NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS idx_usage_user_date ON usage_records(user_id, date)`,
			`CREATE INDEX IF NOT EXISTS idx_usage_date ON usage_records(date)`,

			// Usage insights - RICH table for analytics (1:1 with usage_records)
			`CREATE TABLE IF NOT EXISTS usage_insights (
				id TEXT PRIMARY KEY,
				usage_id TEXT NOT NULL REFERENCES usage_records(id) ON DELETE CASCADE,
				target_url TEXT,
				schema_id TEXT REFERENCES schema_snapshots(id) ON DELETE SET NULL,
				crawl_config_json TEXT,
				error_message TEXT,
				error_code TEXT,
				tokens_input INTEGER NOT NULL DEFAULT 0,
				tokens_output INTEGER NOT NULL DEFAULT 0,
				llm_cost_usd REAL NOT NULL DEFAULT 0,
				markup_rate REAL NOT NULL DEFAULT 0,
				markup_usd REAL NOT NULL DEFAULT 0,
				llm_provider TEXT,
				llm_model TEXT,
				generation_id TEXT,
				byok_provider TEXT,
				pages_attempted INTEGER NOT NULL DEFAULT 0,
				pages_successful INTEGER NOT NULL DEFAULT 0,
				fetch_duration_ms INTEGER NOT NULL DEFAULT 0,
				extract_duration_ms INTEGER NOT NULL DEFAULT 0,
				total_duration_ms INTEGER NOT NULL DEFAULT 0,
				request_id TEXT,
				user_agent TEXT,
				ip_country TEXT,
				created_at TEXT NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS idx_insights_usage ON usage_insights(usage_id)`,

			// User balances - USD balance for API usage
			`CREATE TABLE IF NOT EXISTS user_balances (
				user_id TEXT PRIMARY KEY,
				balance_usd REAL NOT NULL DEFAULT 0,
				lifetime_added REAL NOT NULL DEFAULT 0,
				lifetime_spent REAL NOT NULL DEFAULT 0,
				updated_at TEXT NOT NULL
			)`,

			// Credit transactions - full audit trail for all credit movements
			`CREATE TABLE IF NOT EXISTS credit_transactions (
				id TEXT PRIMARY KEY,
				user_id TEXT NOT NULL,
				type TEXT NOT NULL,
				amount_usd REAL NOT NULL,
				balance_after REAL NOT NULL,
				expires_at TEXT,
				is_expired INTEGER NOT NULL DEFAULT 0,
				stripe_payment_id TEXT UNIQUE,
				job_id TEXT REFERENCES jobs(id) ON DELETE SET NULL,
				description TEXT,
				created_at TEXT NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS idx_credit_tx_user ON credit_transactions(user_id)`,
			`CREATE INDEX IF NOT EXISTS idx_credit_tx_user_expires ON credit_transactions(user_id, expires_at)`,
			`CREATE INDEX IF NOT EXISTS idx_credit_tx_stripe ON credit_transactions(stripe_payment_id)`,

			// Telemetry events - analytics and self-hosted telemetry ingestion
			`CREATE TABLE IF NOT EXISTS telemetry_events (
				id TEXT PRIMARY KEY,
				user_id TEXT,
				event_type TEXT NOT NULL,
				event_data TEXT,
				created_at TEXT NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS idx_telemetry_type_date ON telemetry_events(event_type, created_at)`,

			// Licenses - self-hosted license management
			`CREATE TABLE IF NOT EXISTS licenses (
				id TEXT PRIMARY KEY,
				license_key TEXT UNIQUE NOT NULL,
				organization_name TEXT NOT NULL,
				email TEXT NOT NULL,
				tier TEXT NOT NULL,
				max_users INTEGER DEFAULT 1,
				features TEXT,
				issued_at TEXT NOT NULL,
				expires_at TEXT,
				revoked_at TEXT,
				created_at TEXT NOT NULL
			)`,

			// Service keys - admin-configured LLM provider API keys
			// Used for free tier users and non-BYOK users
			`CREATE TABLE IF NOT EXISTS service_keys (
				id TEXT PRIMARY KEY,
				provider TEXT UNIQUE NOT NULL,
				api_key_encrypted TEXT NOT NULL,
				default_model TEXT NOT NULL,
				is_enabled INTEGER NOT NULL DEFAULT 1,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,

			// Fallback chain - ordered list of provider:model pairs for LLM fallback
			// Extraction service tries each in order until one succeeds
			// tier is NULL for the default chain, or a specific tier name (free, pro, enterprise)
			`CREATE TABLE IF NOT EXISTS fallback_chain (
				id TEXT PRIMARY KEY,
				tier TEXT,
				position INTEGER NOT NULL,
				provider TEXT NOT NULL,
				model TEXT NOT NULL,
				is_enabled INTEGER NOT NULL DEFAULT 1,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_fallback_chain_tier_position ON fallback_chain(COALESCE(tier, ''), position)`,
		},
	})
}
