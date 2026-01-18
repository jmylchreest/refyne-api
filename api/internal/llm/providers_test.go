package llm

import (
	"context"
	"testing"
)

// ========================================
// Provider Capabilities Tests
// ========================================

func TestGetAnthropicCapabilities(t *testing.T) {
	ctx := context.Background()

	caps := getAnthropicCapabilities(ctx, "claude-3-sonnet")

	// Anthropic uses tool_use for structured outputs
	if caps.SupportsStructuredOutputs {
		t.Error("Anthropic should not support structured outputs (uses tool_use)")
	}
	if !caps.SupportsTools {
		t.Error("Anthropic should support tools")
	}
	if !caps.SupportsStreaming {
		t.Error("Anthropic should support streaming")
	}
	if caps.SupportsReasoning {
		t.Error("Anthropic should not support reasoning")
	}
	if caps.SupportsResponseFormat {
		t.Error("Anthropic should not support response_format")
	}
}

func TestGetOpenAICapabilities(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		model                     string
		expectStructuredOutputs   bool
		expectReasoning           bool
	}{
		{"gpt-4o", true, false},
		{"gpt-4o-mini", true, false},
		{"gpt-4", false, false},
		{"gpt-3.5-turbo", false, false},
		{"o1", true, true},
		{"o1-mini", true, true},
		{"o3-mini", true, true},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			caps := getOpenAICapabilities(ctx, tt.model)

			if caps.SupportsStructuredOutputs != tt.expectStructuredOutputs {
				t.Errorf("SupportsStructuredOutputs = %v, want %v", caps.SupportsStructuredOutputs, tt.expectStructuredOutputs)
			}
			if caps.SupportsReasoning != tt.expectReasoning {
				t.Errorf("SupportsReasoning = %v, want %v", caps.SupportsReasoning, tt.expectReasoning)
			}
			// All OpenAI models should support tools and streaming
			if !caps.SupportsTools {
				t.Error("OpenAI should support tools")
			}
			if !caps.SupportsStreaming {
				t.Error("OpenAI should support streaming")
			}
			if !caps.SupportsResponseFormat {
				t.Error("OpenAI should support response_format")
			}
		})
	}
}

func TestGetOllamaCapabilities(t *testing.T) {
	ctx := context.Background()

	models := []string{"llama3.2", "mistral", "gemma2", "qwen2.5"}

	for _, model := range models {
		t.Run(model, func(t *testing.T) {
			caps := getOllamaCapabilities(ctx, model)

			if caps.SupportsStructuredOutputs {
				t.Error("Ollama should not support structured outputs")
			}
			if caps.SupportsTools {
				t.Error("Ollama should not support tools")
			}
			if !caps.SupportsStreaming {
				t.Error("Ollama should support streaming")
			}
			if caps.SupportsReasoning {
				t.Error("Ollama should not support reasoning")
			}
			if caps.SupportsResponseFormat {
				t.Error("Ollama should not support response_format")
			}
		})
	}
}

func TestGetStaticOpenRouterCapabilities(t *testing.T) {
	tests := []struct {
		model                   string
		expectStructuredOutputs bool
	}{
		{"anthropic/claude-sonnet-4", true},
		{"openai/gpt-4o", true},
		{"openai/gpt-4o-mini", true},
		{"google/gemini-2.0-flash-001", true},
		{"unknown/model", false},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			caps := getStaticOpenRouterCapabilities(tt.model)

			if caps.SupportsStructuredOutputs != tt.expectStructuredOutputs {
				t.Errorf("SupportsStructuredOutputs = %v, want %v", caps.SupportsStructuredOutputs, tt.expectStructuredOutputs)
			}
			// OpenRouter always supports streaming
			if !caps.SupportsStreaming {
				t.Error("OpenRouter should always support streaming")
			}
		})
	}
}

// ========================================
// Model Listing Tests
// ========================================

func TestListAnthropicModels(t *testing.T) {
	ctx := context.Background()

	models, err := listAnthropicModels(ctx, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(models) == 0 {
		t.Fatal("expected at least one model")
	}

	// Verify all models have required fields
	for _, m := range models {
		if m.ID == "" {
			t.Error("model ID should not be empty")
		}
		if m.Name == "" {
			t.Error("model Name should not be empty")
		}
		if m.Provider != "anthropic" {
			t.Errorf("Provider = %q, want %q", m.Provider, "anthropic")
		}
		if m.ContextWindow == 0 {
			t.Error("ContextWindow should not be 0")
		}
	}

	// Verify Claude models are present
	foundClaude := false
	for _, m := range models {
		if contains(m.ID, "claude") {
			foundClaude = true
			break
		}
	}
	if !foundClaude {
		t.Error("expected at least one Claude model")
	}
}

func TestListOpenAIModels(t *testing.T) {
	ctx := context.Background()

	models, err := listOpenAIModels(ctx, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(models) == 0 {
		t.Fatal("expected at least one model")
	}

	// Verify all models have required fields
	for _, m := range models {
		if m.ID == "" {
			t.Error("model ID should not be empty")
		}
		if m.Name == "" {
			t.Error("model Name should not be empty")
		}
		if m.Provider != "openai" {
			t.Errorf("Provider = %q, want %q", m.Provider, "openai")
		}
	}

	// Verify GPT models are present
	foundGPT := false
	for _, m := range models {
		if contains(m.ID, "gpt") {
			foundGPT = true
			break
		}
	}
	if !foundGPT {
		t.Error("expected at least one GPT model")
	}
}

func TestGetStaticOllamaModels(t *testing.T) {
	models := getStaticOllamaModels()

	if len(models) == 0 {
		t.Fatal("expected at least one static Ollama model")
	}

	// Verify all models have required fields
	for _, m := range models {
		if m.ID == "" {
			t.Error("model ID should not be empty")
		}
		if m.Provider != "ollama" {
			t.Errorf("Provider = %q, want %q", m.Provider, "ollama")
		}
	}

	// Verify common models are present
	expectedModels := []string{"llama3.2", "mistral", "gemma2"}
	for _, expected := range expectedModels {
		found := false
		for _, m := range models {
			if m.ID == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected model %q to be present", expected)
		}
	}
}

// ========================================
// Helper Functions
// ========================================

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
