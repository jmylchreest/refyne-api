package service

import (
	"testing"
)

func TestEnrichCleanerChainWithCrawlSelectors(t *testing.T) {
	tests := []struct {
		name           string
		chain          []CleanerConfig
		followSelector string
		nextSelector   string
		expectChange   bool
		expectKeep     []string
	}{
		{
			name:           "no selectors returns unchanged chain",
			chain:          []CleanerConfig{{Name: "markdown"}},
			followSelector: "",
			nextSelector:   "",
			expectChange:   false,
		},
		{
			name:           "adds follow selector to refyne cleaner",
			chain:          []CleanerConfig{{Name: "refyne"}},
			followSelector: "a.product-link",
			nextSelector:   "",
			expectChange:   true,
			expectKeep:     []string{"a.product-link"},
		},
		{
			name:           "adds next selector to refyne cleaner",
			chain:          []CleanerConfig{{Name: "refyne"}},
			followSelector: "",
			nextSelector:   "a.next-page",
			expectChange:   true,
			expectKeep:     []string{"a.next-page"},
		},
		{
			name:           "adds both selectors to refyne cleaner",
			chain:          []CleanerConfig{{Name: "refyne"}},
			followSelector: "a.product-link",
			nextSelector:   "a.next-page",
			expectChange:   true,
			expectKeep:     []string{"a.product-link", "a.next-page"},
		},
		{
			name: "appends to existing keep selectors",
			chain: []CleanerConfig{{
				Name: "refyne",
				Options: &CleanerOptions{
					KeepSelectors: []string{".main-content"},
				},
			}},
			followSelector: "a.product-link",
			nextSelector:   "",
			expectChange:   true,
			expectKeep:     []string{".main-content", "a.product-link"},
		},
		{
			name:           "non-refyne cleaner unchanged",
			chain:          []CleanerConfig{{Name: "markdown"}, {Name: "trafilatura"}},
			followSelector: "a.link",
			nextSelector:   "",
			expectChange:   false,
		},
		{
			name: "mixed chain only enriches refyne",
			chain: []CleanerConfig{
				{Name: "refyne"},
				{Name: "markdown"},
			},
			followSelector: "a.link",
			nextSelector:   "",
			expectChange:   true,
			expectKeep:     []string{"a.link"},
		},
		{
			name:           "case insensitive refyne matching",
			chain:          []CleanerConfig{{Name: "REFYNE"}},
			followSelector: "a.link",
			nextSelector:   "",
			expectChange:   true,
			expectKeep:     []string{"a.link"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EnrichCleanerChainWithCrawlSelectors(tt.chain, tt.followSelector, tt.nextSelector)

			if len(result) != len(tt.chain) {
				t.Errorf("expected chain length %d, got %d", len(tt.chain), len(result))
			}

			if !tt.expectChange {
				// Verify chain is unchanged (same reference for non-refyne cases)
				for i := range result {
					if result[i].Name != tt.chain[i].Name {
						t.Errorf("expected cleaner name %q, got %q", tt.chain[i].Name, result[i].Name)
					}
				}
				return
			}

			// Find refyne cleaner and check keep selectors
			for i, cfg := range result {
				if cfg.Name == "refyne" || cfg.Name == "REFYNE" {
					if cfg.Options == nil {
						t.Error("expected refyne options to be non-nil")
						continue
					}

					// Check all expected selectors are present
					keepSet := make(map[string]bool)
					for _, s := range cfg.Options.KeepSelectors {
						keepSet[s] = true
					}

					for _, expected := range tt.expectKeep {
						if !keepSet[expected] {
							t.Errorf("expected keep selector %q not found in refyne at index %d", expected, i)
						}
					}
				}
			}
		})
	}
}

func TestCleanerFactoryCreate(t *testing.T) {
	factory := NewCleanerFactory()

	tests := []struct {
		name        string
		config      CleanerConfig
		expectError bool
	}{
		{"noop cleaner", CleanerConfig{Name: "noop"}, false},
		{"markdown cleaner", CleanerConfig{Name: "markdown"}, false},
		{"trafilatura cleaner", CleanerConfig{Name: "trafilatura"}, false},
		{"readability cleaner", CleanerConfig{Name: "readability"}, false},
		{"refyne cleaner", CleanerConfig{Name: "refyne"}, false},
		{"unknown cleaner", CleanerConfig{Name: "unknown"}, true},
		{
			"trafilatura with options",
			CleanerConfig{
				Name: "trafilatura",
				Options: &CleanerOptions{
					Output: "text",
					Tables: false,
					Links:  true,
					Images: false,
				},
			},
			false,
		},
		{
			"readability with options",
			CleanerConfig{
				Name: "readability",
				Options: &CleanerOptions{
					Output:  "text",
					BaseURL: "https://example.com",
				},
			},
			false,
		},
		{
			"refyne with preset",
			CleanerConfig{
				Name: "refyne",
				Options: &CleanerOptions{
					Preset: "aggressive",
				},
			},
			false,
		},
		{
			"refyne with selectors",
			CleanerConfig{
				Name: "refyne",
				Options: &CleanerOptions{
					RemoveSelectors: []string{".ad", "nav"},
					KeepSelectors:   []string{".main"},
				},
			},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleaner, err := factory.Create(tt.config)

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if cleaner == nil {
				t.Error("expected non-nil cleaner")
			}
		})
	}
}

func TestCleanerFactoryCreateChain(t *testing.T) {
	factory := NewCleanerFactory()

	tests := []struct {
		name        string
		configs     []CleanerConfig
		expectError bool
	}{
		{
			"empty uses default",
			[]CleanerConfig{},
			false,
		},
		{
			"single cleaner",
			[]CleanerConfig{{Name: "markdown"}},
			false,
		},
		{
			"chain of two",
			[]CleanerConfig{{Name: "refyne"}, {Name: "markdown"}},
			false,
		},
		{
			"chain with invalid",
			[]CleanerConfig{{Name: "refyne"}, {Name: "invalid"}},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleaner, err := factory.CreateChain(tt.configs)

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if cleaner == nil {
				t.Error("expected non-nil cleaner")
			}
		})
	}
}

func TestIsValidCleanerType(t *testing.T) {
	valid := []string{"noop", "markdown", "trafilatura", "readability", "refyne"}
	invalid := []string{"invalid", "unknown", "html", ""}

	for _, v := range valid {
		if !IsValidCleanerType(v) {
			t.Errorf("expected %q to be valid", v)
		}
	}

	for _, v := range invalid {
		if IsValidCleanerType(v) {
			t.Errorf("expected %q to be invalid", v)
		}
	}
}

func TestGetAvailableCleaners(t *testing.T) {
	cleaners := GetAvailableCleaners()

	if len(cleaners) == 0 {
		t.Error("expected non-empty list of cleaners")
	}

	// Check that all expected cleaners are present
	expectedNames := []string{"noop", "markdown", "trafilatura", "readability", "refyne"}
	cleanerMap := make(map[string]CleanerInfo)
	for _, c := range cleaners {
		cleanerMap[c.Name] = c
	}

	for _, name := range expectedNames {
		if _, ok := cleanerMap[name]; !ok {
			t.Errorf("expected cleaner %q in available list", name)
		}
	}

	// Check that refyne has expected options
	if refyne, ok := cleanerMap["refyne"]; ok {
		if len(refyne.Options) == 0 {
			t.Error("expected refyne to have options")
		}

		optionNames := make(map[string]bool)
		for _, opt := range refyne.Options {
			optionNames[opt.Name] = true
		}

		if !optionNames["preset"] {
			t.Error("expected refyne to have preset option")
		}
		if !optionNames["remove_selectors"] {
			t.Error("expected refyne to have remove_selectors option")
		}
		if !optionNames["keep_selectors"] {
			t.Error("expected refyne to have keep_selectors option")
		}
	}
}

func TestGetChainName(t *testing.T) {
	tests := []struct {
		configs  []CleanerConfig
		expected string
	}{
		{nil, "default"},
		{[]CleanerConfig{}, "default"},
		{[]CleanerConfig{{Name: "markdown"}}, "markdown"},
		{[]CleanerConfig{{Name: "refyne"}, {Name: "markdown"}}, "refyne->markdown"},
		{[]CleanerConfig{{Name: "trafilatura"}, {Name: "markdown"}}, "trafilatura->markdown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := GetChainName(tt.configs)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}
