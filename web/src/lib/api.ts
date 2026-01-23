// API client for refyne-api
const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080';

export interface ApiError {
  error: string;
  status: number;
  error_category?: string;  // Error classification (rate_limited, invalid_api_key, provider_error, etc.)
  error_details?: string;   // Full error details (only for BYOK users)
  is_byok?: boolean;        // Whether user's own API key was used
  provider?: string;        // LLM provider (only for BYOK users)
  model?: string;           // LLM model (only for BYOK users)
}

// Token getter function - will be set by the auth hook
let getToken: (() => Promise<string | null>) | null = null;

export function setTokenGetter(getter: () => Promise<string | null>) {
  getToken = getter;
}

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
  requireAuth = true
): Promise<T> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
  };

  if (requireAuth && getToken) {
    const token = await getToken();
    if (token) {
      headers['Authorization'] = `Bearer ${token}`;
    }
  }

  const response = await fetch(`${API_BASE_URL}${path}`, {
    method,
    headers,
    body: body ? JSON.stringify(body) : undefined,
  });

  if (!response.ok) {
    // Provide helpful error messages for common HTTP errors
    const statusMessages: Record<number, string> = {
      400: 'Invalid request - please check your input',
      401: 'Authentication required - please sign in',
      403: 'Access denied - you may not have permission for this action',
      404: 'Resource not found',
      408: 'Request timed out - please try again',
      429: 'Too many requests - please wait and try again',
      500: 'Server error - please try again later',
      502: 'Service temporarily unavailable - the extraction provider may be down',
      503: 'Service unavailable - please try again later',
      504: 'Request timed out - extraction took too long. Try a simpler schema or check if the URL is accessible',
    };

    const error = await response.json().catch(() => ({}));
    const errorMessage = error.error || error.message || statusMessages[response.status] || `Request failed (${response.status})`;
    throw { error: errorMessage, status: response.status } as ApiError;
  }

  return response.json();
}

// Extraction with client-side timeout
// Schema can be an object (structured schema) or string (freeform prompt)
export async function extract(url: string, schema: object | string, llmConfig?: LLMConfigInput) {
  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(), 120000); // 2 minute client timeout

  try {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
    };

    if (getToken) {
      const token = await getToken();
      if (token) {
        headers['Authorization'] = `Bearer ${token}`;
      }
    }

    const response = await fetch(`${API_BASE_URL}/api/v1/extract`, {
      method: 'POST',
      headers,
      body: JSON.stringify({ url, schema, llm_config: llmConfig }),
      signal: controller.signal,
    });

    clearTimeout(timeoutId);

    if (!response.ok) {
      const statusMessages: Record<number, string> = {
        400: 'Invalid request - please check your schema format',
        502: 'Extraction provider unavailable - try a different model in your fallback chain',
        504: 'Extraction timed out - the page may be too complex or the model too slow',
      };
      const errorBody = await response.json().catch(() => ({}));
      // Use detail field from our custom ExtractionError, falling back to other fields
      const errorMessage = errorBody.detail || errorBody.error || errorBody.message || statusMessages[response.status] || `Extraction failed (${response.status})`;
      const apiError: ApiError = {
        error: errorMessage,
        status: response.status,
        error_category: errorBody.error_category,
        error_details: errorBody.error_details,
        is_byok: errorBody.is_byok,
        provider: errorBody.provider,
        model: errorBody.model,
      };
      throw apiError;
    }

    return response.json() as Promise<ExtractResult>;
  } catch (err) {
    clearTimeout(timeoutId);
    if (err instanceof Error && err.name === 'AbortError') {
      throw { error: 'Extraction timed out after 2 minutes. The page may be complex or the LLM provider slow. Try using Crawl mode for long-running extractions.', status: 408 };
    }
    throw err;
  }
}

// Crawl Jobs
export async function createCrawlJob(data: CreateCrawlJobInput) {
  return request<{ job_id: string; status: string; status_url: string }>(
    'POST',
    '/api/v1/crawl',
    data
  );
}

export async function listJobs(limit = 20, offset = 0) {
  return request<{ jobs: Job[] }>('GET', `/api/v1/jobs?limit=${limit}&offset=${offset}`);
}

export async function getJob(id: string) {
  return request<Job>('GET', `/api/v1/jobs/${id}`);
}

export async function getJobDownloadUrl(id: string) {
  return request<{ job_id: string; download_url: string; expires_at: string }>(
    'GET',
    `/api/v1/jobs/${id}/download`
  );
}

export interface JobResultEntry {
  id: string;
  url: string;
  data: unknown;
}

export interface JobResultsResponse {
  job_id: string;
  status: string;
  page_count: number;
  results?: JobResultEntry[];
  merged?: Record<string, unknown>;
}

export type OutputFormat = 'json' | 'jsonl' | 'yaml';

export async function getJobResults(id: string, merge = false, format: OutputFormat = 'json') {
  const params = new URLSearchParams();
  if (merge) params.set('merge', 'true');
  if (format !== 'json') params.set('format', format);
  const query = params.toString() ? `?${params.toString()}` : '';
  return request<JobResultsResponse>('GET', `/api/v1/jobs/${id}/results${query}`);
}

// Get job results as raw text (for JSONL/YAML formats)
export async function getJobResultsRaw(id: string, merge = false, format: OutputFormat = 'json'): Promise<string> {
  const headers: Record<string, string> = {};

  if (getToken) {
    const token = await getToken();
    if (token) {
      headers['Authorization'] = `Bearer ${token}`;
    }
  }

  const params = new URLSearchParams();
  if (merge) params.set('merge', 'true');
  params.set('format', format);
  const query = `?${params.toString()}`;

  const response = await fetch(`${API_BASE_URL}/api/v1/jobs/${id}/results${query}`, {
    method: 'GET',
    headers,
  });

  if (!response.ok) {
    const error = await response.json().catch(() => ({}));
    throw { error: error.error || `Request failed (${response.status})`, status: response.status } as ApiError;
  }

  return response.text();
}

// API Keys
export async function listApiKeys() {
  return request<{ keys: ApiKey[] }>('GET', '/api/v1/keys');
}

export async function createApiKey(name: string, scopes?: string[]) {
  return request<ApiKeyWithSecret>('POST', '/api/v1/keys', { name, scopes });
}

export async function revokeApiKey(id: string) {
  return request<{ success: boolean }>('DELETE', `/api/v1/keys/${id}`);
}

// Usage
export async function getUsage(period = 'month') {
  return request<UsageSummary>('GET', `/api/v1/usage?period=${period}`);
}

// LLM Config
export async function getLLMConfig() {
  return request<LLMConfig>('GET', '/api/v1/llm-config');
}

export async function updateLLMConfig(config: LLMConfigInput) {
  return request<LLMConfig>('PUT', '/api/v1/llm-config', config);
}

// Admin - Service Keys
export async function listServiceKeys() {
  return request<{ keys: ServiceKey[] }>('GET', '/api/v1/admin/service-keys');
}

export async function upsertServiceKey(input: ServiceKeyInput) {
  return request<ServiceKey>('PUT', '/api/v1/admin/service-keys', input);
}

export async function deleteServiceKey(provider: string) {
  return request<{ success: boolean }>('DELETE', `/api/v1/admin/service-keys/${provider}`);
}

// Types
export interface ExtractResult {
  job_id: string;  // Job ID for history/tracking
  data: unknown;
  url: string;
  fetched_at: string;
  input_format: 'schema' | 'prompt';  // How the input was interpreted by the server
  usage: {
    input_tokens: number;
    output_tokens: number;
    cost_credits?: number;
    cost_usd?: number;
    llm_cost_usd?: number;
    is_byok?: boolean;
  };
  metadata: {
    fetch_duration_ms: number;
    extract_duration_ms: number;
    model: string;
    provider: string;
  };
}

export interface Job {
  id: string;
  type: 'extract' | 'crawl' | 'analyze';
  status: 'pending' | 'running' | 'completed' | 'failed' | 'cancelled';
  url: string;
  urls_queued: number;
  page_count: number;
  token_usage_input: number;
  token_usage_output: number;
  cost_usd: number;
  capture_debug: boolean;
  error_message?: string;
  error_category?: string;
  started_at?: string;
  completed_at?: string;
  created_at: string;
}

export interface JobWebhookDelivery {
  id: string;
  webhook_id?: string;
  event_type: string;
  url: string;
  status_code?: number;
  response_time_ms?: number;
  status: 'pending' | 'success' | 'failed' | 'retrying';
  error_message?: string;
  attempt_number: number;
  max_attempts: number;
  created_at: string;
  delivered_at?: string;
}

export async function getJobWebhookDeliveries(jobId: string) {
  return request<{ job_id: string; deliveries: JobWebhookDelivery[] }>(
    'GET',
    `/api/v1/jobs/${jobId}/webhook-deliveries`
  );
}

export interface CreateCrawlJobInput {
  url: string;
  schema: object | string;  // Can be structured schema or freeform prompt
  options?: {
    follow_selector?: string;
    follow_pattern?: string;
    max_depth?: number;
    next_selector?: string;
    max_pages?: number;
    max_urls?: number;
    delay?: string;
    concurrency?: number;
    same_domain_only?: boolean;
    extract_from_seeds?: boolean;
    use_sitemap?: boolean;
  };
  webhook_url?: string;
}

export interface ApiKey {
  id: string;
  name: string;
  key_prefix: string;
  scopes: string[];
  last_used_at?: string;
  expires_at?: string;
  created_at: string;
}

export interface ApiKeyWithSecret extends ApiKey {
  key: string;
}

export interface UsageSummary {
  total_jobs: number;
  total_charged_usd: number;
  byok_jobs: number;
}

export interface LLMConfig {
  provider: string;
  has_api_key: boolean;
  base_url?: string;
  model?: string;
  updated_at?: string;
}

export interface LLMConfigInput {
  provider: string;
  api_key?: string;
  base_url?: string;
  model?: string;
}

export interface ServiceKey {
  provider: string;
  default_model: string;
  is_enabled: boolean;
  has_key: boolean;
  created_at: string;
  updated_at: string;
}

export interface ServiceKeyInput {
  provider: 'openrouter' | 'anthropic' | 'openai';
  api_key: string;
  default_model: string;
  is_enabled: boolean;
}

// Fallback Chain
export interface FallbackChainEntry {
  id: string;
  position: number;
  provider: string;
  model: string;
  temperature?: number;
  max_tokens?: number;
  is_enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface FallbackChainEntryInput {
  provider: string; // Dynamic providers from registry
  model: string;
  temperature?: number;
  max_tokens?: number;
  is_enabled: boolean;
}

export async function getFallbackChain(tier?: string) {
  const query = tier !== undefined ? `?tier=${encodeURIComponent(tier)}` : '';
  return request<{ chain: FallbackChainEntry[]; tiers: string[] }>('GET', `/api/v1/admin/fallback-chain${query}`);
}

export async function setFallbackChain(chain: FallbackChainEntryInput[], tier?: string | null) {
  return request<{ chain: FallbackChainEntry[] }>('PUT', '/api/v1/admin/fallback-chain', {
    chain,
    tier: tier === 'default' ? null : tier,
  });
}

// User LLM Provider Keys
export interface UserServiceKey {
  id: string;
  provider: string;
  has_key: boolean;
  base_url?: string;
  is_enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface UserServiceKeyInput {
  provider: 'openrouter' | 'anthropic' | 'openai' | 'ollama';
  api_key?: string;
  base_url?: string;
  is_enabled: boolean;
}

export async function listUserServiceKeys() {
  return request<{ keys: UserServiceKey[] }>('GET', '/api/v1/llm/keys');
}

// LLM Providers (dynamic list based on user features)
export interface LLMProvider {
  name: string;
  display_name: string;
  description: string;
  requires_key: boolean;
  key_placeholder?: string;
  base_url_hint?: string;
  docs_url?: string;
  required_features?: string[];
}

export async function listLLMProviders() {
  return request<{ providers: LLMProvider[] }>('GET', '/api/v1/llm/providers');
}

export async function upsertUserServiceKey(input: UserServiceKeyInput) {
  return request<UserServiceKey>('PUT', '/api/v1/llm/keys', input);
}

export async function deleteUserServiceKey(id: string) {
  return request<{ success: boolean }>('DELETE', `/api/v1/llm/keys/${id}`);
}

// User Fallback Chain
export interface UserFallbackChainEntry {
  id: string;
  position: number;
  provider: string;
  model: string;
  temperature?: number;
  max_tokens?: number;
  is_enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface UserFallbackChainEntryInput {
  provider: string; // Dynamic providers from registry
  model: string;
  temperature?: number;
  max_tokens?: number;
  is_enabled: boolean;
}

export async function getUserFallbackChain() {
  return request<{ chain: UserFallbackChainEntry[] }>('GET', '/api/v1/llm/chain');
}

export async function setUserFallbackChain(chain: UserFallbackChainEntryInput[]) {
  return request<{ chain: UserFallbackChainEntry[] }>('PUT', '/api/v1/llm/chain', { chain });
}

// Provider Models
export interface ProviderModel {
  id: string;
  name: string;
  description?: string;
  is_free: boolean;
  context_size?: number;
}

export async function listProviderModels(provider: string) {
  return request<{ models: ProviderModel[] }>('GET', `/api/v1/admin/models/${provider}`);
}

// User Provider Models (accessible to all authenticated users)
export async function listUserProviderModels(provider: string) {
  return request<{ models: ProviderModel[] }>('GET', `/api/v1/llm/models/${provider}`);
}

// Model Validation
export interface ModelValidationRequest {
  provider: string;
  model: string;
}

export interface ModelValidationResult {
  provider: string;
  model: string;
  status: 'valid' | 'not_found' | 'deprecated' | 'unknown';
  message?: string;
}

export async function validateModels(models: ModelValidationRequest[]) {
  return request<{ results: ModelValidationResult[] }>('POST', '/api/v1/admin/models/validate', { models });
}

// Subscription Tiers (from Clerk)
export interface SubscriptionTier {
  id: string;
  name: string;
  slug: string;
  description?: string;
  is_default: boolean;
}

export async function listSubscriptionTiers() {
  return request<{ tiers: SubscriptionTier[] }>('GET', '/api/v1/admin/tiers');
}

export interface TierValidationRequest {
  tier_id: string;
}

export interface TierValidationResult {
  tier_id: string;
  current_slug?: string;
  status: 'valid' | 'not_found' | 'unknown';
  message?: string;
}

export async function validateTiers(tiers: TierValidationRequest[]) {
  return request<{ results: TierValidationResult[] }>('POST', '/api/v1/admin/tiers/validate', { tiers });
}

// Analyze
export interface AnalyzeTokenUsage {
  input_tokens: number;
  output_tokens: number;
}

export interface AnalyzeResult {
  job_id?: string;
  site_summary: string;
  page_type: string;
  detected_elements: DetectedElement[];
  suggested_schema: string;
  follow_patterns: FollowPattern[];
  sample_links: string[];
  recommended_fetch_mode: string;
  sample_data?: unknown;
  token_usage?: AnalyzeTokenUsage;
}

export interface DetectedElement {
  name: string;
  type: string;
  count?: number;
  description: string;
}

export interface FollowPattern {
  pattern: string;
  description: string;
  sample_urls?: string[];
}

export async function analyze(url: string, depth = 0, fetchMode = 'auto') {
  return request<AnalyzeResult>('POST', '/api/v1/analyze', {
    url,
    depth,
    fetch_mode: fetchMode,
  });
}

// Schema Catalog
export interface Schema {
  id: string;
  organization_id?: string;
  user_id?: string;
  name: string;
  description?: string;
  category?: string;
  schema_yaml: string;
  visibility: 'platform' | 'public' | 'private';
  is_platform: boolean;
  tags?: string[];
  usage_count: number;
  created_at: string;
  updated_at: string;
}

export interface CreateSchemaInput {
  name: string;
  description?: string;
  category?: string;
  schema_yaml: string;
  visibility?: 'private' | 'public';
  tags?: string[];
}

export interface UpdateSchemaInput {
  name?: string;
  description?: string;
  category?: string;
  schema_yaml?: string;
  visibility?: 'private' | 'public';
  tags?: string[];
}

export async function listSchemas(category?: string, includePublic = true) {
  const params = new URLSearchParams();
  if (category) params.set('category', category);
  params.set('include_public', String(includePublic));
  return request<{ schemas: Schema[] }>('GET', `/api/v1/schemas?${params}`);
}

export async function getSchema(id: string) {
  return request<Schema>('GET', `/api/v1/schemas/${id}`);
}

export async function createSchema(data: CreateSchemaInput) {
  return request<Schema>('POST', '/api/v1/schemas', data);
}

export async function updateSchema(id: string, data: UpdateSchemaInput) {
  return request<Schema>('PUT', `/api/v1/schemas/${id}`, data);
}

export async function deleteSchema(id: string) {
  return request<{ success: boolean }>('DELETE', `/api/v1/schemas/${id}`);
}

// Admin Schema Catalog
export async function listAllSchemas() {
  return request<{ schemas: Schema[] }>('GET', '/api/v1/admin/schemas');
}

export interface CreatePlatformSchemaInput {
  name: string;
  description?: string;
  category: string;
  schema_yaml: string;
  tags?: string[];
}

export async function createPlatformSchema(data: CreatePlatformSchemaInput) {
  return request<Schema>('POST', '/api/v1/admin/schemas', data);
}

// Saved Sites
export interface CrawlOptions {
  follow_selector?: string;
  follow_pattern?: string;
  max_pages?: number;
  max_depth?: number;
  use_sitemap?: boolean;
}

export interface SavedSite {
  id: string;
  user_id: string;
  organization_id?: string;
  url: string;
  domain: string;
  name?: string;
  analysis_result?: AnalyzeResult;
  default_schema_id?: string;
  crawl_options?: CrawlOptions;
  fetch_mode: string;
  created_at: string;
  updated_at: string;
}

export interface CreateSavedSiteInput {
  url: string;
  name?: string;
  analysis_result?: AnalyzeResult;
  default_schema_id?: string;
  crawl_options?: CrawlOptions;
  fetch_mode?: string;
}

export interface UpdateSavedSiteInput {
  name?: string;
  default_schema_id?: string;
  crawl_options?: CrawlOptions;
  fetch_mode?: string;
}

export async function listSavedSites() {
  return request<{ sites: SavedSite[] }>('GET', '/api/v1/sites');
}

export async function getSavedSite(id: string) {
  return request<SavedSite>('GET', `/api/v1/sites/${id}`);
}

export async function createSavedSite(data: CreateSavedSiteInput) {
  return request<SavedSite>('POST', '/api/v1/sites', data);
}

export async function updateSavedSite(id: string, data: UpdateSavedSiteInput) {
  return request<SavedSite>('PUT', `/api/v1/sites/${id}`, data);
}

export async function deleteSavedSite(id: string) {
  return request<{ success: boolean }>('DELETE', `/api/v1/sites/${id}`);
}

// Webhooks
export interface Webhook {
  id: string;
  name: string;
  url: string;
  has_secret: boolean;
  events: string[];
  headers?: WebhookHeader[];
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

export interface WebhookHeader {
  name: string;
  value: string;
}

export interface WebhookInput {
  name: string;
  url: string;
  secret?: string;
  events?: string[];
  headers?: WebhookHeader[];
  is_active: boolean;
}

export interface WebhookDelivery {
  id: string;
  webhook_id?: string;
  job_id: string;
  event_type: string;
  url: string;
  status_code?: number;
  response_time_ms?: number;
  status: 'pending' | 'success' | 'failed' | 'retrying';
  error_message?: string;
  attempt_number: number;
  max_attempts: number;
  next_retry_at?: string;
  created_at: string;
  delivered_at?: string;
}

export async function listWebhooks() {
  return request<{ webhooks: Webhook[] }>('GET', '/api/v1/webhooks');
}

export async function getWebhook(id: string) {
  return request<Webhook>('GET', `/api/v1/webhooks/${id}`);
}

export async function createWebhook(data: WebhookInput) {
  return request<Webhook>('POST', '/api/v1/webhooks', data);
}

export async function updateWebhook(id: string, data: WebhookInput) {
  return request<Webhook>('PUT', `/api/v1/webhooks/${id}`, data);
}

export async function deleteWebhook(id: string) {
  return request<{ success: boolean }>('DELETE', `/api/v1/webhooks/${id}`);
}

export async function listWebhookDeliveries(webhookId: string, limit = 50, offset = 0) {
  return request<{ deliveries: WebhookDelivery[] }>(
    'GET',
    `/api/v1/webhooks/${webhookId}/deliveries?limit=${limit}&offset=${offset}`
  );
}

export async function listJobWebhookDeliveries(jobId: string) {
  return request<{ deliveries: WebhookDelivery[] }>(
    'GET',
    `/api/v1/jobs/${jobId}/webhook-deliveries`
  );
}

// Pricing / Tier Limits (public endpoint)
// Feature availability (webhooks, BYOK, etc.) is controlled by Clerk features, not tier limits.
// Only includes limits that are actually enforced by the API.
export interface TierLimits {
  name: string;
  display_name: string;
  monthly_extractions: number;      // 0 = unlimited
  max_concurrent_jobs: number;      // 0 = unlimited
  max_pages_per_crawl: number;      // 0 = unlimited
  requests_per_minute: number;      // 0 = unlimited
  credit_allocation_usd: number;    // Monthly USD credit for premium models (0 = none)
  credit_rollover_months: number;   // -1 = never expires, 0 = current period, N = N additional periods
}

export async function listTierLimits() {
  return request<{ tiers: TierLimits[] }>('GET', '/api/v1/pricing/tiers', undefined, false);
}

// Admin: Sync tier metadata from Clerk Commerce
export async function syncTiers() {
  return request<{ message: string }>('POST', '/api/v1/admin/tiers/sync');
}

// ==================== Admin Analytics ====================

export interface AnalyticsOverview {
  total_jobs: number;
  completed_jobs: number;
  failed_jobs: number;
  total_cost_usd: number;
  total_llm_cost_usd: number;
  total_tokens_input: number;
  total_tokens_output: number;
  active_users: number;
  byok_jobs: number;
  platform_jobs: number;
  error_rate: number;
}

export interface AnalyticsJob {
  id: string;
  user_id: string;
  type: string;
  status: string;
  url: string;
  cost_usd: number;
  llm_cost_usd: number;
  tokens_input: number;
  tokens_output: number;
  error_category?: string;
  error_message?: string;
  provider?: string;
  model?: string;
  discovery_method?: string; // How URLs were discovered: "sitemap", "links", or ""
  is_byok: boolean;
  created_at: string;
  completed_at?: string;
}

export interface AnalyticsJobsParams {
  start_date?: string;
  end_date?: string;
  status?: string;
  type?: string;
  user_id?: string;
  limit?: number;
  offset?: number;
  sort?: string;
  order?: string;
}

export interface ErrorCategorySummary {
  category: string;
  count: number;
  percentage: number;
  sample_messages?: string[];
}

export interface FailingURL {
  url: string;
  count: number;
}

export interface ProviderError {
  provider: string;
  model: string;
  count: number;
}

export interface AnalyticsErrorSummary {
  by_category: ErrorCategorySummary[];
  top_failing_urls: FailingURL[];
  by_provider: ProviderError[];
}

export interface TrendDataPoint {
  date: string;
  job_count: number;
  cost_usd: number;
  llm_cost_usd: number;
  error_count: number;
  tokens: number;
}

export interface AnalyticsUserSummary {
  user_id: string;
  total_jobs: number;
  completed_jobs: number;
  failed_jobs: number;
  total_cost_usd: number;
  total_tokens: number;
  last_active?: string;
}

export interface AnalyticsUsersParams {
  start_date?: string;
  end_date?: string;
  limit?: number;
  offset?: number;
  sort?: string;
  order?: string;
}

export async function getAnalyticsOverview(startDate?: string, endDate?: string) {
  const params = new URLSearchParams();
  if (startDate) params.append('start_date', startDate);
  if (endDate) params.append('end_date', endDate);
  const query = params.toString() ? `?${params.toString()}` : '';
  return request<AnalyticsOverview>('GET', `/api/v1/admin/analytics/overview${query}`);
}

export async function getAnalyticsJobs(params: AnalyticsJobsParams = {}) {
  const urlParams = new URLSearchParams();
  if (params.start_date) urlParams.append('start_date', params.start_date);
  if (params.end_date) urlParams.append('end_date', params.end_date);
  if (params.status) urlParams.append('status', params.status);
  if (params.type) urlParams.append('type', params.type);
  if (params.user_id) urlParams.append('user_id', params.user_id);
  if (params.limit) urlParams.append('limit', params.limit.toString());
  if (params.offset) urlParams.append('offset', params.offset.toString());
  if (params.sort) urlParams.append('sort', params.sort);
  if (params.order) urlParams.append('order', params.order);
  const query = urlParams.toString() ? `?${urlParams.toString()}` : '';
  return request<{ jobs: AnalyticsJob[]; total_count: number }>('GET', `/api/v1/admin/analytics/jobs${query}`);
}

export async function getAnalyticsErrors(startDate?: string, endDate?: string) {
  const params = new URLSearchParams();
  if (startDate) params.append('start_date', startDate);
  if (endDate) params.append('end_date', endDate);
  const query = params.toString() ? `?${params.toString()}` : '';
  return request<AnalyticsErrorSummary>('GET', `/api/v1/admin/analytics/errors${query}`);
}

export async function getAnalyticsTrends(startDate?: string, endDate?: string, interval: string = 'day') {
  const params = new URLSearchParams();
  if (startDate) params.append('start_date', startDate);
  if (endDate) params.append('end_date', endDate);
  params.append('interval', interval);
  const query = `?${params.toString()}`;
  return request<{ trends: TrendDataPoint[] }>('GET', `/api/v1/admin/analytics/trends${query}`);
}

export async function getAnalyticsUsers(params: AnalyticsUsersParams = {}) {
  const urlParams = new URLSearchParams();
  if (params.start_date) urlParams.append('start_date', params.start_date);
  if (params.end_date) urlParams.append('end_date', params.end_date);
  if (params.limit) urlParams.append('limit', params.limit.toString());
  if (params.offset) urlParams.append('offset', params.offset.toString());
  if (params.sort) urlParams.append('sort', params.sort);
  if (params.order) urlParams.append('order', params.order);
  const query = urlParams.toString() ? `?${urlParams.toString()}` : '';
  return request<{ users: AnalyticsUserSummary[]; total_count: number }>('GET', `/api/v1/admin/analytics/users${query}`);
}

export interface AdminJobResultsResponse {
  job_id: string;
  download_url: string;
  expires_at: string;
}

export async function getAdminJobResults(jobId: string) {
  return request<AdminJobResultsResponse>('GET', `/api/v1/admin/analytics/jobs/${jobId}/results`);
}

// Debug Capture Types
export interface DebugCaptureLLMRequest {
  provider: string;
  model: string;
  fetch_mode?: string;
  content_size: number;
  prompt_size: number;
}

export interface DebugCaptureLLMResponse {
  input_tokens: number;
  output_tokens: number;
  duration_ms: number;
  success: boolean;
  error?: string;
}

export interface DebugCaptureEntry {
  id: string;
  url: string;
  timestamp: string;
  job_type: string;
  request: DebugCaptureLLMRequest;
  response: DebugCaptureLLMResponse;
  prompt?: string;
  raw_content?: string;
  schema?: string;
  hints_applied?: Record<string, string>;
}

export interface JobDebugCaptureResponse {
  job_id: string;
  enabled: boolean;
  captures: DebugCaptureEntry[];
}

export async function getJobDebugCapture(jobId: string) {
  return request<JobDebugCaptureResponse>('GET', `/api/v1/jobs/${jobId}/debug-capture`);
}

