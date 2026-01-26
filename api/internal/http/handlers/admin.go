package handlers

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"github.com/jmylchreest/refyne-api/internal/http/mw"
	"github.com/jmylchreest/refyne-api/internal/models"
	"github.com/jmylchreest/refyne-api/internal/service"
)

// AdminHandler handles admin endpoints.
type AdminHandler struct {
	adminSvc    *service.AdminService
	tierSyncSvc *service.TierSyncService
}

// NewAdminHandler creates a new admin handler.
func NewAdminHandler(adminSvc *service.AdminService, tierSyncSvc *service.TierSyncService) *AdminHandler {
	return &AdminHandler{
		adminSvc:    adminSvc,
		tierSyncSvc: tierSyncSvc,
	}
}

// ServiceKeyInput represents a service key in API requests.
type ServiceKeyInput struct {
	Provider     string `json:"provider" enum:"openrouter,anthropic,openai" doc:"LLM provider name"`
	APIKey       string `json:"api_key,omitempty" doc:"API key for the provider (required for new keys, optional for updates)"`
	DefaultModel string `json:"default_model" doc:"Default model to use"`
	IsEnabled    bool   `json:"is_enabled" doc:"Whether this provider is enabled"`
}

// ServiceKeyResponse represents a service key in API responses.
type ServiceKeyResponse struct {
	Provider     string `json:"provider"`
	DefaultModel string `json:"default_model"`
	IsEnabled    bool   `json:"is_enabled"`
	HasKey       bool   `json:"has_key"` // Never expose actual key
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

// ListServiceKeysOutput represents the list service keys response.
type ListServiceKeysOutput struct {
	Body struct {
		Keys []ServiceKeyResponse `json:"keys"`
	}
}

// ListServiceKeys returns all configured service keys.
func (h *AdminHandler) ListServiceKeys(ctx context.Context, input *struct{}) (*ListServiceKeysOutput, error) {
	claims := mw.GetUserClaims(ctx)
	if claims == nil || !claims.GlobalSuperadmin {
		return nil, huma.Error403Forbidden("superadmin access required")
	}

	keys, err := h.adminSvc.ListServiceKeys(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list service keys: " + err.Error())
	}

	responses := make([]ServiceKeyResponse, 0, len(keys))
	for _, k := range keys {
		responses = append(responses, ServiceKeyResponse{
			Provider:     k.Provider,
			DefaultModel: k.DefaultModel,
			IsEnabled:    k.IsEnabled,
			HasKey:       k.APIKeyEncrypted != "",
			CreatedAt:    k.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt:    k.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	return &ListServiceKeysOutput{
		Body: struct {
			Keys []ServiceKeyResponse `json:"keys"`
		}{Keys: responses},
	}, nil
}

// UpsertServiceKeyInput represents the upsert service key request.
type UpsertServiceKeyInput struct {
	Body ServiceKeyInput
}

// UpsertServiceKeyOutput represents the upsert service key response.
type UpsertServiceKeyOutput struct {
	Body ServiceKeyResponse
}

// UpsertServiceKey creates or updates a service key.
func (h *AdminHandler) UpsertServiceKey(ctx context.Context, input *UpsertServiceKeyInput) (*UpsertServiceKeyOutput, error) {
	claims := mw.GetUserClaims(ctx)
	if claims == nil || !claims.GlobalSuperadmin {
		return nil, huma.Error403Forbidden("superadmin access required")
	}

	key, err := h.adminSvc.UpsertServiceKey(ctx, service.ServiceKeyInput{
		Provider:     input.Body.Provider,
		APIKey:       input.Body.APIKey,
		DefaultModel: input.Body.DefaultModel,
		IsEnabled:    input.Body.IsEnabled,
	})
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to save service key: " + err.Error())
	}

	return &UpsertServiceKeyOutput{
		Body: ServiceKeyResponse{
			Provider:     key.Provider,
			DefaultModel: key.DefaultModel,
			IsEnabled:    key.IsEnabled,
			HasKey:       true,
			CreatedAt:    key.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt:    key.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		},
	}, nil
}

// DeleteServiceKeyInput represents the delete service key request.
type DeleteServiceKeyInput struct {
	Provider string `path:"provider" doc:"Provider to delete"`
}

// DeleteServiceKeyOutput represents the delete service key response.
type DeleteServiceKeyOutput struct {
	Body struct {
		Success bool `json:"success"`
	}
}

// DeleteServiceKey removes a service key.
func (h *AdminHandler) DeleteServiceKey(ctx context.Context, input *DeleteServiceKeyInput) (*DeleteServiceKeyOutput, error) {
	claims := mw.GetUserClaims(ctx)
	if claims == nil || !claims.GlobalSuperadmin {
		return nil, huma.Error403Forbidden("superadmin access required")
	}

	if err := h.adminSvc.DeleteServiceKey(ctx, input.Provider); err != nil {
		return nil, huma.Error500InternalServerError("failed to delete service key: " + err.Error())
	}

	return &DeleteServiceKeyOutput{
		Body: struct {
			Success bool `json:"success"`
		}{Success: true},
	}, nil
}

// FallbackChainEntryResponse represents a fallback chain entry in API responses.
type FallbackChainEntryResponse struct {
	ID          string   `json:"id"`
	Tier        *string  `json:"tier,omitempty"`
	Position    int      `json:"position"`
	Provider    string   `json:"provider"`
	Model       string   `json:"model"`
	Temperature *float64 `json:"temperature,omitempty"`
	MaxTokens   *int     `json:"max_tokens,omitempty"`
	IsEnabled   bool     `json:"is_enabled"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
}

// GetFallbackChainInput represents the get fallback chain request.
type GetFallbackChainInput struct {
	Tier string `query:"tier" doc:"Tier to get chain for (empty for all, 'default' for default chain)"`
}

// GetFallbackChainOutput represents the get fallback chain response.
type GetFallbackChainOutput struct {
	Body struct {
		Chain []FallbackChainEntryResponse `json:"chain"`
		Tiers []string                     `json:"tiers"` // List of tiers with custom chains
	}
}

// GetFallbackChain returns the fallback chain configuration.
func (h *AdminHandler) GetFallbackChain(ctx context.Context, input *GetFallbackChainInput) (*GetFallbackChainOutput, error) {
	claims := mw.GetUserClaims(ctx)
	if claims == nil || !claims.GlobalSuperadmin {
		return nil, huma.Error403Forbidden("superadmin access required")
	}

	var entries []*models.FallbackChainEntry
	var err error

	switch input.Tier {
	case "":
		// Get all chains
		entries, err = h.adminSvc.GetFallbackChain(ctx)
	case "default":
		// Get default chain specifically
		entries, err = h.adminSvc.GetFallbackChainByTier(ctx, nil)
	default:
		// Get tier-specific chain
		entries, err = h.adminSvc.GetFallbackChainByTier(ctx, &input.Tier)
	}
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get fallback chain: " + err.Error())
	}

	// Get list of tiers with custom chains
	tiers, err := h.adminSvc.GetAllTiers(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get tiers: " + err.Error())
	}

	responses := make([]FallbackChainEntryResponse, 0, len(entries))
	for _, e := range entries {
		responses = append(responses, FallbackChainEntryResponse{
			ID:          e.ID,
			Tier:        e.Tier,
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

	return &GetFallbackChainOutput{
		Body: struct {
			Chain []FallbackChainEntryResponse `json:"chain"`
			Tiers []string                     `json:"tiers"`
		}{Chain: responses, Tiers: tiers},
	}, nil
}

// FallbackChainEntryInput represents a fallback chain entry in API requests.
type FallbackChainEntryInput struct {
	Provider    string   `json:"provider" enum:"openrouter,anthropic,openai,ollama" doc:"LLM provider name"`
	Model       string   `json:"model" minLength:"1" doc:"Model identifier"`
	Temperature *float64 `json:"temperature,omitempty" doc:"Temperature setting (0.0-1.0, nil for default)"`
	MaxTokens   *int     `json:"max_tokens,omitempty" doc:"Max output tokens (nil for default)"`
	IsEnabled   bool     `json:"is_enabled" doc:"Whether this entry is enabled"`
}

// SetFallbackChainInput represents the set fallback chain request.
type SetFallbackChainInput struct {
	Body struct {
		Tier  *string                   `json:"tier,omitempty" doc:"Tier to set chain for (null for default chain)"`
		Chain []FallbackChainEntryInput `json:"chain" doc:"Ordered list of provider:model pairs"`
	}
}

// SetFallbackChainOutput represents the set fallback chain response.
type SetFallbackChainOutput struct {
	Body struct {
		Chain []FallbackChainEntryResponse `json:"chain"`
	}
}

// SetFallbackChain replaces the entire fallback chain configuration.
func (h *AdminHandler) SetFallbackChain(ctx context.Context, input *SetFallbackChainInput) (*SetFallbackChainOutput, error) {
	claims := mw.GetUserClaims(ctx)
	if claims == nil || !claims.GlobalSuperadmin {
		return nil, huma.Error403Forbidden("superadmin access required")
	}

	// Convert to service input
	svcInput := service.FallbackChainInput{
		Tier:    input.Body.Tier,
		Entries: make([]service.FallbackChainEntryInput, 0, len(input.Body.Chain)),
	}
	for _, e := range input.Body.Chain {
		svcInput.Entries = append(svcInput.Entries, service.FallbackChainEntryInput{
			Provider:    e.Provider,
			Model:       e.Model,
			Temperature: e.Temperature,
			MaxTokens:   e.MaxTokens,
			IsEnabled:   e.IsEnabled,
		})
	}

	entries, err := h.adminSvc.SetFallbackChain(ctx, svcInput)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	responses := make([]FallbackChainEntryResponse, 0, len(entries))
	for _, e := range entries {
		responses = append(responses, FallbackChainEntryResponse{
			ID:          e.ID,
			Tier:        e.Tier,
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

	return &SetFallbackChainOutput{
		Body: struct {
			Chain []FallbackChainEntryResponse `json:"chain"`
		}{Chain: responses},
	}, nil
}

// ProviderModelResponse represents a model available from a provider.
type ProviderModelResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	IsFree      bool   `json:"is_free"`
	ContextSize int    `json:"context_size,omitempty"`
}

// ListModelsInput represents the list models request.
type ListModelsInput struct {
	Provider string `path:"provider" enum:"openrouter,anthropic,openai,ollama" doc:"Provider to list models for"`
}

// ListModelsOutput represents the list models response.
type ListModelsOutput struct {
	Body struct {
		Models []ProviderModelResponse `json:"models"`
	}
}

// ListModels returns available models for a provider.
func (h *AdminHandler) ListModels(ctx context.Context, input *ListModelsInput) (*ListModelsOutput, error) {
	claims := mw.GetUserClaims(ctx)
	if claims == nil || !claims.GlobalSuperadmin {
		return nil, huma.Error403Forbidden("superadmin access required")
	}

	models, err := h.adminSvc.ListModels(ctx, input.Provider)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list models: " + err.Error())
	}

	responses := make([]ProviderModelResponse, 0, len(models))
	for _, m := range models {
		responses = append(responses, ProviderModelResponse{
			ID:          m.ID,
			Name:        m.Name,
			Description: m.Description,
			IsFree:      m.IsFree,
			ContextSize: m.ContextSize,
		})
	}

	return &ListModelsOutput{
		Body: struct {
			Models []ProviderModelResponse `json:"models"`
		}{Models: responses},
	}, nil
}

// ModelValidationRequest represents a single model to validate.
type ModelValidationRequest struct {
	Provider string `json:"provider" doc:"Provider name"`
	Model    string `json:"model" doc:"Model identifier"`
}

// ModelValidationResponse represents the validation result for a model.
type ModelValidationResponse struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Status   string `json:"status"` // valid, not_found, deprecated, unknown
	Message  string `json:"message,omitempty"`
}

// ValidateModelsInput represents the validate models request.
type ValidateModelsInput struct {
	Body struct {
		Models []ModelValidationRequest `json:"models" doc:"Models to validate"`
	}
}

// ValidateModelsOutput represents the validate models response.
type ValidateModelsOutput struct {
	Body struct {
		Results []ModelValidationResponse `json:"results"`
	}
}

// ValidateModels checks if the given provider:model pairs exist and are valid.
func (h *AdminHandler) ValidateModels(ctx context.Context, input *ValidateModelsInput) (*ValidateModelsOutput, error) {
	claims := mw.GetUserClaims(ctx)
	if claims == nil || !claims.GlobalSuperadmin {
		return nil, huma.Error403Forbidden("superadmin access required")
	}

	// Convert to service input
	svcInput := service.ValidateModelsInput{
		Models: make([]struct {
			Provider string `json:"provider"`
			Model    string `json:"model"`
		}, 0, len(input.Body.Models)),
	}
	for _, m := range input.Body.Models {
		svcInput.Models = append(svcInput.Models, struct {
			Provider string `json:"provider"`
			Model    string `json:"model"`
		}{
			Provider: m.Provider,
			Model:    m.Model,
		})
	}

	results, err := h.adminSvc.ValidateModels(ctx, svcInput)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to validate models: " + err.Error())
	}

	responses := make([]ModelValidationResponse, 0, len(results))
	for _, r := range results {
		responses = append(responses, ModelValidationResponse{
			Provider: r.Provider,
			Model:    r.Model,
			Status:   string(r.Status),
			Message:  r.Message,
		})
	}

	return &ValidateModelsOutput{
		Body: struct {
			Results []ModelValidationResponse `json:"results"`
		}{Results: responses},
	}, nil
}

// SubscriptionTierResponse represents a subscription tier in API responses.
type SubscriptionTierResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description,omitempty"`
	IsDefault   bool   `json:"is_default"`
}

// ListTiersOutput represents the list tiers response.
type ListTiersOutput struct {
	Body struct {
		Tiers []SubscriptionTierResponse `json:"tiers"`
	}
}

// ListTiers returns all available subscription tiers from Clerk.
func (h *AdminHandler) ListTiers(ctx context.Context, input *struct{}) (*ListTiersOutput, error) {
	claims := mw.GetUserClaims(ctx)
	if claims == nil || !claims.GlobalSuperadmin {
		return nil, huma.Error403Forbidden("superadmin access required")
	}

	tiers, err := h.adminSvc.ListSubscriptionTiers(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list subscription tiers: " + err.Error())
	}

	responses := make([]SubscriptionTierResponse, 0, len(tiers))
	for _, t := range tiers {
		responses = append(responses, SubscriptionTierResponse{
			ID:          t.ID,
			Name:        t.Name,
			Slug:        t.Slug,
			Description: t.Description,
			IsDefault:   t.IsDefault,
		})
	}

	return &ListTiersOutput{
		Body: struct {
			Tiers []SubscriptionTierResponse `json:"tiers"`
		}{Tiers: responses},
	}, nil
}

// TierValidationRequest represents a single tier to validate.
type TierValidationRequest struct {
	TierID string `json:"tier_id" doc:"Tier ID or slug to validate"`
}

// TierValidationResponse represents the validation result for a tier.
type TierValidationResponse struct {
	TierID      string `json:"tier_id"`
	CurrentSlug string `json:"current_slug,omitempty"`
	Status      string `json:"status"` // valid, not_found, unknown
	Message     string `json:"message,omitempty"`
}

// ValidateTiersInput represents the validate tiers request.
type ValidateTiersInput struct {
	Body struct {
		Tiers []TierValidationRequest `json:"tiers" doc:"Tiers to validate"`
	}
}

// ValidateTiersOutput represents the validate tiers response.
type ValidateTiersOutput struct {
	Body struct {
		Results []TierValidationResponse `json:"results"`
	}
}

// ValidateTiers checks if the given tier IDs exist in Clerk.
func (h *AdminHandler) ValidateTiers(ctx context.Context, input *ValidateTiersInput) (*ValidateTiersOutput, error) {
	claims := mw.GetUserClaims(ctx)
	if claims == nil || !claims.GlobalSuperadmin {
		return nil, huma.Error403Forbidden("superadmin access required")
	}

	tierIDs := make([]string, 0, len(input.Body.Tiers))
	for _, t := range input.Body.Tiers {
		tierIDs = append(tierIDs, t.TierID)
	}

	results, err := h.adminSvc.ValidateTiers(ctx, tierIDs)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to validate tiers: " + err.Error())
	}

	responses := make([]TierValidationResponse, 0, len(results))
	for _, r := range results {
		responses = append(responses, TierValidationResponse{
			TierID:      r.TierID,
			CurrentSlug: r.CurrentSlug,
			Status:      r.Status,
			Message:     r.Message,
		})
	}

	return &ValidateTiersOutput{
		Body: struct {
			Results []TierValidationResponse `json:"results"`
		}{Results: responses},
	}, nil
}

// SyncTiersOutput represents the sync tiers response.
type SyncTiersOutput struct {
	Body struct {
		Message string `json:"message" doc:"Sync result message"`
	}
}

// SyncTiers triggers a manual sync of tier metadata from Clerk Commerce.
// This is useful for local development where webhooks can't reach the API.
func (h *AdminHandler) SyncTiers(ctx context.Context, input *struct{}) (*SyncTiersOutput, error) {
	claims := mw.GetUserClaims(ctx)
	if claims == nil || !claims.GlobalSuperadmin {
		return nil, huma.Error403Forbidden("superadmin access required")
	}

	if h.tierSyncSvc == nil {
		return nil, huma.Error500InternalServerError("tier sync service not configured - check CLERK_SECRET_KEY")
	}

	if err := h.tierSyncSvc.SyncFromClerk(ctx); err != nil {
		return nil, huma.Error500InternalServerError("failed to sync tiers: " + err.Error())
	}

	return &SyncTiersOutput{
		Body: struct {
			Message string `json:"message" doc:"Sync result message"`
		}{Message: "tier metadata synced from Clerk Commerce"},
	}, nil
}
