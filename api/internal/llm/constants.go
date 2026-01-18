// Package llm provides LLM client interfaces and provider implementations.
package llm

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
)

// ValidProviders returns a slice of all valid provider names.
func ValidProviders() []string {
	return []string{
		ProviderOpenRouter,
		ProviderAnthropic,
		ProviderOpenAI,
		ProviderOllama,
	}
}

// IsValidProvider returns true if the provider name is valid.
func IsValidProvider(provider string) bool {
	switch provider {
	case ProviderOpenRouter, ProviderAnthropic, ProviderOpenAI, ProviderOllama:
		return true
	default:
		return false
	}
}
