package version

import (
	"runtime"
	"strings"
	"testing"
)

// ========================================
// Get() Tests
// ========================================

func TestGet(t *testing.T) {
	info := Get()

	// Check that fields are populated
	if info.Version == "" {
		t.Error("Version should not be empty")
	}
	if info.Commit == "" {
		t.Error("Commit should not be empty")
	}
	if info.Date == "" {
		t.Error("Date should not be empty")
	}
	if info.GoVersion == "" {
		t.Error("GoVersion should not be empty")
	}
	if info.Platform == "" {
		t.Error("Platform should not be empty")
	}

	// GoVersion should match runtime
	if info.GoVersion != runtime.Version() {
		t.Errorf("GoVersion = %q, want %q", info.GoVersion, runtime.Version())
	}

	// Platform should contain GOOS and GOARCH
	expectedPlatform := runtime.GOOS + "/" + runtime.GOARCH
	if info.Platform != expectedPlatform {
		t.Errorf("Platform = %q, want %q", info.Platform, expectedPlatform)
	}
}

func TestGet_DefaultValues(t *testing.T) {
	// With default build values
	info := Get()

	// Default version is "0.0.0-dev"
	if info.Version != "0.0.0-dev" && !strings.Contains(info.Version, ".") {
		t.Errorf("Version should be semver format, got %q", info.Version)
	}
}

// ========================================
// Info.String() Tests
// ========================================

func TestInfo_String(t *testing.T) {
	tests := []struct {
		name     string
		info     Info
		contains []string
	}{
		{
			"clean build",
			Info{
				Version: "1.0.0",
				Commit:  "abc1234",
				Date:    "2024-01-15T10:00:00Z",
				Dirty:   false,
			},
			[]string{"1.0.0", "abc1234", "2024-01-15T10:00:00Z"},
		},
		{
			"dirty build",
			Info{
				Version: "1.0.0",
				Commit:  "abc1234",
				Date:    "2024-01-15T10:00:00Z",
				Dirty:   true,
			},
			[]string{"1.0.0", "abc1234-dirty", "2024-01-15T10:00:00Z"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.info.String()
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("String() = %q, should contain %q", got, want)
				}
			}
		})
	}
}

func TestInfo_String_Format(t *testing.T) {
	info := Info{
		Version: "2.1.0",
		Commit:  "deadbeef",
		Date:    "2024-06-01",
		Dirty:   false,
	}

	got := info.String()
	expected := "2.1.0 (deadbeef) built 2024-06-01"
	if got != expected {
		t.Errorf("String() = %q, want %q", got, expected)
	}
}

func TestInfo_String_DirtyFormat(t *testing.T) {
	info := Info{
		Version: "2.1.0",
		Commit:  "deadbeef",
		Date:    "2024-06-01",
		Dirty:   true,
	}

	got := info.String()
	expected := "2.1.0 (deadbeef-dirty) built 2024-06-01"
	if got != expected {
		t.Errorf("String() = %q, want %q", got, expected)
	}
}

// ========================================
// Info.Short() Tests
// ========================================

func TestInfo_Short(t *testing.T) {
	tests := []struct {
		name     string
		info     Info
		expected string
	}{
		{
			"clean version",
			Info{Version: "1.2.3", Dirty: false},
			"1.2.3",
		},
		{
			"dirty version",
			Info{Version: "1.2.3", Dirty: true},
			"1.2.3-dirty",
		},
		{
			"dev version clean",
			Info{Version: "0.0.0-dev", Dirty: false},
			"0.0.0-dev",
		},
		{
			"dev version dirty",
			Info{Version: "0.0.0-dev", Dirty: true},
			"0.0.0-dev-dirty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.info.Short()
			if got != tt.expected {
				t.Errorf("Short() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// ========================================
// Info Struct Tests
// ========================================

func TestInfo_Fields(t *testing.T) {
	info := Info{
		Version:   "3.0.0",
		Commit:    "abc123def",
		Date:      "2024-12-25T00:00:00Z",
		Dirty:     true,
		GoVersion: "go1.22.0",
		Platform:  "linux/amd64",
	}

	if info.Version != "3.0.0" {
		t.Errorf("Version = %q, want %q", info.Version, "3.0.0")
	}
	if info.Commit != "abc123def" {
		t.Errorf("Commit = %q, want %q", info.Commit, "abc123def")
	}
	if info.Date != "2024-12-25T00:00:00Z" {
		t.Errorf("Date = %q, want %q", info.Date, "2024-12-25T00:00:00Z")
	}
	if !info.Dirty {
		t.Error("Dirty should be true")
	}
	if info.GoVersion != "go1.22.0" {
		t.Errorf("GoVersion = %q, want %q", info.GoVersion, "go1.22.0")
	}
	if info.Platform != "linux/amd64" {
		t.Errorf("Platform = %q, want %q", info.Platform, "linux/amd64")
	}
}

func TestInfo_ZeroValue(t *testing.T) {
	var info Info

	if info.Version != "" {
		t.Error("Version should be empty by default")
	}
	if info.Commit != "" {
		t.Error("Commit should be empty by default")
	}
	if info.Dirty {
		t.Error("Dirty should be false by default")
	}
}

// ========================================
// Package Variables Tests
// ========================================

func TestPackageVariables(t *testing.T) {
	// These are set at build time, but have defaults
	if Version == "" {
		t.Error("Version variable should have a default value")
	}
	if Commit == "" {
		t.Error("Commit variable should have a default value")
	}
	if Date == "" {
		t.Error("Date variable should have a default value")
	}
	// Dirty should be "false" or "true" as string
	if Dirty != "false" && Dirty != "true" {
		t.Errorf("Dirty = %q, want 'false' or 'true'", Dirty)
	}
}

// ========================================
// Dirty Flag Conversion Tests
// ========================================

func TestGet_DirtyConversion(t *testing.T) {
	// The Get() function converts Dirty string to bool
	info := Get()

	// Default Dirty is "false", so info.Dirty should be false
	if Dirty == "false" && info.Dirty {
		t.Error("Dirty should be false when package Dirty='false'")
	}
	if Dirty == "true" && !info.Dirty {
		t.Error("Dirty should be true when package Dirty='true'")
	}
}
