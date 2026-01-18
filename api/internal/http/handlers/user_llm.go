package handlers

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"github.com/jmylchreest/refyne-api/internal/constants"
	"github.com/jmylchreest/refyne-api/internal/http/mw"
	"github.com/jmylchreest/refyne-api/internal/llm"
	"github.com/jmylchreest/refyne-api/internal/service"
)

// UserLLMHandler handles user LLM provider key and fallback chain endpoints.
type UserLLMHandler struct {
	userLLMSvc *service.UserLLMService
	adminSvc   *service.AdminService
	registry   *llm.Registry
}

// NewUserLLMHandler creates a new user LLM handler.
func NewUserLLMHandler(userLLMSvc *service.UserLLMService, adminSvc *service.AdminService, registry *llm.Registry) *UserLLMHandler {
	return &UserLLMHandler{userLLMSvc: userLLMSvc, adminSvc: adminSvc, registry: registry}
}

// UserServiceKeyInput represents a service key in API requests.
type UserServiceKeyInput struct {
	Provider  string `json:"provider" enum:"openrouter,anthropic,openai,ollama" doc:"LLM provider name"`
	APIKey    string `json:"api_key,omitempty" doc:"API key for the provider (leave empty to keep existing)"`
	BaseURL   string `json:"base_url,omitempty" doc:"Base URL for the provider (for Ollama or custom endpoints)"`
	IsEnabled bool   `json:"is_enabled" doc:"Whether this provider is enabled"`
}

// UserServiceKeyResponse represents a service key in API responses.
type UserServiceKeyResponse struct {
	ID        string `json:"id"`
	Provider  string `json:"provider"`
	HasKey    bool   `json:"has_key"` // Never expose actual key
	BaseURL   string `json:"base_url,omitempty"`
	IsEnabled bool   `json:"is_enabled"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// ListUserServiceKeysOutput represents the list service keys response.
type ListUserServiceKeysOutput struct {
	Body struct {
		Keys []UserServiceKeyResponse `json:"keys"`
	}
}

// ListServiceKeys returns all configured service keys for the user.
func (h *UserLLMHandler) ListServiceKeys(ctx context.Context, input *struct{}) (*ListUserServiceKeysOutput, error) {
	claims := mw.GetUserClaims(ctx)
	if claims == nil {
		return nil, huma.Error401Unauthorized("authentication required")
	}

	keys, err := h.userLLMSvc.ListServiceKeys(ctx, claims.UserID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list service keys: " + err.Error())
	}

	responses := make([]UserServiceKeyResponse, 0, len(keys))
	for _, k := range keys {
		responses = append(responses, UserServiceKeyResponse{
			ID:        k.ID,
			Provider:  k.Provider,
			HasKey:    k.APIKeyEncrypted != "",
			BaseURL:   k.BaseURL,
			IsEnabled: k.IsEnabled,
			CreatedAt: k.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt: k.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	return &ListUserServiceKeysOutput{
		Body: struct {
			Keys []UserServiceKeyResponse `json:"keys"`
		}{Keys: responses},
	}, nil
}

// ListProvidersOutput represents the list providers response.
type ListProvidersOutput struct {
	Body struct {
		Providers []llm.ProviderInfo `json:"providers"`
	}
}

// ListProviders returns all LLM providers available to the user.
func (h *UserLLMHandler) ListProviders(ctx context.Context, input *struct{}) (*ListProvidersOutput, error) {
	claims := mw.GetUserClaims(ctx)
	if claims == nil {
		return nil, huma.Error401Unauthorized("authentication required")
	}

	providers := h.registry.ListProviders(claims)

	return &ListProvidersOutput{
		Body: struct {
			Providers []llm.ProviderInfo `json:"providers"`
		}{Providers: providers},
	}, nil
}

// UpsertUserServiceKeyInput represents the upsert service key request.
type UpsertUserServiceKeyInput struct {
	Body UserServiceKeyInput
}

// UpsertUserServiceKeyOutput represents the upsert service key response.
type UpsertUserServiceKeyOutput struct {
	Body UserServiceKeyResponse
}

// UpsertServiceKey creates or updates a user service key.
// Requires the provider_byok feature and access to the specific provider.
func (h *UserLLMHandler) UpsertServiceKey(ctx context.Context, input *UpsertUserServiceKeyInput) (*UpsertUserServiceKeyOutput, error) {
	claims := mw.GetUserClaims(ctx)
	if claims == nil {
		return nil, huma.Error401Unauthorized("authentication required")
	}

	// Gate by provider_byok feature
	if !claims.HasFeature(constants.FeatureProviderBYOK) {
		return nil, huma.Error403Forbidden(constants.FeatureNotAvailableMessage(constants.FeatureProviderBYOK))
	}

	// Check provider access via registry
	if !h.registry.IsProviderAllowed(input.Body.Provider, claims) {
		missing := h.registry.GetMissingFeatures(input.Body.Provider, claims)
		if len(missing) > 0 {
			return nil, huma.Error403Forbidden(constants.FeatureNotAvailableMessage(missing[0]))
		}
		return nil, huma.Error403Forbidden("provider not available on your current plan")
	}

	key, err := h.userLLMSvc.UpsertServiceKey(ctx, claims.UserID, service.UserServiceKeyInput{
		Provider:  input.Body.Provider,
		APIKey:    input.Body.APIKey,
		BaseURL:   input.Body.BaseURL,
		IsEnabled: input.Body.IsEnabled,
	})
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	return &UpsertUserServiceKeyOutput{
		Body: UserServiceKeyResponse{
			ID:        key.ID,
			Provider:  key.Provider,
			HasKey:    key.APIKeyEncrypted != "",
			BaseURL:   key.BaseURL,
			IsEnabled: key.IsEnabled,
			CreatedAt: key.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt: key.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		},
	}, nil
}

// DeleteUserServiceKeyInput represents the delete service key request.
type DeleteUserServiceKeyInput struct {
	ID string `path:"id" doc:"Service key ID to delete"`
}

// DeleteUserServiceKeyOutput represents the delete service key response.
type DeleteUserServiceKeyOutput struct {
	Body struct {
		Success bool `json:"success"`
	}
}

// DeleteServiceKey removes a user service key.
// Requires the provider_byok feature.
func (h *UserLLMHandler) DeleteServiceKey(ctx context.Context, input *DeleteUserServiceKeyInput) (*DeleteUserServiceKeyOutput, error) {
	claims := mw.GetUserClaims(ctx)
	if claims == nil {
		return nil, huma.Error401Unauthorized("authentication required")
	}

	// Gate by provider_byok feature
	if !claims.HasFeature(constants.FeatureProviderBYOK) {
		return nil, huma.Error403Forbidden(constants.FeatureNotAvailableMessage(constants.FeatureProviderBYOK))
	}

	if err := h.userLLMSvc.DeleteServiceKey(ctx, claims.UserID, input.ID); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	return &DeleteUserServiceKeyOutput{
		Body: struct {
			Success bool `json:"success"`
		}{Success: true},
	}, nil
}

// UserFallbackChainEntryResponse represents a fallback chain entry in API responses.
type UserFallbackChainEntryResponse struct {
	ID          string   `json:"id"`
	Position    int      `json:"position"`
	Provider    string   `json:"provider"`
	Model       string   `json:"model"`
	Temperature *float64 `json:"temperature,omitempty"`
	MaxTokens   *int     `json:"max_tokens,omitempty"`
	IsEnabled   bool     `json:"is_enabled"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
}

// GetUserFallbackChainOutput represents the get fallback chain response.
type GetUserFallbackChainOutput struct {
	Body struct {
		Chain []UserFallbackChainEntryResponse `json:"chain"`
	}
}

// GetFallbackChain returns the user's fallback chain configuration.
func (h *UserLLMHandler) GetFallbackChain(ctx context.Context, input *struct{}) (*GetUserFallbackChainOutput, error) {
	claims := mw.GetUserClaims(ctx)
	if claims == nil {
		return nil, huma.Error401Unauthorized("authentication required")
	}

	entries, err := h.userLLMSvc.GetFallbackChain(ctx, claims.UserID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get fallback chain: " + err.Error())
	}

	responses := make([]UserFallbackChainEntryResponse, 0, len(entries))
	for _, e := range entries {
		responses = append(responses, UserFallbackChainEntryResponse{
			ID:          e.ID,
			Position:    e.Position,
			Provider:    e.Provider,
			Model:       e.Model,
			Temperature: e.Temperature,
			MaxTokens:   e.MaxTokens,
			IsEnabled:   e.IsEnabled,
			CreatedAt:   e.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt:   e.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	return &GetUserFallbackChainOutput{
		Body: struct {
			Chain []UserFallbackChainEntryResponse `json:"chain"`
		}{Chain: responses},
	}, nil
}

// UserFallbackChainEntryInput represents a fallback chain entry in API requests.
type UserFallbackChainEntryInput struct {
	Provider    string   `json:"provider" enum:"openrouter,anthropic,openai,ollama" doc:"LLM provider name"`
	Model       string   `json:"model" minLength:"1" doc:"Model identifier"`
	Temperature *float64 `json:"temperature,omitempty" doc:"Temperature setting (0.0-1.0, nil for default)"`
	MaxTokens   *int     `json:"max_tokens,omitempty" doc:"Max output tokens (nil for default)"`
	IsEnabled   bool     `json:"is_enabled" doc:"Whether this entry is enabled"`
}

// SetUserFallbackChainInput represents the set fallback chain request.
type SetUserFallbackChainInput struct {
	Body struct {
		Chain []UserFallbackChainEntryInput `json:"chain" doc:"Ordered list of provider:model pairs"`
	}
}

// SetUserFallbackChainOutput represents the set fallback chain response.
type SetUserFallbackChainOutput struct {
	Body struct {
		Chain []UserFallbackChainEntryResponse `json:"chain"`
	}
}

// SetFallbackChain replaces the user's fallback chain configuration.
// Requires the models_custom feature. Provider access is validated for each entry.
func (h *UserLLMHandler) SetFallbackChain(ctx context.Context, input *SetUserFallbackChainInput) (*SetUserFallbackChainOutput, error) {
	claims := mw.GetUserClaims(ctx)
	if claims == nil {
		return nil, huma.Error401Unauthorized("authentication required")
	}

	// Gate by models_custom feature
	if !claims.HasFeature(constants.FeatureModelsCustom) {
		return nil, huma.Error403Forbidden(constants.FeatureNotAvailableMessage(constants.FeatureModelsCustom))
	}

	// Validate provider access for each chain entry
	for _, e := range input.Body.Chain {
		if !h.registry.IsProviderAllowed(e.Provider, claims) {
			missing := h.registry.GetMissingFeatures(e.Provider, claims)
			if len(missing) > 0 {
				return nil, huma.Error403Forbidden(constants.FeatureNotAvailableMessage(missing[0]))
			}
			return nil, huma.Error403Forbidden("provider '" + e.Provider + "' not available on your current plan")
		}
	}

	// Convert to service input
	svcInput := make([]service.UserFallbackChainEntryInput, 0, len(input.Body.Chain))
	for _, e := range input.Body.Chain {
		svcInput = append(svcInput, service.UserFallbackChainEntryInput{
			Provider:    e.Provider,
			Model:       e.Model,
			Temperature: e.Temperature,
			MaxTokens:   e.MaxTokens,
			IsEnabled:   e.IsEnabled,
		})
	}

	entries, err := h.userLLMSvc.SetFallbackChain(ctx, claims.UserID, svcInput)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	responses := make([]UserFallbackChainEntryResponse, 0, len(entries))
	for _, e := range entries {
		responses = append(responses, UserFallbackChainEntryResponse{
			ID:          e.ID,
			Position:    e.Position,
			Provider:    e.Provider,
			Model:       e.Model,
			Temperature: e.Temperature,
			MaxTokens:   e.MaxTokens,
			IsEnabled:   e.IsEnabled,
			CreatedAt:   e.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt:   e.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	return &SetUserFallbackChainOutput{
		Body: struct {
			Chain []UserFallbackChainEntryResponse `json:"chain"`
		}{Chain: responses},
	}, nil
}

// UserModelResponse represents a model available from a provider.
type UserModelResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	IsFree      bool   `json:"is_free"`
	ContextSize int    `json:"context_size,omitempty"`
}

// UserListModelsInput represents the list models request.
type UserListModelsInput struct {
	Provider string `path:"provider" doc:"Provider to list models for (openrouter, anthropic, openai, ollama)"`
}

// UserListModelsOutput represents the list models response.
type UserListModelsOutput struct {
	Body struct {
		Models []UserModelResponse `json:"models"`
	}
}

// ListModels returns available models for a provider.
// Checks provider access before returning models.
func (h *UserLLMHandler) ListModels(ctx context.Context, input *UserListModelsInput) (*UserListModelsOutput, error) {
	claims := mw.GetUserClaims(ctx)
	if claims == nil {
		return nil, huma.Error401Unauthorized("authentication required")
	}

	// Check provider access via registry
	if !h.registry.IsProviderAllowed(input.Provider, claims) {
		missing := h.registry.GetMissingFeatures(input.Provider, claims)
		if len(missing) > 0 {
			return nil, huma.Error403Forbidden(constants.FeatureNotAvailableMessage(missing[0]))
		}
		return nil, huma.Error403Forbidden("provider not available on your current plan")
	}

	models, err := h.adminSvc.ListModels(ctx, input.Provider)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list models: " + err.Error())
	}

	responses := make([]UserModelResponse, 0, len(models))
	for _, m := range models {
		responses = append(responses, UserModelResponse{
			ID:          m.ID,
			Name:        m.Name,
			Description: m.Description,
			IsFree:      m.IsFree,
			ContextSize: m.ContextSize,
		})
	}

	return &UserListModelsOutput{
		Body: struct {
			Models []UserModelResponse `json:"models"`
		}{Models: responses},
	}, nil
}
