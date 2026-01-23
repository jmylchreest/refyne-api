package service

import (
	"fmt"
	"strings"

	"github.com/jmylchreest/refyne/pkg/cleaner"
)

// CleanerType represents a named content cleaner type.
type CleanerType string

// Available cleaner types
const (
	// CleanerNoop passes content through unchanged (raw HTML).
	CleanerNoop CleanerType = "noop"

	// CleanerMarkdown converts HTML to Markdown, preserving structure.
	CleanerMarkdown CleanerType = "markdown"

	// CleanerTrafilatura extracts main content using Trafilatura algorithm.
	CleanerTrafilatura CleanerType = "trafilatura"

	// CleanerReadability extracts main content using Mozilla Readability algorithm.
	CleanerReadability CleanerType = "readability"
)

// DefaultExtractionCleaner is the default cleaner chain for extraction operations.
// Uses markdown-only to preserve all content (Trafilatura strips form elements).
var DefaultExtractionCleanerChain = []CleanerConfig{{Name: "markdown"}}

// DefaultAnalyzerCleaner is the default cleaner for analysis operations.
// Uses noop to preserve maximum detail for schema generation.
var DefaultAnalyzerCleanerChain = []CleanerConfig{{Name: "noop"}}

// AnalyzerFallbackCleanerChain is used when analyzer content exceeds context limits.
var AnalyzerFallbackCleanerChain = []CleanerConfig{{
	Name: "trafilatura",
	Options: &CleanerOptions{
		Output: "html",
		Tables: true,
		Links:  true,
		Images: false, // Exclude images to save tokens
	},
}}

// CleanerOptions contains configuration options for cleaners.
// Not all options apply to all cleaners.
type CleanerOptions struct {
	// Output format: "html" or "text" (trafilatura, readability)
	Output string `json:"output,omitempty"`

	// Tables: include tables in output (trafilatura, readability)
	Tables bool `json:"tables,omitempty"`

	// Links: include links in output (trafilatura, readability)
	Links bool `json:"links,omitempty"`

	// Images: include images in output (trafilatura)
	Images bool `json:"images,omitempty"`

	// BaseURL: base URL for resolving relative links (readability)
	BaseURL string `json:"base_url,omitempty"`
}

// CleanerConfig defines a single cleaner in a chain.
type CleanerConfig struct {
	// Name is the cleaner type name (required)
	Name string `json:"name"`

	// Options contains cleaner-specific configuration (optional)
	Options *CleanerOptions `json:"options,omitempty"`
}

// ValidCleanerTypes returns all valid cleaner type names.
func ValidCleanerTypes() []CleanerType {
	return []CleanerType{
		CleanerNoop,
		CleanerMarkdown,
		CleanerTrafilatura,
		CleanerReadability,
	}
}

// IsValidCleanerType checks if a string is a valid cleaner type.
func IsValidCleanerType(s string) bool {
	for _, ct := range ValidCleanerTypes() {
		if string(ct) == s {
			return true
		}
	}
	return false
}

// CleanerFactory creates cleaner instances by name.
type CleanerFactory struct{}

// NewCleanerFactory creates a new cleaner factory.
func NewCleanerFactory() *CleanerFactory {
	return &CleanerFactory{}
}

// Create creates a single cleaner instance from config.
func (f *CleanerFactory) Create(config CleanerConfig) (cleaner.Cleaner, error) {
	cleanerType := CleanerType(strings.ToLower(config.Name))

	switch cleanerType {
	case CleanerNoop:
		return cleaner.NewNoop(), nil

	case CleanerMarkdown:
		return cleaner.NewMarkdown(), nil

	case CleanerTrafilatura:
		return f.createTrafilatura(config.Options), nil

	case CleanerReadability:
		return f.createReadability(config.Options), nil

	default:
		return nil, fmt.Errorf("unknown cleaner type: %s", config.Name)
	}
}

// createTrafilatura creates a Trafilatura cleaner with optional config.
func (f *CleanerFactory) createTrafilatura(opts *CleanerOptions) cleaner.Cleaner {
	cfg := &cleaner.TrafilaturaConfig{
		Output: cleaner.OutputHTML, // Default to HTML
		Tables: cleaner.Include,
		Links:  cleaner.Include,
		Images: cleaner.Include,
	}

	if opts != nil {
		if opts.Output == "text" {
			cfg.Output = cleaner.OutputText
		}
		if !opts.Tables {
			cfg.Tables = cleaner.Exclude
		}
		if !opts.Links {
			cfg.Links = cleaner.Exclude
		}
		if !opts.Images {
			cfg.Images = cleaner.Exclude
		}
	}

	return cleaner.NewTrafilatura(cfg)
}

// createReadability creates a Readability cleaner with optional config.
func (f *CleanerFactory) createReadability(opts *CleanerOptions) cleaner.Cleaner {
	cfg := &cleaner.ReadabilityConfig{
		Output: cleaner.OutputHTML, // Default to HTML
	}

	if opts != nil {
		if opts.Output == "text" {
			cfg.Output = cleaner.OutputText
		}
		if opts.BaseURL != "" {
			cfg.BaseURL = opts.BaseURL
		}
	}

	return cleaner.NewReadability(cfg)
}

// CreateChain creates a cleaner chain from a list of configs.
// If the list is empty, returns the default extraction cleaner chain.
func (f *CleanerFactory) CreateChain(configs []CleanerConfig) (cleaner.Cleaner, error) {
	// Use default if no configs provided
	if len(configs) == 0 {
		configs = DefaultExtractionCleanerChain
	}

	// Single cleaner - no chain needed
	if len(configs) == 1 {
		return f.Create(configs[0])
	}

	// Multiple cleaners - build chain
	cleaners := make([]cleaner.Cleaner, 0, len(configs))
	for _, cfg := range configs {
		c, err := f.Create(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create cleaner '%s': %w", cfg.Name, err)
		}
		cleaners = append(cleaners, c)
	}

	return cleaner.NewChain(cleaners...), nil
}

// CreateChainWithDefault creates a cleaner chain, using the provided default if configs is empty.
func (f *CleanerFactory) CreateChainWithDefault(configs []CleanerConfig, defaultChain []CleanerConfig) (cleaner.Cleaner, error) {
	if len(configs) == 0 {
		configs = defaultChain
	}
	return f.CreateChain(configs)
}

// CreateFromString creates a cleaner from a simple string name.
// This is a convenience method for simple cases where no options are needed.
// Returns the default extraction cleaner chain if the string is empty.
func (f *CleanerFactory) CreateFromString(name string) (cleaner.Cleaner, error) {
	if name == "" {
		return f.CreateChain(DefaultExtractionCleanerChain)
	}
	return f.Create(CleanerConfig{Name: name})
}

// CreateFromStringWithDefault creates a cleaner from a string, using the provided default if empty.
func (f *CleanerFactory) CreateFromStringWithDefault(name string, defaultChain []CleanerConfig) (cleaner.Cleaner, error) {
	if name == "" {
		return f.CreateChain(defaultChain)
	}
	return f.Create(CleanerConfig{Name: name})
}

// GetChainName returns a descriptive name for a cleaner chain.
func GetChainName(configs []CleanerConfig) string {
	if len(configs) == 0 {
		return "default"
	}
	names := make([]string, len(configs))
	for i, cfg := range configs {
		names[i] = cfg.Name
	}
	return strings.Join(names, "->")
}

// CleanerOptionInfo describes an available option for a cleaner.
type CleanerOptionInfo struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Default     any    `json:"default"`
	Description string `json:"description"`
}

// CleanerInfo describes an available cleaner.
type CleanerInfo struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Options     []CleanerOptionInfo `json:"options,omitempty"`
}

// GetAvailableCleaners returns information about all available cleaners.
func GetAvailableCleaners() []CleanerInfo {
	return []CleanerInfo{
		{
			Name:        "noop",
			Description: "Pass-through cleaner that returns content unchanged. Use for maximum detail when token usage is not a concern.",
			Options:     nil,
		},
		{
			Name:        "markdown",
			Description: "Converts HTML to Markdown, preserving structure (tables, links, images). Reduces tokens while keeping all content.",
			Options:     nil,
		},
		{
			Name:        "trafilatura",
			Description: "Extracts main content using the Trafilatura algorithm. Good for articles but may strip non-article content like forms.",
			Options: []CleanerOptionInfo{
				{Name: "output", Type: "string", Default: "html", Description: "Output format: 'html' preserves structure, 'text' strips all tags"},
				{Name: "tables", Type: "boolean", Default: true, Description: "Include tables in output"},
				{Name: "links", Type: "boolean", Default: true, Description: "Include links in output"},
				{Name: "images", Type: "boolean", Default: true, Description: "Include images in output"},
			},
		},
		{
			Name:        "readability",
			Description: "Extracts main content using Mozilla Readability algorithm. Similar to Trafilatura but uses different heuristics.",
			Options: []CleanerOptionInfo{
				{Name: "output", Type: "string", Default: "html", Description: "Output format: 'html' preserves structure, 'text' strips all tags"},
				{Name: "base_url", Type: "string", Default: "", Description: "Base URL for resolving relative links"},
			},
		},
	}
}

// GetDefaultExtractionChain returns the default cleaner chain for extraction.
func GetDefaultExtractionChain() []CleanerConfig {
	return DefaultExtractionCleanerChain
}

// GetDefaultAnalyzerChain returns the default cleaner chain for analysis.
func GetDefaultAnalyzerChain() []CleanerConfig {
	return DefaultAnalyzerCleanerChain
}
