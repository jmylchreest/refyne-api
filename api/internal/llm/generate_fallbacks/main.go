// Command generate_fallbacks fetches models from LLM providers and generates
// a Go file with static fallback model lists.
//
// Usage:
//
//	go run ./internal/llm/generate_fallbacks -output internal/llm/model_fallbacks_gen.go
//
// This leverages the provider registry to discover all providers and uses
// provider-specific fetching strategies (public APIs, static lists, etc.).
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
	"sort"
	"strings"
	"text/template"
	"time"
)

var outputFile = flag.String("output", "model_fallbacks_gen.go", "Output file path")

// ProviderFetcher defines how to fetch fallback models for a provider.
type ProviderFetcher struct {
	Name        string
	DisplayName string
	FetchFunc   func(ctx context.Context) ([]ModelFallback, error)
	MaxModels   int // Maximum models to include in fallback
}

type ModelFallback struct {
	ID            string
	Name          string
	ContextWindow int
}

type ProviderFallbacks struct {
	Provider    string
	DisplayName string
	Models      []ModelFallback
}

// Registry of provider fetchers - mirrors the LLM registry pattern.
// Each provider defines its own strategy for fetching fallback models.
var providerFetchers = []ProviderFetcher{
	{
		Name:        "helicone",
		DisplayName: "Helicone",
		FetchFunc:   fetchHeliconeModels,
		MaxModels:   15,
	},
	{
		Name:        "openrouter",
		DisplayName: "OpenRouter",
		FetchFunc:   fetchOpenRouterModels,
		MaxModels:   15,
	},
	{
		Name:        "anthropic",
		DisplayName: "Anthropic",
		FetchFunc:   fetchAnthropicModels, // Static - no public API
		MaxModels:   10,
	},
	{
		Name:        "openai",
		DisplayName: "OpenAI",
		FetchFunc:   fetchOpenAIModels, // Static - no public API
		MaxModels:   10,
	},
	{
		Name:        "ollama",
		DisplayName: "Ollama",
		FetchFunc:   fetchOllamaModels, // Static defaults - local only
		MaxModels:   10,
	},
}

func main() {
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	var allFallbacks []ProviderFallbacks

	for _, fetcher := range providerFetchers {
		log.Printf("Fetching %s models...", fetcher.DisplayName)

		models, err := fetcher.FetchFunc(ctx)
		if err != nil {
			log.Printf("Warning: failed to fetch %s models: %v", fetcher.Name, err)
			continue
		}

		// Select top models
		selected := selectTopModels(models, fetcher.MaxModels, fetcher.Name)

		allFallbacks = append(allFallbacks, ProviderFallbacks{
			Provider:    fetcher.Name,
			DisplayName: fetcher.DisplayName,
			Models:      selected,
		})

		log.Printf("Fetched %d %s models, selected top %d", len(models), fetcher.Name, len(selected))
	}

	// Generate the output file
	if err := generateFile(*outputFile, allFallbacks); err != nil {
		log.Fatalf("Failed to generate file: %v", err)
	}

	log.Printf("Generated %s with %d providers", *outputFile, len(allFallbacks))
}

// =============================================================================
// Provider-specific fetchers
// =============================================================================

// fetchHeliconeModels fetches from Helicone's public model registry API.
// Endpoint: https://api.helicone.ai/v1/public/model-registry/models (no auth required)
func fetchHeliconeModels(ctx context.Context) ([]ModelFallback, error) {
	const url = "https://api.helicone.ai/v1/public/model-registry/models"

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

	var result struct {
		Data struct {
			Models []struct {
				ID            string `json:"id"`
				Name          string `json:"name"`
				ContextLength int    `json:"contextLength"`
			} `json:"models"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	models := make([]ModelFallback, 0, len(result.Data.Models))
	for _, m := range result.Data.Models {
		models = append(models, ModelFallback{
			ID:            m.ID,
			Name:          m.Name,
			ContextWindow: m.ContextLength,
		})
	}

	return models, nil
}

// fetchOpenRouterModels fetches from OpenRouter's public models API.
// Endpoint: https://openrouter.ai/api/v1/models (no auth required for listing)
func fetchOpenRouterModels(ctx context.Context) ([]ModelFallback, error) {
	const url = "https://openrouter.ai/api/v1/models"

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

	var result struct {
		Data []struct {
			ID            string `json:"id"`
			Name          string `json:"name"`
			ContextLength int    `json:"context_length"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	models := make([]ModelFallback, 0, len(result.Data))
	for _, m := range result.Data {
		models = append(models, ModelFallback{
			ID:            m.ID,
			Name:          m.Name,
			ContextWindow: m.ContextLength,
		})
	}

	return models, nil
}

// fetchAnthropicModels returns static Anthropic models.
// Anthropic doesn't have a public models API, so we maintain a static list.
// Source: https://docs.anthropic.com/en/docs/about-claude/models
func fetchAnthropicModels(_ context.Context) ([]ModelFallback, error) {
	return []ModelFallback{
		{ID: "claude-opus-4-20250514", Name: "Claude Opus 4", ContextWindow: 200000},
		{ID: "claude-sonnet-4-5-20250514", Name: "Claude Sonnet 4.5", ContextWindow: 200000},
		{ID: "claude-sonnet-4-20250514", Name: "Claude Sonnet 4", ContextWindow: 200000},
		{ID: "claude-3-7-sonnet-20250219", Name: "Claude 3.7 Sonnet", ContextWindow: 200000},
		{ID: "claude-3-5-sonnet-20241022", Name: "Claude 3.5 Sonnet v2", ContextWindow: 200000},
		{ID: "claude-3-5-haiku-20241022", Name: "Claude 3.5 Haiku", ContextWindow: 200000},
		{ID: "claude-3-opus-20240229", Name: "Claude 3 Opus", ContextWindow: 200000},
		{ID: "claude-3-sonnet-20240229", Name: "Claude 3 Sonnet", ContextWindow: 200000},
		{ID: "claude-3-haiku-20240307", Name: "Claude 3 Haiku", ContextWindow: 200000},
	}, nil
}

// fetchOpenAIModels returns static OpenAI models.
// OpenAI's models API requires auth, so we maintain a static list.
// Source: https://platform.openai.com/docs/models
func fetchOpenAIModels(_ context.Context) ([]ModelFallback, error) {
	return []ModelFallback{
		{ID: "gpt-4o", Name: "GPT-4o", ContextWindow: 128000},
		{ID: "gpt-4o-mini", Name: "GPT-4o Mini", ContextWindow: 128000},
		{ID: "gpt-4-turbo", Name: "GPT-4 Turbo", ContextWindow: 128000},
		{ID: "gpt-4", Name: "GPT-4", ContextWindow: 8192},
		{ID: "gpt-3.5-turbo", Name: "GPT-3.5 Turbo", ContextWindow: 16385},
		{ID: "o1", Name: "o1", ContextWindow: 200000},
		{ID: "o1-mini", Name: "o1 Mini", ContextWindow: 128000},
		{ID: "o3-mini", Name: "o3 Mini", ContextWindow: 200000},
	}, nil
}

// fetchOllamaModels returns common Ollama models.
// Ollama is local-only, so we maintain a static list of popular models.
// Source: https://ollama.ai/library
func fetchOllamaModels(_ context.Context) ([]ModelFallback, error) {
	return []ModelFallback{
		{ID: "llama3.2", Name: "Llama 3.2", ContextWindow: 128000},
		{ID: "llama3.1", Name: "Llama 3.1", ContextWindow: 128000},
		{ID: "mistral", Name: "Mistral", ContextWindow: 32768},
		{ID: "gemma2", Name: "Gemma 2", ContextWindow: 8192},
		{ID: "qwen2.5", Name: "Qwen 2.5", ContextWindow: 32768},
		{ID: "deepseek-coder-v2", Name: "DeepSeek Coder V2", ContextWindow: 128000},
		{ID: "phi3", Name: "Phi-3", ContextWindow: 128000},
		{ID: "codellama", Name: "Code Llama", ContextWindow: 16384},
	}, nil
}

// =============================================================================
// Model selection and generation
// =============================================================================

// selectTopModels picks a diverse set of top models.
func selectTopModels(models []ModelFallback, maxCount int, provider string) []ModelFallback {
	if len(models) <= maxCount {
		return models
	}

	// For providers with dynamic APIs, prioritize popular model families
	priorityPrefixes := getPriorityPrefixes(provider)

	// Score models by priority
	type scoredModel struct {
		model ModelFallback
		score int
	}

	scored := make([]scoredModel, 0, len(models))
	for _, m := range models {
		idLower := strings.ToLower(m.ID)
		nameLower := strings.ToLower(m.Name)

		score := 1000 // Default low priority
		for i, prefix := range priorityPrefixes {
			prefixLower := strings.ToLower(prefix)
			if strings.HasPrefix(idLower, prefixLower) || strings.Contains(nameLower, prefixLower) {
				score = i
				break
			}
		}

		scored = append(scored, scoredModel{model: m, score: score})
	}

	// Sort by score (lower is better)
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score < scored[j].score
		}
		return scored[i].model.ID < scored[j].model.ID
	})

	// Take top N with diversity
	result := make([]ModelFallback, 0, maxCount)
	seen := make(map[string]int) // Track provider family counts

	for _, s := range scored {
		if len(result) >= maxCount {
			break
		}

		family := getModelFamily(s.model.ID)
		if seen[family] >= 3 && len(result) < maxCount/2 {
			// Limit per-family in first half for diversity
			continue
		}

		result = append(result, s.model)
		seen[family]++
	}

	return result
}

func getPriorityPrefixes(provider string) []string {
	switch provider {
	case "helicone":
		return []string{
			"gpt-4o", "gpt-5", "o3", "o1",
			"claude-sonnet-4", "claude-opus-4", "claude-4", "claude-3.5",
			"gemini-2.5", "gemini-2.0",
			"gpt-4o-mini", "claude-3.5-haiku",
			"deepseek", "llama-4", "llama-3.3",
		}
	case "openrouter":
		return []string{
			"anthropic/claude-sonnet-4", "anthropic/claude-opus-4", "anthropic/claude-3.5",
			"openai/gpt-4o", "openai/o3", "openai/o1",
			"google/gemini-2.5", "google/gemini-2.0",
			"openai/gpt-4o-mini", "anthropic/claude-3-haiku",
			"meta-llama/llama-4", "meta-llama/llama-3.3",
			"deepseek", "mistralai",
		}
	default:
		return nil
	}
}

func getModelFamily(id string) string {
	// Handle provider/model format
	if idx := strings.Index(id, "/"); idx > 0 {
		return id[:idx]
	}
	// Extract family from model ID
	parts := strings.Split(id, "-")
	if len(parts) >= 2 {
		return parts[0] + "-" + parts[1]
	}
	if len(parts) >= 1 {
		return parts[0]
	}
	return id
}

const fileTemplate = `// Code generated by go run ./internal/llm/generate_fallbacks. DO NOT EDIT.
// Generated: {{.Timestamp}}
//
// To regenerate: go run ./internal/llm/generate_fallbacks -output internal/llm/model_fallbacks_gen.go

package llm

// StaticModelFallback represents a fallback model entry.
type StaticModelFallback struct {
	ID            string
	Name          string
	ContextWindow int
}

{{range .Fallbacks}}
// {{title .Provider}}StaticFallbacks contains fallback models for {{.DisplayName}}.
// These are used when the dynamic API is unavailable.
var {{title .Provider}}StaticFallbacks = []StaticModelFallback{
{{- range .Models}}
	{ID: "{{.ID}}", Name: "{{.Name}}", ContextWindow: {{.ContextWindow}}},
{{- end}}
}
{{end}}

// GetStaticFallbacks returns the static fallback models for a provider.
func GetStaticFallbacks(provider string) []StaticModelFallback {
	switch provider {
{{- range .Fallbacks}}
	case "{{.Provider}}":
		return {{title .Provider}}StaticFallbacks
{{- end}}
	default:
		return nil
	}
}
`

func generateFile(path string, fallbacks []ProviderFallbacks) error {
	funcMap := template.FuncMap{
		"title": strings.Title,
	}

	tmpl, err := template.New("fallbacks").Funcs(funcMap).Parse(fileTemplate)
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	data := struct {
		Timestamp string
		Fallbacks []ProviderFallbacks
	}{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Fallbacks: fallbacks,
	}

	if err := tmpl.Execute(f, data); err != nil {
		return fmt.Errorf("execute template: %w", err)
	}

	return nil
}
