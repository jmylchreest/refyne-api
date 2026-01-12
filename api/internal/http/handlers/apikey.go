package handlers

import (
	"context"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/jmylchreest/refyne-api/internal/service"
)

// APIKeyHandler handles API key endpoints.
type APIKeyHandler struct {
	apiKeySvc *service.APIKeyService
}

// NewAPIKeyHandler creates a new API key handler.
func NewAPIKeyHandler(apiKeySvc *service.APIKeyService) *APIKeyHandler {
	return &APIKeyHandler{apiKeySvc: apiKeySvc}
}

// ListKeysOutput represents API key list response.
type ListKeysOutput struct {
	Body struct {
		Keys []APIKeyResponse `json:"keys"`
	}
}

// APIKeyResponse represents an API key in responses.
type APIKeyResponse struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	KeyPrefix  string   `json:"key_prefix"`
	Scopes     []string `json:"scopes"`
	LastUsedAt string   `json:"last_used_at,omitempty"`
	ExpiresAt  string   `json:"expires_at,omitempty"`
	CreatedAt  string   `json:"created_at"`
}

// ListKeys handles listing API keys.
func (h *APIKeyHandler) ListKeys(ctx context.Context, input *struct{}) (*ListKeysOutput, error) {
	userID := getUserID(ctx)
	if userID == "" {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	keys, err := h.apiKeySvc.ListKeys(ctx, userID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list API keys")
	}

	var responses []APIKeyResponse
	for _, key := range keys {
		resp := APIKeyResponse{
			ID:        key.ID,
			Name:      key.Name,
			KeyPrefix: key.KeyPrefix,
			Scopes:    key.Scopes,
			CreatedAt: key.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
		if key.LastUsedAt != nil {
			resp.LastUsedAt = key.LastUsedAt.Format("2006-01-02T15:04:05Z07:00")
		}
		if key.ExpiresAt != nil {
			resp.ExpiresAt = key.ExpiresAt.Format("2006-01-02T15:04:05Z07:00")
		}
		responses = append(responses, resp)
	}

	return &ListKeysOutput{
		Body: struct {
			Keys []APIKeyResponse `json:"keys"`
		}{
			Keys: responses,
		},
	}, nil
}

// CreateKeyInput represents API key creation request.
type CreateKeyInput struct {
	Body struct {
		Name      string   `json:"name" minLength:"1" doc:"Descriptive name for the key"`
		Scopes    []string `json:"scopes,omitempty" doc:"Permitted scopes (extract, crawl, jobs)"`
		ExpiresAt string   `json:"expires_at,omitempty" doc:"Expiration date (RFC3339)"`
	}
}

// CreateKeyOutput represents API key creation response.
type CreateKeyOutput struct {
	Body struct {
		ID        string   `json:"id"`
		Name      string   `json:"name"`
		Key       string   `json:"key" doc:"Full API key - only shown once!"`
		KeyPrefix string   `json:"key_prefix"`
		Scopes    []string `json:"scopes"`
		ExpiresAt string   `json:"expires_at,omitempty"`
		CreatedAt string   `json:"created_at"`
	}
}

// CreateKey handles API key creation.
func (h *APIKeyHandler) CreateKey(ctx context.Context, input *CreateKeyInput) (*CreateKeyOutput, error) {
	userID := getUserID(ctx)
	if userID == "" {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	var expiresAt *time.Time
	if input.Body.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, input.Body.ExpiresAt)
		if err != nil {
			return nil, huma.Error400BadRequest("invalid expires_at format")
		}
		expiresAt = &t
	}

	result, err := h.apiKeySvc.CreateKey(ctx, userID, service.CreateKeyInput{
		Name:      input.Body.Name,
		Scopes:    input.Body.Scopes,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to create API key")
	}

	output := &CreateKeyOutput{
		Body: struct {
			ID        string   `json:"id"`
			Name      string   `json:"name"`
			Key       string   `json:"key" doc:"Full API key - only shown once!"`
			KeyPrefix string   `json:"key_prefix"`
			Scopes    []string `json:"scopes"`
			ExpiresAt string   `json:"expires_at,omitempty"`
			CreatedAt string   `json:"created_at"`
		}{
			ID:        result.ID,
			Name:      result.Name,
			Key:       result.Key,
			KeyPrefix: result.KeyPrefix,
			Scopes:    result.Scopes,
			CreatedAt: result.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		},
	}
	if result.ExpiresAt != nil {
		output.Body.ExpiresAt = result.ExpiresAt.Format("2006-01-02T15:04:05Z07:00")
	}

	return output, nil
}

// RevokeKeyInput represents API key revocation request.
type RevokeKeyInput struct {
	ID string `path:"id" doc:"API key ID to revoke"`
}

// RevokeKeyOutput represents API key revocation response.
type RevokeKeyOutput struct {
	Body struct {
		Success bool `json:"success"`
	}
}

// RevokeKey handles API key revocation.
func (h *APIKeyHandler) RevokeKey(ctx context.Context, input *RevokeKeyInput) (*RevokeKeyOutput, error) {
	userID := getUserID(ctx)
	if userID == "" {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	if err := h.apiKeySvc.RevokeKey(ctx, userID, input.ID); err != nil {
		return nil, huma.Error500InternalServerError("failed to revoke API key")
	}

	return &RevokeKeyOutput{
		Body: struct {
			Success bool `json:"success"`
		}{
			Success: true,
		},
	}, nil
}
