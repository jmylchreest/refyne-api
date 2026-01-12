// Package logging provides a configured slog logger with:
// - TTY detection for human-readable vs JSON output
// - LOG_FORMAT env var override (text/json)
// - Source file:line info with shortened relative paths
package logging

import (
	"log/slog"
	"os"
	"path/filepath"
)

// New creates a new configured logger.
// Format is determined by:
// 1. LOG_FORMAT env var (text/json)
// 2. TTY detection (text for TTY, JSON otherwise)
func New() *slog.Logger {
	var handler slog.Handler
	logFormat := os.Getenv("LOG_FORMAT")
	useText := logFormat == "text" || (logFormat == "" && isatty(os.Stdout))

	// Get working directory for relative path calculation
	wd, _ := os.Getwd()

	opts := &slog.HandlerOptions{
		Level:     slog.LevelInfo,
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
