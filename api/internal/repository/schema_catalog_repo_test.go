package repository

import (
	"context"
	"testing"

	"github.com/jmylchreest/refyne-api/internal/models"
)

// ========================================
// SchemaCatalogRepository Tests
// ========================================

func TestSchemaCatalogRepository_Create(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	userID := "user-1"
	schema := &models.SchemaCatalog{
		UserID:      &userID,
		Name:        "Test Schema",
		Description: "A test schema for extraction",
		Category:    "ecommerce",
		SchemaYAML:  "type: object\nproperties:\n  title: string",
		Visibility:  "private",
		IsPlatform:  false,
		Tags:        []string{"test", "product"},
		UsageCount:  0,
	}

	err := repos.SchemaCatalog.Create(ctx, schema)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	if schema.ID == "" {
		t.Error("expected ID to be generated")
	}

	// Verify by fetching
	fetched, err := repos.SchemaCatalog.GetByID(ctx, schema.ID)
	if err != nil {
		t.Fatalf("failed to fetch schema: %v", err)
	}
	if fetched == nil {
		t.Fatal("expected schema, got nil")
	}
	if fetched.Name != "Test Schema" {
		t.Errorf("Name = %q, want %q", fetched.Name, "Test Schema")
	}
	if fetched.Description != "A test schema for extraction" {
		t.Errorf("Description = %q, want %q", fetched.Description, "A test schema for extraction")
	}
	if fetched.Category != "ecommerce" {
		t.Errorf("Category = %q, want %q", fetched.Category, "ecommerce")
	}
	if len(fetched.Tags) != 2 {
		t.Errorf("Tags length = %d, want 2", len(fetched.Tags))
	}
}

func TestSchemaCatalogRepository_GetByID_NotFound(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	schema, err := repos.SchemaCatalog.GetByID(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if schema != nil {
		t.Error("expected nil for nonexistent ID")
	}
}

func TestSchemaCatalogRepository_Update(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	userID := "user-update"
	schema := &models.SchemaCatalog{
		UserID:     &userID,
		Name:       "Original Name",
		SchemaYAML: "original yaml",
		Visibility: "private",
	}
	repos.SchemaCatalog.Create(ctx, schema)

	// Update
	schema.Name = "Updated Name"
	schema.Description = "Added description"
	schema.Tags = []string{"new-tag"}
	schema.UsageCount = 5

	err := repos.SchemaCatalog.Update(ctx, schema)
	if err != nil {
		t.Fatalf("failed to update schema: %v", err)
	}

	// Verify
	fetched, _ := repos.SchemaCatalog.GetByID(ctx, schema.ID)
	if fetched.Name != "Updated Name" {
		t.Errorf("Name = %q, want %q", fetched.Name, "Updated Name")
	}
	if fetched.Description != "Added description" {
		t.Errorf("Description = %q, want %q", fetched.Description, "Added description")
	}
	if fetched.UsageCount != 5 {
		t.Errorf("UsageCount = %d, want 5", fetched.UsageCount)
	}
}

func TestSchemaCatalogRepository_Delete(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	userID := "user-delete"
	schema := &models.SchemaCatalog{
		UserID:     &userID,
		Name:       "To Delete",
		SchemaYAML: "yaml",
		Visibility: "private",
	}
	repos.SchemaCatalog.Create(ctx, schema)

	err := repos.SchemaCatalog.Delete(ctx, schema.ID)
	if err != nil {
		t.Fatalf("failed to delete schema: %v", err)
	}

	// Verify deleted
	fetched, _ := repos.SchemaCatalog.GetByID(ctx, schema.ID)
	if fetched != nil {
		t.Error("expected schema to be deleted")
	}
}

func TestSchemaCatalogRepository_ListForUser(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	// Create platform schema
	repos.SchemaCatalog.Create(ctx, &models.SchemaCatalog{
		Name:       "Platform Schema",
		SchemaYAML: "yaml",
		Visibility: "platform",
		IsPlatform: true,
	})

	// Create user's private schema
	userID := "user-list"
	repos.SchemaCatalog.Create(ctx, &models.SchemaCatalog{
		UserID:     &userID,
		Name:       "User Schema",
		SchemaYAML: "yaml",
		Visibility: "private",
	})

	// Create another user's private schema
	otherUserID := "other-user"
	repos.SchemaCatalog.Create(ctx, &models.SchemaCatalog{
		UserID:     &otherUserID,
		Name:       "Other User Schema",
		SchemaYAML: "yaml",
		Visibility: "private",
	})

	// Create public schema
	repos.SchemaCatalog.Create(ctx, &models.SchemaCatalog{
		Name:       "Public Schema",
		SchemaYAML: "yaml",
		Visibility: "public",
	})

	t.Run("include public", func(t *testing.T) {
		schemas, err := repos.SchemaCatalog.ListForUser(ctx, "user-list", nil, true)
		if err != nil {
			t.Fatalf("failed to list schemas: %v", err)
		}

		// Should see platform, user's own, and public
		if len(schemas) != 3 {
			t.Errorf("expected 3 schemas, got %d", len(schemas))
		}
	})

	t.Run("exclude public", func(t *testing.T) {
		schemas, err := repos.SchemaCatalog.ListForUser(ctx, "user-list", nil, false)
		if err != nil {
			t.Fatalf("failed to list schemas: %v", err)
		}

		// Should see platform and user's own
		if len(schemas) != 2 {
			t.Errorf("expected 2 schemas, got %d", len(schemas))
		}
	})
}

func TestSchemaCatalogRepository_ListForUser_WithOrg(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	// Create org schema
	orgID := "org-1"
	repos.SchemaCatalog.Create(ctx, &models.SchemaCatalog{
		OrganizationID: &orgID,
		Name:           "Org Schema",
		SchemaYAML:     "yaml",
		Visibility:     "organization",
	})

	// Create platform schema
	repos.SchemaCatalog.Create(ctx, &models.SchemaCatalog{
		Name:       "Platform",
		SchemaYAML: "yaml",
		Visibility: "platform",
		IsPlatform: true,
	})

	schemas, err := repos.SchemaCatalog.ListForUser(ctx, "user-org", &orgID, false)
	if err != nil {
		t.Fatalf("failed to list schemas: %v", err)
	}

	// Should see platform and org
	if len(schemas) != 2 {
		t.Errorf("expected 2 schemas, got %d", len(schemas))
	}
}

func TestSchemaCatalogRepository_ListPlatform(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	// Create platform schemas
	repos.SchemaCatalog.Create(ctx, &models.SchemaCatalog{
		Name:       "Platform A",
		Category:   "cat-a",
		SchemaYAML: "yaml",
		Visibility: "platform",
		IsPlatform: true,
	})
	repos.SchemaCatalog.Create(ctx, &models.SchemaCatalog{
		Name:       "Platform B",
		Category:   "cat-b",
		SchemaYAML: "yaml",
		Visibility: "platform",
		IsPlatform: true,
	})

	// Create non-platform schema
	userID := "user"
	repos.SchemaCatalog.Create(ctx, &models.SchemaCatalog{
		UserID:     &userID,
		Name:       "User Schema",
		SchemaYAML: "yaml",
		Visibility: "private",
		IsPlatform: false,
	})

	schemas, err := repos.SchemaCatalog.ListPlatform(ctx)
	if err != nil {
		t.Fatalf("failed to list platform schemas: %v", err)
	}

	if len(schemas) != 2 {
		t.Errorf("expected 2 platform schemas, got %d", len(schemas))
	}

	for _, s := range schemas {
		if !s.IsPlatform {
			t.Errorf("got non-platform schema in platform list: %s", s.Name)
		}
	}
}

func TestSchemaCatalogRepository_ListByCategory(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	// Create schemas in different categories
	repos.SchemaCatalog.Create(ctx, &models.SchemaCatalog{
		Name:       "Ecommerce 1",
		Category:   "ecommerce",
		SchemaYAML: "yaml",
		Visibility: "public",
	})
	repos.SchemaCatalog.Create(ctx, &models.SchemaCatalog{
		Name:       "Ecommerce 2",
		Category:   "ecommerce",
		SchemaYAML: "yaml",
		Visibility: "platform",
	})
	repos.SchemaCatalog.Create(ctx, &models.SchemaCatalog{
		Name:       "Real Estate",
		Category:   "realestate",
		SchemaYAML: "yaml",
		Visibility: "public",
	})

	schemas, err := repos.SchemaCatalog.ListByCategory(ctx, "ecommerce")
	if err != nil {
		t.Fatalf("failed to list by category: %v", err)
	}

	if len(schemas) != 2 {
		t.Errorf("expected 2 ecommerce schemas, got %d", len(schemas))
	}

	for _, s := range schemas {
		if s.Category != "ecommerce" {
			t.Errorf("got wrong category: %s", s.Category)
		}
	}
}

func TestSchemaCatalogRepository_ListAll(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	// Create various schemas
	for i := 0; i < 5; i++ {
		userID := "user"
		repos.SchemaCatalog.Create(ctx, &models.SchemaCatalog{
			UserID:     &userID,
			Name:       "Schema",
			SchemaYAML: "yaml",
			Visibility: "private",
		})
	}

	schemas, err := repos.SchemaCatalog.ListAll(ctx)
	if err != nil {
		t.Fatalf("failed to list all: %v", err)
	}

	if len(schemas) != 5 {
		t.Errorf("expected 5 schemas, got %d", len(schemas))
	}
}

func TestSchemaCatalogRepository_IncrementUsage(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	userID := "user"
	schema := &models.SchemaCatalog{
		UserID:     &userID,
		Name:       "Counter Schema",
		SchemaYAML: "yaml",
		Visibility: "private",
		UsageCount: 5,
	}
	repos.SchemaCatalog.Create(ctx, schema)

	err := repos.SchemaCatalog.IncrementUsage(ctx, schema.ID)
	if err != nil {
		t.Fatalf("failed to increment usage: %v", err)
	}

	fetched, _ := repos.SchemaCatalog.GetByID(ctx, schema.ID)
	if fetched.UsageCount != 6 {
		t.Errorf("UsageCount = %d, want 6", fetched.UsageCount)
	}
}
