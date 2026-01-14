// Package models contains domain models and utility types.
package models

import (
	"encoding/json"
	"strconv"
)

// FlexInt is an int that can be unmarshaled from either a JSON number or string.
// This is useful when parsing LLM responses that may return numbers as strings
// (e.g., "count": "5" instead of "count": 5).
type FlexInt int

// UnmarshalJSON implements json.Unmarshaler for FlexInt.
// It accepts both numeric values and string representations of numbers.
func (f *FlexInt) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as an int first
	var intVal int
	if err := json.Unmarshal(data, &intVal); err == nil {
		*f = FlexInt(intVal)
		return nil
	}

	// Try to unmarshal as a string and convert
	var strVal string
	if err := json.Unmarshal(data, &strVal); err == nil {
		if strVal == "" {
			*f = 0
			return nil
		}
		parsed, err := strconv.Atoi(strVal)
		if err != nil {
			// If not a valid number string, default to 0
			*f = 0
			return nil
		}
		*f = FlexInt(parsed)
		return nil
	}

	// Default to 0 for other cases (null, etc.)
	*f = 0
	return nil
}

// MarshalJSON implements json.Marshaler for FlexInt.
// Always marshals as a numeric value.
func (f FlexInt) MarshalJSON() ([]byte, error) {
	return json.Marshal(int(f))
}

// Int returns the FlexInt as a standard int.
func (f FlexInt) Int() int {
	return int(f)
}
