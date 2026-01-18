package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/jmylchreest/refyne-api/internal/config"
	"github.com/jmylchreest/refyne-api/internal/constants"
)

// InitRegistry creates and initializes the provider registry with all supported providers.
func InitRegistry(cfg *config.Config) *Registry {
	r := NewRegistry(cfg)

	// OpenRouter - always available
	r.Register("openrouter", ProviderRegistration{
		Info: ProviderInfo{
			Name:           "openrouter",
			DisplayName:    "OpenRouter",
			Description:    "Access multiple LLM providers through one API",
			RequiresKey:    true,
			KeyPlaceholder: "sk-or-...",
			DocsURL:        "https://openrouter.ai/docs",
		},
		RequiredFeatures: nil, // Always available
		ListModels:       listOpenRouterModels,
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
		RequiredFeatures: nil, // Always available
		ListModels:       listAnthropicModels,
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
		RequiredFeatures: nil, // Always available
		ListModels:       listOpenAIModels,
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
	})

	return r
}

// listOpenRouterModels fetches models from the OpenRouter API.
func listOpenRouterModels(ctx context.Context, baseURL, apiKey string) ([]ModelInfo, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://openrouter.ai/api/v1/models", nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return getStaticOpenRouterModels(), nil
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return getStaticOpenRouterModels(), nil
	}

	var result struct {
		Data []struct {
			ID            string `json:"id"`
			Name          string `json:"name"`
			ContextLength int    `json:"context_length"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return getStaticOpenRouterModels(), nil
	}

	models := make([]ModelInfo, 0, len(result.Data))
	for _, m := range result.Data {
		settings := GetDefaultSettings("openrouter", m.ID)
		models = append(models, ModelInfo{
			ID:               m.ID,
			Name:             m.Name,
			Provider:         "openrouter",
			ContextWindow:    m.ContextLength,
			SupportsStrict:   settings.StrictMode,
			DefaultTemp:      settings.Temperature,
			DefaultMaxTokens: settings.MaxTokens,
		})
	}

	sort.Slice(models, func(i, j int) bool {
		return models[i].Name < models[j].Name
	})

	return models, nil
}

func getStaticOpenRouterModels() []ModelInfo {
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
		models = append(models, ModelInfo{
			ID:               m.id,
			Name:             m.name,
			Provider:         "openrouter",
			ContextWindow:    m.context,
			SupportsStrict:   settings.StrictMode,
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

	models := make([]ModelInfo, 0, len(staticModels))
	for _, m := range staticModels {
		settings := GetDefaultSettings("anthropic", m.id)
		models = append(models, ModelInfo{
			ID:               m.id,
			Name:             m.name,
			Provider:         "anthropic",
			ContextWindow:    m.context,
			SupportsStrict:   settings.StrictMode,
			DefaultTemp:      settings.Temperature,
			DefaultMaxTokens: settings.MaxTokens,
		})
	}

	return models, nil
}

// listOpenAIModels returns static OpenAI models.
func listOpenAIModels(ctx context.Context, baseURL, apiKey string) ([]ModelInfo, error) {
	staticModels := []struct {
		id      string
		name    string
		context int
	}{
		{"gpt-4o", "GPT-4o", 128000},
		{"gpt-4o-mini", "GPT-4o Mini", 128000},
		{"gpt-4-turbo", "GPT-4 Turbo", 128000},
		{"gpt-4", "GPT-4", 8192},
		{"gpt-3.5-turbo", "GPT-3.5 Turbo", 16385},
		{"o1", "o1", 200000},
		{"o1-mini", "o1 Mini", 128000},
		{"o3-mini", "o3 Mini", 200000},
	}

	models := make([]ModelInfo, 0, len(staticModels))
	for _, m := range staticModels {
		settings := GetDefaultSettings("openai", m.id)
		models = append(models, ModelInfo{
			ID:               m.id,
			Name:             m.name,
			Provider:         "openai",
			ContextWindow:    m.context,
			SupportsStrict:   settings.StrictMode,
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

	models := make([]ModelInfo, 0, len(result.Models))
	for _, m := range result.Models {
		name := strings.TrimSuffix(m.Name, ":latest")
		settings := GetDefaultSettings("ollama", m.Name)
		models = append(models, ModelInfo{
			ID:               m.Name,
			Name:             name,
			Provider:         "ollama",
			SupportsStrict:   settings.StrictMode,
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
			SupportsStrict:   settings.StrictMode,
			DefaultTemp:      settings.Temperature,
			DefaultMaxTokens: settings.MaxTokens,
		})
	}

	return models
}
