package llm

import (
	"testing"
)

// ========================================
// ModelSettings Tests
// ========================================

func TestModelSettings_Fields(t *testing.T) {
	settings := ModelSettings{
		Temperature: 0.5,
		MaxTokens:   4096,
		StrictMode:  true,
	}

	if settings.Temperature != 0.5 {
		t.Errorf("Temperature = %f, want 0.5", settings.Temperature)
	}
	if settings.MaxTokens != 4096 {
		t.Errorf("MaxTokens = %d, want 4096", settings.MaxTokens)
	}
	if !settings.StrictMode {
		t.Error("StrictMode should be true")
	}
}

// ========================================
// ProviderDefaults Tests
// ========================================

func TestProviderDefaults(t *testing.T) {
	// Verify expected providers exist
	expectedProviders := []string{"anthropic", "openai", "openrouter", "ollama", "helicone"}
	for _, p := range expectedProviders {
		if _, ok := ProviderDefaults[p]; !ok {
			t.Errorf("expected ProviderDefaults to contain %q", p)
		}
	}

	// Verify some specific values
	if ProviderDefaults["openai"].StrictMode != true {
		t.Error("expected OpenAI to have StrictMode=true")
	}
	if ProviderDefaults["anthropic"].StrictMode != false {
		t.Error("expected Anthropic to have StrictMode=false")
	}
	if ProviderDefaults["helicone"].StrictMode != true {
		t.Error("expected Helicone to have StrictMode=true")
	}
}

// ========================================
// ModelOverrides Tests
// ========================================

func TestModelOverrides(t *testing.T) {
	// Verify OpenAI models support strict mode
	gpt4o, ok := ModelOverrides["openai/gpt-4o"]
	if !ok {
		t.Fatal("expected openai/gpt-4o in ModelOverrides")
	}
	if !gpt4o.StrictMode {
		t.Error("expected gpt-4o to have StrictMode=true")
	}

	// Verify ollama models don't support strict mode
	llama, ok := ModelOverrides["llama3.2"]
	if !ok {
		t.Fatal("expected llama3.2 in ModelOverrides")
	}
	if llama.StrictMode {
		t.Error("expected llama3.2 to have StrictMode=false")
	}
}

// ========================================
// GetModelSettings Tests
// ========================================

func TestGetModelSettings_ProviderDefault(t *testing.T) {
	temp, maxTokens, strictMode := GetModelSettings("openai", "unknown-model", nil, nil, nil)

	// Should use openai provider defaults
	if temp != ProviderDefaults["openai"].Temperature {
		t.Errorf("Temperature = %f, want %f", temp, ProviderDefaults["openai"].Temperature)
	}
	if maxTokens != ProviderDefaults["openai"].MaxTokens {
		t.Errorf("MaxTokens = %d, want %d", maxTokens, ProviderDefaults["openai"].MaxTokens)
	}
	if strictMode != ProviderDefaults["openai"].StrictMode {
		t.Errorf("StrictMode = %v, want %v", strictMode, ProviderDefaults["openai"].StrictMode)
	}
}

func TestGetModelSettings_ModelOverride(t *testing.T) {
	temp, maxTokens, strictMode := GetModelSettings("openrouter", "openai/gpt-4o", nil, nil, nil)

	// Should use model-specific override
	override := ModelOverrides["openai/gpt-4o"]
	if temp != override.Temperature {
		t.Errorf("Temperature = %f, want %f (from model override)", temp, override.Temperature)
	}
	if maxTokens != override.MaxTokens {
		t.Errorf("MaxTokens = %d, want %d (from model override)", maxTokens, override.MaxTokens)
	}
	if strictMode != override.StrictMode {
		t.Errorf("StrictMode = %v, want %v (from model override)", strictMode, override.StrictMode)
	}
}

func TestGetModelSettings_ChainOverride(t *testing.T) {
	chainTemp := 0.9
	chainMaxTokens := 1000
	chainStrictMode := true

	temp, maxTokens, strictMode := GetModelSettings("anthropic", "model", &chainTemp, &chainMaxTokens, &chainStrictMode)

	// Chain overrides should take priority
	if temp != chainTemp {
		t.Errorf("Temperature = %f, want %f (from chain)", temp, chainTemp)
	}
	if maxTokens != chainMaxTokens {
		t.Errorf("MaxTokens = %d, want %d (from chain)", maxTokens, chainMaxTokens)
	}
	if strictMode != chainStrictMode {
		t.Errorf("StrictMode = %v, want %v (from chain)", strictMode, chainStrictMode)
	}
}

func TestGetModelSettings_UnknownProvider(t *testing.T) {
	temp, maxTokens, strictMode := GetModelSettings("unknown-provider", "unknown-model", nil, nil, nil)

	// Should return fallback defaults
	if temp != 0.2 {
		t.Errorf("Temperature = %f, want 0.2 (fallback)", temp)
	}
	if maxTokens != 4096 {
		t.Errorf("MaxTokens = %d, want 4096 (fallback)", maxTokens)
	}
	if strictMode {
		t.Error("StrictMode should be false by default")
	}
}

// ========================================
// GetDefaultSettings Tests
// ========================================

func TestGetDefaultSettings_KnownModel(t *testing.T) {
	settings := GetDefaultSettings("openrouter", "openai/gpt-4o")

	expected := ModelOverrides["openai/gpt-4o"]
	if settings.Temperature != expected.Temperature {
		t.Errorf("Temperature = %f, want %f", settings.Temperature, expected.Temperature)
	}
	if settings.MaxTokens != expected.MaxTokens {
		t.Errorf("MaxTokens = %d, want %d", settings.MaxTokens, expected.MaxTokens)
	}
	if settings.StrictMode != expected.StrictMode {
		t.Errorf("StrictMode = %v, want %v", settings.StrictMode, expected.StrictMode)
	}
}

func TestGetDefaultSettings_UnknownModel(t *testing.T) {
	settings := GetDefaultSettings("openai", "unknown-model")

	// Should use provider defaults
	expected := ProviderDefaults["openai"]
	if settings.Temperature != expected.Temperature {
		t.Errorf("Temperature = %f, want %f", settings.Temperature, expected.Temperature)
	}
}

func TestGetDefaultSettings_UnknownProvider(t *testing.T) {
	settings := GetDefaultSettings("unknown-provider", "unknown-model")

	// Should return fallback
	if settings.Temperature != 0.2 {
		t.Errorf("Temperature = %f, want 0.2", settings.Temperature)
	}
	if settings.MaxTokens != 4096 {
		t.Errorf("MaxTokens = %d, want 4096", settings.MaxTokens)
	}
}

// ========================================
// AnalyzeSystemPrompt Tests
// ========================================

func TestAnalyzeSystemPrompt(t *testing.T) {
	if AnalyzeSystemPrompt == "" {
		t.Error("AnalyzeSystemPrompt should not be empty")
	}
	// Verify it contains key elements
	if len(AnalyzeSystemPrompt) < 50 {
		t.Error("AnalyzeSystemPrompt seems too short")
	}
}
