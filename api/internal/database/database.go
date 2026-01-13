// Package database handles database connections and migrations.
package database

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"runtime"
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
	isTurso := tursoURL != "" && tursoToken != ""

	if isTurso {
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

		// Turso handles write serialization on their servers
		// Use generous connection pool for concurrent operations
		db.SetMaxOpenConns(25)
		db.SetMaxIdleConns(10)
	} else {
		// Local mode: file or http URL
		var err error
		db, err = sql.Open("libsql", dsn)
		if err != nil {
			return nil, fmt.Errorf("failed to open database: %w", err)
		}

		// Local SQLite: reads can be parallel, writes are serialized
		// Use number of CPUs for reads, Go's sql.DB handles write serialization
		maxConns := runtime.NumCPU()
		if maxConns < 4 {
			maxConns = 4
		}
		db.SetMaxOpenConns(maxConns)
		db.SetMaxIdleConns(maxConns / 2)
	}

	// Configure SQLite PRAGMAs for performance and concurrency
	// Note: PRAGMAs that return values must use QueryRow, not Exec
	pragmas := []struct {
		query string
		name  string
	}{
		{"PRAGMA journal_mode = WAL", "WAL mode"},           // Better concurrency
		{"PRAGMA busy_timeout = 30000", "busy timeout"},     // Wait 30s on lock
		{"PRAGMA foreign_keys = ON", "foreign keys"},        // Referential integrity
		{"PRAGMA synchronous = NORMAL", "synchronous mode"}, // Safe with WAL, faster
		{"PRAGMA temp_store = memory", "temp store"},        // Faster temp tables
	}

	for _, p := range pragmas {
		// Use QueryRow to handle PRAGMAs that return values
		var result string
		if err := db.QueryRow(p.query).Scan(&result); err != nil {
			// Some PRAGMAs don't return values, try Exec as fallback
			if _, execErr := db.Exec(p.query); execErr != nil {
				return nil, fmt.Errorf("failed to set %s: %w", p.name, execErr)
			}
		}
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

// GetLatestSchemaVersion returns the latest applied migration version.
func GetLatestSchemaVersion(db *sql.DB) (string, error) {
	return migrations.GetLatestVersion(db)
}

// GetMigrationCount returns the total number of applied migrations.
func GetMigrationCount(db *sql.DB) (int, error) {
	return migrations.GetMigrationCount(db)
}
