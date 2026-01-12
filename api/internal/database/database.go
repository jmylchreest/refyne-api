// Package database handles database connections and migrations.
package database

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/tursodatabase/go-libsql"

	"github.com/jmylchreest/refyne-api/internal/database/migrations"
)

// New creates a new database connection using libsql.
// Supports:
//   - Local files: DATABASE_URL="file:path/to/db.sqlite" (no Turso config needed)
//   - Embedded replica: set TURSO_URL + TURSO_AUTH_TOKEN for sync with Turso cloud
//   - Local libsql server: run `turso dev` and use DATABASE_URL="http://127.0.0.1:8080"
func New(dsn string) (*sql.DB, error) {
	tursoURL := os.Getenv("TURSO_URL")
	tursoToken := os.Getenv("TURSO_AUTH_TOKEN")

	var db *sql.DB

	if tursoURL != "" && tursoToken != "" {
		// Embedded replica mode: local file synced with remote Turso
		dbPath := strings.TrimPrefix(dsn, "file:")
		dbPath = strings.Split(dbPath, "?")[0] // Remove query params

		connector, err := libsql.NewEmbeddedReplicaConnector(dbPath, tursoURL,
			libsql.WithAuthToken(tursoToken),
			libsql.WithReadYourWrites(true),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create Turso connector: %w", err)
		}
		db = sql.OpenDB(connector)
	} else {
		// Local mode: file or http URL
		var err error
		db, err = sql.Open("libsql", dsn)
		if err != nil {
			return nil, fmt.Errorf("failed to open database: %w", err)
		}
	}

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return db, nil
}

// Migrate runs database migrations.
// Note: User management, OAuth, sessions, subscriptions, and credits are handled by Clerk.
// The user_id fields reference Clerk user IDs (e.g., "user_xxx").
func Migrate(db *sql.DB) error {
	return MigrateWithLogger(db, nil)
}

// MigrateWithLogger runs database migrations with a custom logger.
func MigrateWithLogger(db *sql.DB, logger *slog.Logger) error {
	return migrations.Run(db, logger)
}

// GetAppliedMigrations returns information about applied migrations.
func GetAppliedMigrations(db *sql.DB) ([]migrations.AppliedMigration, error) {
	return migrations.GetAppliedMigrations(db)
}

// GetPendingMigrations returns migrations that haven't been applied yet.
func GetPendingMigrations(db *sql.DB) ([]migrations.Migration, error) {
	return migrations.GetPendingMigrations(db)
}
