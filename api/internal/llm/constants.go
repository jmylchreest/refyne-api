// Package llm provides LLM client interfaces and provider implementations.
package llm

import "time"

// Provider name constants for use throughout the codebase.
// Use these constants instead of string literals to prevent typos
// and enable compile-time checking.
const (
	// ProviderOpenRouter is the OpenRouter provider name.
	ProviderOpenRouter = "openrouter"

	// ProviderAnthropic is the Anthropic provider name.
	ProviderAnthropic = "anthropic"

	// ProviderOpenAI is the OpenAI provider name.
	ProviderOpenAI = "openai"

	// ProviderOllama is the Ollama provider name.
	ProviderOllama = "ollama"

	// ProviderHelicone is the Helicone provider name.
	ProviderHelicone = "helicone"
)

// Helicone URL constants.
const (
	// HeliconeCloudBaseURL is the default Helicone cloud gateway URL for managed credits mode.
	// This endpoint handles provider routing automatically - no Helicone-Target-Url header needed.
	HeliconeCloudBaseURL = "https://ai-gateway.helicone.ai"
)

// Timeout constants for LLM operations.
const (
	// LLMTimeout is the timeout for LLM completion requests.
	// Set higher to accommodate free models under load.
	LLMTimeout = 120 * time.Second
)

// ValidProviders returns a slice of all valid provider names.
func ValidProviders() []string {
	return []string{
		ProviderOpenRouter,
		ProviderAnthropic,
		ProviderOpenAI,
		ProviderOllama,
		ProviderHelicone,
	}
}

// IsValidProvider returns true if the provider name is valid.
func IsValidProvider(provider string) bool {
	switch provider {
	case ProviderOpenRouter, ProviderAnthropic, ProviderOpenAI, ProviderOllama, ProviderHelicone:
		return true
	default:
		return false
	}
}
