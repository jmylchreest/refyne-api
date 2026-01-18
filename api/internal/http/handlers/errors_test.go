package handlers

import (
	"testing"
)

// ========================================
// ResultInfo Tests
// ========================================

func TestResultInfo_HasError(t *testing.T) {
	tests := []struct {
		name     string
		info     ResultInfo
		expected bool
	}{
		{
			name:     "no error",
			info:     ResultInfo{},
			expected: false,
		},
		{
			name:     "with error message",
			info:     ResultInfo{ErrorMessage: "something went wrong"},
			expected: true,
		},
		{
			name:     "with error category",
			info:     ResultInfo{ErrorCategory: "rate_limit"},
			expected: true,
		},
		{
			name:     "with both",
			info:     ResultInfo{ErrorMessage: "rate limited", ErrorCategory: "rate_limit"},
			expected: true,
		},
		{
			name:     "with only details (not considered error)",
			info:     ResultInfo{ErrorDetails: "some details"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.info.HasError()
			if got != tt.expected {
				t.Errorf("HasError() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// ========================================
// BuildClientResult Tests
// ========================================

func TestBuildClientResult_NonBYOK(t *testing.T) {
	info := ResultInfo{
		ErrorMessage:  "request failed",
		ErrorCategory: "api_error",
		ErrorDetails:  "sensitive API response details",
		LLMProvider:   "openai",
		LLMModel:      "gpt-4o",
		IsBYOK:        false,
	}

	result := BuildClientResult(info)

	// Non-BYOK should only have error message and category
	if result.ErrorMessage != info.ErrorMessage {
		t.Errorf("ErrorMessage = %q, want %q", result.ErrorMessage, info.ErrorMessage)
	}
	if result.ErrorCategory != info.ErrorCategory {
		t.Errorf("ErrorCategory = %q, want %q", result.ErrorCategory, info.ErrorCategory)
	}

	// Sensitive fields should be empty
	if result.ErrorDetails != "" {
		t.Errorf("ErrorDetails = %q, want empty (non-BYOK)", result.ErrorDetails)
	}
	if result.LLMProvider != "" {
		t.Errorf("LLMProvider = %q, want empty (non-BYOK)", result.LLMProvider)
	}
	if result.LLMModel != "" {
		t.Errorf("LLMModel = %q, want empty (non-BYOK)", result.LLMModel)
	}
}

func TestBuildClientResult_BYOK(t *testing.T) {
	info := ResultInfo{
		ErrorMessage:  "request failed",
		ErrorCategory: "api_error",
		ErrorDetails:  "sensitive API response details",
		LLMProvider:   "openai",
		LLMModel:      "gpt-4o",
		IsBYOK:        true,
	}

	result := BuildClientResult(info)

	// BYOK should have all fields
	if result.ErrorMessage != info.ErrorMessage {
		t.Errorf("ErrorMessage = %q, want %q", result.ErrorMessage, info.ErrorMessage)
	}
	if result.ErrorCategory != info.ErrorCategory {
		t.Errorf("ErrorCategory = %q, want %q", result.ErrorCategory, info.ErrorCategory)
	}
	if result.ErrorDetails != info.ErrorDetails {
		t.Errorf("ErrorDetails = %q, want %q", result.ErrorDetails, info.ErrorDetails)
	}
	if result.LLMProvider != info.LLMProvider {
		t.Errorf("LLMProvider = %q, want %q", result.LLMProvider, info.LLMProvider)
	}
	if result.LLMModel != info.LLMModel {
		t.Errorf("LLMModel = %q, want %q", result.LLMModel, info.LLMModel)
	}
}

func TestBuildClientResult_Empty(t *testing.T) {
	info := ResultInfo{}
	result := BuildClientResult(info)

	if result.ErrorMessage != "" || result.ErrorCategory != "" || result.ErrorDetails != "" {
		t.Error("empty ResultInfo should produce empty ClientResult")
	}
}

// ========================================
// ApplyToMap Tests
// ========================================

func TestResultInfo_ApplyToMap_NonBYOK(t *testing.T) {
	info := ResultInfo{
		ErrorMessage:  "error occurred",
		ErrorCategory: "validation",
		ErrorDetails:  "detailed error info",
		LLMProvider:   "anthropic",
		LLMModel:      "claude-3-sonnet",
		IsBYOK:        false,
	}

	m := make(map[string]any)
	info.ApplyToMap(m)

	// Should have public fields
	if m["error_message"] != info.ErrorMessage {
		t.Errorf("error_message = %v, want %q", m["error_message"], info.ErrorMessage)
	}
	if m["error_category"] != info.ErrorCategory {
		t.Errorf("error_category = %v, want %q", m["error_category"], info.ErrorCategory)
	}

	// Should NOT have sensitive fields
	if _, ok := m["error_details"]; ok {
		t.Error("error_details should not be present for non-BYOK")
	}
	if _, ok := m["llm_provider"]; ok {
		t.Error("llm_provider should not be present for non-BYOK")
	}
	if _, ok := m["llm_model"]; ok {
		t.Error("llm_model should not be present for non-BYOK")
	}
}

func TestResultInfo_ApplyToMap_BYOK(t *testing.T) {
	info := ResultInfo{
		ErrorMessage:  "error occurred",
		ErrorCategory: "validation",
		ErrorDetails:  "detailed error info",
		LLMProvider:   "anthropic",
		LLMModel:      "claude-3-sonnet",
		IsBYOK:        true,
	}

	m := make(map[string]any)
	info.ApplyToMap(m)

	// Should have all fields
	if m["error_message"] != info.ErrorMessage {
		t.Errorf("error_message = %v, want %q", m["error_message"], info.ErrorMessage)
	}
	if m["error_category"] != info.ErrorCategory {
		t.Errorf("error_category = %v, want %q", m["error_category"], info.ErrorCategory)
	}
	if m["error_details"] != info.ErrorDetails {
		t.Errorf("error_details = %v, want %q", m["error_details"], info.ErrorDetails)
	}
	if m["llm_provider"] != info.LLMProvider {
		t.Errorf("llm_provider = %v, want %q", m["llm_provider"], info.LLMProvider)
	}
	if m["llm_model"] != info.LLMModel {
		t.Errorf("llm_model = %v, want %q", m["llm_model"], info.LLMModel)
	}
}

func TestResultInfo_ApplyToMap_Empty(t *testing.T) {
	info := ResultInfo{}
	m := make(map[string]any)
	info.ApplyToMap(m)

	// Empty info should not add any fields
	if len(m) != 0 {
		t.Errorf("map has %d entries, want 0", len(m))
	}
}

// ========================================
// ApplyErrorToMap Tests
// ========================================

func TestResultInfo_ApplyErrorToMap_NonBYOK(t *testing.T) {
	info := ResultInfo{
		ErrorMessage:  "error",
		ErrorCategory: "timeout",
		ErrorDetails:  "detailed info",
		LLMProvider:   "openai",
		LLMModel:      "gpt-4",
		IsBYOK:        false,
	}

	m := make(map[string]any)
	info.ApplyErrorToMap(m)

	// Should have error fields only
	if m["error_message"] != info.ErrorMessage {
		t.Errorf("error_message = %v, want %q", m["error_message"], info.ErrorMessage)
	}
	if m["error_category"] != info.ErrorCategory {
		t.Errorf("error_category = %v, want %q", m["error_category"], info.ErrorCategory)
	}

	// Should NOT have error_details for non-BYOK
	if _, ok := m["error_details"]; ok {
		t.Error("error_details should not be present for non-BYOK")
	}

	// Should NOT have LLM fields
	if _, ok := m["llm_provider"]; ok {
		t.Error("llm_provider should not be present in error map")
	}
}

func TestResultInfo_ApplyErrorToMap_BYOK(t *testing.T) {
	info := ResultInfo{
		ErrorMessage:  "error",
		ErrorCategory: "timeout",
		ErrorDetails:  "detailed info",
		LLMProvider:   "openai",
		LLMModel:      "gpt-4",
		IsBYOK:        true,
	}

	m := make(map[string]any)
	info.ApplyErrorToMap(m)

	// Should have error fields including details
	if m["error_message"] != info.ErrorMessage {
		t.Errorf("error_message = %v, want %q", m["error_message"], info.ErrorMessage)
	}
	if m["error_category"] != info.ErrorCategory {
		t.Errorf("error_category = %v, want %q", m["error_category"], info.ErrorCategory)
	}
	if m["error_details"] != info.ErrorDetails {
		t.Errorf("error_details = %v, want %q", m["error_details"], info.ErrorDetails)
	}

	// Should NOT have LLM fields
	if _, ok := m["llm_provider"]; ok {
		t.Error("llm_provider should not be present in error map")
	}
}

// ========================================
// ClientResult Tests
// ========================================

func TestClientResult_ZeroValue(t *testing.T) {
	var result ClientResult

	if result.ErrorMessage != "" {
		t.Error("ErrorMessage should be empty by default")
	}
	if result.ErrorCategory != "" {
		t.Error("ErrorCategory should be empty by default")
	}
	if result.ErrorDetails != "" {
		t.Error("ErrorDetails should be empty by default")
	}
	if result.LLMProvider != "" {
		t.Error("LLMProvider should be empty by default")
	}
	if result.LLMModel != "" {
		t.Error("LLMModel should be empty by default")
	}
}
