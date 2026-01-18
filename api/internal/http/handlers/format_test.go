package handlers

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// ========================================
// ParseOutputFormat Tests
// ========================================

func TestParseOutputFormat(t *testing.T) {
	tests := []struct {
		input    string
		expected OutputFormat
	}{
		// JSON (default)
		{"json", FormatJSON},
		{"JSON", FormatJSON},
		{"Json", FormatJSON},
		{"", FormatJSON},
		{"invalid", FormatJSON},
		{"xml", FormatJSON}, // Unsupported, falls back to JSON

		// JSONL
		{"jsonl", FormatJSONL},
		{"JSONL", FormatJSONL},
		{"Jsonl", FormatJSONL},

		// YAML
		{"yaml", FormatYAML},
		{"YAML", FormatYAML},
		{"yml", FormatYAML},
		{"YML", FormatYAML},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseOutputFormat(tt.input)
			if got != tt.expected {
				t.Errorf("ParseOutputFormat(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// ========================================
// OutputFormat.ContentType Tests
// ========================================

func TestOutputFormat_ContentType(t *testing.T) {
	tests := []struct {
		format   OutputFormat
		expected string
	}{
		{FormatJSON, "application/json"},
		{FormatJSONL, "application/x-ndjson"},
		{FormatYAML, "application/yaml"},
		{OutputFormat("unknown"), "application/json"}, // Default
	}

	for _, tt := range tests {
		t.Run(string(tt.format), func(t *testing.T) {
			got := tt.format.ContentType()
			if got != tt.expected {
				t.Errorf("ContentType() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// ========================================
// OutputFormat.FileExtension Tests
// ========================================

func TestOutputFormat_FileExtension(t *testing.T) {
	tests := []struct {
		format   OutputFormat
		expected string
	}{
		{FormatJSON, ".json"},
		{FormatJSONL, ".jsonl"},
		{FormatYAML, ".yaml"},
		{OutputFormat("unknown"), ".json"}, // Default
	}

	for _, tt := range tests {
		t.Run(string(tt.format), func(t *testing.T) {
			got := tt.format.FileExtension()
			if got != tt.expected {
				t.Errorf("FileExtension() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// ========================================
// FormatResults Tests
// ========================================

func TestFormatResults_JSON(t *testing.T) {
	results := []JobResultEntry{
		{URL: "https://example.com/1", Data: json.RawMessage(`{"title":"Page 1"}`)},
		{URL: "https://example.com/2", Data: json.RawMessage(`{"title":"Page 2"}`)},
	}

	output, err := FormatResults(results, FormatJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be valid JSON array
	var parsed []map[string]any
	if err := json.Unmarshal(output, &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if len(parsed) != 2 {
		t.Errorf("parsed %d results, want 2", len(parsed))
	}
}

func TestFormatResults_JSONL(t *testing.T) {
	results := []JobResultEntry{
		{URL: "https://example.com/1", Data: json.RawMessage(`{"title":"Page 1"}`)},
		{URL: "https://example.com/2", Data: json.RawMessage(`{"title":"Page 2"}`)},
	}

	output, err := FormatResults(results, FormatJSONL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have one JSON object per line
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) != 2 {
		t.Errorf("got %d lines, want 2", len(lines))
	}

	// Each line should be valid JSON
	for i, line := range lines {
		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			t.Errorf("line %d is not valid JSON: %v", i, err)
		}
		if _, ok := obj["url"]; !ok {
			t.Errorf("line %d missing 'url' field", i)
		}
		if _, ok := obj["data"]; !ok {
			t.Errorf("line %d missing 'data' field", i)
		}
	}
}

func TestFormatResults_YAML(t *testing.T) {
	results := []JobResultEntry{
		{URL: "https://example.com/1", Data: json.RawMessage(`{"title":"Page 1"}`)},
		{URL: "https://example.com/2", Data: json.RawMessage(`{"title":"Page 2"}`)},
	}

	output, err := FormatResults(results, FormatYAML)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be valid YAML
	var parsed []map[string]any
	if err := yaml.Unmarshal(output, &parsed); err != nil {
		t.Fatalf("output is not valid YAML: %v", err)
	}
	if len(parsed) != 2 {
		t.Errorf("parsed %d results, want 2", len(parsed))
	}
}

func TestFormatResults_Empty(t *testing.T) {
	results := []JobResultEntry{}

	// JSON
	jsonOutput, err := FormatResults(results, FormatJSON)
	if err != nil {
		t.Fatalf("JSON error: %v", err)
	}
	if string(jsonOutput) != "[]" {
		t.Errorf("JSON output = %q, want %q", string(jsonOutput), "[]")
	}

	// JSONL
	jsonlOutput, err := FormatResults(results, FormatJSONL)
	if err != nil {
		t.Fatalf("JSONL error: %v", err)
	}
	if string(jsonlOutput) != "" {
		t.Errorf("JSONL output = %q, want empty", string(jsonlOutput))
	}
}

// ========================================
// FormatMergedResults Tests
// ========================================

func TestFormatMergedResults_JSON(t *testing.T) {
	merged := map[string]any{
		"total": 2,
		"items": []any{
			map[string]any{"title": "Item 1"},
			map[string]any{"title": "Item 2"},
		},
	}

	output, err := FormatMergedResults(merged, FormatJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be valid JSON
	var parsed map[string]any
	if err := json.Unmarshal(output, &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
}

func TestFormatMergedResults_JSONL(t *testing.T) {
	merged := map[string]any{
		"items": []any{
			map[string]any{"title": "Item 1"},
			map[string]any{"title": "Item 2"},
		},
	}

	output, err := FormatMergedResults(merged, FormatJSONL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have one line per item
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) != 2 {
		t.Errorf("got %d lines, want 2", len(lines))
	}
}

func TestFormatMergedResults_YAML(t *testing.T) {
	merged := map[string]any{
		"total": 1,
		"items": []any{
			map[string]any{"title": "Item 1"},
		},
	}

	output, err := FormatMergedResults(merged, FormatYAML)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be valid YAML
	var parsed map[string]any
	if err := yaml.Unmarshal(output, &parsed); err != nil {
		t.Fatalf("output is not valid YAML: %v", err)
	}
}

func TestFormatMergedResults_NoItems(t *testing.T) {
	// When "items" is not present, JSONL should return the whole object as one line
	merged := map[string]any{
		"count": 0,
	}

	output, err := FormatMergedResults(merged, FormatJSONL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be a single JSON line
	var parsed map[string]any
	if err := json.Unmarshal(output, &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if parsed["count"] != float64(0) {
		t.Errorf("count = %v, want 0", parsed["count"])
	}
}

// ========================================
// Format Constants Tests
// ========================================

func TestFormatConstants(t *testing.T) {
	if FormatJSON != "json" {
		t.Errorf("FormatJSON = %q, want %q", FormatJSON, "json")
	}
	if FormatJSONL != "jsonl" {
		t.Errorf("FormatJSONL = %q, want %q", FormatJSONL, "jsonl")
	}
	if FormatYAML != "yaml" {
		t.Errorf("FormatYAML = %q, want %q", FormatYAML, "yaml")
	}
}
