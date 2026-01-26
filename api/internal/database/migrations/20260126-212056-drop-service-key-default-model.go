package migrations

func init() {
	Register(Migration{
		Timestamp:   "20260126-212056",
		Description: "Drop default_model column from service_keys (no longer used for fallback)",
		Up: []string{
			// SQLite doesn't support DROP COLUMN directly, so we recreate the table
			// Step 1: Create temp table to hold data
			`CREATE TABLE IF NOT EXISTS service_keys_backup (
				id TEXT PRIMARY KEY,
				provider TEXT UNIQUE NOT NULL,
				api_key_encrypted TEXT NOT NULL,
				is_enabled INTEGER NOT NULL DEFAULT 1,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
			// Step 2: Copy existing data (excluding default_model)
			`INSERT INTO service_keys_backup (id, provider, api_key_encrypted, is_enabled, created_at, updated_at)
			SELECT id, provider, api_key_encrypted, is_enabled, created_at, updated_at FROM service_keys`,
			// Step 3: Drop original table
			`DROP TABLE service_keys`,
			// Step 4: Create new table with correct schema
			`CREATE TABLE service_keys (
				id TEXT PRIMARY KEY,
				provider TEXT UNIQUE NOT NULL,
				api_key_encrypted TEXT NOT NULL,
				is_enabled INTEGER NOT NULL DEFAULT 1,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
			// Step 5: Copy data back
			`INSERT INTO service_keys (id, provider, api_key_encrypted, is_enabled, created_at, updated_at)
			SELECT id, provider, api_key_encrypted, is_enabled, created_at, updated_at FROM service_keys_backup`,
			// Step 6: Drop backup table
			`DROP TABLE service_keys_backup`,
		},
	})
}
