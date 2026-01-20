// Package logging provides a configured slog logger with:
// - TTY detection for human-readable vs JSON output
// - LOG_FORMAT env var override (text/json)
// - LOG_LEVEL env var (debug/info/warn/error)
// - Context-based requestID/userID extraction for filtering
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
	// RequestIDKey is the context key for request ID.
	RequestIDKey ContextKey = "log_request_id"
	// UserIDKey is the context key for user ID (for filtering only - NOT logged due to PII).
	UserIDKey ContextKey = "log_user_id"
)

// WithRequestID adds a request ID to the context for logging.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, RequestIDKey, requestID)
}

// WithUserID adds a user ID to the context for logging.
// Note: userID is used for filter matching only - NOT logged due to PII concerns.
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, UserIDKey, userID)
}

// GetRequestID extracts the request ID from context.
func GetRequestID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v := ctx.Value(RequestIDKey); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// GetUserID extracts the user ID from context.
func GetUserID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v := ctx.Value(UserIDKey); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// FromContext returns a logger with requestID from context added as attributes.
// Note: userID is NOT included in logs (PII) - only used for filter matching.
func FromContext(ctx context.Context, logger *slog.Logger) *slog.Logger {
	if ctx == nil {
		return logger
	}

	if requestID := GetRequestID(ctx); requestID != "" {
		return logger.With("request_id", requestID)
	}

	return logger
}

// registerContextExtractors registers the context extractors for filtering.
func registerContextExtractors() {
	// Register request_id extractor
	logfilter.RegisterContextExtractor("request_id", func(ctx context.Context) (string, bool) {
		if ctx == nil {
			return "", false
		}
		if v := ctx.Value(RequestIDKey); v != nil {
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
func New() *slog.Logger {
	logFormat := os.Getenv("LOG_FORMAT")
	format := "json"
	if logFormat == "text" || (logFormat == "" && isatty(os.Stdout)) {
		format = "text"
	}

	level := parseLogLevel(os.Getenv("LOG_LEVEL"))

	registerContextExtractors()

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
func SetFilters(filters []logfilter.LogFilter) {
	logfilter.SetFilters(filters)
}

// GetFilters returns a copy of the current filters.
func GetFilters() []logfilter.LogFilter {
	return logfilter.GetFilters()
}

// isatty returns true if the file is a terminal.
func isatty(f *os.File) bool {
	stat, err := f.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}
