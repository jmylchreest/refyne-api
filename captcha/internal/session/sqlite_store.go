// Package session provides session management for persistent browser instances.
package session

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/go-rod/rod/lib/proto"
	_ "modernc.org/sqlite" // Pure Go SQLite driver
)

// SQLiteStore provides persistent storage for session state.
type SQLiteStore struct {
	db       *sql.DB
	logger   *slog.Logger
	isMemory bool // True if using in-memory database
}

// PersistedSession represents session data that can be persisted.
type PersistedSession struct {
	ID           string                 `json:"id"`
	UserID       string                 `json:"user_id"`
	UserAgent    string                 `json:"user_agent"`
	Proxy        string                 `json:"proxy"`
	CreatedAt    time.Time              `json:"created_at"`
	LastUsedAt   time.Time              `json:"last_used_at"`
	RequestCount int                    `json:"request_count"`
	Cookies      []*proto.NetworkCookie `json:"cookies"`
}

// NewSQLiteStore creates a new SQLite-backed session store.
func NewSQLiteStore(dbPath string, logger *slog.Logger) (*SQLiteStore, error) {
	var connStr string
	isMemory := dbPath == ":memory:"

	if isMemory {
		// In-memory database - no WAL mode, use shared cache for same connection
		connStr = "file::memory:?cache=shared&_timeout=5000&_busy_timeout=5000"
		logger.Info("using in-memory SQLite database")
	} else {
		// File-based database - ensure directory exists and use WAL mode
		dir := filepath.Dir(dbPath)
		if dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return nil, fmt.Errorf("failed to create directory: %w", err)
			}
		}
		connStr = dbPath + "?_journal=WAL&_timeout=5000&_busy_timeout=5000"
	}

	db, err := sql.Open("sqlite", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Set connection pool settings
	db.SetMaxOpenConns(1) // SQLite is single-writer
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	store := &SQLiteStore{
		db:       db,
		logger:   logger,
		isMemory: isMemory,
	}

	// Run migrations
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	logger.Info("SQLite session store initialized", "path", dbPath, "in_memory", isMemory)
	return store, nil
}

// migrate creates the necessary tables.
func (s *SQLiteStore) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL DEFAULT '',
		user_agent TEXT NOT NULL DEFAULT '',
		proxy TEXT NOT NULL DEFAULT '',
		cookies_json TEXT NOT NULL DEFAULT '[]',
		request_count INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL,
		last_used_at TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
	CREATE INDEX IF NOT EXISTS idx_sessions_last_used_at ON sessions(last_used_at);
	`

	_, err := s.db.Exec(schema)
	return err
}

// Save persists a session to the database.
func (s *SQLiteStore) Save(session *PersistedSession) error {
	cookiesJSON, err := json.Marshal(session.Cookies)
	if err != nil {
		return fmt.Errorf("failed to marshal cookies: %w", err)
	}

	query := `
	INSERT INTO sessions (id, user_id, user_agent, proxy, cookies_json, request_count, created_at, last_used_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		user_agent = excluded.user_agent,
		proxy = excluded.proxy,
		cookies_json = excluded.cookies_json,
		request_count = excluded.request_count,
		last_used_at = excluded.last_used_at
	`

	_, err = s.db.Exec(query,
		session.ID,
		session.UserID,
		session.UserAgent,
		session.Proxy,
		string(cookiesJSON),
		session.RequestCount,
		session.CreatedAt.Format(time.RFC3339),
		session.LastUsedAt.Format(time.RFC3339),
	)

	if err != nil {
		return fmt.Errorf("failed to save session: %w", err)
	}

	s.logger.Debug("session persisted", "id", session.ID)
	return nil
}

// Load retrieves a session from the database.
func (s *SQLiteStore) Load(id string) (*PersistedSession, error) {
	query := `
	SELECT id, user_id, user_agent, proxy, cookies_json, request_count, created_at, last_used_at
	FROM sessions
	WHERE id = ?
	`

	var session PersistedSession
	var cookiesJSON string
	var createdAtStr, lastUsedAtStr string

	err := s.db.QueryRow(query, id).Scan(
		&session.ID,
		&session.UserID,
		&session.UserAgent,
		&session.Proxy,
		&cookiesJSON,
		&session.RequestCount,
		&createdAtStr,
		&lastUsedAtStr,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil // Not found
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load session: %w", err)
	}

	// Parse timestamps
	session.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
	session.LastUsedAt, _ = time.Parse(time.RFC3339, lastUsedAtStr)

	// Parse cookies
	if err := json.Unmarshal([]byte(cookiesJSON), &session.Cookies); err != nil {
		s.logger.Warn("failed to unmarshal cookies", "id", id, "error", err)
		session.Cookies = nil
	}

	return &session, nil
}

// Delete removes a session from the database.
func (s *SQLiteStore) Delete(id string) error {
	_, err := s.db.Exec("DELETE FROM sessions WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}
	s.logger.Debug("session deleted from store", "id", id)
	return nil
}

// ListByUser returns all sessions for a given user.
func (s *SQLiteStore) ListByUser(userID string) ([]*PersistedSession, error) {
	query := `
	SELECT id, user_id, user_agent, proxy, cookies_json, request_count, created_at, last_used_at
	FROM sessions
	WHERE user_id = ?
	ORDER BY last_used_at DESC
	`

	rows, err := s.db.Query(query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*PersistedSession
	for rows.Next() {
		var session PersistedSession
		var cookiesJSON string
		var createdAtStr, lastUsedAtStr string

		if err := rows.Scan(
			&session.ID,
			&session.UserID,
			&session.UserAgent,
			&session.Proxy,
			&cookiesJSON,
			&session.RequestCount,
			&createdAtStr,
			&lastUsedAtStr,
		); err != nil {
			return nil, fmt.Errorf("failed to scan session: %w", err)
		}

		session.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
		session.LastUsedAt, _ = time.Parse(time.RFC3339, lastUsedAtStr)
		if err := json.Unmarshal([]byte(cookiesJSON), &session.Cookies); err != nil {
			session.Cookies = nil
		}

		sessions = append(sessions, &session)
	}

	return sessions, nil
}

// ListAll returns all sessions.
func (s *SQLiteStore) ListAll() ([]*PersistedSession, error) {
	query := `
	SELECT id, user_id, user_agent, proxy, cookies_json, request_count, created_at, last_used_at
	FROM sessions
	ORDER BY last_used_at DESC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*PersistedSession
	for rows.Next() {
		var session PersistedSession
		var cookiesJSON string
		var createdAtStr, lastUsedAtStr string

		if err := rows.Scan(
			&session.ID,
			&session.UserID,
			&session.UserAgent,
			&session.Proxy,
			&cookiesJSON,
			&session.RequestCount,
			&createdAtStr,
			&lastUsedAtStr,
		); err != nil {
			return nil, fmt.Errorf("failed to scan session: %w", err)
		}

		session.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
		session.LastUsedAt, _ = time.Parse(time.RFC3339, lastUsedAtStr)
		if err := json.Unmarshal([]byte(cookiesJSON), &session.Cookies); err != nil {
			session.Cookies = nil
		}

		sessions = append(sessions, &session)
	}

	return sessions, nil
}

// CleanupOlderThan removes sessions that haven't been used since the given time.
// If rows were deleted, also vacuums the database to reclaim space.
func (s *SQLiteStore) CleanupOlderThan(ctx context.Context, threshold time.Time) (int64, error) {
	result, err := s.db.ExecContext(ctx,
		"DELETE FROM sessions WHERE last_used_at < ?",
		threshold.Format(time.RFC3339),
	)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup sessions: %w", err)
	}

	count, _ := result.RowsAffected()
	if count > 0 {
		s.logger.Info("cleaned up old sessions", "count", count)
		// Vacuum to reclaim space
		if err := s.Vacuum(); err != nil {
			s.logger.Warn("failed to vacuum after cleanup", "error", err)
		}
	}
	return count, nil
}

// Vacuum reclaims unused space in the database.
// Should be called periodically after deleting many sessions.
func (s *SQLiteStore) Vacuum() error {
	_, err := s.db.Exec("VACUUM")
	if err != nil {
		return fmt.Errorf("failed to vacuum database: %w", err)
	}
	s.logger.Debug("database vacuumed")
	return nil
}

// Close closes the database connection.
// Performs a WAL checkpoint first to ensure all data is flushed to the main DB file.
func (s *SQLiteStore) Close() error {
	// Checkpoint WAL to ensure all data is written to main database file
	// (only for file-based databases, not in-memory)
	if !s.isMemory {
		if _, err := s.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
			s.logger.Warn("failed to checkpoint WAL before close", "error", err)
		}
	}
	s.logger.Debug("SQLite store closing", "in_memory", s.isMemory)
	return s.db.Close()
}
