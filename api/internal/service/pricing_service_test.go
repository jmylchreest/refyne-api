package service

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jmylchreest/refyne-api/internal/llm"
)

// ========================================
// EstimateCost Tests
// ========================================

func TestPricingService_EstimateCost_WithCachedPricing(t *testing.T) {
	// Create service with no API key (won't try to fetch)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewPricingService(PricingServiceConfig{
		RefreshInterval: 24 * time.Hour, // Long interval to prevent refresh
	}, logger)

	// Manually populate cache
	svc.openRouterMu.Lock()
	svc.openRouterPrices["anthropic/claude-3-haiku"] = &ModelPricing{
		ID:              "anthropic/claude-3-haiku",
		Name:            "Claude 3 Haiku",
		PromptPrice:     0.00000025,  // $0.25/1M tokens
		CompletionPrice: 0.00000125,  // $1.25/1M tokens
		ContextLength:   200000,
	}
	svc.lastRefresh = time.Now()
	svc.openRouterMu.Unlock()

	t.Run("uses cached pricing", func(t *testing.T) {
		cost := svc.EstimateCost("openrouter", "anthropic/claude-3-haiku", 1000, 500)

		// Expected: 1000 * 0.00000025 + 500 * 0.00000125 = 0.000875
		expected := 0.000875
		if cost < expected*0.99 || cost > expected*1.01 {
			t.Errorf("cost = %f, want %f (within 1%%)", cost, expected)
		}
	})

	t.Run("falls back for unknown provider", func(t *testing.T) {
		cost := svc.EstimateCost("anthropic", "claude-3-opus-20240229", 1000, 500)

		// Should use fallback pricing for claude-3-opus
		// Fallback: 15.0/1M input, 75.0/1M output
		// Expected: 1000 * 15/1M + 500 * 75/1M = 0.0525
		if cost < 0.05 || cost > 0.06 {
			t.Errorf("fallback cost = %f, expected ~0.0525", cost)
		}
	})
}

func TestPricingService_EstimateCostFallback(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewPricingService(PricingServiceConfig{}, logger)

	tests := []struct {
		name         string
		model        string
		inputTokens  int
		outputTokens int
		minCost      float64
		maxCost      float64
	}{
		{
			name:         "gpt-4 (expensive)",
			model:        "gpt-4-turbo",
			inputTokens:  1000,
			outputTokens: 500,
			minCost:      0.04,  // 1000*15/1M + 500*60/1M = 0.045
			maxCost:      0.05,
		},
		{
			name:         "claude-3-opus (expensive)",
			model:        "claude-3-opus-20240229",
			inputTokens:  1000,
			outputTokens: 500,
			minCost:      0.05,  // 1000*15/1M + 500*75/1M = 0.0525
			maxCost:      0.06,
		},
		{
			name:         "gpt-4o-mini (cheap)",
			model:        "gpt-4o-mini",
			inputTokens:  1000,
			outputTokens: 500,
			minCost:      0.0,
			maxCost:      0.001, // 1000*0.15/1M + 500*0.60/1M = 0.00045
		},
		{
			name:         "claude-3-haiku (cheap)",
			model:        "claude-3-haiku-20240307",
			inputTokens:  1000,
			outputTokens: 500,
			minCost:      0.0,
			maxCost:      0.001, // 1000*0.15/1M + 500*0.60/1M = 0.00045
		},
		{
			name:         "llama model (very cheap)",
			model:        "meta-llama/llama-3-8b-instruct",
			inputTokens:  1000,
			outputTokens: 500,
			minCost:      0.0,
			maxCost:      0.00035, // 1000*0.10/1M + 500*0.40/1M = 0.0003
		},
		{
			name:         "free model",
			model:        "some-model:free",
			inputTokens:  10000,
			outputTokens: 5000,
			minCost:      0.0,
			maxCost:      0.0,
		},
		{
			name:         "default pricing for unknown model",
			model:        "unknown-model-xyz",
			inputTokens:  1000,
			outputTokens: 500,
			minCost:      0.0,
			maxCost:      0.001, // 1000*0.25/1M + 500*1.0/1M = 0.00075
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cost := svc.estimateCostFallback(tt.model, tt.inputTokens, tt.outputTokens)
			if cost < tt.minCost || cost > tt.maxCost {
				t.Errorf("cost = %f, want between %f and %f", cost, tt.minCost, tt.maxCost)
			}
		})
	}
}

// ========================================
// GetCachedModelCount and LastRefresh Tests
// ========================================

func TestPricingService_CacheInfo(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewPricingService(PricingServiceConfig{}, logger)

	t.Run("initial state", func(t *testing.T) {
		count := svc.GetCachedModelCount()
		if count != 0 {
			t.Errorf("initial count = %d, want 0", count)
		}

		lastRefresh := svc.LastRefresh()
		if !lastRefresh.IsZero() {
			t.Errorf("initial LastRefresh should be zero time")
		}
	})

	t.Run("after populating cache", func(t *testing.T) {
		svc.openRouterMu.Lock()
		svc.openRouterPrices["model-1"] = &ModelPricing{ID: "model-1"}
		svc.openRouterPrices["model-2"] = &ModelPricing{ID: "model-2"}
		svc.lastRefresh = time.Now()
		svc.openRouterMu.Unlock()

		count := svc.GetCachedModelCount()
		if count != 2 {
			t.Errorf("count = %d, want 2", count)
		}

		lastRefresh := svc.LastRefresh()
		if lastRefresh.IsZero() {
			t.Error("LastRefresh should not be zero after setting")
		}
	})
}

// ========================================
// Cache Structure Tests
// ========================================

func TestPricingService_CacheStructure(t *testing.T) {
	// Tests the cache structure and GetModelPricing retrieval
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewPricingService(PricingServiceConfig{
		RefreshInterval: time.Hour,
	}, logger)

	// Manually populate cache (simulating what RefreshOpenRouterPricing does)
	svc.openRouterMu.Lock()
	svc.openRouterPrices["anthropic/claude-3-haiku"] = &ModelPricing{
		ID:              "anthropic/claude-3-haiku",
		Name:            "Claude 3 Haiku",
		PromptPrice:     0.00000025,
		CompletionPrice: 0.00000125,
		ContextLength:   200000,
		IsFree:          false,
		Capabilities: llm.ModelCapabilities{
			SupportsStreaming:         true,
			SupportsStructuredOutputs: true,
			SupportsTools:             true,
		},
	}
	svc.lastRefresh = time.Now()
	svc.openRouterMu.Unlock()

	pricing := svc.GetModelPricing("openrouter", "anthropic/claude-3-haiku")
	if pricing == nil {
		t.Fatal("expected pricing to be cached")
	}
	if pricing.Name != "Claude 3 Haiku" {
		t.Errorf("Name = %q, want %q", pricing.Name, "Claude 3 Haiku")
	}
	if pricing.ContextLength != 200000 {
		t.Errorf("ContextLength = %d, want 200000", pricing.ContextLength)
	}
	if !pricing.Capabilities.SupportsStructuredOutputs {
		t.Error("expected SupportsStructuredOutputs to be true")
	}
}

// ========================================
// GetActualCost Tests
// ========================================

func TestPricingService_GetActualCost_Validation(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewPricingService(PricingServiceConfig{}, logger)

	t.Run("empty generation ID", func(t *testing.T) {
		_, err := svc.GetActualCost(context.Background(), "openrouter", "", "api-key")
		if err == nil {
			t.Error("expected error for empty generation ID")
		}
	})

	t.Run("unsupported provider", func(t *testing.T) {
		_, err := svc.GetActualCost(context.Background(), "anthropic", "gen-123", "api-key")
		if err == nil {
			t.Error("expected error for unsupported provider")
		}
	})

	t.Run("missing API key", func(t *testing.T) {
		_, err := svc.GetActualCost(context.Background(), "openrouter", "gen-123", "")
		if err == nil {
			t.Error("expected error for missing API key")
		}
	})
}

// ========================================
// ModelPricing Struct Tests
// ========================================

func TestModelPricing_Fields(t *testing.T) {
	pricing := &ModelPricing{
		ID:              "anthropic/claude-3-opus",
		Name:            "Claude 3 Opus",
		PromptPrice:     0.000015,
		CompletionPrice: 0.000075,
		ContextLength:   200000,
		IsFree:          false,
		Capabilities: llm.ModelCapabilities{
			SupportsStreaming:         true,
			SupportsStructuredOutputs: true,
		},
	}

	if pricing.ID != "anthropic/claude-3-opus" {
		t.Errorf("ID = %q, want %q", pricing.ID, "anthropic/claude-3-opus")
	}
	if pricing.IsFree {
		t.Error("expected IsFree to be false")
	}
	if !pricing.Capabilities.SupportsStreaming {
		t.Error("expected SupportsStreaming to be true")
	}
}
