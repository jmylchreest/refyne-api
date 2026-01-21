# Extraction Unification & S3 Storage Refactor Plan

## Goals

1. **Shared post-extraction logic** - URL resolution and result processing used by both single-page extract and crawl
2. **Unified result storage** - Both extract and crawl store results in S3, not DB
3. **Slim `job_results` table** - Metadata only, no `DataJSON`
4. **Simplified SSE** - Stream status/errors only, not full data

## Phase 1: Shared Post-Extraction Processing

### 1.1 Create shared result processor in `extraction_service.go`

```go
// ExtractionResult is the common result format for all extraction activities
type ExtractionResult struct {
    URL               string
    Data              any           // Already processed (URLs resolved)
    TokenUsage        TokenUsage
    FetchDurationMs   int
    ExtractDurationMs int
    GenerationID      string
    Provider          string
    Model             string
    Error             error
    ErrorCategory     string
}

// processExtractionResult applies common post-processing to raw extraction results.
// This is the single place where URL resolution and any other post-processing happens.
func (s *ExtractionService) processExtractionResult(rawData any, pageURL string) any {
    // 1. Resolve relative URLs to absolute
    resolved := ResolveRelativeURLs(rawData, pageURL)

    // 2. Future: any other post-processing (sanitization, validation, etc.)

    return resolved
}
```

### 1.2 Update `handleSuccessfulExtraction` to use it

```go
func (s *ExtractionService) handleSuccessfulExtraction(...) (*ExtractOutput, error) {
    // ... billing logic ...

    // Use shared post-processing
    processedData := s.processExtractionResult(result.Data, result.URL)

    return &ExtractOutput{
        Data: processedData,
        // ...
    }
}
```

### 1.3 Update `CrawlWithCallback` to process results before passing to callback

```go
// In the results loop:
for result := range results {
    // Process the result using shared logic
    processedData := s.processExtractionResult(result.Data, result.URL)

    pageResult := PageResult{
        URL:  result.URL,
        Data: processedData,  // Already resolved
        // ...
    }

    if callbacks.OnResult != nil {
        callbacks.OnResult(pageResult)
    }
}
```

### 1.4 Remove URL resolution from worker

```go
// worker.go - remove this:
// resolvedData := service.ResolveRelativeURLs(pageResult.Data, pageResult.URL)

// Just use pageResult.Data directly (already processed by service)
```

## Phase 2: S3 Storage for Crawl Results

### 2.1 Update worker to accumulate results in memory

```go
func (w *Worker) processCrawlJob(ctx context.Context, job *models.Job) {
    // Accumulate results for S3 storage
    var allResults []service.JobResultData

    resultCallback := func(pageResult service.PageResult) error {
        // Save metadata only to job_results (for progress tracking)
        jobResult := &models.JobResult{
            JobID:       job.ID,
            URL:         pageResult.URL,
            CrawlStatus: status,
            // NO DataJSON - just metadata
            ErrorMessage:      pageResult.Error,
            TokenUsageInput:   pageResult.TokenUsageInput,
            TokenUsageOutput:  pageResult.TokenUsageOutput,
            FetchDurationMs:   pageResult.FetchDurationMs,
            ExtractDurationMs: pageResult.ExtractDurationMs,
        }
        w.jobResultRepo.Create(ctx, jobResult)

        // Accumulate for S3
        if pageResult.Data != nil {
            dataJSON, _ := json.Marshal(pageResult.Data)
            allResults = append(allResults, service.JobResultData{
                ID:   pageResult.URL,
                URL:  pageResult.URL,
                Data: dataJSON,
            })
        }
        return nil
    }

    // ... run crawl ...

    // On completion, save all results to S3
    if len(allResults) > 0 {
        jobResults := &service.JobResults{
            JobID:   job.ID,
            UserID:  job.UserID,
            Results: allResults,
        }
        w.storageSvc.StoreJobResults(ctx, jobResults)
    }
}
```

### 2.2 Update `job_results` model - remove DataJSON

```go
// models/models.go
type JobResult struct {
    ID                string       `json:"id"`
    JobID             string       `json:"job_id"`
    URL               string       `json:"url"`
    ParentURL         *string      `json:"parent_url,omitempty"`
    Depth             int          `json:"depth"`
    CrawlStatus       CrawlStatus  `json:"crawl_status"`
    // DataJSON removed - now in S3
    ErrorMessage      string       `json:"error_message,omitempty"`
    ErrorDetails      string       `json:"error_details,omitempty"`
    ErrorCategory     string       `json:"error_category,omitempty"`
    // ... rest unchanged
}
```

### 2.3 Database migration

```sql
-- Migration: Remove data_json from job_results
ALTER TABLE job_results DROP COLUMN data_json;
```

### 2.4 Update `GetJobResults` to always fetch from S3

```go
// job_service.go
func (s *JobService) GetJobResults(ctx context.Context, userID, jobID string) ([]*models.JobResult, error) {
    job, err := s.GetJob(ctx, userID, jobID)
    if err != nil || job == nil {
        return nil, err
    }

    // Both extract and crawl now use S3
    return s.getJobResultsFromStorage(ctx, job)
}
```

## Phase 3: Simplify SSE

### 3.1 Update SSE handler to not include data

```go
// handlers/jobs.go
for _, result := range results {
    event := map[string]any{
        "id":     result.ID,
        "url":    result.URL,
        "status": string(result.CrawlStatus),
        // NO "data" field - client fetches on completion
    }
    ResultInfo{
        ErrorMessage:  result.ErrorMessage,
        ErrorCategory: result.ErrorCategory,
        // ...
    }.ApplyToMap(event)
    sendSSEEvent(w, flusher, "result", event)
}
```

### 3.2 Update SSE complete event to signal results are ready

```go
sendSSEEvent(w, flusher, "complete", map[string]any{
    "job_id":       job.ID,
    "status":       string(job.Status),
    "page_count":   job.PageCount,
    "results_url":  fmt.Sprintf("/api/v1/jobs/%s/results", job.ID),  // Where to fetch full results
})
```

## Phase 4: Memory Considerations for Large Crawls

### 4.1 Streaming to S3 for very large crawls

For crawls with many pages, accumulating all results in memory may not be ideal. Options:

**Option A: Batch writes to S3**
- Write results to S3 in batches (e.g., every 10 pages)
- Final aggregation on completion

**Option B: Append to S3 object**
- Use S3 multipart upload to append results as they come in

**Option C: Temporary file**
- Write results to temp file during crawl
- Upload to S3 on completion

Recommendation: Start with in-memory accumulation (Option A with batch writes if needed), optimize later if memory becomes an issue.

## File Changes Summary

| File | Changes |
|------|---------|
| `internal/service/extraction_service.go` | Add `processExtractionResult()`, update `handleSuccessfulExtraction`, update `CrawlWithCallback` |
| `internal/worker/worker.go` | Remove URL resolution, accumulate results for S3, save to S3 on completion |
| `internal/models/models.go` | Remove `DataJSON` from `JobResult` |
| `internal/repository/job_repo.go` | Remove `data_json` from queries |
| `internal/service/job_service.go` | Update `GetJobResults` to always use S3 |
| `internal/http/handlers/jobs.go` | Remove `data` from SSE events |
| `internal/database/migrations/` | New migration to drop `data_json` column |

## Testing Plan

1. **Unit tests**: `processExtractionResult` with various URL formats
2. **Integration tests**:
   - Single-page extract returns resolved URLs
   - Crawl returns resolved URLs
   - SSE streams metadata only
   - Results fetched from S3 correctly
3. **Manual testing**:
   - Crawl job with 10+ pages
   - Verify DB storage is minimal
   - Verify S3 contains full results

## Migration Strategy

1. Deploy Phase 1 first (shared processing) - no DB changes needed
2. Deploy Phase 2+3 together with migration
3. Old jobs with `DataJSON` will still work (column nullable)
4. New jobs use S3 only

## Rollback Plan

- Keep `data_json` column nullable initially
- Feature flag for S3 storage vs DB storage
- Can revert to DB storage if S3 issues
