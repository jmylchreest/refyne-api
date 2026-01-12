package handlers

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"github.com/jmylchreest/refyne-api/internal/service"
)

// LLMConfigHandler handles LLM config endpoints.
type LLMConfigHandler struct {
	llmConfigSvc *service.LLMConfigService
}

// NewLLMConfigHandler creates a new LLM config handler.
func NewLLMConfigHandler(llmConfigSvc *service.LLMConfigService) *LLMConfigHandler {
	return &LLMConfigHandler{llmConfigSvc: llmConfigSvc}
}

// GetLLMConfigOutput represents LLM config response.
type GetLLMConfigOutput struct {
	Body struct {
		Provider  string `json:"provider" doc:"LLM provider (anthropic, openai, openrouter, ollama, credits)"`
		HasAPIKey bool   `json:"has_api_key" doc:"Whether an API key is configured"`
		BaseURL   string `json:"base_url,omitempty" doc:"Custom base URL"`
		Model     string `json:"model,omitempty" doc:"Configured model"`
		UpdatedAt string `json:"updated_at,omitempty" doc:"Last update timestamp"`
	}
}

// GetConfig handles getting LLM config.
func (h *LLMConfigHandler) GetConfig(ctx context.Context, input *struct{}) (*GetLLMConfigOutput, error) {
	userID := getUserID(ctx)
	if userID == "" {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	cfg, err := h.llmConfigSvc.GetConfig(ctx, userID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get LLM config")
	}

	return &GetLLMConfigOutput{
		Body: struct {
			Provider  string `json:"provider" doc:"LLM provider (anthropic, openai, openrouter, ollama, credits)"`
			HasAPIKey bool   `json:"has_api_key" doc:"Whether an API key is configured"`
			BaseURL   string `json:"base_url,omitempty" doc:"Custom base URL"`
			Model     string `json:"model,omitempty" doc:"Configured model"`
			UpdatedAt string `json:"updated_at,omitempty" doc:"Last update timestamp"`
		}{
			Provider:  cfg.Provider,
			HasAPIKey: cfg.HasAPIKey,
			BaseURL:   cfg.BaseURL,
			Model:     cfg.Model,
			UpdatedAt: cfg.UpdatedAt,
		},
	}, nil
}

// UpdateLLMConfigInput represents LLM config update request.
type UpdateLLMConfigInput struct {
	Body struct {
		Provider string `json:"provider" enum:"anthropic,openai,openrouter,ollama,credits" doc:"LLM provider"`
		APIKey   string `json:"api_key,omitempty" doc:"API key for the provider"`
		BaseURL  string `json:"base_url,omitempty" doc:"Custom base URL (for Ollama)"`
		Model    string `json:"model,omitempty" doc:"Model to use"`
	}
}

// UpdateLLMConfigOutput represents LLM config update response.
type UpdateLLMConfigOutput struct {
	Body struct {
		Provider  string `json:"provider"`
		HasAPIKey bool   `json:"has_api_key"`
		BaseURL   string `json:"base_url,omitempty"`
		Model     string `json:"model,omitempty"`
		UpdatedAt string `json:"updated_at"`
	}
}

// UpdateConfig handles updating LLM config.
func (h *LLMConfigHandler) UpdateConfig(ctx context.Context, input *UpdateLLMConfigInput) (*UpdateLLMConfigOutput, error) {
	userID := getUserID(ctx)
	if userID == "" {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	cfg, err := h.llmConfigSvc.UpdateConfig(ctx, userID, service.UpdateLLMConfigInput{
		Provider: input.Body.Provider,
		APIKey:   input.Body.APIKey,
		BaseURL:  input.Body.BaseURL,
		Model:    input.Body.Model,
	})
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to update LLM config")
	}

	return &UpdateLLMConfigOutput{
		Body: struct {
			Provider  string `json:"provider"`
			HasAPIKey bool   `json:"has_api_key"`
			BaseURL   string `json:"base_url,omitempty"`
			Model     string `json:"model,omitempty"`
			UpdatedAt string `json:"updated_at"`
		}{
			Provider:  cfg.Provider,
			HasAPIKey: cfg.HasAPIKey,
			BaseURL:   cfg.BaseURL,
			Model:     cfg.Model,
			UpdatedAt: cfg.UpdatedAt,
		},
	}, nil
}
