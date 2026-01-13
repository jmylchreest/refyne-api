package handlers

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"github.com/jmylchreest/refyne-api/internal/http/mw"
	"github.com/jmylchreest/refyne-api/internal/models"
	"github.com/jmylchreest/refyne-api/internal/repository"
)

// SchemaCatalogHandler handles schema catalog endpoints.
type SchemaCatalogHandler struct {
	repo repository.SchemaCatalogRepository
}

// NewSchemaCatalogHandler creates a new schema catalog handler.
func NewSchemaCatalogHandler(repo repository.SchemaCatalogRepository) *SchemaCatalogHandler {
	return &SchemaCatalogHandler{repo: repo}
}

// SchemaOutput represents a schema in API responses.
type SchemaOutput struct {
	ID             string   `json:"id" doc:"Schema ID"`
	OrganizationID *string  `json:"organization_id,omitempty" doc:"Organization ID (Clerk)"`
	UserID         *string  `json:"user_id,omitempty" doc:"Creator user ID"`
	Name           string   `json:"name" doc:"Schema name"`
	Description    string   `json:"description,omitempty" doc:"Schema description"`
	Category       string   `json:"category,omitempty" doc:"Schema category"`
	SchemaYAML     string   `json:"schema_yaml" doc:"YAML schema content"`
	Visibility     string   `json:"visibility" doc:"Visibility: platform, public, private"`
	IsPlatform     bool     `json:"is_platform" doc:"True for admin-managed schemas"`
	Tags           []string `json:"tags,omitempty" doc:"Schema tags"`
	UsageCount     int      `json:"usage_count" doc:"Number of times schema has been used"`
	CreatedAt      string   `json:"created_at" doc:"Creation timestamp"`
	UpdatedAt      string   `json:"updated_at" doc:"Last update timestamp"`
}

// ListSchemasInput represents list schemas request.
type ListSchemasInput struct {
	Category      string `query:"category" doc:"Filter by category"`
	IncludePublic bool   `query:"include_public" default:"true" doc:"Include public schemas"`
}

// ListSchemasOutput represents list schemas response.
type ListSchemasOutput struct {
	Body struct {
		Schemas []SchemaOutput `json:"schemas" doc:"List of schemas"`
	}
}

// ListSchemas returns schemas visible to the user.
func (h *SchemaCatalogHandler) ListSchemas(ctx context.Context, input *ListSchemasInput) (*ListSchemasOutput, error) {
	claims := getUserClaims(ctx)
	if claims == nil {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	var schemas []*models.SchemaCatalog
	var err error

	if input.Category != "" {
		schemas, err = h.repo.ListByCategory(ctx, input.Category)
	} else {
		schemas, err = h.repo.ListForUser(ctx, claims.UserID, nil, input.IncludePublic)
	}

	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list schemas: " + err.Error())
	}

	output := &ListSchemasOutput{}
	for _, s := range schemas {
		output.Body.Schemas = append(output.Body.Schemas, schemaToOutput(s))
	}

	return output, nil
}

// GetSchemaInput represents get schema request.
type GetSchemaInput struct {
	ID string `path:"id" doc:"Schema ID"`
}

// GetSchemaOutput represents get schema response.
type GetSchemaOutput struct {
	Body SchemaOutput
}

// GetSchema retrieves a single schema by ID.
func (h *SchemaCatalogHandler) GetSchema(ctx context.Context, input *GetSchemaInput) (*GetSchemaOutput, error) {
	claims := getUserClaims(ctx)
	if claims == nil {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	schema, err := h.repo.GetByID(ctx, input.ID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get schema: " + err.Error())
	}
	if schema == nil {
		return nil, huma.Error404NotFound("schema not found")
	}

	// Check access
	if !canAccessSchema(claims, schema) {
		return nil, huma.Error403Forbidden("access denied")
	}

	return &GetSchemaOutput{Body: schemaToOutput(schema)}, nil
}

// CreateSchemaInput represents create schema request.
type CreateSchemaInput struct {
	Body struct {
		Name        string   `json:"name" minLength:"1" doc:"Schema name"`
		Description string   `json:"description,omitempty" doc:"Schema description"`
		Category    string   `json:"category,omitempty" doc:"Schema category"`
		SchemaYAML  string   `json:"schema_yaml" minLength:"1" doc:"YAML schema content"`
		Visibility  string   `json:"visibility" enum:"private,public" default:"private" doc:"Schema visibility"`
		Tags        []string `json:"tags,omitempty" doc:"Schema tags"`
	}
}

// CreateSchemaOutput represents create schema response.
type CreateSchemaOutput struct {
	Body SchemaOutput
}

// CreateSchema creates a new schema.
func (h *SchemaCatalogHandler) CreateSchema(ctx context.Context, input *CreateSchemaInput) (*CreateSchemaOutput, error) {
	claims := getUserClaims(ctx)
	if claims == nil {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	schema := &models.SchemaCatalog{
		UserID:      &claims.UserID,
		Name:        input.Body.Name,
		Description: input.Body.Description,
		Category:    input.Body.Category,
		SchemaYAML:  input.Body.SchemaYAML,
		Visibility:  models.SchemaVisibility(input.Body.Visibility),
		IsPlatform:  false,
		Tags:        input.Body.Tags,
	}

	if err := h.repo.Create(ctx, schema); err != nil {
		return nil, huma.Error500InternalServerError("failed to create schema: " + err.Error())
	}

	return &CreateSchemaOutput{Body: schemaToOutput(schema)}, nil
}

// UpdateSchemaInput represents update schema request.
type UpdateSchemaInput struct {
	ID   string `path:"id" doc:"Schema ID"`
	Body struct {
		Name        string   `json:"name,omitempty" doc:"Schema name"`
		Description string   `json:"description,omitempty" doc:"Schema description"`
		Category    string   `json:"category,omitempty" doc:"Schema category"`
		SchemaYAML  string   `json:"schema_yaml,omitempty" doc:"YAML schema content"`
		Visibility  string   `json:"visibility,omitempty" enum:"private,public" doc:"Schema visibility"`
		Tags        []string `json:"tags,omitempty" doc:"Schema tags"`
	}
}

// UpdateSchemaOutput represents update schema response.
type UpdateSchemaOutput struct {
	Body SchemaOutput
}

// UpdateSchema updates an existing schema.
func (h *SchemaCatalogHandler) UpdateSchema(ctx context.Context, input *UpdateSchemaInput) (*UpdateSchemaOutput, error) {
	claims := getUserClaims(ctx)
	if claims == nil {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	schema, err := h.repo.GetByID(ctx, input.ID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get schema: " + err.Error())
	}
	if schema == nil {
		return nil, huma.Error404NotFound("schema not found")
	}

	// Check ownership (only owner can update, unless superadmin)
	if !canModifySchema(claims, schema) {
		return nil, huma.Error403Forbidden("access denied")
	}

	// Update fields
	if input.Body.Name != "" {
		schema.Name = input.Body.Name
	}
	if input.Body.Description != "" {
		schema.Description = input.Body.Description
	}
	if input.Body.Category != "" {
		schema.Category = input.Body.Category
	}
	if input.Body.SchemaYAML != "" {
		schema.SchemaYAML = input.Body.SchemaYAML
	}
	if input.Body.Visibility != "" {
		// Non-admins cannot set platform visibility
		if input.Body.Visibility == "platform" && !claims.GlobalSuperadmin {
			return nil, huma.Error403Forbidden("only superadmins can set platform visibility")
		}
		schema.Visibility = models.SchemaVisibility(input.Body.Visibility)
	}
	if input.Body.Tags != nil {
		schema.Tags = input.Body.Tags
	}

	if err := h.repo.Update(ctx, schema); err != nil {
		return nil, huma.Error500InternalServerError("failed to update schema: " + err.Error())
	}

	return &UpdateSchemaOutput{Body: schemaToOutput(schema)}, nil
}

// DeleteSchemaInput represents delete schema request.
type DeleteSchemaInput struct {
	ID string `path:"id" doc:"Schema ID"`
}

// DeleteSchemaOutput represents delete schema response.
type DeleteSchemaOutput struct {
	Body struct {
		Success bool `json:"success" doc:"Whether deletion was successful"`
	}
}

// DeleteSchema deletes a schema.
func (h *SchemaCatalogHandler) DeleteSchema(ctx context.Context, input *DeleteSchemaInput) (*DeleteSchemaOutput, error) {
	claims := getUserClaims(ctx)
	if claims == nil {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	schema, err := h.repo.GetByID(ctx, input.ID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get schema: " + err.Error())
	}
	if schema == nil {
		return nil, huma.Error404NotFound("schema not found")
	}

	// Check ownership
	if !canModifySchema(claims, schema) {
		return nil, huma.Error403Forbidden("access denied")
	}

	if err := h.repo.Delete(ctx, input.ID); err != nil {
		return nil, huma.Error500InternalServerError("failed to delete schema: " + err.Error())
	}

	return &DeleteSchemaOutput{Body: struct {
		Success bool `json:"success" doc:"Whether deletion was successful"`
	}{Success: true}}, nil
}

// ListAllSchemasOutput represents list all schemas response (admin).
type ListAllSchemasOutput struct {
	Body struct {
		Schemas []SchemaOutput `json:"schemas" doc:"List of all schemas"`
	}
}

// ListAllSchemas returns all schemas (admin only).
func (h *SchemaCatalogHandler) ListAllSchemas(ctx context.Context, input *struct{}) (*ListAllSchemasOutput, error) {
	claims := getUserClaims(ctx)
	if claims == nil || !claims.GlobalSuperadmin {
		return nil, huma.Error403Forbidden("superadmin access required")
	}

	schemas, err := h.repo.ListAll(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list schemas: " + err.Error())
	}

	output := &ListAllSchemasOutput{}
	for _, s := range schemas {
		output.Body.Schemas = append(output.Body.Schemas, schemaToOutput(s))
	}

	return output, nil
}

// CreatePlatformSchemaInput represents create platform schema request (admin).
type CreatePlatformSchemaInput struct {
	Body struct {
		Name        string   `json:"name" minLength:"1" doc:"Schema name"`
		Description string   `json:"description,omitempty" doc:"Schema description"`
		Category    string   `json:"category" doc:"Schema category"`
		SchemaYAML  string   `json:"schema_yaml" minLength:"1" doc:"YAML schema content"`
		Tags        []string `json:"tags,omitempty" doc:"Schema tags"`
	}
}

// CreatePlatformSchema creates a new platform schema (admin only).
func (h *SchemaCatalogHandler) CreatePlatformSchema(ctx context.Context, input *CreatePlatformSchemaInput) (*CreateSchemaOutput, error) {
	claims := getUserClaims(ctx)
	if claims == nil || !claims.GlobalSuperadmin {
		return nil, huma.Error403Forbidden("superadmin access required")
	}

	schema := &models.SchemaCatalog{
		Name:        input.Body.Name,
		Description: input.Body.Description,
		Category:    input.Body.Category,
		SchemaYAML:  input.Body.SchemaYAML,
		Visibility:  models.SchemaVisibilityPlatform,
		IsPlatform:  true,
		Tags:        input.Body.Tags,
	}

	if err := h.repo.Create(ctx, schema); err != nil {
		return nil, huma.Error500InternalServerError("failed to create schema: " + err.Error())
	}

	return &CreateSchemaOutput{Body: schemaToOutput(schema)}, nil
}

// Helper functions

func schemaToOutput(s *models.SchemaCatalog) SchemaOutput {
	return SchemaOutput{
		ID:             s.ID,
		OrganizationID: s.OrganizationID,
		UserID:         s.UserID,
		Name:           s.Name,
		Description:    s.Description,
		Category:       s.Category,
		SchemaYAML:     s.SchemaYAML,
		Visibility:     string(s.Visibility),
		IsPlatform:     s.IsPlatform,
		Tags:           s.Tags,
		UsageCount:     s.UsageCount,
		CreatedAt:      s.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:      s.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func canAccessSchema(claims *mw.UserClaims, schema *models.SchemaCatalog) bool {
	// Platform and public schemas are accessible to all
	if schema.Visibility == models.SchemaVisibilityPlatform || schema.Visibility == models.SchemaVisibilityPublic {
		return true
	}
	// Owner can always access
	if schema.UserID != nil && *schema.UserID == claims.UserID {
		return true
	}
	// Superadmin can access all
	if claims.GlobalSuperadmin {
		return true
	}
	return false
}

func canModifySchema(claims *mw.UserClaims, schema *models.SchemaCatalog) bool {
	// Superadmin can modify anything
	if claims.GlobalSuperadmin {
		return true
	}
	// Owner can modify their own schemas
	if schema.UserID != nil && *schema.UserID == claims.UserID {
		return true
	}
	return false
}
