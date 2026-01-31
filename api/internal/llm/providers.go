package llm

import (
	"context"
	"log/slog"
	"sort"

	"github.com/jmylchreest/refyne-api/internal/config"
	"github.com/jmylchreest/refyne-api/internal/constants"
	refynellm "github.com/jmylchreest/refyne/pkg/llm"
)

// InitRegistry creates and initializes the provider registry with all supported providers.
func InitRegistry(cfg *config.Config, logger *slog.Logger) *Registry {
	r := NewRegistry(cfg, logger)

	// OpenRouter - always available
	// Capabilities are dynamically populated by PricingService, fallback to static defaults
	r.Register("openrouter", ProviderRegistration{
		Info: ProviderInfo{
			Name:           "openrouter",
			DisplayName:    "OpenRouter",
			Description:    "Access multiple LLM providers through one API",
			RequiresKey:    true,
			KeyPlaceholder: "sk-or-...",
			DocsURL:        "https://openrouter.ai/docs",
		},
		RequiredFeatures: nil,
		ListModels:       listOpenRouterModels(r),
		GetCapabilities:  getOpenRouterCapabilities(r),
		APIConfig: ProviderAPIConfig{
			BaseURL:              "https://openrouter.ai/api",
			ChatEndpoint:         "/v1/chat/completions",
			AuthType:             AuthTypeBearer,
			APIFormat:            APIFormatOpenAI,
			AllowBaseURLOverride: false,
			ExtraHeaders: map[string]string{
				"HTTP-Referer": "https://refyne.io",
				"X-Title":      "Refyne",
			},
		},
		Status: ProviderStatusActive,
	})

	// Anthropic - always available
	r.Register("anthropic", ProviderRegistration{
		Info: ProviderInfo{
			Name:           "anthropic",
			DisplayName:    "Anthropic",
			Description:    "Claude models - Claude 4, Claude 3.5 Sonnet, and more",
			RequiresKey:    true,
			KeyPlaceholder: "sk-ant-...",
			DocsURL:        "https://docs.anthropic.com",
		},
		RequiredFeatures: nil,
		ListModels:       listAnthropicModels,
		GetCapabilities:  getAnthropicCapabilities,
		APIConfig: ProviderAPIConfig{
			BaseURL:              "https://api.anthropic.com",
			ChatEndpoint:         "/v1/messages",
			AuthType:             AuthTypeAPIKey,
			AuthHeader:           "x-api-key",
			APIFormat:            APIFormatAnthropic,
			AllowBaseURLOverride: false,
			ExtraHeaders: map[string]string{
				"anthropic-version": "2023-06-01",
			},
		},
		Status: ProviderStatusActive,
	})

	// OpenAI - always available
	r.Register("openai", ProviderRegistration{
		Info: ProviderInfo{
			Name:           "openai",
			DisplayName:    "OpenAI",
			Description:    "GPT-4o, GPT-4 Turbo, and other OpenAI models",
			RequiresKey:    true,
			KeyPlaceholder: "sk-...",
			DocsURL:        "https://platform.openai.com/docs",
		},
		RequiredFeatures: nil,
		ListModels:       listOpenAIModels,
		GetCapabilities:  getOpenAICapabilities,
		APIConfig: ProviderAPIConfig{
			BaseURL:              "https://api.openai.com",
			ChatEndpoint:         "/v1/chat/completions",
			AuthType:             AuthTypeBearer,
			APIFormat:            APIFormatOpenAI,
			AllowBaseURLOverride: false,
		},
		Status: ProviderStatusActive,
	})

	// Ollama - requires provider_ollama feature (self-hosted only)
	r.Register("ollama", ProviderRegistration{
		Info: ProviderInfo{
			Name:           "ollama",
			DisplayName:    "Ollama",
			Description:    "Self-hosted local models (Llama, Mistral, Gemma, etc.)",
			RequiresKey:    false,
			BaseURLHint:    "http://localhost:11434",
			DocsURL:        "https://ollama.ai",
		},
		RequiredFeatures: []string{constants.FeatureProviderOllama},
		ListModels:       listOllamaModels,
		GetCapabilities:  getOllamaCapabilities,
		APIConfig: ProviderAPIConfig{
			BaseURL:              "http://localhost:11434",
			ChatEndpoint:         "/api/chat",
			AuthType:             AuthTypeNone,
			APIFormat:            APIFormatOllama,
			AllowBaseURLOverride: true, // Self-hosted, allow custom URLs
		},
		Status: ProviderStatusActive,
	})

	// Helicone - LLM gateway with observability (cloud or self-hosted)
	r.Register("helicone", ProviderRegistration{
		Info: ProviderInfo{
			Name:           "helicone",
			DisplayName:    "Helicone",
			Description:    "LLM gateway with observability (cloud or self-hosted)",
			RequiresKey:    true,
			KeyPlaceholder: "sk-helicone-...",
			BaseURLHint:    "Leave empty for cloud, or enter self-hosted URL",
			DocsURL:        "https://docs.helicone.ai",
		},
		RequiredFeatures: nil,
		ListModels:       listHeliconeModels,
		GetCapabilities:  getHeliconeCapabilities,
		APIConfig: ProviderAPIConfig{
			BaseURL:              HeliconeCloudBaseURL,
			ChatEndpoint:         "/v1/chat/completions",
			AuthType:             AuthTypeBearer,
			APIFormat:            APIFormatOpenAI,
			AllowBaseURLOverride: true, // Self-hostable
		},
		Status: ProviderStatusActive,
	})

	return r
}

// listOpenRouterModels returns a ModelLister that fetches models from OpenRouter.
// It uses refyne's OpenRouter provider when an API key is available.
func listOpenRouterModels(r *Registry) ModelLister {
	return func(ctx context.Context, baseURL, apiKey string) ([]ModelInfo, error) {
		// If we have an API key, use refyne's provider to fetch live models
		if apiKey != "" {
			cfg := refynellm.ProviderConfig{
				APIKey:  apiKey,
				BaseURL: baseURL,
			}
			provider, err := refynellm.NewOpenRouterProvider(cfg)
			if err == nil {
				models, err := provider.ListModels(ctx)
				if err == nil && len(models) > 0 {
					// Convert refyne models to refyne-api format
					result := make([]ModelInfo, 0, len(models))
					for _, m := range models {
						result = append(result, ConvertModelInfo("openrouter", m))
					}
					return result, nil
				}
			}
		}

		// Fall back to static models with cached capabilities
		return getStaticOpenRouterModels(r), nil
	}
}

// getOpenRouterCapabilities returns a CapabilitiesLookup that checks the registry cache first,
// then falls back to refyne's provider or static defaults.
func getOpenRouterCapabilities(r *Registry) CapabilitiesLookup {
	return func(ctx context.Context, model string) ModelCapabilities {
		// Check if cached by PricingService
		if caps, ok := r.getCachedCapabilities("openrouter", model); ok {
			return caps
		}
		// Fall back to static defaults for known models
		return getStaticOpenRouterCapabilities(model)
	}
}

// getStaticOpenRouterCapabilities returns static capability defaults for OpenRouter models.
func getStaticOpenRouterCapabilities(model string) ModelCapabilities {
	// Known models with structured output support
	structuredOutputModels := map[string]bool{
		"anthropic/claude-sonnet-4":     true,
		"openai/gpt-4o":                 true,
		"openai/gpt-4o-mini":            true,
		"google/gemini-2.0-flash-001":   true,
	}

	return ModelCapabilities{
		SupportsStructuredOutputs: structuredOutputModels[model],
		SupportsStreaming:         true, // OpenRouter always supports streaming
	}
}

func getStaticOpenRouterModels(r *Registry) []ModelInfo {
	staticModels := []struct {
		id      string
		name    string
		context int
	}{
		{"anthropic/claude-sonnet-4", "Claude Sonnet 4", 200000},
		{"anthropic/claude-3.5-sonnet", "Claude 3.5 Sonnet", 200000},
		{"openai/gpt-4o", "GPT-4o", 128000},
		{"openai/gpt-4o-mini", "GPT-4o Mini", 128000},
		{"google/gemini-2.0-flash-001", "Gemini 2.0 Flash", 1000000},
		{"meta-llama/llama-3.3-70b-instruct", "Llama 3.3 70B", 131072},
	}

	models := make([]ModelInfo, 0, len(staticModels))
	for _, m := range staticModels {
		settings := GetDefaultSettings("openrouter", m.id)
		// Get capabilities from registry cache or static defaults
		caps := r.GetModelCapabilities(context.Background(), "openrouter", m.id)
		models = append(models, ModelInfo{
			ID:               m.id,
			Name:             m.name,
			Provider:         "openrouter",
			ContextWindow:    m.context,
			Capabilities:     caps,
			DefaultTemp:      settings.Temperature,
			DefaultMaxTokens: settings.MaxTokens,
		})
	}

	return models
}

// listAnthropicModels returns static Anthropic models (no public API).
func listAnthropicModels(ctx context.Context, baseURL, apiKey string) ([]ModelInfo, error) {
	staticModels := []struct {
		id      string
		name    string
		context int
	}{
		{"claude-opus-4-20250514", "Claude Opus 4", 200000},
		{"claude-sonnet-4-5-20250514", "Claude Sonnet 4.5", 200000},
		{"claude-sonnet-4-20250514", "Claude Sonnet 4", 200000},
		{"claude-3-7-sonnet-20250219", "Claude 3.7 Sonnet", 200000},
		{"claude-3-5-sonnet-20241022", "Claude 3.5 Sonnet v2", 200000},
		{"claude-3-5-haiku-20241022", "Claude 3.5 Haiku", 200000},
		{"claude-3-opus-20240229", "Claude 3 Opus", 200000},
		{"claude-3-sonnet-20240229", "Claude 3 Sonnet", 200000},
		{"claude-3-haiku-20240307", "Claude 3 Haiku", 200000},
	}

	// Anthropic uses tool_use for structured outputs, not response_format
	anthropicCaps := ModelCapabilities{
		SupportsStructuredOutputs: false,
		SupportsTools:             true,
		SupportsStreaming:         true,
	}

	models := make([]ModelInfo, 0, len(staticModels))
	for _, m := range staticModels {
		settings := GetDefaultSettings("anthropic", m.id)
		models = append(models, ModelInfo{
			ID:               m.id,
			Name:             m.name,
			Provider:         "anthropic",
			ContextWindow:    m.context,
			Capabilities:     anthropicCaps,
			DefaultTemp:      settings.Temperature,
			DefaultMaxTokens: settings.MaxTokens,
		})
	}

	return models, nil
}

// listOpenAIModels returns static OpenAI models.
func listOpenAIModels(ctx context.Context, baseURL, apiKey string) ([]ModelInfo, error) {
	staticModels := []struct {
		id                        string
		name                      string
		context                   int
		supportsStructuredOutputs bool
		supportsReasoning         bool
	}{
		{"gpt-4o", "GPT-4o", 128000, true, false},
		{"gpt-4o-mini", "GPT-4o Mini", 128000, true, false},
		{"gpt-4-turbo", "GPT-4 Turbo", 128000, true, false},
		{"gpt-4", "GPT-4", 8192, false, false},
		{"gpt-3.5-turbo", "GPT-3.5 Turbo", 16385, false, false},
		{"o1", "o1", 200000, true, true},
		{"o1-mini", "o1 Mini", 128000, true, true},
		{"o3-mini", "o3 Mini", 200000, true, true},
	}

	models := make([]ModelInfo, 0, len(staticModels))
	for _, m := range staticModels {
		settings := GetDefaultSettings("openai", m.id)
		models = append(models, ModelInfo{
			ID:            m.id,
			Name:          m.name,
			Provider:      "openai",
			ContextWindow: m.context,
			Capabilities: ModelCapabilities{
				SupportsStructuredOutputs: m.supportsStructuredOutputs,
				SupportsTools:             true,
				SupportsStreaming:         true,
				SupportsReasoning:         m.supportsReasoning,
				SupportsResponseFormat:    true,
			},
			DefaultTemp:      settings.Temperature,
			DefaultMaxTokens: settings.MaxTokens,
		})
	}

	return models, nil
}

// listOllamaModels fetches models from a local Ollama instance using refyne's provider.
func listOllamaModels(ctx context.Context, baseURL, apiKey string) ([]ModelInfo, error) {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}

	// Use refyne's Ollama provider
	cfg := refynellm.ProviderConfig{
		BaseURL: baseURL,
	}
	provider, err := refynellm.NewOllamaProvider(cfg)
	if err != nil {
		return getStaticOllamaModels(), nil
	}

	models, err := provider.ListModels(ctx)
	if err != nil || len(models) == 0 {
		// Ollama not running or no models - return defaults
		return getStaticOllamaModels(), nil
	}

	// Convert refyne models to refyne-api format
	result := make([]ModelInfo, 0, len(models))
	for _, m := range models {
		result = append(result, ConvertModelInfo("ollama", m))
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result, nil
}

func getStaticOllamaModels() []ModelInfo {
	staticModels := []string{
		"llama3.2",
		"llama3.1",
		"mistral",
		"gemma2",
		"qwen2.5",
		"deepseek-coder-v2",
	}

	models := make([]ModelInfo, 0, len(staticModels))
	for _, id := range staticModels {
		settings := GetDefaultSettings("ollama", id)
		models = append(models, ModelInfo{
			ID:               id,
			Name:             id,
			Provider:         "ollama",
			Capabilities:     getOllamaCapabilities(context.Background(), id),
			DefaultTemp:      settings.Temperature,
			DefaultMaxTokens: settings.MaxTokens,
		})
	}

	return models
}

// Provider capability lookup functions
// These return static defaults; dynamic capabilities are cached in the registry by PricingService

// getAnthropicCapabilities returns capabilities for Anthropic models.
// Anthropic uses tool_use for structured outputs, not response_format.
func getAnthropicCapabilities(_ context.Context, _ string) ModelCapabilities {
	return ModelCapabilities{
		SupportsStructuredOutputs: false, // Uses tool_use instead
		SupportsTools:             true,
		SupportsStreaming:         true,
		SupportsReasoning:         false,
		SupportsResponseFormat:    false,
	}
}

// getOpenAICapabilities returns capabilities for OpenAI models.
func getOpenAICapabilities(_ context.Context, model string) ModelCapabilities {
	// o1/o3 models have reasoning support
	reasoningModels := map[string]bool{
		"o1":      true,
		"o1-mini": true,
		"o3-mini": true,
	}

	// Older models don't support structured outputs
	noStructuredOutputs := map[string]bool{
		"gpt-4":        true,
		"gpt-3.5-turbo": true,
	}

	return ModelCapabilities{
		SupportsStructuredOutputs: !noStructuredOutputs[model],
		SupportsTools:             true,
		SupportsStreaming:         true,
		SupportsReasoning:         reasoningModels[model],
		SupportsResponseFormat:    true,
	}
}

// getOllamaCapabilities returns capabilities for Ollama models.
// Most Ollama models support streaming but not structured outputs.
func getOllamaCapabilities(_ context.Context, _ string) ModelCapabilities {
	return ModelCapabilities{
		SupportsStructuredOutputs: false,
		SupportsTools:             false,
		SupportsStreaming:         true,
		SupportsReasoning:         false,
		SupportsResponseFormat:    false,
	}
}

// listHeliconeModels returns available models through Helicone.
// Helicone Cloud provides access to multiple provider models.
func listHeliconeModels(ctx context.Context, baseURL, apiKey string) ([]ModelInfo, error) {
	// Helicone is a gateway - it proxies to underlying providers
	// Return common models available through the gateway
	staticModels := []struct {
		id      string
		name    string
		context int
	}{
		{"gpt-4o", "GPT-4o", 128000},
		{"gpt-4o-mini", "GPT-4o Mini", 128000},
		{"gpt-4-turbo", "GPT-4 Turbo", 128000},
		{"claude-3-5-sonnet-20241022", "Claude 3.5 Sonnet", 200000},
		{"claude-3-haiku-20240307", "Claude 3 Haiku", 200000},
		{"gemini-1.5-pro", "Gemini 1.5 Pro", 1000000},
	}

	models := make([]ModelInfo, 0, len(staticModels))
	for _, m := range staticModels {
		settings := GetDefaultSettings("helicone", m.id)
		models = append(models, ModelInfo{
			ID:               m.id,
			Name:             m.name,
			Provider:         "helicone",
			ContextWindow:    m.context,
			Capabilities:     getHeliconeCapabilities(ctx, m.id),
			DefaultTemp:      settings.Temperature,
			DefaultMaxTokens: settings.MaxTokens,
		})
	}

	return models, nil
}

// getHeliconeCapabilities returns capabilities for Helicone models.
// Since Helicone proxies to OpenAI-compatible APIs, most support standard features.
func getHeliconeCapabilities(_ context.Context, model string) ModelCapabilities {
	// Models proxied through Helicone generally support OpenAI-style features
	return ModelCapabilities{
		SupportsStructuredOutputs: true,
		SupportsTools:             true,
		SupportsStreaming:         true,
		SupportsReasoning:         false,
		SupportsResponseFormat:    true,
	}
}
