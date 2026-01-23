# Content Cleansing Approaches

This document outlines HTML content cleansing strategies for the Refyne API, particularly for the analyzer and extraction services.

## API Configuration

Cleaner chains are **configurable per-request** via the API. Clients can specify which cleaners to use and in what order.

### Endpoints

**List Available Cleaners:**
```
GET /api/v1/cleaners
```

Returns available cleaners with their options and default chains:
```json
{
  "cleaners": [
    {
      "name": "noop",
      "description": "Pass-through cleaner that preserves raw HTML"
    },
    {
      "name": "markdown",
      "description": "Converts HTML to Markdown format"
    },
    {
      "name": "trafilatura",
      "description": "Extracts main content using Trafilatura algorithm",
      "options": [
        {"name": "output", "type": "string", "default": "html", "description": "Output format: html or text"},
        {"name": "tables", "type": "boolean", "default": true, "description": "Include tables"},
        {"name": "links", "type": "boolean", "default": true, "description": "Include links"},
        {"name": "images", "type": "boolean", "default": false, "description": "Include images"}
      ]
    },
    {
      "name": "readability",
      "description": "Extracts main content using Mozilla Readability",
      "options": [
        {"name": "output", "type": "string", "default": "html", "description": "Output format: html or text"},
        {"name": "base_url", "type": "string", "default": "", "description": "Base URL for resolving relative links"}
      ]
    }
  ],
  "defaultExtractionChain": [{"name": "markdown"}],
  "defaultAnalysisChain": [{"name": "noop"}]
}
```

### Using Custom Cleaner Chains

**Extract endpoint:**
```json
POST /api/v1/extract
{
  "url": "https://example.com/product",
  "schema": {"name": "string", "price": "number"},
  "cleanerChain": [
    {"name": "trafilatura", "options": {"tables": true, "links": true}},
    {"name": "markdown"}
  ]
}
```

**Crawl endpoint:**
```json
POST /api/v1/crawl
{
  "url": "https://example.com/products",
  "schema": {"name": "string", "price": "number"},
  "cleanerChain": [
    {"name": "readability", "options": {"output": "html"}},
    {"name": "markdown"}
  ]
}
```

### TypeScript SDK

```typescript
// List available cleaners
const cleaners = await refyne.listCleaners();

// Extract with custom cleaner chain
const result = await refyne.extract({
  url: 'https://example.com/product',
  schema: { name: 'string', price: 'number' },
  cleanerChain: [
    { name: 'trafilatura', options: { tables: true, links: true } },
    { name: 'markdown' },
  ],
});
```

## Cleaner Chain Pattern

The refinery library uses a **chain builder pattern** where multiple cleaners can be composed:

```go
// Chain example: Trafilatura -> Markdown
cleanerChain := cleaner.NewChain(
    cleaner.NewTrafilatura(&cleaner.TrafilaturaConfig{
        Output: cleaner.OutputHTML,
        Tables: cleaner.Include,
        Links:  cleaner.Include,
    }),
    cleaner.NewMarkdown(),
)
```

Available cleaners:
- `cleaner.NewNoop()` - Pass-through, no cleaning (raw HTML)
- `cleaner.NewTrafilatura(config)` - Main content extraction (article-focused)
- `cleaner.NewReadability(config)` - Mozilla Readability-based extraction
- `cleaner.NewMarkdown()` - HTML to Markdown conversion
- `cleaner.NewChain(cleaners...)` - Compose multiple cleaners

## Default Behavior

### Extraction Service
Uses: `Markdown` cleaner by default (HTML to Markdown conversion)
- Converts raw HTML to Markdown preserving structure
- Links become `[text](url)`, images become `![alt](src)`
- Tables preserved as Markdown tables
- Override with `cleanerChain` parameter for different behavior

### Analyzer Service
Uses: `Noop` cleaner by default (raw HTML)
- Preserves maximum detail for comprehensive analysis
- Best for understanding page structure

## Problem Statement

Different use cases have conflicting requirements:

| Use Case | Need | Risk |
|----------|------|------|
| **Analysis** | Maximum detail to understand page structure | Context length errors |
| **Extraction** | Clean, focused content for accurate extraction | May lose relevant data |
| **E-commerce/Listings** | Preserve tables, product grids, prices | Article extractors remove these |

## Cleaner Comparison (Tested)

| Cleaner | Token Usage | Extraction Quality | Notes |
|---------|-------------|-------------------|-------|
| **Noop (raw HTML)** | Very high (~32K/page) | Best | Too expensive for production |
| **Markdown only** | High (~28-30K/page) | Good | ~10% reduction, still expensive |
| **Trafilatura -> MD** | Low | Poor for e-commerce | Strips product grids, images |
| **Readability -> MD** | Medium (TBD) | TBD | Worth testing as middle ground |

The challenge: article extractors (Trafilatura, Readability) are designed to find the "main content" and strip "boilerplate". For e-commerce sites, product grids ARE the main content but look like navigation to these algorithms.

## Available In-Process Go Libraries

### 1. go-trafilatura (Currently Used)
**Repo:** https://github.com/markusmobius/go-trafilatura

**Pros:**
- Native Go port of Python trafilatura
- Multiple output formats: HTML, text, JSON
- Configurable table/link/image preservation
- Fallback to readability/dom-distiller

**Cons:**
- Designed for article extraction
- May discard product listings, non-article content
- Text output loses all structure

**Configuration Options:**
```go
// HTML output with tables preserved
trafilatura.Extract(reader, trafilatura.Options{
    Output:        trafilatura.OutputHTML,
    IncludeTables: true,
    IncludeLinks:  true,
    IncludeImages: true,
})
```

### 2. go-readability v2 (IMPLEMENTED)
**Repo:** https://codeberg.org/readeck/go-readability
**Package:** `codeberg.org/readeck/go-readability/v2`

**Pros:**
- Mozilla Readability.js v0.6.0 compatible
- Produces HTML and text output
- Extracts metadata (title, author, published date, etc.)
- Better content preservation than older versions
- Preserves images, links, and tables in main content
- Actively maintained

**Cons:**
- Article-focused (like Trafilatura)
- May strip product grids/listings on e-commerce sites

**Configuration Options:**
```go
readabilityCleaner := cleaner.NewReadability(&cleaner.ReadabilityConfig{
    Output:            cleaner.OutputHTML,  // or OutputText
    MaxElemsToParse:   0,                   // 0 = no limit
    NTopCandidates:    5,                   // candidates to analyze
    CharThreshold:     500,                 // min chars for valid content
    KeepClasses:       false,               // preserve CSS classes
    ClassesToPreserve: []string{},          // specific classes to keep
    BaseURL:           "https://example.com", // for resolving relative URLs
})
```

**Usage in chain:**
```go
// Readability extracts main content, then Markdown converts to clean format
cleanerChain := cleaner.NewChain(
    cleaner.NewReadability(&cleaner.ReadabilityConfig{
        Output: cleaner.OutputHTML,
    }),
    cleaner.NewMarkdown(),
)
```

### 3. GoOse
**Repo:** https://github.com/advancedlogic/GoOse

**Pros:**
- Article/content extraction
- Metadata extraction

**Cons:**
- Article-focused
- Less configurable than trafilatura

## Out-of-Process Options (Future Consideration)

### MinerU-HTML
**Repo:** https://github.com/opendatalab/MinerU-HTML
**Type:** Python with REST API

**Why it's interesting:**
- Uses 0.6B language model for semantic content understanding
- 32% better accuracy than Trafilatura (81.8% vs 63.6% ROUGE-N F1)
- Excellent structure preservation:
  - 90.9% for code blocks
  - 94.0% for tables/formulas
- REST API via FastAPI

**Integration approach:**
- Deploy as sidecar container
- Call `/extract` endpoint with HTML
- Returns clean, LLM-ready content

**Why not now:**
- Adds deployment complexity
- Python dependency
- Latency overhead of HTTP call

## Implemented Strategy

### Cleaner Factory Pattern

The API uses a factory pattern to create cleaners from configuration:

```go
// In cleaner_factory.go
factory := NewCleanerFactory()
chain, err := factory.CreateChainWithDefault(inputChain, DefaultExtractionCleanerChain)
```

**Default chains:**
- `DefaultExtractionCleanerChain = [{name: "markdown"}]`
- `DefaultAnalyzerCleanerChain = [{name: "noop"}]`

### For Analyzer Service

Uses `noop` cleaner by default to preserve maximum HTML detail for page structure analysis.

### For Extraction Service

Uses `markdown` cleaner by default for clean content extraction. This approach:
- Converts raw HTML to Markdown preserving structure
- Links become `[text](url)`, images become `![alt](src)`
- Tables preserved as Markdown tables
- Strikes a balance between token usage and content preservation

### Custom Cleaner Chains

Users can override defaults via the `cleanerChain` parameter. Common patterns:

```go
// For article content: extract main content then convert to markdown
cleanerChain: [
    {name: "trafilatura", options: {tables: true, links: true}},
    {name: "markdown"}
]

// For maximum content preservation (high token usage)
cleanerChain: [
    {name: "noop"}
]

// For minimal token usage (may lose content)
cleanerChain: [
    {name: "trafilatura", options: {output: "text"}}
]
```

**Note on Trafilatura:** While Trafilatura is effective for article extraction, it may strip product grids, form elements, and other non-article content. For e-commerce sites, the default `markdown` cleaner often works better.

## Error Detection

Context length errors typically manifest as:
- `context_length_exceeded` in error message
- `max_tokens` or `token limit` in error
- HTTP 400 with specific error codes from LLM providers

```go
func isContextLengthError(err error) bool {
    errStr := strings.ToLower(err.Error())
    return strings.Contains(errStr, "context_length") ||
           strings.Contains(errStr, "max_tokens") ||
           strings.Contains(errStr, "token limit") ||
           strings.Contains(errStr, "too long") ||
           strings.Contains(errStr, "maximum context")
}
```

## Implementation Priority

1. **DONE:** API-configurable cleaner chains via `cleanerChain` parameter
2. **DONE:** `/api/v1/cleaners` endpoint to list available cleaners
3. **DONE:** Default cleaner chains (markdown for extraction, noop for analysis)
4. **DONE:** Auto mini-crawl for better schema generation
5. **Medium-term:** Evaluate MinerU-HTML for complex sites (out-of-process)
6. **Long-term:** Page-type-aware cleaner selection

## Analyzer Auto Mini-Crawl

The analyzer now automatically fetches 1-2 sample detail pages to generate better combined schemas:

1. **Smart link detection** - Identifies product/detail page links based on URL patterns:
   - `/product/`, `/products/`, `/item/`, `/p/`
   - `/article/`, `/post/`, `/job/`
   - URLs with slugs (hyphenated paths like `/products/raspberry-pi-5`)
   - Paths with numeric IDs

2. **Combined schema generation** - The prompt instructs the LLM to create schemas that work across:
   - Listing pages (partial data - title, url, price)
   - Detail pages (full data - description, specs, images)
   - Only `title` and `url` are marked required for flexible merging

3. **Minimal prompt philosophy** - Less prescriptive rules, more focus on:
   - Clear model types (products, jobs, articles, properties)
   - Show don't tell (examples over rules)
   - Trust the model to understand context

## References

- [go-trafilatura](https://github.com/markusmobius/go-trafilatura)
- [MinerU-HTML](https://github.com/opendatalab/MinerU-HTML)
- [go-readability v2](https://codeberg.org/readeck/go-readability/v2)
- [Trafilatura evaluation](https://trafilatura.readthedocs.io/en/latest/evaluation.html)
