package llm

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/jmylchreest/refyne-api/internal/config"
	"github.com/jmylchreest/refyne-api/internal/constants"
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
	})

	return r
}

// listOpenRouterModels returns a ModelLister that fetches models from OpenRouter.
// It uses the registry's cached capabilities when available.
func listOpenRouterModels(r *Registry) ModelLister {
	return func(ctx context.Context, baseURL, apiKey string) ([]ModelInfo, error) {
		// Return static models - capabilities are populated by PricingService via the registry cache
		return getStaticOpenRouterModels(r), nil
	}
}

// getOpenRouterCapabilities returns a CapabilitiesLookup that checks the registry cache first.
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

// listOllamaModels fetches models from a local Ollama instance.
func listOllamaModels(ctx context.Context, baseURL, apiKey string) ([]ModelInfo, error) {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}

	client := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/tags", nil)
	if err != nil {
		return getStaticOllamaModels(), nil
	}

	resp, err := client.Do(req)
	if err != nil {
		// Ollama not running - return default models
		return getStaticOllamaModels(), nil
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return getStaticOllamaModels(), nil
	}

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return getStaticOllamaModels(), nil
	}

	// Ollama default capabilities - most models support streaming but not structured outputs
	ollamaCaps := ModelCapabilities{
		SupportsStructuredOutputs: false,
		SupportsTools:             false,
		SupportsStreaming:         true,
		SupportsReasoning:         false,
		SupportsResponseFormat:    false,
	}

	models := make([]ModelInfo, 0, len(result.Models))
	for _, m := range result.Models {
		name := strings.TrimSuffix(m.Name, ":latest")
		settings := GetDefaultSettings("ollama", m.Name)
		models = append(models, ModelInfo{
			ID:               m.Name,
			Name:             name,
			Provider:         "ollama",
			Capabilities:     ollamaCaps,
			DefaultTemp:      settings.Temperature,
			DefaultMaxTokens: settings.MaxTokens,
		})
	}

	sort.Slice(models, func(i, j int) bool {
		return models[i].Name < models[j].Name
	})

	return models, nil
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
