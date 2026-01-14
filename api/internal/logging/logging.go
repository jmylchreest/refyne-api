// Package logging provides a configured slog logger with:
// - TTY detection for human-readable vs JSON output
// - LOG_FORMAT env var override (text/json)
// - LOG_LEVEL env var (debug/info/warn/error)
// - Source file:line info with shortened relative paths
// - Context-based jobID/userID extraction for filtering
// - Dynamic filter-based logging via slog-logfilter library
package logging

import (
	"context"
	"log/slog"
	"os"
	"strings"

	logfilter "github.com/jmylchreest/slog-logfilter"
)

// ContextKey is a type for context keys used in logging.
type ContextKey string

const (
	// JobIDKey is the context key for job ID.
	JobIDKey ContextKey = "log_job_id"
	// UserIDKey is the context key for user ID (for filtering only - NOT logged due to PII).
	UserIDKey ContextKey = "log_user_id"
)

// WithJobID adds a job ID to the context for logging.
func WithJobID(ctx context.Context, jobID string) context.Context {
	return context.WithValue(ctx, JobIDKey, jobID)
}

// WithUserID adds a user ID to the context for logging.
// Note: userID is used for filter matching only - NOT logged due to PII concerns.
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, UserIDKey, userID)
}

// GetJobID extracts the job ID from context.
func GetJobID(ctx context.Context) string {
	if v := ctx.Value(JobIDKey); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// GetUserID extracts the user ID from context.
func GetUserID(ctx context.Context) string {
	if v := ctx.Value(UserIDKey); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// FromContext returns a logger with jobID from context added as attributes.
// Note: userID is NOT included in logs (PII) - only used for filter matching.
// Use this when you want to include context information in your logs.
func FromContext(ctx context.Context, logger *slog.Logger) *slog.Logger {
	if ctx == nil {
		return logger
	}

	if jobID := GetJobID(ctx); jobID != "" {
		return logger.With("job_id", jobID)
	}

	return logger
}

// registerContextExtractors registers the context extractors for filtering.
// This allows filters to match on context:job_id and context:user_id.
func registerContextExtractors() {
	// Register job_id extractor - can be used for log attribute AND filtering
	logfilter.RegisterContextExtractor("job_id", func(ctx context.Context) (string, bool) {
		if ctx == nil {
			return "", false
		}
		if v := ctx.Value(JobIDKey); v != nil {
			if s, ok := v.(string); ok && s != "" {
				return s, true
			}
		}
		return "", false
	})

	// Register user_id extractor - used for filtering only, NOT logged (PII)
	logfilter.RegisterContextExtractor("user_id", func(ctx context.Context) (string, bool) {
		if ctx == nil {
			return "", false
		}
		if v := ctx.Value(UserIDKey); v != nil {
			if s, ok := v.(string); ok && s != "" {
				return s, true
			}
		}
		return "", false
	})
}

// New creates a new configured logger using slog-logfilter.
// Format is determined by:
// 1. LOG_FORMAT env var (text/json)
// 2. TTY detection (text for TTY, JSON otherwise)
// Level is determined by LOG_LEVEL env var (debug/info/warn/error, default: info)
//
// Filters can be set at runtime via logfilter.SetFilters() or loaded from S3.
func New() *slog.Logger {
	logFormat := os.Getenv("LOG_FORMAT")
	format := "json"
	if logFormat == "text" || (logFormat == "" && isatty(os.Stdout)) {
		format = "text"
	}

	// Parse log level from env var
	level := parseLogLevel(os.Getenv("LOG_LEVEL"))

	// Register context extractors for filtering (job_id, user_id)
	registerContextExtractors()

	// Create logger with slog-logfilter
	logger := logfilter.New(
		logfilter.WithLevel(level),
		logfilter.WithFormat(format),
		logfilter.WithOutput(os.Stdout),
		logfilter.WithSource(true),
	)

	return logger
}

// parseLogLevel converts a string log level to slog.Level.
func parseLogLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// SetDefault creates a new logger and sets it as the default slog logger.
// Returns the created logger for additional use.
func SetDefault() *slog.Logger {
	logger := New()
	slog.SetDefault(logger)
	return logger
}

// SetLevel changes the global log level at runtime.
func SetLevel(level slog.Level) {
	logfilter.SetLevel(level)
}

// GetLevel returns the current global log level.
func GetLevel() slog.Level {
	return logfilter.GetLevel()
}

// SetFilters replaces all log filters.
// Filters are applied in order; first match wins.
func SetFilters(filters []logfilter.LogFilter) {
	logfilter.SetFilters(filters)
}

// GetFilters returns a copy of the current filters.
func GetFilters() []logfilter.LogFilter {
	return logfilter.GetFilters()
}

// AddFilter adds a filter to the global handler.
func AddFilter(filter logfilter.LogFilter) {
	logfilter.AddFilter(filter)
}

// RemoveFilter removes filters matching the given type and pattern.
func RemoveFilter(filterType, pattern string) {
	logfilter.RemoveFilter(filterType, pattern)
}

// ClearFilters removes all filters from the global handler.
func ClearFilters() {
	logfilter.ClearFilters()
}

// isatty returns true if the file is a terminal.
func isatty(f *os.File) bool {
	stat, err := f.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}
