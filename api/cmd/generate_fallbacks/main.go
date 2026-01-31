// Command generate_fallbacks fetches models from LLM provider APIs and generates
// configuration files for dynamic model loading.
//
// Usage:
//
//	go run ./cmd/generate_fallbacks
//
// Outputs:
//   - config/provider_models.json       - S3-compatible JSON config with pricing/capabilities
//   - ../web/public/provider-models.json - Static asset for web frontend
//
// The generator fetches from provider APIs where available (Helicone, OpenRouter),
// and uses curated lists for providers without public APIs (Anthropic, OpenAI, Ollama).
//
// IMPORTANT: No embedded Go fallbacks are generated. Models are loaded dynamically
// from provider APIs at runtime, with S3 config as the fallback source.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var (
	configOutput = flag.String("config", "config/provider_models.json", "S3-compatible JSON config output")
	webOutput    = flag.String("web", "../web/public/provider-models.json", "Web static asset output")
	embedOutput  = flag.String("embed", "internal/llm/provider_models_embed.json", "Embedded fallback for binary")
	skipWeb      = flag.Bool("skip-web", false, "Skip web output generation")
	skipEmbed    = flag.Bool("skip-embed", false, "Skip embedded output generation")
)

// =============================================================================
// Data structures for output files
// =============================================================================

// ProviderModelsConfig is the S3-compatible JSON structure
type ProviderModelsConfig struct {
	GeneratedAt string                       `json:"generated_at"`
	Version     string                       `json:"version"`
	Providers   map[string]ProviderModelList `json:"providers"`
}

type ProviderModelList struct {
	DisplayName string      `json:"display_name"`
	Models      []ModelData `json:"models"`
}

type ModelData struct {
	ID                  string           `json:"id"`
	Name                string           `json:"name"`
	ContextWindow       int              `json:"context_window"`
	MaxOutput           int              `json:"max_output,omitempty"`
	Pricing             *ModelPricing    `json:"pricing,omitempty"`
	Capabilities        ModelCapabilites `json:"capabilities,omitempty"`
}

type ModelPricing struct {
	PromptPricePer1M     float64 `json:"prompt_price_per_1m"`
	CompletionPricePer1M float64 `json:"completion_price_per_1m"`
	IsFree               bool    `json:"is_free,omitempty"`
}

type ModelCapabilites struct {
	SupportsStructuredOutputs bool `json:"supports_structured_outputs,omitempty"`
	SupportsTools             bool `json:"supports_tools,omitempty"`
	SupportsStreaming         bool `json:"supports_streaming,omitempty"`
	SupportsVision            bool `json:"supports_vision,omitempty"`
	SupportsReasoning         bool `json:"supports_reasoning,omitempty"`
}

// ProviderFetcher defines how to fetch models for a provider
type ProviderFetcher struct {
	Name        string
	DisplayName string
	FetchFunc   func(ctx context.Context) ([]ModelData, error)
	MaxModels   int // Max models to include in fallback (0 = all)
}

// Registry of provider fetchers
var providerFetchers = []ProviderFetcher{
	{Name: "helicone", DisplayName: "Helicone", FetchFunc: fetchHeliconeModels, MaxModels: 0},
	{Name: "openrouter", DisplayName: "OpenRouter", FetchFunc: fetchOpenRouterModels, MaxModels: 0},
	{Name: "anthropic", DisplayName: "Anthropic", FetchFunc: fetchAnthropicModels, MaxModels: 0},
	{Name: "openai", DisplayName: "OpenAI", FetchFunc: fetchOpenAIModels, MaxModels: 0},
	{Name: "ollama", DisplayName: "Ollama", FetchFunc: fetchOllamaModels, MaxModels: 0},
}

func main() {
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	config := ProviderModelsConfig{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Version:     "1.0.0",
		Providers:   make(map[string]ProviderModelList),
	}

	for _, fetcher := range providerFetchers {
		log.Printf("Fetching %s models...", fetcher.DisplayName)

		models, err := fetcher.FetchFunc(ctx)
		if err != nil {
			log.Printf("Warning: failed to fetch %s models: %v", fetcher.Name, err)
			continue
		}

		// Apply max limit if specified
		if fetcher.MaxModels > 0 && len(models) > fetcher.MaxModels {
			models = selectTopModels(models, fetcher.MaxModels, fetcher.Name)
		}

		config.Providers[fetcher.Name] = ProviderModelList{
			DisplayName: fetcher.DisplayName,
			Models:      models,
		}

		log.Printf("Fetched %d %s models", len(models), fetcher.Name)
	}

	// Generate outputs
	if err := generateJSONConfig(*configOutput, config); err != nil {
		log.Printf("Warning: failed to generate config JSON: %v", err)
	} else {
		log.Printf("Generated %s", *configOutput)
	}

	if !*skipWeb {
		if err := generateJSONConfig(*webOutput, config); err != nil {
			log.Printf("Warning: failed to generate web JSON: %v", err)
		} else {
			log.Printf("Generated %s", *webOutput)
		}
	}

	if !*skipEmbed {
		if err := generateJSONConfig(*embedOutput, config); err != nil {
			log.Printf("Warning: failed to generate embed JSON: %v", err)
		} else {
			log.Printf("Generated %s", *embedOutput)
		}
	}

	log.Printf("Done! Generated model configs for %d providers", len(config.Providers))
}

// =============================================================================
// Provider fetchers - API-based where available
// =============================================================================

// fetchHeliconeModels fetches from Helicone's public model registry API.
// Includes pricing data from the API response.
func fetchHeliconeModels(ctx context.Context) ([]ModelData, error) {
	const url = "https://api.helicone.ai/v1/public/model-registry/models"

	body, err := httpGet(ctx, url)
	if err != nil {
		return nil, err
	}

	var result struct {
		Data struct {
			Models []struct {
				ID            string `json:"id"`
				Name          string `json:"name"`
				ContextLength int    `json:"contextLength"`
				MaxOutput     int    `json:"maxOutput"`
				Capabilities  []string `json:"capabilities"`
				Endpoints     []struct {
					Pricing struct {
						Prompt     float64 `json:"prompt"`
						Completion float64 `json:"completion"`
					} `json:"pricing"`
				} `json:"endpoints"`
			} `json:"models"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	models := make([]ModelData, 0, len(result.Data.Models))
	for _, m := range result.Data.Models {
		model := ModelData{
			ID:            m.ID,
			Name:          m.Name,
			ContextWindow: m.ContextLength,
			MaxOutput:     m.MaxOutput,
			Capabilities:  parseCapabilities(m.Capabilities),
		}

		// Extract pricing from first endpoint
		if len(m.Endpoints) > 0 {
			p := m.Endpoints[0].Pricing
			// Helicone returns per-token prices, convert to per-1M
			model.Pricing = &ModelPricing{
				PromptPricePer1M:     p.Prompt * 1_000_000,
				CompletionPricePer1M: p.Completion * 1_000_000,
				IsFree:               p.Prompt == 0 && p.Completion == 0,
			}
		}

		models = append(models, model)
	}

	return models, nil
}

// fetchOpenRouterModels fetches from OpenRouter's public models API.
// Includes pricing and capabilities from the API response.
func fetchOpenRouterModels(ctx context.Context) ([]ModelData, error) {
	const url = "https://openrouter.ai/api/v1/models"

	body, err := httpGet(ctx, url)
	if err != nil {
		return nil, err
	}

	var result struct {
		Data []struct {
			ID                  string   `json:"id"`
			Name                string   `json:"name"`
			ContextLength       int      `json:"context_length"`
			SupportedParameters []string `json:"supported_parameters"`
			Pricing             struct {
				Prompt     string `json:"prompt"`
				Completion string `json:"completion"`
			} `json:"pricing"`
			TopProvider struct {
				MaxCompletionTokens int `json:"max_completion_tokens"`
			} `json:"top_provider"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	models := make([]ModelData, 0, len(result.Data))
	for _, m := range result.Data {
		promptPrice := parsePrice(m.Pricing.Prompt)
		completionPrice := parsePrice(m.Pricing.Completion)

		model := ModelData{
			ID:            m.ID,
			Name:          m.Name,
			ContextWindow: m.ContextLength,
			MaxOutput:     m.TopProvider.MaxCompletionTokens,
			Capabilities:  parseCapabilities(m.SupportedParameters),
			Pricing: &ModelPricing{
				PromptPricePer1M:     promptPrice * 1_000_000,
				CompletionPricePer1M: completionPrice * 1_000_000,
				IsFree:               promptPrice == 0 && completionPrice == 0,
			},
		}

		models = append(models, model)
	}

	return models, nil
}

// fetchAnthropicModels returns static Anthropic models with pricing.
// Anthropic doesn't have a public models API.
// Pricing source: https://www.anthropic.com/pricing
func fetchAnthropicModels(_ context.Context) ([]ModelData, error) {
	return []ModelData{
		{ID: "claude-opus-4-20250514", Name: "Claude Opus 4", ContextWindow: 200000, MaxOutput: 32000,
			Pricing: &ModelPricing{PromptPricePer1M: 15.0, CompletionPricePer1M: 75.0},
			Capabilities: ModelCapabilites{SupportsTools: true, SupportsStreaming: true, SupportsVision: true}},
		{ID: "claude-sonnet-4-5-20250514", Name: "Claude Sonnet 4.5", ContextWindow: 200000, MaxOutput: 64000,
			Pricing: &ModelPricing{PromptPricePer1M: 3.0, CompletionPricePer1M: 15.0},
			Capabilities: ModelCapabilites{SupportsTools: true, SupportsStreaming: true, SupportsVision: true}},
		{ID: "claude-sonnet-4-20250514", Name: "Claude Sonnet 4", ContextWindow: 200000, MaxOutput: 64000,
			Pricing: &ModelPricing{PromptPricePer1M: 3.0, CompletionPricePer1M: 15.0},
			Capabilities: ModelCapabilites{SupportsTools: true, SupportsStreaming: true, SupportsVision: true}},
		{ID: "claude-3-7-sonnet-20250219", Name: "Claude 3.7 Sonnet", ContextWindow: 200000, MaxOutput: 64000,
			Pricing: &ModelPricing{PromptPricePer1M: 3.0, CompletionPricePer1M: 15.0},
			Capabilities: ModelCapabilites{SupportsTools: true, SupportsStreaming: true, SupportsVision: true}},
		{ID: "claude-3-5-sonnet-20241022", Name: "Claude 3.5 Sonnet v2", ContextWindow: 200000, MaxOutput: 8192,
			Pricing: &ModelPricing{PromptPricePer1M: 3.0, CompletionPricePer1M: 15.0},
			Capabilities: ModelCapabilites{SupportsTools: true, SupportsStreaming: true, SupportsVision: true}},
		{ID: "claude-3-5-haiku-20241022", Name: "Claude 3.5 Haiku", ContextWindow: 200000, MaxOutput: 8192,
			Pricing: &ModelPricing{PromptPricePer1M: 0.80, CompletionPricePer1M: 4.0},
			Capabilities: ModelCapabilites{SupportsTools: true, SupportsStreaming: true, SupportsVision: true}},
		{ID: "claude-3-opus-20240229", Name: "Claude 3 Opus", ContextWindow: 200000, MaxOutput: 4096,
			Pricing: &ModelPricing{PromptPricePer1M: 15.0, CompletionPricePer1M: 75.0},
			Capabilities: ModelCapabilites{SupportsTools: true, SupportsStreaming: true, SupportsVision: true}},
		{ID: "claude-3-sonnet-20240229", Name: "Claude 3 Sonnet", ContextWindow: 200000, MaxOutput: 4096,
			Pricing: &ModelPricing{PromptPricePer1M: 3.0, CompletionPricePer1M: 15.0},
			Capabilities: ModelCapabilites{SupportsTools: true, SupportsStreaming: true, SupportsVision: true}},
		{ID: "claude-3-haiku-20240307", Name: "Claude 3 Haiku", ContextWindow: 200000, MaxOutput: 4096,
			Pricing: &ModelPricing{PromptPricePer1M: 0.25, CompletionPricePer1M: 1.25},
			Capabilities: ModelCapabilites{SupportsTools: true, SupportsStreaming: true, SupportsVision: true}},
	}, nil
}

// fetchOpenAIModels returns static OpenAI models with pricing.
// OpenAI's models API requires auth.
// Pricing source: https://openai.com/api/pricing/
func fetchOpenAIModels(_ context.Context) ([]ModelData, error) {
	return []ModelData{
		{ID: "gpt-4o", Name: "GPT-4o", ContextWindow: 128000, MaxOutput: 16384,
			Pricing: &ModelPricing{PromptPricePer1M: 2.50, CompletionPricePer1M: 10.0},
			Capabilities: ModelCapabilites{SupportsStructuredOutputs: true, SupportsTools: true, SupportsStreaming: true, SupportsVision: true}},
		{ID: "gpt-4o-mini", Name: "GPT-4o Mini", ContextWindow: 128000, MaxOutput: 16384,
			Pricing: &ModelPricing{PromptPricePer1M: 0.15, CompletionPricePer1M: 0.60},
			Capabilities: ModelCapabilites{SupportsStructuredOutputs: true, SupportsTools: true, SupportsStreaming: true, SupportsVision: true}},
		{ID: "gpt-4-turbo", Name: "GPT-4 Turbo", ContextWindow: 128000, MaxOutput: 4096,
			Pricing: &ModelPricing{PromptPricePer1M: 10.0, CompletionPricePer1M: 30.0},
			Capabilities: ModelCapabilites{SupportsStructuredOutputs: true, SupportsTools: true, SupportsStreaming: true, SupportsVision: true}},
		{ID: "gpt-4", Name: "GPT-4", ContextWindow: 8192, MaxOutput: 4096,
			Pricing: &ModelPricing{PromptPricePer1M: 30.0, CompletionPricePer1M: 60.0},
			Capabilities: ModelCapabilites{SupportsTools: true, SupportsStreaming: true}},
		{ID: "gpt-3.5-turbo", Name: "GPT-3.5 Turbo", ContextWindow: 16385, MaxOutput: 4096,
			Pricing: &ModelPricing{PromptPricePer1M: 0.50, CompletionPricePer1M: 1.50},
			Capabilities: ModelCapabilites{SupportsTools: true, SupportsStreaming: true}},
		{ID: "o1", Name: "o1", ContextWindow: 200000, MaxOutput: 100000,
			Pricing: &ModelPricing{PromptPricePer1M: 15.0, CompletionPricePer1M: 60.0},
			Capabilities: ModelCapabilites{SupportsStructuredOutputs: true, SupportsStreaming: true, SupportsReasoning: true}},
		{ID: "o1-mini", Name: "o1 Mini", ContextWindow: 128000, MaxOutput: 65536,
			Pricing: &ModelPricing{PromptPricePer1M: 3.0, CompletionPricePer1M: 12.0},
			Capabilities: ModelCapabilites{SupportsStructuredOutputs: true, SupportsStreaming: true, SupportsReasoning: true}},
		{ID: "o3-mini", Name: "o3 Mini", ContextWindow: 200000, MaxOutput: 100000,
			Pricing: &ModelPricing{PromptPricePer1M: 1.10, CompletionPricePer1M: 4.40},
			Capabilities: ModelCapabilites{SupportsStructuredOutputs: true, SupportsStreaming: true, SupportsReasoning: true}},
	}, nil
}

// fetchOllamaModels returns common Ollama models.
// Ollama is local-only, so we maintain a static list. No pricing (free).
func fetchOllamaModels(_ context.Context) ([]ModelData, error) {
	return []ModelData{
		{ID: "llama3.2", Name: "Llama 3.2", ContextWindow: 128000,
			Pricing: &ModelPricing{IsFree: true},
			Capabilities: ModelCapabilites{SupportsStreaming: true}},
		{ID: "llama3.1", Name: "Llama 3.1", ContextWindow: 128000,
			Pricing: &ModelPricing{IsFree: true},
			Capabilities: ModelCapabilites{SupportsStreaming: true}},
		{ID: "mistral", Name: "Mistral", ContextWindow: 32768,
			Pricing: &ModelPricing{IsFree: true},
			Capabilities: ModelCapabilites{SupportsStreaming: true}},
		{ID: "gemma2", Name: "Gemma 2", ContextWindow: 8192,
			Pricing: &ModelPricing{IsFree: true},
			Capabilities: ModelCapabilites{SupportsStreaming: true}},
		{ID: "qwen2.5", Name: "Qwen 2.5", ContextWindow: 32768,
			Pricing: &ModelPricing{IsFree: true},
			Capabilities: ModelCapabilites{SupportsStreaming: true}},
		{ID: "deepseek-coder-v2", Name: "DeepSeek Coder V2", ContextWindow: 128000,
			Pricing: &ModelPricing{IsFree: true},
			Capabilities: ModelCapabilites{SupportsStreaming: true}},
		{ID: "phi3", Name: "Phi-3", ContextWindow: 128000,
			Pricing: &ModelPricing{IsFree: true},
			Capabilities: ModelCapabilites{SupportsStreaming: true}},
		{ID: "codellama", Name: "Code Llama", ContextWindow: 16384,
			Pricing: &ModelPricing{IsFree: true},
			Capabilities: ModelCapabilites{SupportsStreaming: true}},
	}, nil
}

// =============================================================================
// Helpers
// =============================================================================

func httpGet(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

func parsePrice(s string) float64 {
	if s == "" || s == "0" {
		return 0
	}
	var price float64
	fmt.Sscanf(s, "%f", &price)
	return price
}

func parseCapabilities(caps []string) ModelCapabilites {
	result := ModelCapabilites{
		SupportsStreaming: true, // Most models support streaming
	}
	for _, c := range caps {
		switch strings.ToLower(c) {
		case "structured_outputs", "json_mode", "response_format":
			result.SupportsStructuredOutputs = true
		case "tools", "tool_choice", "function_calling":
			result.SupportsTools = true
		case "vision", "image":
			result.SupportsVision = true
		case "reasoning", "thinking":
			result.SupportsReasoning = true
		}
	}
	return result
}

func selectTopModels(models []ModelData, maxCount int, provider string) []ModelData {
	if len(models) <= maxCount {
		return models
	}

	// Priority prefixes for popular models
	priorityPrefixes := getPriorityPrefixes(provider)

	type scoredModel struct {
		model ModelData
		score int
	}

	scored := make([]scoredModel, 0, len(models))
	for _, m := range models {
		idLower := strings.ToLower(m.ID)
		score := 1000
		for i, prefix := range priorityPrefixes {
			if strings.HasPrefix(idLower, strings.ToLower(prefix)) {
				score = i
				break
			}
		}
		scored = append(scored, scoredModel{model: m, score: score})
	}

	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score < scored[j].score
		}
		return scored[i].model.ID < scored[j].model.ID
	})

	result := make([]ModelData, 0, maxCount)
	for i := 0; i < len(scored) && len(result) < maxCount; i++ {
		result = append(result, scored[i].model)
	}
	return result
}

func getPriorityPrefixes(provider string) []string {
	switch provider {
	case "helicone":
		return []string{"gpt-4o", "gpt-5", "o3", "claude-sonnet-4", "claude-opus-4", "gemini-2.5"}
	case "openrouter":
		return []string{"anthropic/claude", "openai/gpt-4o", "openai/o3", "google/gemini-2.5"}
	default:
		return nil
	}
}

// =============================================================================
// Output generators
// =============================================================================

func generateJSONConfig(path string, config ProviderModelsConfig) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}

