package llm

import "testing"

func TestValidProviders(t *testing.T) {
	providers := ValidProviders()
	if len(providers) != 5 {
		t.Errorf("expected 5 providers, got %d", len(providers))
	}

	expected := map[string]bool{
		ProviderOpenRouter: true,
		ProviderAnthropic:  true,
		ProviderOpenAI:     true,
		ProviderOllama:     true,
		ProviderHelicone:   true,
	}

	for _, p := range providers {
		if !expected[p] {
			t.Errorf("unexpected provider: %s", p)
		}
	}
}

func TestIsValidProvider(t *testing.T) {
	tests := []struct {
		provider string
		want     bool
	}{
		{ProviderOpenRouter, true},
		{ProviderAnthropic, true},
		{ProviderOpenAI, true},
		{ProviderOllama, true},
		{ProviderHelicone, true},
		{"openrouter", true},
		{"anthropic", true},
		{"openai", true},
		{"ollama", true},
		{"helicone", true},
		{"invalid", false},
		{"", false},
		{"OpenRouter", false}, // case sensitive
		{"OPENAI", false},     // case sensitive
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			got := IsValidProvider(tt.provider)
			if got != tt.want {
				t.Errorf("IsValidProvider(%q) = %v, want %v", tt.provider, got, tt.want)
			}
		})
	}
}

func TestProviderConstants(t *testing.T) {
	// Ensure constants have expected values
	if ProviderOpenRouter != "openrouter" {
		t.Errorf("ProviderOpenRouter = %q, want %q", ProviderOpenRouter, "openrouter")
	}
	if ProviderAnthropic != "anthropic" {
		t.Errorf("ProviderAnthropic = %q, want %q", ProviderAnthropic, "anthropic")
	}
	if ProviderOpenAI != "openai" {
		t.Errorf("ProviderOpenAI = %q, want %q", ProviderOpenAI, "openai")
	}
	if ProviderOllama != "ollama" {
		t.Errorf("ProviderOllama = %q, want %q", ProviderOllama, "ollama")
	}
	if ProviderHelicone != "helicone" {
		t.Errorf("ProviderHelicone = %q, want %q", ProviderHelicone, "helicone")
	}
}
