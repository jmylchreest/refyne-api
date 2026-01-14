package models

import (
	"time"
)

// SchemaVisibility defines the visibility levels for schemas.
type SchemaVisibility string

const (
	SchemaVisibilityPlatform SchemaVisibility = "platform" // Admin-managed, visible to all
	SchemaVisibilityPublic   SchemaVisibility = "public"   // User-created, visible to all
	SchemaVisibilityPrivate  SchemaVisibility = "private"  // User-created, visible only to owner/org
)

// SchemaCategory defines common schema categories.
type SchemaCategory string

const (
	SchemaCategoryEcommerce  SchemaCategory = "ecommerce"
	SchemaCategoryRecipes    SchemaCategory = "recipes"
	SchemaCategoryRealEstate SchemaCategory = "realestate"
	SchemaCategoryJobs       SchemaCategory = "jobs"
	SchemaCategoryNews       SchemaCategory = "news"
	SchemaCategoryEvents     SchemaCategory = "events"
	SchemaCategoryCustom     SchemaCategory = "custom"
)

// SchemaCatalog represents a schema in the catalog.
type SchemaCatalog struct {
	ID             string           `json:"id"`
	OrganizationID *string          `json:"organization_id,omitempty"` // Clerk org ID (NULL for platform schemas)
	UserID         *string          `json:"user_id,omitempty"`         // Creator user ID (NULL for platform schemas)
	Name           string           `json:"name"`
	Description    string           `json:"description,omitempty"`
	Category       string           `json:"category,omitempty"` // e.g., 'ecommerce', 'recipes', 'realestate'
	SchemaYAML     string           `json:"schema_yaml"`        // YAML schema content
	Visibility     SchemaVisibility `json:"visibility"`         // 'platform', 'public', 'private'
	IsPlatform     bool             `json:"is_platform"`        // True for admin-managed schemas
	Tags           []string         `json:"tags,omitempty"`
	UsageCount     int              `json:"usage_count"`
	CreatedAt      time.Time        `json:"created_at"`
	UpdatedAt      time.Time        `json:"updated_at"`
}

// FetchMode represents the content fetching strategy.
type FetchMode string

const (
	FetchModeAuto    FetchMode = "auto"    // Let the system decide
	FetchModeStatic  FetchMode = "static"  // Static HTML fetching (Colly)
	FetchModeDynamic FetchMode = "dynamic" // JavaScript rendering (headless Chrome)
)

// SavedSiteCrawlOptions represents saved crawl configuration for a site.
type SavedSiteCrawlOptions struct {
	FollowSelector string `json:"follow_selector,omitempty"` // CSS selector for links to follow
	FollowPattern  string `json:"follow_pattern,omitempty"`  // Regex pattern for URLs to filter
	MaxPages       int    `json:"max_pages,omitempty"`       // Max pages (0 = no limit)
	MaxDepth       int    `json:"max_depth,omitempty"`       // Max crawl depth
	UseSitemap     bool   `json:"use_sitemap,omitempty"`     // Discover URLs from sitemap.xml
}

// SavedSite represents a user's saved site configuration.
type SavedSite struct {
	ID              string                 `json:"id"`
	UserID          string                 `json:"user_id"`
	OrganizationID  *string                `json:"organization_id,omitempty"` // Clerk org ID for team sharing
	URL             string                 `json:"url"`
	Domain          string                 `json:"domain"` // Extracted domain for grouping
	Name            string                 `json:"name,omitempty"`
	AnalysisResult  *AnalysisResult        `json:"analysis_result,omitempty"`
	DefaultSchemaID *string                `json:"default_schema_id,omitempty"`
	CrawlOptions    *SavedSiteCrawlOptions `json:"crawl_options,omitempty"` // Saved crawl options
	FetchMode       FetchMode              `json:"fetch_mode"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
}

// PageType represents the detected type of a web page.
type PageType string

const (
	PageTypeListing   PageType = "listing"   // List of items (search results, product grid)
	PageTypeDetail    PageType = "detail"    // Single item detail page
	PageTypeArticle   PageType = "article"   // Blog post, news article
	PageTypeProduct   PageType = "product"   // E-commerce product page
	PageTypeRecipe    PageType = "recipe"    // Recipe page
	PageTypeCompany   PageType = "company"   // Company overview/about page
	PageTypeService   PageType = "service"   // Services or offerings page
	PageTypeTeam      PageType = "team"      // Team members/leadership page
	PageTypeContact   PageType = "contact"   // Contact information page
	PageTypePortfolio PageType = "portfolio" // Portfolio/case studies page
	PageTypeUnknown   PageType = "unknown"
)

// AnalysisResult contains the output from analyzing a URL.
type AnalysisResult struct {
	SiteSummary          string            `json:"site_summary"`
	PageType             PageType          `json:"page_type"`
	DetectedElements     []DetectedElement `json:"detected_elements"`
	SuggestedSchema      string            `json:"suggested_schema"` // YAML
	FollowPatterns       []FollowPattern   `json:"follow_patterns"`
	SampleLinks          []string          `json:"sample_links"`
	RecommendedFetchMode FetchMode         `json:"recommended_fetch_mode"`
}

// DetectedElement represents a data element detected on the page.
type DetectedElement struct {
	Name        string  `json:"name"`
	Type        string  `json:"type"` // text, number, url, image, date, etc.
	Count       FlexInt `json:"count,omitempty"`
	Description string  `json:"description"`
}

// FollowPattern represents a URL/selector pattern for crawling.
type FollowPattern struct {
	Pattern     string   `json:"pattern"`
	Description string   `json:"description"`
	SampleURLs  []string `json:"sample_urls,omitempty"`
}
