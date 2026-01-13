package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/jmylchreest/refyne-api/internal/models"
)

// SQLiteSavedSitesRepository implements SavedSitesRepository for SQLite/libsql.
type SQLiteSavedSitesRepository struct {
	db *sql.DB
}

// NewSQLiteSavedSitesRepository creates a new SQLite saved sites repository.
func NewSQLiteSavedSitesRepository(db *sql.DB) *SQLiteSavedSitesRepository {
	return &SQLiteSavedSitesRepository{db: db}
}

// Create creates a new saved site entry.
func (r *SQLiteSavedSitesRepository) Create(ctx context.Context, site *models.SavedSite) error {
	now := time.Now()
	if site.ID == "" {
		site.ID = ulid.Make().String()
	}
	site.CreatedAt = now
	site.UpdatedAt = now

	analysisJSON, err := json.Marshal(site.AnalysisResult)
	if err != nil {
		analysisJSON = nil
	}

	crawlOptionsJSON, err := json.Marshal(site.CrawlOptions)
	if err != nil {
		crawlOptionsJSON = nil
	}

	_, err = r.db.ExecContext(ctx, `
		INSERT INTO saved_sites (
			id, user_id, organization_id, url, domain, name,
			analysis_result, default_schema_id, crawl_options, fetch_mode,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		site.ID,
		site.UserID,
		site.OrganizationID,
		site.URL,
		site.Domain,
		site.Name,
		string(analysisJSON),
		site.DefaultSchemaID,
		string(crawlOptionsJSON),
		site.FetchMode,
		now.Format(time.RFC3339),
		now.Format(time.RFC3339),
	)

	return err
}

// GetByID retrieves a saved site by ID.
func (r *SQLiteSavedSitesRepository) GetByID(ctx context.Context, id string) (*models.SavedSite, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, user_id, organization_id, url, domain, name,
			   analysis_result, default_schema_id, crawl_options, fetch_mode,
			   created_at, updated_at
		FROM saved_sites
		WHERE id = ?
	`, id)

	return r.scanSite(row)
}

// Update updates an existing saved site.
func (r *SQLiteSavedSitesRepository) Update(ctx context.Context, site *models.SavedSite) error {
	site.UpdatedAt = time.Now()

	analysisJSON, err := json.Marshal(site.AnalysisResult)
	if err != nil {
		analysisJSON = nil
	}

	crawlOptionsJSON, err := json.Marshal(site.CrawlOptions)
	if err != nil {
		crawlOptionsJSON = nil
	}

	_, err = r.db.ExecContext(ctx, `
		UPDATE saved_sites SET
			organization_id = ?,
			url = ?,
			domain = ?,
			name = ?,
			analysis_result = ?,
			default_schema_id = ?,
			crawl_options = ?,
			fetch_mode = ?,
			updated_at = ?
		WHERE id = ?
	`,
		site.OrganizationID,
		site.URL,
		site.Domain,
		site.Name,
		string(analysisJSON),
		site.DefaultSchemaID,
		string(crawlOptionsJSON),
		site.FetchMode,
		site.UpdatedAt.Format(time.RFC3339),
		site.ID,
	)

	return err
}

// Delete removes a saved site by ID.
func (r *SQLiteSavedSitesRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM saved_sites WHERE id = ?`, id)
	return err
}

// ListByUserID returns saved sites for a user.
func (r *SQLiteSavedSitesRepository) ListByUserID(ctx context.Context, userID string) ([]*models.SavedSite, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, organization_id, url, domain, name,
			   analysis_result, default_schema_id, crawl_options, fetch_mode,
			   created_at, updated_at
		FROM saved_sites
		WHERE user_id = ?
		ORDER BY updated_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanSites(rows)
}

// ListByOrganizationID returns saved sites for an organization.
func (r *SQLiteSavedSitesRepository) ListByOrganizationID(ctx context.Context, orgID string) ([]*models.SavedSite, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, organization_id, url, domain, name,
			   analysis_result, default_schema_id, crawl_options, fetch_mode,
			   created_at, updated_at
		FROM saved_sites
		WHERE organization_id = ?
		ORDER BY updated_at DESC
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanSites(rows)
}

// ListByDomain returns saved sites for a domain.
func (r *SQLiteSavedSitesRepository) ListByDomain(ctx context.Context, userID, domain string) ([]*models.SavedSite, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, organization_id, url, domain, name,
			   analysis_result, default_schema_id, crawl_options, fetch_mode,
			   created_at, updated_at
		FROM saved_sites
		WHERE user_id = ? AND domain = ?
		ORDER BY updated_at DESC
	`, userID, domain)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanSites(rows)
}

// scanSite scans a single row into a SavedSite.
func (r *SQLiteSavedSitesRepository) scanSite(row *sql.Row) (*models.SavedSite, error) {
	var site models.SavedSite
	var orgID, name, defaultSchemaID sql.NullString
	var analysisJSON, crawlOptionsJSON sql.NullString
	var createdAt, updatedAt string

	err := row.Scan(
		&site.ID,
		&site.UserID,
		&orgID,
		&site.URL,
		&site.Domain,
		&name,
		&analysisJSON,
		&defaultSchemaID,
		&crawlOptionsJSON,
		&site.FetchMode,
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
		site.OrganizationID = &orgID.String
	}
	if name.Valid {
		site.Name = name.String
	}
	if defaultSchemaID.Valid {
		site.DefaultSchemaID = &defaultSchemaID.String
	}

	if analysisJSON.Valid && analysisJSON.String != "" {
		var result models.AnalysisResult
		if err := json.Unmarshal([]byte(analysisJSON.String), &result); err == nil {
			site.AnalysisResult = &result
		}
	}

	if crawlOptionsJSON.Valid && crawlOptionsJSON.String != "" {
		var opts models.SavedSiteCrawlOptions
		if err := json.Unmarshal([]byte(crawlOptionsJSON.String), &opts); err == nil {
			site.CrawlOptions = &opts
		}
	}

	site.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	site.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

	return &site, nil
}

// scanSites scans multiple rows into SavedSite slice.
func (r *SQLiteSavedSitesRepository) scanSites(rows *sql.Rows) ([]*models.SavedSite, error) {
	var sites []*models.SavedSite

	for rows.Next() {
		var site models.SavedSite
		var orgID, name, defaultSchemaID sql.NullString
		var analysisJSON, crawlOptionsJSON sql.NullString
		var createdAt, updatedAt string

		err := rows.Scan(
			&site.ID,
			&site.UserID,
			&orgID,
			&site.URL,
			&site.Domain,
			&name,
			&analysisJSON,
			&defaultSchemaID,
			&crawlOptionsJSON,
			&site.FetchMode,
			&createdAt,
			&updatedAt,
		)
		if err != nil {
			return nil, err
		}

		if orgID.Valid {
			site.OrganizationID = &orgID.String
		}
		if name.Valid {
			site.Name = name.String
		}
		if defaultSchemaID.Valid {
			site.DefaultSchemaID = &defaultSchemaID.String
		}

		if analysisJSON.Valid && analysisJSON.String != "" {
			var result models.AnalysisResult
			if err := json.Unmarshal([]byte(analysisJSON.String), &result); err == nil {
				site.AnalysisResult = &result
			}
		}

		if crawlOptionsJSON.Valid && crawlOptionsJSON.String != "" {
			var opts models.SavedSiteCrawlOptions
			if err := json.Unmarshal([]byte(crawlOptionsJSON.String), &opts); err == nil {
				site.CrawlOptions = &opts
			}
		}

		site.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		site.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

		sites = append(sites, &site)
	}

	return sites, rows.Err()
}
