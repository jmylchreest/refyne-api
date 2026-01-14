// API client for refyne-api
const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080';

interface ApiError {
  error: string;
  status: number;
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
    const error = await response.json().catch(() => ({ error: 'Request failed' }));
    throw { error: error.error || error.message || 'Request failed', status: response.status } as ApiError;
  }

  return response.json();
}

// Extraction
export async function extract(url: string, schema: object, llmConfig?: LLMConfigInput) {
  return request<ExtractResult>('POST', '/api/v1/extract', {
    url,
    schema,
    llm_config: llmConfig,
  });
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

export async function getJobResults(id: string, merge = false) {
  const query = merge ? '?merge=true' : '';
  return request<JobResultsResponse>('GET', `/api/v1/jobs/${id}/results${query}`);
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
  data: unknown;
  url: string;
  fetched_at: string;
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
  type: 'extract' | 'crawl';
  status: 'pending' | 'running' | 'completed' | 'failed' | 'cancelled';
  url: string;
  page_count: number;
  token_usage_input: number;
  token_usage_output: number;
  cost_credits: number;
  error_message?: string;
  started_at?: string;
  completed_at?: string;
  created_at: string;
}

export interface CreateCrawlJobInput {
  url: string;
  schema: object;
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
  provider: 'openrouter' | 'anthropic' | 'openai' | 'ollama';
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
  provider: 'openrouter' | 'anthropic' | 'openai' | 'ollama';
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

export type { ApiError };
