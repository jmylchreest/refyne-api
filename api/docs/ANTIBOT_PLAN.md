# Anti-Bot Feature Implementation Plan

## Overview

The anti-bot feature enables bypassing Cloudflare and other bot protection systems for web scraping. This is a **Pro-tier only** feature (tier gating already implemented in `middleware/tier.go`).

## Architecture

```
                                    ┌─────────────────────┐
                                    │   FlareSolverr      │
                                    │   Container Pool    │
                                    │  (start with 1)     │
                                    └─────────┬───────────┘
                                              │
┌──────────┐    ┌──────────────┐    ┌────────▼────────┐
│  Client  │───►│  refyne-api  │───►│  Anti-Bot Queue │
│  (Pro)   │    │  /api/v1/*   │    │  (SQLite/Redis) │
└──────────┘    └──────────────┘    └────────┬────────┘
                                              │
                                    ┌─────────▼───────────┐
                                    │   Anti-Bot Worker   │
                                    │   (polls queue)     │
                                    └─────────────────────┘
```

## Components

### 1. FlareSolverr Integration

**Location:** `internal/antibot/flaresolverr/`

Port the existing code from `refyne/cmd/refyne/fetcher/`:
- `flaresolverr.go` - FlareSolverr client with session management
- `stealth.go` - Stealth mode scripts (for chromedp fallback)

**Key Types:**
```go
type FlareSolverr struct {
    baseURL    string
    httpClient *http.Client
    maxTimeout int
}

type FlareSolverSolution struct {
    URL       string
    Status    int
    Response  string  // HTML content
    Cookies   []Cookie
    UserAgent string
}
```

### 2. Anti-Bot Fetcher

**Location:** `internal/antibot/fetcher.go`

Implements `refyne/pkg/fetcher.Fetcher` interface:

```go
type AntiBotFetcher struct {
    flareSolverr *FlareSolverr
    config       Config
}

func (f *AntiBotFetcher) Fetch(ctx context.Context, url string, opts fetcher.Options) (fetcher.Content, error)
func (f *AntiBotFetcher) Close() error
func (f *AntiBotFetcher) Type() string // "antibot"
```

### 3. Queue System

**Option A: SQLite-based (recommended for v1)**

Add table to existing database:

```sql
CREATE TABLE antibot_jobs (
    id TEXT PRIMARY KEY,
    job_id TEXT REFERENCES jobs(id),  -- Parent extraction job
    url TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',  -- pending, processing, completed, failed
    result_html TEXT,
    cookies_json TEXT,
    error_message TEXT,
    attempts INTEGER DEFAULT 0,
    created_at TEXT NOT NULL,
    started_at TEXT,
    completed_at TEXT
);
CREATE INDEX idx_antibot_status ON antibot_jobs(status);
```

**Option B: Redis (for scale)**

Use Redis lists for queue with sorted sets for scheduling.

### 4. Anti-Bot Worker

**Location:** `internal/antibot/worker.go`

Separate from main job worker to isolate anti-bot processing:

```go
type AntiBotWorker struct {
    flareSolverr *FlareSolverr
    repo         *repository.AntiBotRepository
    concurrency  int  // Start with 1
    pollInterval time.Duration
}

func (w *AntiBotWorker) Start(ctx context.Context)
func (w *AntiBotWorker) Stop()
func (w *AntiBotWorker) processJob(job *AntiBotJob) error
```

### 5. Container Orchestration

**Phase 1: Single Container**
- Run FlareSolverr as sidecar container
- Configure via `FLARESOLVERR_URL` environment variable
- Health check endpoint to verify availability

**Phase 2: Auto-scaling (future)**
- Kubernetes HPA based on queue depth
- Container pool manager for dynamic scaling
- Session affinity for domain-specific sessions

## API Integration

### Extraction Service Changes

```go
// In ExtractionService.resolveConfig()
func (s *ExtractionService) getFetcher(ctx context.Context, tier string, opts ExtractOptions) (fetcher.Fetcher, error) {
    if opts.AntiBot && tier != "pro" {
        return nil, NewTierFeatureError("antibot", tier)
    }

    if opts.AntiBot {
        return s.antiBotFetcher, nil
    }

    // Fall back to standard fetchers
    return s.standardFetcher, nil
}
```

### API Schema Updates

```go
type ExtractInput struct {
    URL     string          `json:"url" required:"true"`
    Schema  json.RawMessage `json:"schema" required:"true"`
    AntiBot bool            `json:"anti_bot,omitempty"`  // NEW
    // ...
}
```

## Configuration

```env
# FlareSolverr Configuration
FLARESOLVERR_URL=http://localhost:8191/v1
FLARESOLVERR_TIMEOUT=60000

# Anti-Bot Worker
ANTIBOT_WORKER_CONCURRENCY=1
ANTIBOT_WORKER_POLL_INTERVAL=5s
```

## Error Handling

Map FlareSolverr errors to user-friendly messages:

| FlareSolverr Error | User Message | Retryable |
|-------------------|--------------|-----------|
| Timeout | "Challenge took too long to solve" | Yes |
| CAPTCHA detected | "Site requires human verification" | No |
| Blocked/403 | "Site blocked the request" | Maybe |
| Service unavailable | "Anti-bot service temporarily unavailable" | Yes |

## Billing Considerations

Anti-bot requests are more expensive:
- Higher compute costs (browser instances)
- Longer processing times
- Consider separate pricing tier or multiplier

## Implementation Phases

### Phase 1: Foundation (MVP)
1. Port FlareSolverr client from refyne CLI
2. Add anti-bot job queue table
3. Create anti-bot worker (concurrency=1)
4. Wire up to extraction service
5. Add `anti_bot` flag to extract endpoint

### Phase 2: Reliability
1. Add retry logic with exponential backoff
2. Implement session caching per domain
3. Add health checks and monitoring
4. Error classification and metrics

### Phase 3: Scale
1. Redis queue for horizontal scaling
2. Container pool management
3. Auto-scaling based on queue depth
4. Geographic distribution (optional)

## Testing Strategy

1. **Unit Tests:** Mock FlareSolverr responses
2. **Integration Tests:** Test against real FlareSolverr container
3. **E2E Tests:** Test full extraction with anti-bot enabled

## Security Considerations

- FlareSolverr should be internal-only (not exposed to internet)
- Rate limit anti-bot requests per user
- Log all anti-bot requests for audit
- Consider abuse detection (repeated failures)

## Monitoring

Metrics to track:
- Anti-bot queue depth
- Processing time per request
- Success/failure rates
- FlareSolverr health status
- Challenge types encountered

## Dependencies

- FlareSolverr Docker image: `ghcr.io/flaresolverr/flaresolverr:latest`
- Requires Chrome/Chromium in the container
- Memory: ~512MB per FlareSolverr instance

## Files to Create

```
internal/antibot/
├── config.go           # Configuration
├── fetcher.go          # AntiBotFetcher implementation
├── worker.go           # Queue worker
├── flaresolverr/
│   ├── client.go       # FlareSolverr API client
│   ├── types.go        # Request/Response types
│   └── errors.go       # Error classification
└── repository.go       # Database operations

internal/repository/
└── antibot_repository.go   # Anti-bot job CRUD
```

## Docker Compose (Development)

```yaml
services:
  flaresolverr:
    image: ghcr.io/flaresolverr/flaresolverr:latest
    container_name: flaresolverr
    environment:
      - LOG_LEVEL=info
      - TZ=UTC
    ports:
      - "8191:8191"
    restart: unless-stopped
```
