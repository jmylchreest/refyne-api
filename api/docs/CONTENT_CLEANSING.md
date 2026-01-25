# Content Cleansing Approaches

This document outlines HTML content cleansing strategies for the Refyne API, particularly for the analyzer and extraction services.

## Available Cleaners

The API supports two cleaners:

| Cleaner | Description |
|---------|-------------|
| `noop` | Pass-through cleaner that preserves raw HTML |
| `refyne` | Configurable HTML cleaner with multiple output formats |

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
      "name": "refyne",
      "description": "Configurable HTML cleaner with heuristic-based cleaning",
      "options": [
        {"name": "output", "type": "string", "default": "html", "description": "Output format: html, text, or markdown"},
        {"name": "preset", "type": "string", "default": "default", "description": "Preset: default, minimal, or aggressive"},
        {"name": "include_frontmatter", "type": "boolean", "default": false, "description": "Include YAML frontmatter (markdown only)"},
        {"name": "extract_images", "type": "boolean", "default": true, "description": "Extract images to frontmatter with placeholders"},
        {"name": "extract_headings", "type": "boolean", "default": true, "description": "Include heading structure in frontmatter"},
        {"name": "remove_selectors", "type": "array", "default": [], "description": "CSS selectors to remove"},
        {"name": "keep_selectors", "type": "array", "default": [], "description": "CSS selectors to always keep"}
      ]
    }
  ],
  "defaultExtractionChain": [{"name": "refyne", "options": {"output": "markdown", "include_frontmatter": true}}],
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
  "cleaner_chain": [
    {
      "name": "refyne",
      "options": {
        "output": "markdown",
        "preset": "aggressive",
        "keep_selectors": [".product-details", ".price"]
      }
    }
  ]
}
```

**Crawl endpoint:**
```json
POST /api/v1/crawl
{
  "url": "https://example.com/products",
  "schema": {"name": "string", "price": "number"},
  "cleaner_chain": [
    {
      "name": "refyne",
      "options": {
        "output": "markdown",
        "include_frontmatter": true,
        "extract_images": true
      }
    }
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
    {
      name: 'refyne',
      options: {
        output: 'markdown',
        preset: 'aggressive',
        keep_selectors: ['.product-details'],
      },
    },
  ],
});
```

## Cleaner Chain Pattern

The refyne library uses a **chain builder pattern** where multiple cleaners can be composed:

```go
// Chain example (though single refyne cleaner usually suffices)
cleanerChain := cleaner.NewChain(
    refynecleaner.New(&refynecleaner.Config{
        Output: refynecleaner.OutputMarkdown,
        IncludeFrontmatter: true,
    }),
    cleaner.NewNoop(), // Additional processing if needed
)
```

Available cleaners:
- `cleaner.NewNoop()` - Pass-through, no cleaning (raw HTML)
- `refynecleaner.New(config)` - Configurable cleaner with html/text/markdown output
- `cleaner.NewChain(cleaners...)` - Compose multiple cleaners

## Default Behavior

### Extraction Service
Uses: `refyne` cleaner with markdown output by default
- Converts HTML to LLM-optimized markdown
- Includes YAML frontmatter with metadata
- Images replaced with `{{IMG_001}}` placeholders
- Links and structure preserved
- Override with `cleaner_chain` parameter for different behavior

### Analyzer Service
Uses: `noop` cleaner by default (raw HTML)
- Preserves maximum detail for comprehensive analysis
- Best for understanding page structure

## Refyne Cleaner Configuration

### Output Formats

| Format | Use Case | Token Reduction |
|--------|----------|-----------------|
| `html` | Downstream HTML processing | 50-70% |
| `text` | Maximum compression, raw text only | 70-85% |
| `markdown` | LLM consumption with structure preserved | 60-80% |

### Presets

| Preset | Description |
|--------|-------------|
| `default` | Safe for all content types, removes scripts/styles/hidden elements |
| `minimal` | Only removes scripts and styles |
| `aggressive` | Enables link density and short text heuristics |

### Custom Selectors

```json
{
  "cleaner_chain": [
    {
      "name": "refyne",
      "options": {
        "output": "markdown",
        "remove_selectors": [".sidebar", "nav", "footer", ".advertisement"],
        "keep_selectors": [".product-details", "#main-content"]
      }
    }
  ]
}
```

## Token Reduction Comparison

Example results from a typical e-commerce product page (~500KB input):

| Configuration | Input Tokens | Reduction |
|---------------|--------------|-----------|
| No cleaning (noop) | ~125,000 | 0% |
| refyne (html output) | ~50,000 | 60% |
| refyne (markdown output) | ~35,000 | 72% |
| refyne (markdown + frontmatter) | ~30,000 | 76% |
| refyne (aggressive + custom selectors) | ~15,000 | 88% |

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

## Cleaner Factory Pattern

The API uses a factory pattern to create cleaners from configuration:

```go
// In cleaner_factory.go
factory := NewCleanerFactory()
chain, err := factory.CreateChainWithDefault(inputChain, DefaultExtractionCleanerChain)
```

**Default chains:**
- `DefaultExtractionCleanerChain = [{name: "refyne", options: {output: "markdown", include_frontmatter: true}}]`
- `DefaultAnalyzerCleanerChain = [{name: "noop"}]`

## Analyzer Auto Mini-Crawl

The analyzer automatically fetches 1-2 sample detail pages to generate better combined schemas:

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

- [Refyne Cleaner Documentation](https://github.com/jmylchreest/refyne/blob/main/pkg/cleaner/refyne/README.md)
