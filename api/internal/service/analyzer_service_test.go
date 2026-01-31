package service

import (
	"errors"
	"log/slog"
	"testing"

	"github.com/jmylchreest/refyne-api/internal/config"
	"github.com/jmylchreest/refyne-api/internal/models"
	"github.com/jmylchreest/refyne-api/internal/repository"
)

// ========================================
// AnalyzerService Tests
// ========================================

// ----------------------------------------
// Constructor Tests
// ----------------------------------------

func TestNewAnalyzerService(t *testing.T) {
	cfg := &config.Config{
		EncryptionKey: []byte("12345678901234567890123456789012"), // 32 bytes
	}
	repos := &repository.Repositories{}
	logger := slog.Default()

	svc := NewAnalyzerService(cfg, repos, logger)
	if svc == nil {
		t.Fatal("expected service, got nil")
	}
	if svc.cfg != cfg {
		t.Error("cfg not set correctly")
	}
	if svc.repos != repos {
		t.Error("repos not set correctly")
	}
	if svc.logger != logger {
		t.Error("logger not set correctly")
	}
}

func TestNewAnalyzerServiceWithBilling(t *testing.T) {
	cfg := &config.Config{
		EncryptionKey: []byte("12345678901234567890123456789012"),
	}
	repos := &repository.Repositories{}
	logger := slog.Default()

	svc := NewAnalyzerServiceWithBilling(cfg, repos, nil, nil, logger)
	if svc == nil {
		t.Fatal("expected service, got nil")
	}
	if svc.billing != nil {
		t.Error("expected billing to be nil when passed nil")
	}
	if svc.resolver != nil {
		t.Error("expected resolver to be nil when passed nil")
	}
}

// ----------------------------------------
// normalizeURL Tests
// ----------------------------------------

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "whitespace only",
			input:    "   ",
			expected: "",
		},
		{
			name:     "already has https",
			input:    "https://example.com",
			expected: "https://example.com",
		},
		{
			name:     "already has http",
			input:    "http://example.com",
			expected: "http://example.com",
		},
		{
			name:     "no scheme",
			input:    "example.com",
			expected: "https://example.com",
		},
		{
			name:     "with path no scheme",
			input:    "example.com/path/page",
			expected: "https://example.com/path/page",
		},
		{
			name:     "with leading whitespace",
			input:    "  example.com",
			expected: "https://example.com",
		},
		{
			name:     "with trailing whitespace",
			input:    "example.com  ",
			expected: "https://example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeURL(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeURL(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// ----------------------------------------
// ExtractDomain Tests
// ----------------------------------------

func TestExtractDomain(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "simple URL",
			url:      "https://example.com",
			expected: "example.com",
		},
		{
			name:     "URL with path",
			url:      "https://example.com/path/page",
			expected: "example.com",
		},
		{
			name:     "URL with port",
			url:      "https://example.com:8080/path",
			expected: "example.com:8080",
		},
		{
			name:     "URL with subdomain",
			url:      "https://www.example.com/page",
			expected: "www.example.com",
		},
		{
			name:     "URL with query string",
			url:      "https://example.com/page?foo=bar",
			expected: "example.com",
		},
		{
			name:     "invalid URL",
			url:      "not-a-url",
			expected: "",
		},
		{
			name:     "empty URL",
			url:      "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractDomain(tt.url)
			if result != tt.expected {
				t.Errorf("ExtractDomain(%q) = %q, want %q", tt.url, result, tt.expected)
			}
		})
	}
}

// ----------------------------------------
// isContextLengthError Tests
// ----------------------------------------

func TestIsContextLengthError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "context_length error",
			err:      errors.New("context_length_exceeded"),
			expected: true,
		},
		{
			name:     "context length error with spaces",
			err:      errors.New("context length exceeded"),
			expected: true,
		},
		{
			name:     "max_tokens error",
			err:      errors.New("max_tokens limit reached"),
			expected: true,
		},
		{
			name:     "token limit error",
			err:      errors.New("token limit exceeded"),
			expected: true,
		},
		{
			name:     "too long error",
			err:      errors.New("input is too long"),
			expected: true,
		},
		{
			name:     "maximum context error",
			err:      errors.New("maximum context size exceeded"),
			expected: true,
		},
		{
			name:     "exceeds limit error",
			err:      errors.New("input exceeds the limit"),
			expected: true,
		},
		{
			name:     "input too large error",
			err:      errors.New("input too large"),
			expected: true,
		},
		{
			name:     "content_too_large error",
			err:      errors.New("content_too_large"),
			expected: true,
		},
		{
			name:     "request too large error",
			err:      errors.New("request too large"),
			expected: true,
		},
		{
			name:     "unrelated error",
			err:      errors.New("network timeout"),
			expected: false,
		},
		{
			name:     "authentication error",
			err:      errors.New("invalid API key"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isContextLengthError(tt.err)
			if result != tt.expected {
				t.Errorf("isContextLengthError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

// ----------------------------------------
// CleanHTML Tests
// ----------------------------------------

func TestCleanHTML(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "removes script tags",
			input:    "<div>Hello</div><script>alert('test');</script><p>World</p>",
			expected: "<div>Hello</div><p>World</p>",
		},
		{
			name:     "removes style tags",
			input:    "<div>Hello</div><style>.foo { color: red; }</style><p>World</p>",
			expected: "<div>Hello</div><p>World</p>",
		},
		{
			name:     "removes HTML comments",
			input:    "<div>Hello</div><!-- This is a comment --><p>World</p>",
			expected: "<div>Hello</div><p>World</p>",
		},
		{
			name:     "normalizes whitespace",
			input:    "<div>Hello</div>    \n\n   <p>World</p>",
			expected: "<div>Hello</div> <p>World</p>",
		},
		{
			name:     "handles multiline script",
			input:    "<div>Hello</div><script>\nfunction test() {\n  return true;\n}\n</script><p>World</p>",
			expected: "<div>Hello</div><p>World</p>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CleanHTML(tt.input)
			if result != tt.expected {
				t.Errorf("CleanHTML() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// ----------------------------------------
// truncateContent Tests
// ----------------------------------------

func TestAnalyzerService_TruncateContent(t *testing.T) {
	cfg := &config.Config{EncryptionKey: []byte("12345678901234567890123456789012")}
	svc := NewAnalyzerService(cfg, &repository.Repositories{}, slog.Default())

	tests := []struct {
		name    string
		content string
		maxLen  int
		wantLen int
	}{
		{
			name:    "content shorter than max",
			content: "<div>Hello</div>",
			maxLen:  100,
			wantLen: 16,
		},
		{
			name:    "content exactly at max",
			content: "<div>Hello</div>",
			maxLen:  16,
			wantLen: 16,
		},
		{
			name:    "content longer truncated at tag",
			content: "<div>Hello</div><p>World</p>",
			maxLen:  20,
			wantLen: 19, // Truncates at "<p>" (last > is at position 18)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := svc.truncateContent(tt.content, tt.maxLen)
			if len(result) > tt.maxLen {
				t.Errorf("truncateContent() length = %d, should not exceed %d", len(result), tt.maxLen)
			}
			if len(result) != tt.wantLen {
				t.Errorf("truncateContent() length = %d, want %d", len(result), tt.wantLen)
			}
		})
	}
}

// ----------------------------------------
// filterLinks Tests
// ----------------------------------------

func TestAnalyzerService_FilterLinks(t *testing.T) {
	cfg := &config.Config{EncryptionKey: []byte("12345678901234567890123456789012")}
	svc := NewAnalyzerService(cfg, &repository.Repositories{}, slog.Default())

	tests := []struct {
		name     string
		baseURL  string
		links    []string
		expected []string
	}{
		{
			name:    "filters external links",
			baseURL: "https://example.com/page",
			links: []string{
				"https://example.com/page1",
				"https://external.com/page",
				"https://example.com/page2",
			},
			expected: []string{
				"https://example.com/page1",
				"https://example.com/page2",
			},
		},
		{
			name:    "filters login/signup links",
			baseURL: "https://example.com/",
			links: []string{
				"https://example.com/products",
				"https://example.com/login",
				"https://example.com/signup",
				"https://example.com/cart",
				"https://example.com/checkout",
			},
			expected: []string{
				"https://example.com/products",
			},
		},
		{
			name:    "deduplicates links",
			baseURL: "https://example.com/",
			links: []string{
				"https://example.com/page1",
				"https://example.com/page1",
				"https://example.com/page1",
			},
			expected: []string{
				"https://example.com/page1",
			},
		},
		{
			name:    "invalid base URL returns original",
			baseURL: "://invalid",
			links: []string{
				"https://example.com/page",
			},
			expected: []string{
				"https://example.com/page",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := svc.filterLinks(tt.baseURL, tt.links)
			if len(result) != len(tt.expected) {
				t.Errorf("filterLinks() returned %d links, want %d", len(result), len(tt.expected))
				return
			}
			for i, link := range result {
				if link != tt.expected[i] {
					t.Errorf("filterLinks()[%d] = %q, want %q", i, link, tt.expected[i])
				}
			}
		})
	}
}

// ----------------------------------------
// identifyDetailLinks Tests
// ----------------------------------------

func TestAnalyzerService_IdentifyDetailLinks(t *testing.T) {
	cfg := &config.Config{EncryptionKey: []byte("12345678901234567890123456789012")}
	svc := NewAnalyzerService(cfg, &repository.Repositories{}, slog.Default())

	tests := []struct {
		name         string
		baseURL      string
		links        []string
		expectsLinks bool
	}{
		{
			name:    "identifies product links",
			baseURL: "https://example.com/",
			links: []string{
				"https://example.com/products/item-1",
				"https://example.com/about",
				"https://example.com/contact",
			},
			expectsLinks: true,
		},
		{
			name:    "identifies article links",
			baseURL: "https://example.com/blog",
			links: []string{
				"https://example.com/article/my-blog-post-title",
				"https://example.com/",
			},
			expectsLinks: true,
		},
		{
			name:    "identifies numeric ID paths",
			baseURL: "https://example.com/",
			links: []string{
				"https://example.com/item/12345",
				"https://example.com/home",
			},
			expectsLinks: true,
		},
		{
			name:    "no detail links",
			baseURL: "https://example.com/",
			links: []string{
				"https://example.com/",
				"https://example.com/about",
			},
			expectsLinks: false,
		},
		{
			name:         "invalid base URL",
			baseURL:      "://invalid",
			links:        []string{"https://example.com/products/item"},
			expectsLinks: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := svc.identifyDetailLinks(tt.baseURL, tt.links)
			hasLinks := len(result) > 0
			if hasLinks != tt.expectsLinks {
				t.Errorf("identifyDetailLinks() returned %d links, expected links: %v", len(result), tt.expectsLinks)
			}
			// Should return at most 2 links
			if len(result) > 2 {
				t.Errorf("identifyDetailLinks() returned %d links, expected at most 2", len(result))
			}
		})
	}
}

// ----------------------------------------
// parsePageType Tests
// ----------------------------------------

func TestAnalyzerService_ParsePageType(t *testing.T) {
	cfg := &config.Config{EncryptionKey: []byte("12345678901234567890123456789012")}
	svc := NewAnalyzerService(cfg, &repository.Repositories{}, slog.Default())

	tests := []struct {
		name     string
		input    string
		expected models.PageType
	}{
		{"listing", "listing", models.PageTypeListing},
		{"detail", "detail", models.PageTypeDetail},
		{"article", "article", models.PageTypeArticle},
		{"product", "product", models.PageTypeProduct},
		{"recipe", "recipe", models.PageTypeRecipe},
		{"company", "company", models.PageTypeCompany},
		{"service", "service", models.PageTypeService},
		{"team", "team", models.PageTypeTeam},
		{"contact", "contact", models.PageTypeContact},
		{"portfolio", "portfolio", models.PageTypePortfolio},
		{"unknown", "unknown", models.PageTypeUnknown},
		{"empty", "", models.PageTypeUnknown},
		{"random", "foobar", models.PageTypeUnknown},
		{"uppercase", "LISTING", models.PageTypeListing},
		{"mixed case", "Detail", models.PageTypeDetail},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := svc.parsePageType(tt.input)
			if result != tt.expected {
				t.Errorf("parsePageType(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// ----------------------------------------
// LLMClient ParseResponse Tests (moved from analyzer-specific tests)
// ----------------------------------------

func TestLLMClient_ParseResponse_OpenAI(t *testing.T) {
	client := NewLLMClient(slog.Default(), nil)

	body := []byte(`{
		"choices": [
			{
				"message": {
					"content": "Test response content"
				}
			}
		],
		"usage": {
			"prompt_tokens": 100,
			"completion_tokens": 50
		}
	}`)

	result, err := client.ParseResponse("openai", body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Content != "Test response content" {
		t.Errorf("Content = %q, want %q", result.Content, "Test response content")
	}
	if result.InputTokens != 100 {
		t.Errorf("InputTokens = %d, want 100", result.InputTokens)
	}
	if result.OutputTokens != 50 {
		t.Errorf("OutputTokens = %d, want 50", result.OutputTokens)
	}
}

func TestLLMClient_ParseResponse_OpenRouter(t *testing.T) {
	client := NewLLMClient(slog.Default(), nil)

	body := []byte(`{
		"choices": [
			{
				"message": {
					"content": "OpenRouter response"
				}
			}
		],
		"usage": {
			"prompt_tokens": 200,
			"completion_tokens": 75
		}
	}`)

	result, err := client.ParseResponse("openrouter", body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Content != "OpenRouter response" {
		t.Errorf("Content = %q, want %q", result.Content, "OpenRouter response")
	}
	if result.InputTokens != 200 {
		t.Errorf("InputTokens = %d, want 200", result.InputTokens)
	}
}

func TestLLMClient_ParseResponse_Anthropic(t *testing.T) {
	client := NewLLMClient(slog.Default(), nil)

	body := []byte(`{
		"content": [
			{
				"text": "Anthropic response text"
			}
		],
		"usage": {
			"input_tokens": 150,
			"output_tokens": 60
		}
	}`)

	result, err := client.ParseResponse("anthropic", body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Content != "Anthropic response text" {
		t.Errorf("Content = %q, want %q", result.Content, "Anthropic response text")
	}
	if result.InputTokens != 150 {
		t.Errorf("InputTokens = %d, want 150", result.InputTokens)
	}
	if result.OutputTokens != 60 {
		t.Errorf("OutputTokens = %d, want 60", result.OutputTokens)
	}
}

func TestLLMClient_ParseResponse_EmptyChoices(t *testing.T) {
	client := NewLLMClient(slog.Default(), nil)

	body := []byte(`{"choices": [], "usage": {}}`)

	_, err := client.ParseResponse("openai", body)
	if err == nil {
		t.Fatal("expected error for empty choices")
	}
}

func TestLLMClient_ParseResponse_EmptyContent(t *testing.T) {
	client := NewLLMClient(slog.Default(), nil)

	body := []byte(`{"content": [], "usage": {}}`)

	_, err := client.ParseResponse("anthropic", body)
	if err == nil {
		t.Fatal("expected error for empty content")
	}
}

func TestLLMClient_ParseResponse_InvalidJSON(t *testing.T) {
	client := NewLLMClient(slog.Default(), nil)

	_, err := client.ParseResponse("openai", []byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// ----------------------------------------
// parseAnalysisResponse Tests
// ----------------------------------------

func TestAnalyzerService_ParseAnalysisResponse(t *testing.T) {
	cfg := &config.Config{EncryptionKey: []byte("12345678901234567890123456789012")}
	svc := NewAnalyzerService(cfg, &repository.Repositories{}, slog.Default())

	response := `{
		"site_summary": "An e-commerce website",
		"page_type": "listing",
		"detected_elements": [
			{"name": "product_title", "type": "string", "count": 10, "description": "Product names"},
			{"name": "price", "type": "number", "count": 10, "description": "Product prices"}
		],
		"suggested_schema": "name: Products\nfields:\n  - name: items\n    type: array",
		"follow_patterns": [
			{"pattern": "a[href*='/products/']", "description": "Product detail pages"}
		]
	}`

	output, err := svc.parseAnalysisResponse(response)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.SiteSummary != "An e-commerce website" {
		t.Errorf("SiteSummary = %q, want %q", output.SiteSummary, "An e-commerce website")
	}
	if output.PageType != models.PageTypeListing {
		t.Errorf("PageType = %q, want %q", output.PageType, models.PageTypeListing)
	}
	if len(output.DetectedElements) != 2 {
		t.Errorf("DetectedElements count = %d, want 2", len(output.DetectedElements))
	}
	if len(output.FollowPatterns) != 1 {
		t.Errorf("FollowPatterns count = %d, want 1", len(output.FollowPatterns))
	}
}

func TestAnalyzerService_ParseAnalysisResponse_WithMarkdown(t *testing.T) {
	cfg := &config.Config{EncryptionKey: []byte("12345678901234567890123456789012")}
	svc := NewAnalyzerService(cfg, &repository.Repositories{}, slog.Default())

	// Response wrapped in markdown code block
	response := "```json\n" + `{
		"site_summary": "A blog",
		"page_type": "article",
		"detected_elements": [],
		"suggested_schema": "",
		"follow_patterns": []
	}` + "\n```"

	output, err := svc.parseAnalysisResponse(response)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.SiteSummary != "A blog" {
		t.Errorf("SiteSummary = %q, want %q", output.SiteSummary, "A blog")
	}
}

func TestAnalyzerService_ParseAnalysisResponse_WithExtraText(t *testing.T) {
	cfg := &config.Config{EncryptionKey: []byte("12345678901234567890123456789012")}
	svc := NewAnalyzerService(cfg, &repository.Repositories{}, slog.Default())

	// Response with extra text before JSON
	response := `Here is my analysis:

{
	"site_summary": "Test site",
	"page_type": "product",
	"detected_elements": [],
	"suggested_schema": "",
	"follow_patterns": []
}

I hope this helps!`

	output, err := svc.parseAnalysisResponse(response)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.SiteSummary != "Test site" {
		t.Errorf("SiteSummary = %q, want %q", output.SiteSummary, "Test site")
	}
}

func TestAnalyzerService_ParseAnalysisResponse_InvalidJSON(t *testing.T) {
	cfg := &config.Config{EncryptionKey: []byte("12345678901234567890123456789012")}
	svc := NewAnalyzerService(cfg, &repository.Repositories{}, slog.Default())

	_, err := svc.parseAnalysisResponse("not json at all")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// ----------------------------------------
// getStrictMode Tests
// ----------------------------------------

func TestAnalyzerService_GetStrictMode_NilResolver(t *testing.T) {
	cfg := &config.Config{EncryptionKey: []byte("12345678901234567890123456789012")}
	svc := NewAnalyzerService(cfg, &repository.Repositories{}, slog.Default())
	// Ensure resolver is nil
	svc.resolver = nil

	// Should use static defaults when resolver is nil
	// Result depends on model - just verify no panic
	_ = svc.getStrictMode(nil, "openrouter", "gpt-4")
	_ = svc.getStrictMode(nil, "anthropic", "claude-3-5-sonnet")
	_ = svc.getStrictMode(nil, "ollama", "llama3.2")
}

// ----------------------------------------
// Struct Tests
// ----------------------------------------

func TestAnalyzeInput_Fields(t *testing.T) {
	input := AnalyzeInput{
		URL:       "https://example.com",
		Depth:     1,
		FetchMode: "static",
	}

	if input.URL != "https://example.com" {
		t.Errorf("URL = %q, want %q", input.URL, "https://example.com")
	}
	if input.Depth != 1 {
		t.Errorf("Depth = %d, want 1", input.Depth)
	}
	if input.FetchMode != "static" {
		t.Errorf("FetchMode = %q, want %q", input.FetchMode, "static")
	}
}

func TestAnalyzeOutput_Fields(t *testing.T) {
	output := AnalyzeOutput{
		SiteSummary:          "Test summary",
		PageType:             models.PageTypeListing,
		RecommendedFetchMode: models.FetchModeStatic,
		SampleLinks:          []string{"https://example.com/page"},
	}

	if output.SiteSummary != "Test summary" {
		t.Errorf("SiteSummary = %q, want %q", output.SiteSummary, "Test summary")
	}
	if output.PageType != models.PageTypeListing {
		t.Errorf("PageType = %q, want %q", output.PageType, models.PageTypeListing)
	}
	if output.RecommendedFetchMode != models.FetchModeStatic {
		t.Errorf("RecommendedFetchMode = %q, want %q", output.RecommendedFetchMode, models.FetchModeStatic)
	}
}

func TestAnalyzeTokenUsage_Fields(t *testing.T) {
	usage := AnalyzeTokenUsage{
		InputTokens:  100,
		OutputTokens: 50,
	}

	if usage.InputTokens != 100 {
		t.Errorf("InputTokens = %d, want 100", usage.InputTokens)
	}
	if usage.OutputTokens != 50 {
		t.Errorf("OutputTokens = %d, want 50", usage.OutputTokens)
	}
}

// ----------------------------------------
// min function Tests
// ----------------------------------------

func TestMin(t *testing.T) {
	tests := []struct {
		a, b     int
		expected int
	}{
		{1, 2, 1},
		{2, 1, 1},
		{5, 5, 5},
		{-1, 0, -1},
		{0, -1, -1},
	}

	for _, tt := range tests {
		result := min(tt.a, tt.b)
		if result != tt.expected {
			t.Errorf("min(%d, %d) = %d, want %d", tt.a, tt.b, result, tt.expected)
		}
	}
}
