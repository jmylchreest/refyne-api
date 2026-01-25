package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

// APIKeyRateLimit represents a rate limit entry for an API key.
type APIKeyRateLimit struct {
	KeyHash        string    // SHA256 hash of the API key (for security)
	SuspendedUntil time.Time // When the suspension expires
	BackoffCount   int       // Consecutive rate limit count (for exponential backoff)
	UpdatedAt      time.Time
}

// RateLimitRepository handles API key rate limit persistence.
type RateLimitRepository interface {
	// IsSuspended checks if an API key is currently suspended.
	IsSuspended(ctx context.Context, apiKey string) (bool, error)
	// MarkRateLimited marks an API key as rate limited with exponential backoff.
	// Returns the backoff duration.
	MarkRateLimited(ctx context.Context, apiKey string) (time.Duration, error)
	// ClearSuspension removes the suspension for an API key.
	ClearSuspension(ctx context.Context, apiKey string) error
	// CleanupExpired removes expired rate limit entries.
	CleanupExpired(ctx context.Context) (int64, error)
	// GetStats returns rate limiting statistics.
	GetStats(ctx context.Context) (RateLimitStats, error)
}

// RateLimitStats contains rate limiting statistics.
type RateLimitStats struct {
	ActiveSuspensions int `json:"active_suspensions"`
	TotalEntries      int `json:"total_entries"`
}

// SQLiteRateLimitRepository implements RateLimitRepository using SQLite.
type SQLiteRateLimitRepository struct {
	db          *sql.DB
	baseBackoff time.Duration
	maxBackoff  time.Duration
}

// NewSQLiteRateLimitRepository creates a new rate limit repository.
func NewSQLiteRateLimitRepository(db *sql.DB) *SQLiteRateLimitRepository {
	return &SQLiteRateLimitRepository{
		db:          db,
		baseBackoff: 5 * time.Second,
		maxBackoff:  5 * time.Minute,
	}
}

// hashKey returns the SHA256 hash of an API key for secure storage.
func hashKey(apiKey string) string {
	h := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(h[:])
}

// IsSuspended checks if an API key is currently suspended.
func (r *SQLiteRateLimitRepository) IsSuspended(ctx context.Context, apiKey string) (bool, error) {
	if apiKey == "" {
		return false, nil
	}

	keyHash := hashKey(apiKey)
	var suspendedUntil string

	err := r.db.QueryRowContext(ctx, `
		SELECT suspended_until FROM api_key_rate_limits
		WHERE key_hash = ?
	`, keyHash).Scan(&suspendedUntil)

	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to check rate limit: %w", err)
	}

	// Parse the timestamp
	t, err := time.Parse(time.RFC3339, suspendedUntil)
	if err != nil {
		return false, nil // Treat parse errors as not suspended
	}

	return time.Now().Before(t), nil
}

// MarkRateLimited marks an API key as rate limited with exponential backoff.
func (r *SQLiteRateLimitRepository) MarkRateLimited(ctx context.Context, apiKey string) (time.Duration, error) {
	if apiKey == "" {
		return 0, nil
	}

	keyHash := hashKey(apiKey)
	now := time.Now()

	// Get current backoff count (or 0 if not exists)
	var backoffCount int
	err := r.db.QueryRowContext(ctx, `
		SELECT COALESCE(backoff_count, 0) FROM api_key_rate_limits
		WHERE key_hash = ?
	`, keyHash).Scan(&backoffCount)
	if err != nil && err != sql.ErrNoRows {
		return 0, fmt.Errorf("failed to get backoff count: %w", err)
	}

	// Increment backoff count
	backoffCount++

	// Calculate backoff: base * 2^(count-1), capped at max
	backoff := r.baseBackoff
	for i := 1; i < backoffCount && backoff < r.maxBackoff; i++ {
		backoff *= 2
	}
	if backoff > r.maxBackoff {
		backoff = r.maxBackoff
	}

	suspendedUntil := now.Add(backoff)

	// Upsert the rate limit entry
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO api_key_rate_limits (key_hash, suspended_until, backoff_count, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(key_hash) DO UPDATE SET
			suspended_until = excluded.suspended_until,
			backoff_count = excluded.backoff_count,
			updated_at = excluded.updated_at
	`, keyHash, suspendedUntil.Format(time.RFC3339), backoffCount, now.Format(time.RFC3339))

	if err != nil {
		return 0, fmt.Errorf("failed to mark rate limited: %w", err)
	}

	return backoff, nil
}

// ClearSuspension removes the suspension for an API key.
func (r *SQLiteRateLimitRepository) ClearSuspension(ctx context.Context, apiKey string) error {
	if apiKey == "" {
		return nil
	}

	keyHash := hashKey(apiKey)

	_, err := r.db.ExecContext(ctx, `
		DELETE FROM api_key_rate_limits WHERE key_hash = ?
	`, keyHash)

	if err != nil {
		return fmt.Errorf("failed to clear suspension: %w", err)
	}

	return nil
}

// CleanupExpired removes expired rate limit entries.
func (r *SQLiteRateLimitRepository) CleanupExpired(ctx context.Context) (int64, error) {
	now := time.Now().Format(time.RFC3339)

	result, err := r.db.ExecContext(ctx, `
		DELETE FROM api_key_rate_limits WHERE suspended_until < ?
	`, now)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup expired: %w", err)
	}

	return result.RowsAffected()
}

// GetStats returns rate limiting statistics.
func (r *SQLiteRateLimitRepository) GetStats(ctx context.Context) (RateLimitStats, error) {
	var stats RateLimitStats
	now := time.Now().Format(time.RFC3339)

	// Count active suspensions
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM api_key_rate_limits WHERE suspended_until > ?
	`, now).Scan(&stats.ActiveSuspensions)
	if err != nil {
		return stats, fmt.Errorf("failed to count active suspensions: %w", err)
	}

	// Count total entries
	err = r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM api_key_rate_limits
	`).Scan(&stats.TotalEntries)
	if err != nil {
		return stats, fmt.Errorf("failed to count total entries: %w", err)
	}

	return stats, nil
}
