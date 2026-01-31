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
	// ModelOverrides is now empty - model settings come from S3 config
	// This test verifies the map exists and is initialized
	if ModelOverrides == nil {
		t.Fatal("ModelOverrides should be initialized (empty map)")
	}
	// ModelOverrides being empty is the expected behavior
	// Model-specific settings are loaded from S3 configuration
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
	// With no S3 config, model-specific settings fall back to provider defaults
	temp, maxTokens, _ := GetModelSettings("openrouter", "openai/gpt-4o", nil, nil, nil)

	// Without S3 config, should use openrouter provider defaults
	providerDefaults := ProviderDefaults["openrouter"]
	if temp != providerDefaults.Temperature {
		t.Errorf("Temperature = %f, want %f (from provider default)", temp, providerDefaults.Temperature)
	}
	if maxTokens != providerDefaults.MaxTokens {
		t.Errorf("MaxTokens = %d, want %d (from provider default)", maxTokens, providerDefaults.MaxTokens)
	}
	// Model-specific settings like StrictMode now come from S3 config
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
	// Without S3 config, model settings fall back to provider defaults
	settings := GetDefaultSettings("openrouter", "openai/gpt-4o")

	// Should use openrouter provider defaults since no S3 config is loaded
	expected := ProviderDefaults["openrouter"]
	if settings.Temperature != expected.Temperature {
		t.Errorf("Temperature = %f, want %f (from provider default)", settings.Temperature, expected.Temperature)
	}
	if settings.MaxTokens != expected.MaxTokens {
		t.Errorf("MaxTokens = %d, want %d (from provider default)", settings.MaxTokens, expected.MaxTokens)
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
