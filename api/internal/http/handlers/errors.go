package handlers

// ResultInfo represents result metadata from a job or result.
// Use BuildClientResult to create a client-safe representation.
// BYOK users see full details; non-BYOK users see sanitized info.
type ResultInfo struct {
	// Error fields
	ErrorMessage  string // User-visible, sanitized error message
	ErrorCategory string // Error classification (rate_limit, quota_exceeded, etc.)
	ErrorDetails  string // Full error details (sensitive, contains API responses)

	// LLM fields
	LLMProvider string // Provider used (anthropic, openai, openrouter, etc.)
	LLMModel    string // Model used

	// Visibility control
	IsBYOK bool // Whether user's own API key was used
}

// ClientResult represents result metadata safe to return to clients.
// Sensitive fields (ErrorDetails, LLMProvider, LLMModel) only populated for BYOK users.
type ClientResult struct {
	ErrorMessage  string `json:"error_message,omitempty"`
	ErrorCategory string `json:"error_category,omitempty"`
	ErrorDetails  string `json:"error_details,omitempty"`
	LLMProvider   string `json:"llm_provider,omitempty"`
	LLMModel      string `json:"llm_model,omitempty"`
}

// BuildClientResult creates a client-safe result representation.
// Sensitive fields only included for BYOK users.
func BuildClientResult(info ResultInfo) ClientResult {
	result := ClientResult{
		ErrorMessage:  info.ErrorMessage,
		ErrorCategory: info.ErrorCategory,
	}
	// Only include sensitive details for BYOK users
	if info.IsBYOK {
		result.ErrorDetails = info.ErrorDetails
		result.LLMProvider = info.LLMProvider
		result.LLMModel = info.LLMModel
	}
	return result
}

// HasError returns true if there is any error information.
func (info ResultInfo) HasError() bool {
	return info.ErrorMessage != "" || info.ErrorCategory != ""
}

// ApplyToMap adds result fields to an existing map.
// Only includes non-empty fields. Sensitive fields only for BYOK users.
func (info ResultInfo) ApplyToMap(m map[string]any) {
	if info.ErrorMessage != "" {
		m["error_message"] = info.ErrorMessage
	}
	if info.ErrorCategory != "" {
		m["error_category"] = info.ErrorCategory
	}
	// Only include sensitive details for BYOK users
	if info.IsBYOK {
		if info.ErrorDetails != "" {
			m["error_details"] = info.ErrorDetails
		}
		if info.LLMProvider != "" {
			m["llm_provider"] = info.LLMProvider
		}
		if info.LLMModel != "" {
			m["llm_model"] = info.LLMModel
		}
	}
}

// ApplyErrorToMap adds only error fields to an existing map.
// Use when you don't want LLM info included.
func (info ResultInfo) ApplyErrorToMap(m map[string]any) {
	if info.ErrorMessage != "" {
		m["error_message"] = info.ErrorMessage
	}
	if info.ErrorCategory != "" {
		m["error_category"] = info.ErrorCategory
	}
	if info.IsBYOK && info.ErrorDetails != "" {
		m["error_details"] = info.ErrorDetails
	}
}
