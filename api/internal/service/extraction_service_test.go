package service

import (
	"context"
	"log/slog"
	"testing"

	"github.com/jmylchreest/refyne-api/internal/config"
	"github.com/jmylchreest/refyne-api/internal/repository"
)

// ========================================
// ExtractionService Tests
// ========================================

// Note: The core Extract/Crawl methods require the external refyne package
// and are better tested via integration tests. These unit tests focus on
// the helper functions and configuration resolution.

func TestNewExtractionService(t *testing.T) {
	cfg := &config.Config{}
	repos := &repository.Repositories{}
	logger := slog.Default()

	svc := NewExtractionService(cfg, repos, nil, nil, logger)
	if svc == nil {
		t.Fatal("expected service, got nil")
	}
	if svc.cfg != cfg {
		t.Error("expected cfg to be set")
	}
	if svc.repos != repos {
		t.Error("expected repos to be set")
	}
	if svc.logger == nil {
		t.Error("expected logger to be set")
	}
}

func TestNewExtractionServiceWithBilling(t *testing.T) {
	cfg := &config.Config{}
	repos := &repository.Repositories{}
	logger := slog.Default()

	svc := NewExtractionServiceWithBilling(cfg, repos, nil, nil, nil, logger)
	if svc == nil {
		t.Fatal("expected service, got nil")
	}
	if svc.billing != nil {
		t.Error("expected billing to be nil when not provided")
	}
}

// ========================================
// URL Resolution Tests
// ========================================

func TestIsURLField(t *testing.T) {
	tests := []struct {
		fieldName string
		expected  bool
	}{
		// Standard URL fields
		{"url", true},
		{"link", true},
		{"href", true},
		{"URL", true},  // Case insensitive
		{"Link", true}, // Case insensitive

		// Image URL fields
		{"image_url", true},
		{"image", true},
		{"img_url", true},
		{"img", true},
		{"thumbnail_url", true},
		{"thumbnail", true},

		// Other URL fields
		{"source_url", true},
		{"page_url", true},
		{"canonical_url", true},
		{"video_url", true},
		{"audio_url", true},
		{"media_url", true},
		{"profile_url", true},
		{"avatar_url", true},

		// Fields ending in _url or _link
		{"product_url", true},
		{"download_link", true},
		{"custom_url", true},

		// Non-URL fields
		{"title", false},
		{"name", false},
		{"description", false},
		{"price", false},
		{"id", false},
		{"urlencoded", false}, // doesn't match pattern
	}

	for _, tt := range tests {
		t.Run(tt.fieldName, func(t *testing.T) {
			result := isURLField(tt.fieldName)
			if result != tt.expected {
				t.Errorf("isURLField(%q) = %v, want %v", tt.fieldName, result, tt.expected)
			}
		})
	}
}

func TestResolveURL(t *testing.T) {
	tests := []struct {
		name     string
		rawURL   string
		baseURL  string
		expected string
	}{
		{
			name:     "empty URL",
			rawURL:   "",
			baseURL:  "https://example.com/page",
			expected: "",
		},
		{
			name:     "absolute HTTP URL",
			rawURL:   "http://other.com/image.jpg",
			baseURL:  "https://example.com/page",
			expected: "http://other.com/image.jpg",
		},
		{
			name:     "absolute HTTPS URL",
			rawURL:   "https://cdn.example.com/image.jpg",
			baseURL:  "https://example.com/page",
			expected: "https://cdn.example.com/image.jpg",
		},
		{
			name:     "protocol-relative URL",
			rawURL:   "//cdn.example.com/image.jpg",
			baseURL:  "https://example.com/page",
			expected: "https://cdn.example.com/image.jpg",
		},
		{
			name:     "root-relative URL",
			rawURL:   "/images/photo.jpg",
			baseURL:  "https://example.com/products/item",
			expected: "https://example.com/images/photo.jpg",
		},
		{
			name:     "relative URL",
			rawURL:   "images/photo.jpg",
			baseURL:  "https://example.com/products/",
			expected: "https://example.com/products/images/photo.jpg",
		},
		{
			name:     "relative URL with parent",
			rawURL:   "../images/photo.jpg",
			baseURL:  "https://example.com/products/items/",
			expected: "https://example.com/products/images/photo.jpg",
		},
		{
			name:     "query string preserved",
			rawURL:   "/image.jpg?size=large",
			baseURL:  "https://example.com/page",
			expected: "https://example.com/image.jpg?size=large",
		},
		{
			name:     "fragment preserved",
			rawURL:   "/page#section",
			baseURL:  "https://example.com/",
			expected: "https://example.com/page#section",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ResolveRelativeURLs returns the original data for non-map/slice types
			// We need to test resolveURL directly or wrap in a map
			testData := map[string]any{"url": tt.rawURL}
			resolved := ResolveRelativeURLs(testData, tt.baseURL)
			resolvedMap, ok := resolved.(map[string]any)
			if !ok {
				t.Fatalf("expected map, got %T", resolved)
			}
			if resolvedMap["url"] != tt.expected {
				t.Errorf("resolved URL = %q, want %q", resolvedMap["url"], tt.expected)
			}
		})
	}
}

func TestResolveRelativeURLs_Map(t *testing.T) {
	baseURL := "https://example.com/products/"
	data := map[string]any{
		"title":     "Product Name",
		"price":     29.99,
		"url":       "/products/123",
		"image_url": "images/product.jpg",
		"details": map[string]any{
			"link":        "/details/123",
			"description": "A great product",
		},
	}

	result := ResolveRelativeURLs(data, baseURL)
	resolved, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", result)
	}

	// Non-URL fields should be unchanged
	if resolved["title"] != "Product Name" {
		t.Errorf("title = %q, want %q", resolved["title"], "Product Name")
	}
	if resolved["price"] != 29.99 {
		t.Errorf("price = %v, want %v", resolved["price"], 29.99)
	}

	// URL fields should be resolved
	if resolved["url"] != "https://example.com/products/123" {
		t.Errorf("url = %q, want %q", resolved["url"], "https://example.com/products/123")
	}
	if resolved["image_url"] != "https://example.com/products/images/product.jpg" {
		t.Errorf("image_url = %q, want %q", resolved["image_url"], "https://example.com/products/images/product.jpg")
	}

	// Nested URL fields should also be resolved
	details, ok := resolved["details"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested map, got %T", resolved["details"])
	}
	if details["link"] != "https://example.com/details/123" {
		t.Errorf("details.link = %q, want %q", details["link"], "https://example.com/details/123")
	}
	if details["description"] != "A great product" {
		t.Errorf("details.description = %q, want %q", details["description"], "A great product")
	}
}

func TestResolveRelativeURLs_Slice(t *testing.T) {
	baseURL := "https://example.com/"
	data := []any{
		map[string]any{
			"url":   "/page1",
			"title": "Page 1",
		},
		map[string]any{
			"url":   "/page2",
			"title": "Page 2",
		},
	}

	result := ResolveRelativeURLs(data, baseURL)
	resolved, ok := result.([]any)
	if !ok {
		t.Fatalf("expected slice, got %T", result)
	}

	if len(resolved) != 2 {
		t.Fatalf("expected 2 items, got %d", len(resolved))
	}

	item0, ok := resolved[0].(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", resolved[0])
	}
	if item0["url"] != "https://example.com/page1" {
		t.Errorf("item[0].url = %q, want %q", item0["url"], "https://example.com/page1")
	}

	item1, ok := resolved[1].(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", resolved[1])
	}
	if item1["url"] != "https://example.com/page2" {
		t.Errorf("item[1].url = %q, want %q", item1["url"], "https://example.com/page2")
	}
}

func TestResolveRelativeURLs_NilAndEmpty(t *testing.T) {
	// Nil data should return nil
	result := ResolveRelativeURLs(nil, "https://example.com/")
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}

	// Empty base URL should return data unchanged
	data := map[string]any{"url": "/page"}
	result = ResolveRelativeURLs(data, "")
	resolved, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", result)
	}
	if resolved["url"] != "/page" {
		t.Errorf("url = %q, want %q (unchanged)", resolved["url"], "/page")
	}
}

// ========================================
// BuildCrawlOptions Tests
// ========================================

func TestBuildCrawlOptions(t *testing.T) {
	svc := &ExtractionService{
		logger: slog.Default(),
	}

	// Test with empty options
	opts := svc.buildCrawlOptions(CrawlOptions{})
	if len(opts) != 0 {
		t.Errorf("expected 0 options for empty CrawlOptions, got %d", len(opts))
	}

	// Test with some options set
	opts = svc.buildCrawlOptions(CrawlOptions{
		FollowSelector: "a.next",
		MaxDepth:       3,
		MaxPages:       10,
		Delay:          "1s",
		Concurrency:    2,
		SameDomainOnly: true,
	})

	// We expect options to be added (can't easily inspect refyne.CrawlOption values)
	if len(opts) == 0 {
		t.Error("expected options to be added")
	}
}

func TestBuildCrawlOptions_InvalidDelay(t *testing.T) {
	svc := &ExtractionService{
		logger: slog.Default(),
	}

	// Invalid delay should be ignored (not cause panic)
	opts := svc.buildCrawlOptions(CrawlOptions{
		Delay: "invalid",
	})

	// Should still work, just with no delay option added
	if opts == nil {
		t.Error("expected non-nil options slice")
	}
}

// ========================================
// ExtractContext Tests
// ========================================

func TestExtractContext_Defaults(t *testing.T) {
	ctx := &ExtractContext{
		UserID: "user-123",
	}

	if ctx.UserID != "user-123" {
		t.Errorf("UserID = %q, want %q", ctx.UserID, "user-123")
	}
	if ctx.Tier != "" {
		t.Errorf("Tier = %q, want empty string", ctx.Tier)
	}
	if ctx.IsBYOK {
		t.Error("expected IsBYOK to be false by default")
	}
	if ctx.BYOKAllowed {
		t.Error("expected BYOKAllowed to be false by default")
	}
	if ctx.ModelsCustomAllowed {
		t.Error("expected ModelsCustomAllowed to be false by default")
	}
}

// ========================================
// LLMConfigInput Tests
// ========================================

func TestLLMConfigInput_Fields(t *testing.T) {
	cfg := &LLMConfigInput{
		Provider:   "openrouter",
		APIKey:     "test-key",
		BaseURL:    "https://api.example.com",
		Model:      "claude-3-haiku",
		StrictMode: true,
	}

	if cfg.Provider != "openrouter" {
		t.Errorf("Provider = %q, want %q", cfg.Provider, "openrouter")
	}
	if cfg.APIKey != "test-key" {
		t.Errorf("APIKey = %q, want %q", cfg.APIKey, "test-key")
	}
	if cfg.BaseURL != "https://api.example.com" {
		t.Errorf("BaseURL = %q, want %q", cfg.BaseURL, "https://api.example.com")
	}
	if cfg.Model != "claude-3-haiku" {
		t.Errorf("Model = %q, want %q", cfg.Model, "claude-3-haiku")
	}
	if !cfg.StrictMode {
		t.Error("expected StrictMode to be true")
	}
}

// ========================================
// Default Config Resolution Tests
// ========================================

func TestGetDefaultLLMConfig_NoResolver(t *testing.T) {
	svc := &ExtractionService{
		logger: slog.Default(),
	}
	ctx := context.Background()

	// Without a resolver, should return nil (no hardcoded fallback)
	cfg := svc.getDefaultLLMConfig(ctx)
	if cfg != nil {
		t.Errorf("expected nil config without resolver, got %+v", cfg)
	}
}

func TestGetStrictMode_NoResolver(t *testing.T) {
	svc := &ExtractionService{
		logger: slog.Default(),
	}
	ctx := context.Background()

	// Without resolver, should fall back to static defaults
	strictMode := svc.getStrictMode(ctx, "openai", "gpt-4", nil)
	// The exact value depends on llm.GetModelSettings, but it shouldn't panic
	_ = strictMode
}

func TestGetStrictMode_WithOverride(t *testing.T) {
	svc := &ExtractionService{
		logger: slog.Default(),
	}
	ctx := context.Background()

	// With explicit override, should use that value
	override := true
	strictMode := svc.getStrictMode(ctx, "openai", "gpt-4", &override)
	if !strictMode {
		t.Error("expected strictMode to be true when override is true")
	}

	override = false
	strictMode = svc.getStrictMode(ctx, "openai", "gpt-4", &override)
	if strictMode {
		t.Error("expected strictMode to be false when override is false")
	}
}

// ========================================
// UsageInfo and ExtractMeta Tests
// ========================================

func TestUsageInfo_Fields(t *testing.T) {
	usage := UsageInfo{
		InputTokens:  1000,
		OutputTokens: 500,
		CostUSD:      0.015,
		LLMCostUSD:   0.012,
		IsBYOK:       true,
	}

	if usage.InputTokens != 1000 {
		t.Errorf("InputTokens = %d, want %d", usage.InputTokens, 1000)
	}
	if usage.OutputTokens != 500 {
		t.Errorf("OutputTokens = %d, want %d", usage.OutputTokens, 500)
	}
	if usage.CostUSD != 0.015 {
		t.Errorf("CostUSD = %f, want %f", usage.CostUSD, 0.015)
	}
	if usage.LLMCostUSD != 0.012 {
		t.Errorf("LLMCostUSD = %f, want %f", usage.LLMCostUSD, 0.012)
	}
	if !usage.IsBYOK {
		t.Error("expected IsBYOK to be true")
	}
}

func TestExtractMeta_Fields(t *testing.T) {
	meta := ExtractMeta{
		FetchDurationMs:   150,
		ExtractDurationMs: 2500,
		Model:             "claude-3-haiku",
		Provider:          "anthropic",
	}

	if meta.FetchDurationMs != 150 {
		t.Errorf("FetchDurationMs = %d, want %d", meta.FetchDurationMs, 150)
	}
	if meta.ExtractDurationMs != 2500 {
		t.Errorf("ExtractDurationMs = %d, want %d", meta.ExtractDurationMs, 2500)
	}
	if meta.Model != "claude-3-haiku" {
		t.Errorf("Model = %q, want %q", meta.Model, "claude-3-haiku")
	}
	if meta.Provider != "anthropic" {
		t.Errorf("Provider = %q, want %q", meta.Provider, "anthropic")
	}
}

// ========================================
// CrawlResult Tests
// ========================================

func TestCrawlResult_Fields(t *testing.T) {
	result := &CrawlResult{
		Results:           []any{"data1", "data2"},
		PageResults:       []PageResult{{URL: "http://example.com"}},
		PageCount:         2,
		TotalTokensInput:  2000,
		TotalTokensOutput: 1000,
		TotalCostUSD:      0.05,
		StoppedEarly:      true,
		StopReason:        "insufficient_balance",
	}

	if len(result.Results) != 2 {
		t.Errorf("Results length = %d, want 2", len(result.Results))
	}
	if result.PageCount != 2 {
		t.Errorf("PageCount = %d, want 2", result.PageCount)
	}
	if result.TotalTokensInput != 2000 {
		t.Errorf("TotalTokensInput = %d, want 2000", result.TotalTokensInput)
	}
	if result.TotalTokensOutput != 1000 {
		t.Errorf("TotalTokensOutput = %d, want 1000", result.TotalTokensOutput)
	}
	if result.TotalCostUSD != 0.05 {
		t.Errorf("TotalCostUSD = %f, want 0.05", result.TotalCostUSD)
	}
	if !result.StoppedEarly {
		t.Error("expected StoppedEarly to be true")
	}
	if result.StopReason != "insufficient_balance" {
		t.Errorf("StopReason = %q, want %q", result.StopReason, "insufficient_balance")
	}
}

func TestPageResult_Fields(t *testing.T) {
	parentURL := "http://example.com/"
	result := PageResult{
		URL:               "http://example.com/page1",
		ParentURL:         &parentURL,
		Depth:             1,
		Data:              map[string]any{"title": "Page 1"},
		Error:             "",
		ErrorDetails:      "",
		ErrorCategory:     "",
		LLMProvider:       "openrouter",
		LLMModel:          "claude-3-haiku",
		GenerationID:      "gen-123",
		IsBYOK:            false,
		RetryCount:        0,
		TokenUsageInput:   500,
		TokenUsageOutput:  200,
		FetchDurationMs:   100,
		ExtractDurationMs: 1500,
	}

	if result.URL != "http://example.com/page1" {
		t.Errorf("URL = %q, want %q", result.URL, "http://example.com/page1")
	}
	if result.ParentURL == nil || *result.ParentURL != "http://example.com/" {
		t.Error("ParentURL not set correctly")
	}
	if result.Depth != 1 {
		t.Errorf("Depth = %d, want 1", result.Depth)
	}
	if result.TokenUsageInput != 500 {
		t.Errorf("TokenUsageInput = %d, want 500", result.TokenUsageInput)
	}
}

func TestPageResult_WithError(t *testing.T) {
	result := PageResult{
		URL:           "http://example.com/error-page",
		Error:         "Rate limit exceeded",
		ErrorDetails:  "429 Too Many Requests: Rate limit exceeded for model claude-3-haiku",
		ErrorCategory: "rate_limit",
		LLMProvider:   "anthropic",
		LLMModel:      "claude-3-haiku",
		IsBYOK:        true,
	}

	if result.Error != "Rate limit exceeded" {
		t.Errorf("Error = %q, want %q", result.Error, "Rate limit exceeded")
	}
	if result.ErrorCategory != "rate_limit" {
		t.Errorf("ErrorCategory = %q, want %q", result.ErrorCategory, "rate_limit")
	}
	if !result.IsBYOK {
		t.Error("expected IsBYOK to be true")
	}
}
