package logging

import (
	"context"
	"log/slog"
	"testing"
)

// ========================================
// Context Key Tests
// ========================================

func TestContextKeys(t *testing.T) {
	if JobIDKey != "log_job_id" {
		t.Errorf("JobIDKey = %q, want %q", JobIDKey, "log_job_id")
	}
	if UserIDKey != "log_user_id" {
		t.Errorf("UserIDKey = %q, want %q", UserIDKey, "log_user_id")
	}
}

// ========================================
// WithJobID Tests
// ========================================

func TestWithJobID(t *testing.T) {
	ctx := context.Background()
	jobID := "job-123-abc"

	newCtx := WithJobID(ctx, jobID)

	// Should not modify original context
	if ctx.Value(JobIDKey) != nil {
		t.Error("original context should not be modified")
	}

	// New context should have the job ID
	got := newCtx.Value(JobIDKey)
	if got != jobID {
		t.Errorf("context value = %v, want %q", got, jobID)
	}
}

func TestWithJobID_Empty(t *testing.T) {
	ctx := WithJobID(context.Background(), "")

	got := ctx.Value(JobIDKey)
	if got != "" {
		t.Errorf("context value = %v, want empty string", got)
	}
}

// ========================================
// WithUserID Tests
// ========================================

func TestWithUserID(t *testing.T) {
	ctx := context.Background()
	userID := "user_456_xyz"

	newCtx := WithUserID(ctx, userID)

	// Should not modify original context
	if ctx.Value(UserIDKey) != nil {
		t.Error("original context should not be modified")
	}

	// New context should have the user ID
	got := newCtx.Value(UserIDKey)
	if got != userID {
		t.Errorf("context value = %v, want %q", got, userID)
	}
}

func TestWithUserID_Empty(t *testing.T) {
	ctx := WithUserID(context.Background(), "")

	got := ctx.Value(UserIDKey)
	if got != "" {
		t.Errorf("context value = %v, want empty string", got)
	}
}

// ========================================
// GetJobID Tests
// ========================================

func TestGetJobID(t *testing.T) {
	tests := []struct {
		name     string
		ctx      context.Context
		expected string
	}{
		{
			"with job ID",
			WithJobID(context.Background(), "job-999"),
			"job-999",
		},
		{
			"without job ID",
			context.Background(),
			"",
		},
		{
			"empty job ID",
			WithJobID(context.Background(), ""),
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetJobID(tt.ctx)
			if got != tt.expected {
				t.Errorf("GetJobID() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestGetJobID_WrongType(t *testing.T) {
	// Put a non-string value in the context
	ctx := context.WithValue(context.Background(), JobIDKey, 12345)

	got := GetJobID(ctx)
	if got != "" {
		t.Errorf("GetJobID() = %q, want empty for wrong type", got)
	}
}

// ========================================
// GetUserID Tests
// ========================================

func TestGetUserID(t *testing.T) {
	tests := []struct {
		name     string
		ctx      context.Context
		expected string
	}{
		{
			"with user ID",
			WithUserID(context.Background(), "user_abc"),
			"user_abc",
		},
		{
			"without user ID",
			context.Background(),
			"",
		},
		{
			"empty user ID",
			WithUserID(context.Background(), ""),
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetUserID(tt.ctx)
			if got != tt.expected {
				t.Errorf("GetUserID() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestGetUserID_WrongType(t *testing.T) {
	// Put a non-string value in the context
	ctx := context.WithValue(context.Background(), UserIDKey, struct{}{})

	got := GetUserID(ctx)
	if got != "" {
		t.Errorf("GetUserID() = %q, want empty for wrong type", got)
	}
}

// ========================================
// FromContext Tests
// ========================================

func TestFromContext_NilContext(t *testing.T) {
	logger := slog.Default()
	result := FromContext(nil, logger)

	if result != logger {
		t.Error("FromContext with nil context should return original logger")
	}
}

func TestFromContext_NoJobID(t *testing.T) {
	logger := slog.Default()
	ctx := context.Background()

	result := FromContext(ctx, logger)

	if result != logger {
		t.Error("FromContext without job ID should return original logger")
	}
}

func TestFromContext_WithJobID(t *testing.T) {
	logger := slog.Default()
	ctx := WithJobID(context.Background(), "job-test-123")

	result := FromContext(ctx, logger)

	// Result should be a different logger (with added attributes)
	if result == logger {
		t.Error("FromContext with job ID should return a new logger with attributes")
	}
}

// ========================================
// parseLogLevel Tests
// ========================================

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"Debug", slog.LevelDebug},
		{" debug ", slog.LevelDebug},

		{"info", slog.LevelInfo},
		{"INFO", slog.LevelInfo},
		{"", slog.LevelInfo}, // default

		{"warn", slog.LevelWarn},
		{"WARN", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"WARNING", slog.LevelWarn},

		{"error", slog.LevelError},
		{"ERROR", slog.LevelError},

		{"invalid", slog.LevelInfo},  // default
		{"unknown", slog.LevelInfo},  // default
		{"trace", slog.LevelInfo},    // unsupported, default
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseLogLevel(tt.input)
			if got != tt.expected {
				t.Errorf("parseLogLevel(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

// ========================================
// Combined Context Tests
// ========================================

func TestCombinedContext(t *testing.T) {
	ctx := context.Background()
	ctx = WithJobID(ctx, "job-combined")
	ctx = WithUserID(ctx, "user-combined")

	jobID := GetJobID(ctx)
	userID := GetUserID(ctx)

	if jobID != "job-combined" {
		t.Errorf("GetJobID() = %q, want %q", jobID, "job-combined")
	}
	if userID != "user-combined" {
		t.Errorf("GetUserID() = %q, want %q", userID, "user-combined")
	}
}

func TestContextOverwrite(t *testing.T) {
	ctx := WithJobID(context.Background(), "job-1")
	ctx = WithJobID(ctx, "job-2")

	got := GetJobID(ctx)
	if got != "job-2" {
		t.Errorf("GetJobID() = %q, want %q (should be overwritten)", got, "job-2")
	}
}

// ========================================
// ContextKey Type Tests
// ========================================

func TestContextKey_Type(t *testing.T) {
	// Verify ContextKey is a distinct type
	var key ContextKey = "test_key"

	if string(key) != "test_key" {
		t.Errorf("ContextKey conversion = %q, want %q", string(key), "test_key")
	}
}

func TestContextKey_Uniqueness(t *testing.T) {
	// Using string directly vs ContextKey should be different context keys
	ctx := context.Background()

	// Set with ContextKey type
	ctx = context.WithValue(ctx, JobIDKey, "typed-value")

	// Try to get with raw string (should not find it)
	rawValue := ctx.Value("log_job_id")

	// The raw string key should not match the typed ContextKey
	// (Go's context uses type + value for key comparison)
	if rawValue != nil {
		t.Error("raw string key should not match ContextKey type")
	}

	// But typed key should work
	typedValue := ctx.Value(JobIDKey)
	if typedValue != "typed-value" {
		t.Errorf("typed key value = %v, want %q", typedValue, "typed-value")
	}
}

// ========================================
// New Logger Tests
// ========================================

func TestNew(t *testing.T) {
	logger := New()
	if logger == nil {
		t.Fatal("New() should return a logger")
	}
}

func TestSetDefault(t *testing.T) {
	logger := SetDefault()
	if logger == nil {
		t.Fatal("SetDefault() should return a logger")
	}

	// Default logger should be set
	defaultLogger := slog.Default()
	if defaultLogger == nil {
		t.Error("slog.Default() should not be nil after SetDefault()")
	}
}
