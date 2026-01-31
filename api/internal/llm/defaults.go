// Package llm provides LLM provider integrations and configuration.
package llm

// ModelSettings contains the recommended settings for an LLM model.
type ModelSettings struct {
	Temperature float64
	MaxTokens   int
	StrictMode  bool // Whether the model supports strict JSON schema mode
}

// ProviderDefaults contains minimal default settings per provider.
// These are used only as last-resort fallbacks when S3 config is unavailable.
// Model-specific settings should be configured via S3 config file: config/model_defaults.json
var ProviderDefaults = map[string]ModelSettings{
	"anthropic":  {Temperature: 0.2, MaxTokens: 16384, StrictMode: false}, // Anthropic uses tool_use, not strict
	"openai":     {Temperature: 0.2, MaxTokens: 16384, StrictMode: true},  // Native OpenAI supports strict
	"openrouter": {Temperature: 0.2, MaxTokens: 16384, StrictMode: false}, // Default false, override per model
	"ollama":     {Temperature: 0.1, MaxTokens: 16384, StrictMode: false}, // Local models don't support strict
	"helicone":   {Temperature: 0.2, MaxTokens: 16384, StrictMode: true},  // Helicone proxies OpenAI-compatible APIs
}

// ModelOverrides is deprecated - model settings should come from S3 config.
// This is kept minimal for bootstrapping only.
var ModelOverrides = map[string]ModelSettings{}

// GetModelSettings returns the recommended settings for a model.
// Priority: chain config override > model override > provider default
// Uses S3-backed defaults if configured via InitGlobalModelDefaults.
// Note: StrictMode from this function uses static defaults. For dynamic capability detection
// (e.g., from OpenRouter API), use GetModelSettingsWithCapabilities instead.
func GetModelSettings(provider, model string, chainTemp *float64, chainMaxTokens *int, chainStrictMode *bool) (temperature float64, maxTokens int, strictMode bool) {
	// Use global loader if available (supports S3-backed configuration)
	return GlobalModelDefaults().GetModelSettings(provider, model, chainTemp, chainMaxTokens, chainStrictMode)
}


// GetDefaultSettings returns the default settings for a model without chain overrides.
// This is useful for frontend to show recommended defaults.
// Uses S3-backed defaults if configured via InitGlobalModelDefaults.
func GetDefaultSettings(provider, model string) ModelSettings {
	return GlobalModelDefaults().GetDefaultSettings(provider, model)
}

// AnalyzeSystemPrompt is the system prompt used for schema generation.
// This reinforces the key instructions for generating good extraction schemas.
const AnalyzeSystemPrompt = `You generate data extraction schemas. Your field descriptions are INSTRUCTIONS to other AI models that will perform extraction. Every description must specify: what to extract, the expected format, and how to handle edge cases. Schema-level descriptions must include CRITICAL OUTPUT REQUIREMENTS.`
