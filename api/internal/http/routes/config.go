// Package routes provides shared route registration for the Refyne API.
// This allows both the main server and the OpenAPI generator to use
// the same route definitions, ensuring the spec is always in sync.
package routes

import (
	"github.com/danielgtaylor/huma/v2"

	"github.com/jmylchreest/refyne-api/internal/http/mw"
	"github.com/jmylchreest/refyne-api/internal/version"
)

// NewHumaConfig creates the shared Huma configuration for the API.
// This includes API metadata, security schemes, and tag definitions.
func NewHumaConfig(baseURL string) huma.Config {
	cfg := huma.DefaultConfig("Refyne API", version.Get().Short())
	cfg.Info.Description = "LLM-powered web extraction API that transforms unstructured websites into clean, typed JSON."

	// Disable $schema field in responses - it conflicts with "schema" field in SDK code generators
	cfg.CreateHooks = nil

	if baseURL != "" {
		cfg.Servers = []*huma.Server{
			{URL: baseURL, Description: "API Server"},
		}
	}

	// Add security scheme for Bearer auth
	cfg.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		mw.SecurityScheme: {
			Type:        "http",
			Scheme:      "bearer",
			Description: "API key authentication. Include your API key in the Authorization header as `Bearer rf_your_key`.",
		},
	}

	// Define OpenAPI tags with display names for documentation
	cfg.Tags = []*huma.Tag{
		{Name: "Extraction", Description: "Data extraction, crawling, and analysis endpoints", Extensions: map[string]any{"x-displayName": "Extraction"}},
		{Name: "Jobs", Description: "Job status and results retrieval", Extensions: map[string]any{"x-displayName": "Jobs"}},
		{Name: "Schemas", Description: "Schema catalog management", Extensions: map[string]any{"x-displayName": "Schemas"}},
		{Name: "Sites", Description: "Saved site configuration", Extensions: map[string]any{"x-displayName": "Sites"}},
		{Name: "Webhooks", Description: "Webhook management for real-time notifications", Extensions: map[string]any{"x-displayName": "Webhooks"}},
		{Name: "API Keys", Description: "API key management", Extensions: map[string]any{"x-displayName": "API Keys"}},
		{Name: "LLM Providers", Description: "Available LLM providers and models", Extensions: map[string]any{"x-displayName": "LLM Providers"}},
		{Name: "LLM Keys", Description: "User LLM API key management", Extensions: map[string]any{"x-displayName": "LLM Keys"}},
		{Name: "LLM Chain", Description: "LLM fallback chain configuration", Extensions: map[string]any{"x-displayName": "LLM Chain"}},
		{Name: "Usage", Description: "Usage statistics and billing", Extensions: map[string]any{"x-displayName": "Usage"}},
		{Name: "Health", Description: "System health and status", Extensions: map[string]any{"x-displayName": "Health"}},
		{Name: "Pricing", Description: "Pricing and tier information", Extensions: map[string]any{"x-displayName": "Pricing"}},
	}

	return cfg
}
