package migrations

func init() {
	Register(Migration{
		Timestamp:   "20260117-210000",
		Description: "Add webhooks and webhook_deliveries tables",
		Up: []string{
			// Webhooks table - user-defined webhook endpoints
			`CREATE TABLE IF NOT EXISTS webhooks (
				id TEXT PRIMARY KEY,
				user_id TEXT NOT NULL,
				name TEXT NOT NULL,
				url TEXT NOT NULL,
				secret_encrypted TEXT,
				events TEXT NOT NULL DEFAULT '["*"]',
				headers_json TEXT,
				is_active INTEGER NOT NULL DEFAULT 1,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL,
				UNIQUE(user_id, name)
			)`,
			`CREATE INDEX IF NOT EXISTS idx_webhooks_user_id ON webhooks(user_id)`,
			`CREATE INDEX IF NOT EXISTS idx_webhooks_user_active ON webhooks(user_id, is_active)`,

			// Webhook deliveries table - tracks all webhook delivery attempts
			`CREATE TABLE IF NOT EXISTS webhook_deliveries (
				id TEXT PRIMARY KEY,
				webhook_id TEXT,
				job_id TEXT NOT NULL,
				event_type TEXT NOT NULL,
				url TEXT NOT NULL,
				payload_json TEXT NOT NULL,
				request_headers_json TEXT,
				status_code INTEGER,
				response_body TEXT,
				response_time_ms INTEGER,
				status TEXT NOT NULL DEFAULT 'pending',
				error_message TEXT,
				attempt_number INTEGER NOT NULL DEFAULT 1,
				max_attempts INTEGER NOT NULL DEFAULT 3,
				next_retry_at TEXT,
				created_at TEXT NOT NULL,
				delivered_at TEXT,
				FOREIGN KEY (webhook_id) REFERENCES webhooks(id) ON DELETE SET NULL,
				FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE CASCADE
			)`,
			`CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_job_id ON webhook_deliveries(job_id)`,
			`CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_webhook_id ON webhook_deliveries(webhook_id)`,
			`CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_status ON webhook_deliveries(status)`,
			`CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_next_retry ON webhook_deliveries(status, next_retry_at)`,
		},
	})
}
