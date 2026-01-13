// Package migrations handles database schema migrations.
// Migrations are versioned using timestamps (YYYYMMDD-HHmmss format) and
// tracked in the database to ensure each migration runs exactly once.
//
// Migration files should be named: YYYYMMDD-HHmmss-description.go
// Example: 20260111-143022-add-tier-support.go
package migrations

import (
	"database/sql"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"
)

// Migration represents a single database migration.
type Migration struct {
	// Timestamp in YYYYMMDD-HHmmss format (e.g., "20260111-143022")
	// Used for ordering and tracking applied migrations.
	Timestamp   string
	Description string   // Human-readable description
	Up          []string // SQL statements to run
}

// registry holds all registered migrations.
var registry []Migration

// Register adds a migration to the registry.
// Called by init() functions in individual migration files.
func Register(m Migration) {
	registry = append(registry, m)
}

// Run executes all pending migrations.
// Creates a migrations tracking table if it doesn't exist.
func Run(db *sql.DB, logger *slog.Logger) error {
	if logger == nil {
		logger = slog.Default()
	}

	// Create migrations tracking table if it doesn't exist (version is a timestamp string)
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			description TEXT NOT NULL,
			applied_at TEXT NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	// Get applied migrations
	applied, err := getAppliedVersions(db)
	if err != nil {
		return fmt.Errorf("failed to get applied migrations: %w", err)
	}

	// Sort migrations by timestamp
	sort.Slice(registry, func(i, j int) bool {
		return registry[i].Timestamp < registry[j].Timestamp
	})

	// Run pending migrations
	for _, m := range registry {
		if applied[m.Timestamp] {
			continue
		}

		logger.Info("running migration", "timestamp", m.Timestamp, "description", m.Description)

		if err := runMigration(db, m); err != nil {
			return fmt.Errorf("migration %s (%s) failed: %w", m.Timestamp, m.Description, err)
		}

		logger.Info("migration completed", "timestamp", m.Timestamp)
	}

	return nil
}

// getAppliedVersions returns a map of applied migration timestamps.
func getAppliedVersions(db *sql.DB) (map[string]bool, error) {
	rows, err := db.Query("SELECT version FROM schema_migrations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		applied[version] = true
	}

	return applied, rows.Err()
}

// runMigration executes a single migration within a transaction.
func runMigration(db *sql.DB, m Migration) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, stmt := range m.Up {
		if _, err := tx.Exec(stmt); err != nil {
			// Handle expected errors gracefully
			if isExpectedError(err, stmt) {
				continue
			}
			return fmt.Errorf("failed to execute statement: %w\n%s", err, stmt)
		}
	}

	// Record migration
	if _, err := tx.Exec(
		"INSERT INTO schema_migrations (version, description, applied_at) VALUES (?, ?, ?)",
		m.Timestamp, m.Description, time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		return fmt.Errorf("failed to record migration: %w", err)
	}

	return tx.Commit()
}

// isExpectedError checks if an error is expected and can be ignored.
func isExpectedError(err error, stmt string) bool {
	errStr := err.Error()

	// Duplicate column from ALTER TABLE ADD COLUMN
	if strings.Contains(errStr, "duplicate column") {
		return true
	}

	// Index already exists
	if strings.Contains(errStr, "already exists") && strings.Contains(stmt, "CREATE INDEX") {
		return true
	}

	return false
}

// GetAppliedMigrations returns info about applied migrations.
func GetAppliedMigrations(db *sql.DB) ([]AppliedMigration, error) {
	rows, err := db.Query("SELECT version, description, applied_at FROM schema_migrations ORDER BY version")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var migrations []AppliedMigration
	for rows.Next() {
		var m AppliedMigration
		var appliedAt string
		if err := rows.Scan(&m.Timestamp, &m.Description, &appliedAt); err != nil {
			return nil, err
		}
		m.AppliedAt, _ = time.Parse(time.RFC3339, appliedAt)
		migrations = append(migrations, m)
	}

	return migrations, rows.Err()
}

// AppliedMigration represents a migration that has been applied.
type AppliedMigration struct {
	Timestamp   string
	Description string
	AppliedAt   time.Time
}

// GetPendingMigrations returns migrations that haven't been applied yet.
func GetPendingMigrations(db *sql.DB) ([]Migration, error) {
	applied, err := getAppliedVersions(db)
	if err != nil {
		return nil, err
	}

	var pending []Migration
	for _, m := range registry {
		if !applied[m.Timestamp] {
			pending = append(pending, m)
		}
	}

	sort.Slice(pending, func(i, j int) bool {
		return pending[i].Timestamp < pending[j].Timestamp
	})

	return pending, nil
}

// GetLatestVersion returns the latest applied migration version.
// Returns empty string if no migrations have been applied.
func GetLatestVersion(db *sql.DB) (string, error) {
	var version sql.NullString
	err := db.QueryRow("SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1").Scan(&version)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return version.String, nil
}

// GetMigrationCount returns the total number of applied migrations.
func GetMigrationCount(db *sql.DB) (int, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}
