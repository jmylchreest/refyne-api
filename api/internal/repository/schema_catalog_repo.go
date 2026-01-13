package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/jmylchreest/refyne-api/internal/models"
)

// SQLiteSchemaCatalogRepository implements SchemaCatalogRepository for SQLite/libsql.
type SQLiteSchemaCatalogRepository struct {
	db *sql.DB
}

// NewSQLiteSchemaCatalogRepository creates a new SQLite schema catalog repository.
func NewSQLiteSchemaCatalogRepository(db *sql.DB) *SQLiteSchemaCatalogRepository {
	return &SQLiteSchemaCatalogRepository{db: db}
}

// Create creates a new schema catalog entry.
func (r *SQLiteSchemaCatalogRepository) Create(ctx context.Context, schema *models.SchemaCatalog) error {
	now := time.Now()
	if schema.ID == "" {
		schema.ID = ulid.Make().String()
	}
	schema.CreatedAt = now
	schema.UpdatedAt = now

	tagsJSON, err := json.Marshal(schema.Tags)
	if err != nil {
		tagsJSON = []byte("[]")
	}

	isPlatform := 0
	if schema.IsPlatform {
		isPlatform = 1
	}

	_, err = r.db.ExecContext(ctx, `
		INSERT INTO schema_catalog (
			id, organization_id, user_id, name, description, category,
			schema_yaml, visibility, is_platform, tags, usage_count,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		schema.ID,
		schema.OrganizationID,
		schema.UserID,
		schema.Name,
		schema.Description,
		schema.Category,
		schema.SchemaYAML,
		schema.Visibility,
		isPlatform,
		string(tagsJSON),
		schema.UsageCount,
		now.Format(time.RFC3339),
		now.Format(time.RFC3339),
	)

	return err
}

// GetByID retrieves a schema by ID.
func (r *SQLiteSchemaCatalogRepository) GetByID(ctx context.Context, id string) (*models.SchemaCatalog, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, organization_id, user_id, name, description, category,
			   schema_yaml, visibility, is_platform, tags, usage_count,
			   created_at, updated_at
		FROM schema_catalog
		WHERE id = ?
	`, id)

	return r.scanSchema(row)
}

// Update updates an existing schema.
func (r *SQLiteSchemaCatalogRepository) Update(ctx context.Context, schema *models.SchemaCatalog) error {
	schema.UpdatedAt = time.Now()

	tagsJSON, err := json.Marshal(schema.Tags)
	if err != nil {
		tagsJSON = []byte("[]")
	}

	isPlatform := 0
	if schema.IsPlatform {
		isPlatform = 1
	}

	_, err = r.db.ExecContext(ctx, `
		UPDATE schema_catalog SET
			organization_id = ?,
			user_id = ?,
			name = ?,
			description = ?,
			category = ?,
			schema_yaml = ?,
			visibility = ?,
			is_platform = ?,
			tags = ?,
			usage_count = ?,
			updated_at = ?
		WHERE id = ?
	`,
		schema.OrganizationID,
		schema.UserID,
		schema.Name,
		schema.Description,
		schema.Category,
		schema.SchemaYAML,
		schema.Visibility,
		isPlatform,
		string(tagsJSON),
		schema.UsageCount,
		schema.UpdatedAt.Format(time.RFC3339),
		schema.ID,
	)

	return err
}

// Delete removes a schema by ID.
func (r *SQLiteSchemaCatalogRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM schema_catalog WHERE id = ?`, id)
	return err
}

// ListForUser returns schemas visible to a user (their own + platform + public).
func (r *SQLiteSchemaCatalogRepository) ListForUser(ctx context.Context, userID string, orgID *string, includePublic bool) ([]*models.SchemaCatalog, error) {
	var query string
	var args []interface{}

	if includePublic {
		if orgID != nil {
			query = `
				SELECT id, organization_id, user_id, name, description, category,
					   schema_yaml, visibility, is_platform, tags, usage_count,
					   created_at, updated_at
				FROM schema_catalog
				WHERE visibility = 'platform'
				   OR visibility = 'public'
				   OR user_id = ?
				   OR organization_id = ?
				ORDER BY is_platform DESC, usage_count DESC, name
			`
			args = []interface{}{userID, *orgID}
		} else {
			query = `
				SELECT id, organization_id, user_id, name, description, category,
					   schema_yaml, visibility, is_platform, tags, usage_count,
					   created_at, updated_at
				FROM schema_catalog
				WHERE visibility = 'platform'
				   OR visibility = 'public'
				   OR user_id = ?
				ORDER BY is_platform DESC, usage_count DESC, name
			`
			args = []interface{}{userID}
		}
	} else {
		if orgID != nil {
			query = `
				SELECT id, organization_id, user_id, name, description, category,
					   schema_yaml, visibility, is_platform, tags, usage_count,
					   created_at, updated_at
				FROM schema_catalog
				WHERE visibility = 'platform'
				   OR user_id = ?
				   OR organization_id = ?
				ORDER BY is_platform DESC, usage_count DESC, name
			`
			args = []interface{}{userID, *orgID}
		} else {
			query = `
				SELECT id, organization_id, user_id, name, description, category,
					   schema_yaml, visibility, is_platform, tags, usage_count,
					   created_at, updated_at
				FROM schema_catalog
				WHERE visibility = 'platform'
				   OR user_id = ?
				ORDER BY is_platform DESC, usage_count DESC, name
			`
			args = []interface{}{userID}
		}
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanSchemas(rows)
}

// ListPlatform returns all platform schemas.
func (r *SQLiteSchemaCatalogRepository) ListPlatform(ctx context.Context) ([]*models.SchemaCatalog, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, organization_id, user_id, name, description, category,
			   schema_yaml, visibility, is_platform, tags, usage_count,
			   created_at, updated_at
		FROM schema_catalog
		WHERE is_platform = 1
		ORDER BY category, name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanSchemas(rows)
}

// ListByCategory returns schemas in a category.
func (r *SQLiteSchemaCatalogRepository) ListByCategory(ctx context.Context, category string) ([]*models.SchemaCatalog, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, organization_id, user_id, name, description, category,
			   schema_yaml, visibility, is_platform, tags, usage_count,
			   created_at, updated_at
		FROM schema_catalog
		WHERE category = ? AND (visibility = 'platform' OR visibility = 'public')
		ORDER BY usage_count DESC, name
	`, category)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanSchemas(rows)
}

// ListAll returns all schemas (for admin).
func (r *SQLiteSchemaCatalogRepository) ListAll(ctx context.Context) ([]*models.SchemaCatalog, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, organization_id, user_id, name, description, category,
			   schema_yaml, visibility, is_platform, tags, usage_count,
			   created_at, updated_at
		FROM schema_catalog
		ORDER BY is_platform DESC, visibility, category, name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanSchemas(rows)
}

// IncrementUsage increments the usage count for a schema.
func (r *SQLiteSchemaCatalogRepository) IncrementUsage(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE schema_catalog SET usage_count = usage_count + 1 WHERE id = ?
	`, id)
	return err
}

// scanSchema scans a single row into a SchemaCatalog.
func (r *SQLiteSchemaCatalogRepository) scanSchema(row *sql.Row) (*models.SchemaCatalog, error) {
	var schema models.SchemaCatalog
	var orgID, userID sql.NullString
	var description, category sql.NullString
	var tagsJSON string
	var isPlatform int
	var createdAt, updatedAt string

	err := row.Scan(
		&schema.ID,
		&orgID,
		&userID,
		&schema.Name,
		&description,
		&category,
		&schema.SchemaYAML,
		&schema.Visibility,
		&isPlatform,
		&tagsJSON,
		&schema.UsageCount,
		&createdAt,
		&updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if orgID.Valid {
		schema.OrganizationID = &orgID.String
	}
	if userID.Valid {
		schema.UserID = &userID.String
	}
	if description.Valid {
		schema.Description = description.String
	}
	if category.Valid {
		schema.Category = category.String
	}
	schema.IsPlatform = isPlatform == 1

	if tagsJSON != "" {
		json.Unmarshal([]byte(tagsJSON), &schema.Tags)
	}

	schema.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	schema.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

	return &schema, nil
}

// scanSchemas scans multiple rows into SchemaCatalog slice.
func (r *SQLiteSchemaCatalogRepository) scanSchemas(rows *sql.Rows) ([]*models.SchemaCatalog, error) {
	var schemas []*models.SchemaCatalog

	for rows.Next() {
		var schema models.SchemaCatalog
		var orgID, userID sql.NullString
		var description, category sql.NullString
		var tagsJSON string
		var isPlatform int
		var createdAt, updatedAt string

		err := rows.Scan(
			&schema.ID,
			&orgID,
			&userID,
			&schema.Name,
			&description,
			&category,
			&schema.SchemaYAML,
			&schema.Visibility,
			&isPlatform,
			&tagsJSON,
			&schema.UsageCount,
			&createdAt,
			&updatedAt,
		)
		if err != nil {
			return nil, err
		}

		if orgID.Valid {
			schema.OrganizationID = &orgID.String
		}
		if userID.Valid {
			schema.UserID = &userID.String
		}
		if description.Valid {
			schema.Description = description.String
		}
		if category.Valid {
			schema.Category = category.String
		}
		schema.IsPlatform = isPlatform == 1

		if tagsJSON != "" {
			json.Unmarshal([]byte(tagsJSON), &schema.Tags)
		}

		schema.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		schema.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

		schemas = append(schemas, &schema)
	}

	return schemas, rows.Err()
}
