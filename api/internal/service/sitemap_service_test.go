package service

import (
	"context"
	"encoding/xml"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ========================================
// SitemapService Tests
// ========================================

// ----------------------------------------
// Constructor Tests
// ----------------------------------------

func TestNewSitemapService(t *testing.T) {
	logger := slog.Default()

	svc := NewSitemapService(logger)
	if svc == nil {
		t.Fatal("expected service, got nil")
	}
	if svc.logger != logger {
		t.Error("logger not set correctly")
	}
	if svc.client == nil {
		t.Error("expected HTTP client to be set")
	}
}

// ----------------------------------------
// matchesPattern Tests
// ----------------------------------------

func TestMatchesPattern(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		pattern  string
		expected bool
	}{
		{
			name:     "exact match",
			url:      "https://example.com/products/item",
			pattern:  "/products/",
			expected: true,
		},
		{
			name:     "no match",
			url:      "https://example.com/blog/post",
			pattern:  "/products/",
			expected: false,
		},
		{
			name:     "multi-line pattern first matches",
			url:      "https://example.com/products/item",
			pattern:  "/products/\n/blog/",
			expected: true,
		},
		{
			name:     "multi-line pattern second matches",
			url:      "https://example.com/blog/post",
			pattern:  "/products/\n/blog/",
			expected: true,
		},
		{
			name:     "multi-line pattern no match",
			url:      "https://example.com/about",
			pattern:  "/products/\n/blog/",
			expected: false,
		},
		{
			name:     "empty pattern",
			url:      "https://example.com/anything",
			pattern:  "",
			expected: false,
		},
		{
			name:     "pattern with only whitespace",
			url:      "https://example.com/anything",
			pattern:  "  \n  \n  ",
			expected: false,
		},
		{
			name:     "pattern with whitespace around values",
			url:      "https://example.com/products/item",
			pattern:  "  /products/  \n  /blog/  ",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchesPattern(tt.url, tt.pattern)
			if result != tt.expected {
				t.Errorf("matchesPattern(%q, %q) = %v, want %v", tt.url, tt.pattern, result, tt.expected)
			}
		})
	}
}

// ----------------------------------------
// filterURLs Tests
// ----------------------------------------

func TestSitemapService_FilterURLs(t *testing.T) {
	svc := NewSitemapService(slog.Default())

	tests := []struct {
		name        string
		urls        []SitemapURL
		pattern     string
		expectedLen int
	}{
		{
			name: "no filter",
			urls: []SitemapURL{
				{Loc: "https://example.com/page1"},
				{Loc: "https://example.com/page2"},
				{Loc: "https://example.com/page3"},
			},
			pattern:     "",
			expectedLen: 3,
		},
		{
			name: "filter matches some",
			urls: []SitemapURL{
				{Loc: "https://example.com/products/1"},
				{Loc: "https://example.com/blog/1"},
				{Loc: "https://example.com/products/2"},
			},
			pattern:     "/products/",
			expectedLen: 2,
		},
		{
			name: "filter matches none",
			urls: []SitemapURL{
				{Loc: "https://example.com/blog/1"},
				{Loc: "https://example.com/blog/2"},
			},
			pattern:     "/products/",
			expectedLen: 0,
		},
		{
			name: "skips empty URLs",
			urls: []SitemapURL{
				{Loc: "https://example.com/page1"},
				{Loc: ""},
				{Loc: "https://example.com/page2"},
			},
			pattern:     "",
			expectedLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := svc.filterURLs(tt.urls, tt.pattern)
			if len(result) != tt.expectedLen {
				t.Errorf("filterURLs() returned %d URLs, want %d", len(result), tt.expectedLen)
			}
		})
	}
}

// ----------------------------------------
// Struct Tests
// ----------------------------------------

func TestSitemapURL_Fields(t *testing.T) {
	url := SitemapURL{
		Loc:        "https://example.com/page",
		LastMod:    "2024-01-15",
		ChangeFreq: "weekly",
		Priority:   0.8,
	}

	if url.Loc != "https://example.com/page" {
		t.Errorf("Loc = %q, want %q", url.Loc, "https://example.com/page")
	}
	if url.LastMod != "2024-01-15" {
		t.Errorf("LastMod = %q, want %q", url.LastMod, "2024-01-15")
	}
	if url.ChangeFreq != "weekly" {
		t.Errorf("ChangeFreq = %q, want %q", url.ChangeFreq, "weekly")
	}
	if url.Priority != 0.8 {
		t.Errorf("Priority = %f, want 0.8", url.Priority)
	}
}

func TestSitemap_XMLParsing(t *testing.T) {
	xmlData := `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url>
    <loc>https://example.com/page1</loc>
    <lastmod>2024-01-15</lastmod>
    <changefreq>weekly</changefreq>
    <priority>0.8</priority>
  </url>
  <url>
    <loc>https://example.com/page2</loc>
  </url>
</urlset>`

	var sitemap Sitemap
	err := xml.Unmarshal([]byte(xmlData), &sitemap)
	if err != nil {
		t.Fatalf("failed to unmarshal sitemap: %v", err)
	}

	if len(sitemap.URLs) != 2 {
		t.Errorf("expected 2 URLs, got %d", len(sitemap.URLs))
	}

	if sitemap.URLs[0].Loc != "https://example.com/page1" {
		t.Errorf("URL[0].Loc = %q, want %q", sitemap.URLs[0].Loc, "https://example.com/page1")
	}
	if sitemap.URLs[0].LastMod != "2024-01-15" {
		t.Errorf("URL[0].LastMod = %q, want %q", sitemap.URLs[0].LastMod, "2024-01-15")
	}
	if sitemap.URLs[0].ChangeFreq != "weekly" {
		t.Errorf("URL[0].ChangeFreq = %q, want %q", sitemap.URLs[0].ChangeFreq, "weekly")
	}
	if sitemap.URLs[0].Priority != 0.8 {
		t.Errorf("URL[0].Priority = %f, want 0.8", sitemap.URLs[0].Priority)
	}

	if sitemap.URLs[1].Loc != "https://example.com/page2" {
		t.Errorf("URL[1].Loc = %q, want %q", sitemap.URLs[1].Loc, "https://example.com/page2")
	}
}

func TestSitemapIndex_XMLParsing(t *testing.T) {
	xmlData := `<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <sitemap>
    <loc>https://example.com/sitemap-products.xml</loc>
    <lastmod>2024-01-15</lastmod>
  </sitemap>
  <sitemap>
    <loc>https://example.com/sitemap-blog.xml</loc>
  </sitemap>
</sitemapindex>`

	var index SitemapIndex
	err := xml.Unmarshal([]byte(xmlData), &index)
	if err != nil {
		t.Fatalf("failed to unmarshal sitemap index: %v", err)
	}

	if len(index.Sitemaps) != 2 {
		t.Errorf("expected 2 sitemaps, got %d", len(index.Sitemaps))
	}

	if index.Sitemaps[0].Loc != "https://example.com/sitemap-products.xml" {
		t.Errorf("Sitemaps[0].Loc = %q, want %q", index.Sitemaps[0].Loc, "https://example.com/sitemap-products.xml")
	}
	if index.Sitemaps[0].LastMod != "2024-01-15" {
		t.Errorf("Sitemaps[0].LastMod = %q, want %q", index.Sitemaps[0].LastMod, "2024-01-15")
	}

	if index.Sitemaps[1].Loc != "https://example.com/sitemap-blog.xml" {
		t.Errorf("Sitemaps[1].Loc = %q, want %q", index.Sitemaps[1].Loc, "https://example.com/sitemap-blog.xml")
	}
}

func TestSitemapEntry_Fields(t *testing.T) {
	entry := SitemapEntry{
		Loc:     "https://example.com/sitemap-1.xml",
		LastMod: "2024-01-15",
	}

	if entry.Loc != "https://example.com/sitemap-1.xml" {
		t.Errorf("Loc = %q, want %q", entry.Loc, "https://example.com/sitemap-1.xml")
	}
	if entry.LastMod != "2024-01-15" {
		t.Errorf("LastMod = %q, want %q", entry.LastMod, "2024-01-15")
	}
}

// ----------------------------------------
// HTTP Integration Tests (with mock server)
// ----------------------------------------

func TestSitemapService_DiscoverURLsFromSitemap(t *testing.T) {
	sitemapXML := `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>https://example.com/products/1</loc></url>
  <url><loc>https://example.com/products/2</loc></url>
  <url><loc>https://example.com/blog/1</loc></url>
</urlset>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sitemap.xml" {
			w.Header().Set("Content-Type", "application/xml")
			w.Write([]byte(sitemapXML))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	svc := NewSitemapService(slog.Default())
	ctx := context.Background()

	// Test without filter
	urls, err := svc.DiscoverURLsFromSitemap(ctx, server.URL+"/page", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(urls) != 3 {
		t.Errorf("expected 3 URLs, got %d", len(urls))
	}

	// Test with filter
	urls, err = svc.DiscoverURLsFromSitemap(ctx, server.URL+"/page", "/products/")
	if err != nil {
		t.Fatalf("unexpected error with filter: %v", err)
	}
	if len(urls) != 2 {
		t.Errorf("expected 2 filtered URLs, got %d", len(urls))
	}
}

func TestSitemapService_DiscoverURLsFromSitemap_Index(t *testing.T) {
	productsXML := `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>https://example.com/products/1</loc></url>
  <url><loc>https://example.com/products/2</loc></url>
</urlset>`

	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		switch r.URL.Path {
		case "/sitemap.xml":
			// Build index XML with actual server URL
			indexXML := `<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <sitemap><loc>` + serverURL + `/sitemap-products.xml</loc></sitemap>
</sitemapindex>`
			w.Write([]byte(indexXML))
		case "/sitemap-products.xml":
			w.Write([]byte(productsXML))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	svc := NewSitemapService(slog.Default())
	ctx := context.Background()

	urls, err := svc.DiscoverURLsFromSitemap(ctx, server.URL+"/page", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(urls) != 2 {
		t.Errorf("expected 2 URLs from index, got %d", len(urls))
	}
}

func TestSitemapService_DiscoverURLsFromSitemap_InvalidBaseURL(t *testing.T) {
	svc := NewSitemapService(slog.Default())
	ctx := context.Background()

	_, err := svc.DiscoverURLsFromSitemap(ctx, "://invalid", "")
	if err == nil {
		t.Fatal("expected error for invalid base URL")
	}
}

func TestSitemapService_DiscoverURLsFromSitemap_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	svc := NewSitemapService(slog.Default())
	ctx := context.Background()

	_, err := svc.DiscoverURLsFromSitemap(ctx, server.URL+"/page", "")
	if err == nil {
		t.Fatal("expected error for 404")
	}
}

func TestSitemapService_TrySitemapDiscovery(t *testing.T) {
	sitemapXML := `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>https://example.com/page1</loc></url>
</urlset>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sitemap.xml" {
			w.Header().Set("Content-Type", "application/xml")
			w.Write([]byte(sitemapXML))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	svc := NewSitemapService(slog.Default())
	ctx := context.Background()

	urls, ok := svc.TrySitemapDiscovery(ctx, server.URL+"/page", "")
	if !ok {
		t.Error("expected discovery to succeed")
	}
	if len(urls) != 1 {
		t.Errorf("expected 1 URL, got %d", len(urls))
	}
}

func TestSitemapService_TrySitemapDiscovery_Fails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	svc := NewSitemapService(slog.Default())
	ctx := context.Background()

	urls, ok := svc.TrySitemapDiscovery(ctx, server.URL+"/page", "")
	if ok {
		t.Error("expected discovery to fail")
	}
	if urls != nil {
		t.Error("expected nil URLs on failure")
	}
}

func TestSitemapService_TrySitemapDiscovery_EmptyResult(t *testing.T) {
	sitemapXML := `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
</urlset>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sitemap.xml" {
			w.Header().Set("Content-Type", "application/xml")
			w.Write([]byte(sitemapXML))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	svc := NewSitemapService(slog.Default())
	ctx := context.Background()

	_, ok := svc.TrySitemapDiscovery(ctx, server.URL+"/page", "")
	if ok {
		t.Error("expected discovery to return false for empty sitemap")
	}
}

func TestSitemapService_GetSitemapURLs(t *testing.T) {
	sitemapXML := `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url>
    <loc>https://example.com/page1</loc>
    <lastmod>2024-01-15</lastmod>
  </url>
  <url>
    <loc>https://example.com/page2</loc>
  </url>
  <url>
    <loc>https://example.com/page3</loc>
  </url>
</urlset>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sitemap.xml" {
			w.Header().Set("Content-Type", "application/xml")
			w.Write([]byte(sitemapXML))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	svc := NewSitemapService(slog.Default())
	ctx := context.Background()

	// Without limit
	urls, err := svc.GetSitemapURLs(ctx, server.URL+"/page", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(urls) != 3 {
		t.Errorf("expected 3 URLs, got %d", len(urls))
	}

	// With limit
	urls, err = svc.GetSitemapURLs(ctx, server.URL+"/page", 2)
	if err != nil {
		t.Fatalf("unexpected error with limit: %v", err)
	}
	if len(urls) != 2 {
		t.Errorf("expected 2 URLs with limit, got %d", len(urls))
	}
}

func TestSitemapService_GetSitemapURLs_InvalidURL(t *testing.T) {
	svc := NewSitemapService(slog.Default())
	ctx := context.Background()

	_, err := svc.GetSitemapURLs(ctx, "://invalid", 10)
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestSitemapService_GetSitemapURLs_InvalidXML(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte("not valid xml"))
	}))
	defer server.Close()

	svc := NewSitemapService(slog.Default())
	ctx := context.Background()

	_, err := svc.GetSitemapURLs(ctx, server.URL+"/page", 10)
	if err == nil {
		t.Fatal("expected error for invalid XML")
	}
}
