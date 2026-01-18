package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// OutputFormat represents the supported output formats.
type OutputFormat string

const (
	FormatJSON  OutputFormat = "json"
	FormatJSONL OutputFormat = "jsonl"
	FormatYAML  OutputFormat = "yaml"
)

// ParseOutputFormat parses a format string and returns the OutputFormat.
// Returns FormatJSON if the format is empty or invalid.
func ParseOutputFormat(format string) OutputFormat {
	switch strings.ToLower(format) {
	case "jsonl":
		return FormatJSONL
	case "yaml", "yml":
		return FormatYAML
	default:
		return FormatJSON
	}
}

// ContentType returns the Content-Type header for the format.
func (f OutputFormat) ContentType() string {
	switch f {
	case FormatJSONL:
		return "application/x-ndjson"
	case FormatYAML:
		return "application/yaml"
	default:
		return "application/json"
	}
}

// FileExtension returns the file extension for the format.
func (f OutputFormat) FileExtension() string {
	switch f {
	case FormatJSONL:
		return ".jsonl"
	case FormatYAML:
		return ".yaml"
	default:
		return ".json"
	}
}

// FormatResults formats job results according to the specified format.
// For JSON, returns the full response structure.
// For JSONL, returns one result per line.
// For YAML, returns YAML-formatted output.
func FormatResults(results []JobResultEntry, format OutputFormat) ([]byte, error) {
	switch format {
	case FormatJSONL:
		return formatJSONL(results)
	case FormatYAML:
		return formatYAML(results)
	default:
		return formatJSON(results)
	}
}

// FormatMergedResults formats merged results according to the specified format.
func FormatMergedResults(merged map[string]any, format OutputFormat) ([]byte, error) {
	switch format {
	case FormatJSONL:
		// For merged results with JSONL, output each item in "items" as a line
		if items, ok := merged["items"].([]any); ok {
			return formatJSONLItems(items)
		}
		// Fallback: single line with the merged object
		return json.Marshal(merged)
	case FormatYAML:
		return yaml.Marshal(merged)
	default:
		return json.MarshalIndent(merged, "", "  ")
	}
}

func formatJSON(results []JobResultEntry) ([]byte, error) {
	return json.MarshalIndent(results, "", "  ")
}

func formatJSONL(results []JobResultEntry) ([]byte, error) {
	var buf bytes.Buffer
	for i, r := range results {
		// Create a simplified object for JSONL (just url and data)
		entry := map[string]any{
			"url":  r.URL,
			"data": json.RawMessage(r.Data),
		}
		line, err := json.Marshal(entry)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result %d: %w", i, err)
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}
	return buf.Bytes(), nil
}

func formatJSONLItems(items []any) ([]byte, error) {
	var buf bytes.Buffer
	for i, item := range items {
		line, err := json.Marshal(item)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal item %d: %w", i, err)
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}
	return buf.Bytes(), nil
}

func formatYAML(results []JobResultEntry) ([]byte, error) {
	// Convert to a structure suitable for YAML
	var items []map[string]any
	for _, r := range results {
		var data any
		if len(r.Data) > 0 {
			_ = json.Unmarshal(r.Data, &data)
		}
		items = append(items, map[string]any{
			"url":  r.URL,
			"data": data,
		})
	}
	return yaml.Marshal(items)
}
