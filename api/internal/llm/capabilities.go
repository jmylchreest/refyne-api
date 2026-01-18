// Package llm provides LLM provider integrations and configuration.
package llm

// ModelCapabilities represents normalized capabilities across all LLM providers.
// This provides a consistent interface for checking what features a model supports.
// Capabilities are populated by:
// - PricingService for OpenRouter (fetched from API)
// - Static defaults for other providers (defined in providers.go)
type ModelCapabilities struct {
	SupportsStructuredOutputs bool `json:"supports_structured_outputs"` // JSON schema enforcement
	SupportsTools             bool `json:"supports_tools"`              // Function calling
	SupportsStreaming         bool `json:"supports_streaming"`          // Streaming responses
	SupportsReasoning         bool `json:"supports_reasoning"`          // Reasoning tokens (o1-style)
	SupportsResponseFormat    bool `json:"supports_response_format"`    // response_format parameter
}
