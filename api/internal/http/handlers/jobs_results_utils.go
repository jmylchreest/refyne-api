package handlers

import (
	"encoding/json"

	"github.com/jmylchreest/refyne-api/internal/models"
)

// collectAllResults intelligently merges multiple page extraction results.
//
// For structured extraction (most common case), each page returns an object like:
//
//	{"page_info": {...}, "products": [...], "site_name": "..."}
//
// Output becomes:
//
//	{"products": [all products merged and deduplicated], "site_name": "..."}
func collectAllResults(results []*models.JobResult) map[string]any {
	// First pass: collect all parsed results as objects
	var objects []map[string]any
	var arrays [][]any

	for _, r := range results {
		if r.DataJSON == "" {
			continue
		}

		// Try as object first (most common case for structured extraction)
		var objData map[string]any
		if err := json.Unmarshal([]byte(r.DataJSON), &objData); err == nil {
			objects = append(objects, objData)
			continue
		}

		// Try as array
		var arrData []any
		if err := json.Unmarshal([]byte(r.DataJSON), &arrData); err == nil {
			arrays = append(arrays, arrData)
			continue
		}
	}

	// If we have objects with consistent structure, merge them intelligently
	if len(objects) > 0 {
		merged := smartMergeObjects(objects)
		// If there are also top-level arrays, add them as an "items" field
		if len(arrays) > 0 {
			var allItems []any
			for _, arr := range arrays {
				allItems = append(allItems, arr...)
			}
			if existing, ok := merged["items"].([]any); ok {
				merged["items"] = dedupeArrayByURL(append(existing, allItems...))
			} else if len(allItems) > 0 {
				merged["items"] = dedupeArrayByURL(allItems)
			}
		}
		return merged
	}

	// If only arrays, merge them into items
	if len(arrays) > 0 {
		var allItems []any
		for _, arr := range arrays {
			allItems = append(allItems, arr...)
		}
		return map[string]any{"items": dedupeArrayByURL(allItems)}
	}

	return map[string]any{"items": []any{}}
}

// smartMergeObjects merges multiple objects with the same structure.
// Arrays are concatenated and deduplicated, scalars take first non-null value.
func smartMergeObjects(objects []map[string]any) map[string]any {
	if len(objects) == 0 {
		return map[string]any{}
	}
	if len(objects) == 1 {
		return objects[0]
	}

	result := make(map[string]any)

	// Collect all keys across all objects
	allKeys := make(map[string]bool)
	for _, obj := range objects {
		for key := range obj {
			allKeys[key] = true
		}
	}

	// Process each key
	for key := range allKeys {
		var arrays [][]any
		var firstScalar any
		var firstObject map[string]any
		hasArray := false

		for _, obj := range objects {
			val, exists := obj[key]
			if !exists || val == nil {
				continue
			}

			switch v := val.(type) {
			case []any:
				hasArray = true
				arrays = append(arrays, v)
			case map[string]any:
				if firstObject == nil {
					firstObject = v
				}
			default:
				if firstScalar == nil {
					firstScalar = v
				}
			}
		}

		// Determine what to store for this key
		if hasArray {
			// Merge all arrays
			var merged []any
			for _, arr := range arrays {
				merged = append(merged, arr...)
			}
			result[key] = dedupeArrayByURL(merged)
		} else if firstObject != nil {
			result[key] = firstObject
		} else if firstScalar != nil {
			result[key] = firstScalar
		}
	}

	return result
}

// countNonNullFields counts non-null fields in a map (used to determine data richness).
func countNonNullFields(obj map[string]any) int {
	count := 0
	for _, val := range obj {
		if val == nil {
			continue
		}
		switch v := val.(type) {
		case []any:
			if len(v) > 0 {
				count++
			}
		case string:
			if v != "" {
				count++
			}
		default:
			count++
		}
	}
	return count
}

// dedupeArrayByURL removes duplicate items from an array, preferring items with MORE data.
// For objects with a "url" field, uses URL as the key for deduplication.
// When duplicates are found, keeps the version with more non-null fields.
// This ensures that when a product appears on multiple pages (e.g., homepage with minimal
// data, then collection page with full data), we keep the richer version.
func dedupeArrayByURL(arr []any) []any {
	if len(arr) == 0 {
		return arr
	}

	// Track best version of each URL-keyed item (most non-null fields wins)
	type urlEntry struct {
		item       any
		fieldCount int
	}
	bestByURL := make(map[string]urlEntry)
	seenByJSON := make(map[string]bool)
	nonURLItems := make([]any, 0)

	for _, item := range arr {
		// Try to dedupe by URL if it's an object with a url field
		if obj, ok := item.(map[string]any); ok {
			if url, urlOk := obj["url"].(string); urlOk && url != "" {
				fieldCount := countNonNullFields(obj)
				existing, exists := bestByURL[url]

				// Keep the version with more non-null fields
				if !exists || fieldCount > existing.fieldCount {
					bestByURL[url] = urlEntry{item: item, fieldCount: fieldCount}
				}
				continue
			}
		}

		// Fall back to JSON serialization for deduplication
		key, err := json.Marshal(item)
		if err != nil {
			nonURLItems = append(nonURLItems, item)
			continue
		}

		keyStr := string(key)
		if !seenByJSON[keyStr] {
			seenByJSON[keyStr] = true
			nonURLItems = append(nonURLItems, item)
		}
	}

	// Combine URL-deduped items with non-URL items
	result := make([]any, 0, len(bestByURL)+len(nonURLItems))
	for _, entry := range bestByURL {
		result = append(result, entry.item)
	}
	result = append(result, nonURLItems...)

	return result
}
