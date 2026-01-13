// Package logging provides a configured slog logger with:
// - TTY detection for human-readable vs JSON output
// - LOG_FORMAT env var override (text/json)
// - LOG_LEVEL env var (debug/info/warn/error)
// - Source file:line info with shortened relative paths
package logging

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// New creates a new configured logger.
// Format is determined by:
// 1. LOG_FORMAT env var (text/json)
// 2. TTY detection (text for TTY, JSON otherwise)
// Level is determined by LOG_LEVEL env var (debug/info/warn/error, default: info)
func New() *slog.Logger {
	var handler slog.Handler
	logFormat := os.Getenv("LOG_FORMAT")
	useText := logFormat == "text" || (logFormat == "" && isatty(os.Stdout))

	// Get working directory for relative path calculation
	wd, _ := os.Getwd()

	// Parse log level from env var
	level := parseLogLevel(os.Getenv("LOG_LEVEL"))

	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: true,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Shorten source paths to be relative
			if a.Key == slog.SourceKey {
				if src, ok := a.Value.Any().(*slog.Source); ok {
					// Try to make the path relative to working directory
					if rel, err := filepath.Rel(wd, src.File); err == nil {
						src.File = rel
					} else {
						// Fallback: just use the filename
						src.File = filepath.Base(src.File)
					}
				}
			}
			return a
		},
	}

	if useText {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	return slog.New(handler)
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

// isatty returns true if the file is a terminal.
func isatty(f *os.File) bool {
	stat, err := f.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}
