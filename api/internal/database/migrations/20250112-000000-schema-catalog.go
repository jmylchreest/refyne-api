package migrations

func init() {
	Register(Migration{
		Timestamp:   "20250112-000000",
		Description: "Schema catalog and saved sites",
		Up: []string{
			// Schema catalog - platform and user-created extraction schemas
			`CREATE TABLE IF NOT EXISTS schema_catalog (
				id TEXT PRIMARY KEY,
				organization_id TEXT,
				user_id TEXT,
				name TEXT NOT NULL,
				description TEXT,
				category TEXT,
				schema_yaml TEXT NOT NULL,
				visibility TEXT NOT NULL DEFAULT 'private',
				is_platform INTEGER NOT NULL DEFAULT 0,
				tags TEXT,
				usage_count INTEGER NOT NULL DEFAULT 0,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS idx_schema_catalog_visibility ON schema_catalog(visibility)`,
			`CREATE INDEX IF NOT EXISTS idx_schema_catalog_user_id ON schema_catalog(user_id)`,
			`CREATE INDEX IF NOT EXISTS idx_schema_catalog_org_id ON schema_catalog(organization_id)`,
			`CREATE INDEX IF NOT EXISTS idx_schema_catalog_category ON schema_catalog(category)`,
			`CREATE INDEX IF NOT EXISTS idx_schema_catalog_is_platform ON schema_catalog(is_platform)`,

			// Saved sites - user's analyzed and saved site configurations
			`CREATE TABLE IF NOT EXISTS saved_sites (
				id TEXT PRIMARY KEY,
				user_id TEXT NOT NULL,
				organization_id TEXT,
				url TEXT NOT NULL,
				domain TEXT NOT NULL,
				name TEXT,
				analysis_result TEXT,
				default_schema_id TEXT REFERENCES schema_catalog(id) ON DELETE SET NULL,
				follow_patterns TEXT,
				fetch_mode TEXT DEFAULT 'auto',
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS idx_saved_sites_user_id ON saved_sites(user_id)`,
			`CREATE INDEX IF NOT EXISTS idx_saved_sites_org_id ON saved_sites(organization_id)`,
			`CREATE INDEX IF NOT EXISTS idx_saved_sites_domain ON saved_sites(domain)`,
		},
	})
}
