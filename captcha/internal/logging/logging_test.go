package logging

import (
	"context"
	"log/slog"
	"testing"
)

func TestWithRequestID(t *testing.T) {
	ctx := context.Background()
	ctx = WithRequestID(ctx, "req-123")

	requestID := GetRequestID(ctx)
	if requestID != "req-123" {
		t.Errorf("GetRequestID() = %q, want %q", requestID, "req-123")
	}
}

func TestWithUserID(t *testing.T) {
	ctx := context.Background()
	ctx = WithUserID(ctx, "user-456")

	userID := GetUserID(ctx)
	if userID != "user-456" {
		t.Errorf("GetUserID() = %q, want %q", userID, "user-456")
	}
}

func TestGetRequestID_Empty(t *testing.T) {
	ctx := context.Background()
	requestID := GetRequestID(ctx)
	if requestID != "" {
		t.Errorf("GetRequestID() on empty context = %q, want empty", requestID)
	}
}

func TestGetUserID_Empty(t *testing.T) {
	ctx := context.Background()
	userID := GetUserID(ctx)
	if userID != "" {
		t.Errorf("GetUserID() on empty context = %q, want empty", userID)
	}
}

func TestGetRequestID_NilContext(t *testing.T) {
	var ctx context.Context
	requestID := GetRequestID(ctx)
	if requestID != "" {
		t.Errorf("GetRequestID() on nil context = %q, want empty", requestID)
	}
}

func TestFromContext(t *testing.T) {
	logger := slog.Default()

	t.Run("nil context returns original logger", func(t *testing.T) {
		result := FromContext(nil, logger)
		if result != logger {
			t.Error("FromContext(nil, logger) should return original logger")
		}
	})

	t.Run("context with requestID adds attribute", func(t *testing.T) {
		ctx := WithRequestID(context.Background(), "req-abc")
		result := FromContext(ctx, logger)
		// The returned logger should be different (has additional attribute)
		if result == logger {
			t.Error("FromContext with requestID should return a new logger")
		}
	})

	t.Run("context without requestID returns original", func(t *testing.T) {
		ctx := context.Background()
		result := FromContext(ctx, logger)
		if result != logger {
			t.Error("FromContext without requestID should return original logger")
		}
	})
}

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input string
		want  slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"INFO", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"WARN", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"ERROR", slog.LevelError},
		{"", slog.LevelInfo},       // default
		{"unknown", slog.LevelInfo}, // default for unknown
		{"  debug  ", slog.LevelDebug}, // with whitespace
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseLogLevel(tt.input)
			if got != tt.want {
				t.Errorf("parseLogLevel(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestNew(t *testing.T) {
	logger := New()
	if logger == nil {
		t.Error("New() returned nil")
	}
}

func TestSetDefault(t *testing.T) {
	logger := SetDefault()
	if logger == nil {
		t.Error("SetDefault() returned nil")
	}
	// Verify it was set as default
	if slog.Default() != logger {
		t.Error("SetDefault() did not set the logger as default")
	}
}
