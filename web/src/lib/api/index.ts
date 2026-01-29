// API Module - Re-exports types and provides all API functions

export * from './types';
export { setTokenGetter, API_BASE_URL } from './client';
import { request, getTokenGetter, API_BASE_URL } from './client';
import type {
  ExtractResult,
  AnalyzeResult,
  Job,
  JobResultsResponse,
  JobWebhookDelivery,
  CreateCrawlJobInput,
  OutputFormat,
  ApiKey,
  ApiKeyWithSecret,
  UsageSummary,
  LLMConfig,
  LLMConfigInput,
  ServiceKey,
  ServiceKeyInput,
  UserServiceKey,
  UserServiceKeyInput,
  LLMProvider,
  ProviderModel,
  FallbackChainEntry,
  FallbackChainEntryInput,
  UserFallbackChainEntry,
  UserFallbackChainEntryInput,
  ModelValidationRequest,
  ModelValidationResult,
  SubscriptionTier,
  TierValidationRequest,
  TierValidationResult,
  TierLimits,
  Schema,
  CreateSchemaInput,
  UpdateSchemaInput,
  CreatePlatformSchemaInput,
  SavedSite,
  CreateSavedSiteInput,
  UpdateSavedSiteInput,
  Webhook,
  WebhookInput,
  WebhookDelivery,
  AnalyticsOverview,
  AnalyticsJob,
  AnalyticsJobsParams,
  AnalyticsErrorSummary,
  TrendDataPoint,
  AnalyticsUserSummary,
  AnalyticsUsersParams,
  AdminJobResultsResponse,
  JobDebugCaptureResponse,
  LLMConfigInput as ExtractLLMConfig,
} from './types';

// ==================== Extraction ====================

// Extraction with client-side timeout
// Schema can be an object (structured schema) or string (freeform prompt)
// fetchMode: 'auto' (default) | 'static' | 'dynamic'
export async function extract(url: string, schema: object | string, llmConfig?: ExtractLLMConfig, fetchMode = 'auto') {
  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(), 120000); // 2 minute client timeout

  try {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
    };

    const tokenGetter = getTokenGetter();
    if (tokenGetter) {
      const token = await tokenGetter();
      if (token) {
        headers['Authorization'] = `Bearer ${token}`;
      }
    }

    const response = await fetch(`${API_BASE_URL}/api/v1/extract`, {
      method: 'POST',
      headers,
      body: JSON.stringify({ url, schema, llm_config: llmConfig, fetch_mode: fetchMode }),
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
      const errorMessage = errorBody.detail || errorBody.error || errorBody.message || statusMessages[response.status] || `Extraction failed (${response.status})`;
      throw {
        error: errorMessage,
        status: response.status,
        error_category: errorBody.error_category,
        error_details: errorBody.error_details,
        is_byok: errorBody.is_byok,
        provider: errorBody.provider,
        model: errorBody.model,
        protection_type: errorBody.protection_type,
        suggested_fetch_mode: errorBody.suggested_fetch_mode,
      };
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

export async function analyze(url: string, depth = 0, fetchMode = 'auto') {
  return request<AnalyzeResult>('POST', '/api/v1/analyze', {
    url,
    depth,
    fetch_mode: fetchMode,
  });
}

// ==================== Crawl Jobs ====================

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

  const tokenGetter = getTokenGetter();
  if (tokenGetter) {
    const token = await tokenGetter();
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
    throw { error: error.error || `Request failed (${response.status})`, status: response.status };
  }

  return response.text();
}

export async function getJobWebhookDeliveries(jobId: string) {
  return request<{ job_id: string; deliveries: JobWebhookDelivery[] }>(
    'GET',
    `/api/v1/jobs/${jobId}/webhook-deliveries`
  );
}

export async function getJobDebugCapture(jobId: string) {
  return request<JobDebugCaptureResponse>('GET', `/api/v1/jobs/${jobId}/debug-capture`);
}

export interface DebugCaptureDownloadResponse {
  job_id: string;
  download_url: string;
  expires_at: string;
  filename: string;
}

export async function getJobDebugCaptureDownload(jobId: string) {
  return request<DebugCaptureDownloadResponse>('GET', `/api/v1/jobs/${jobId}/debug-capture/download`);
}

// ==================== API Keys ====================

export async function listApiKeys() {
  return request<{ keys: ApiKey[] }>('GET', '/api/v1/keys');
}

export async function createApiKey(name: string, scopes?: string[]) {
  return request<ApiKeyWithSecret>('POST', '/api/v1/keys', { name, scopes });
}

export async function revokeApiKey(id: string) {
  return request<{ success: boolean }>('DELETE', `/api/v1/keys/${id}`);
}

// ==================== Usage ====================

export async function getUsage(period = 'month') {
  return request<UsageSummary>('GET', `/api/v1/usage?period=${period}`);
}

// ==================== LLM Config ====================

export async function getLLMConfig() {
  return request<LLMConfig>('GET', '/api/v1/llm-config');
}

export async function updateLLMConfig(config: LLMConfigInput) {
  return request<LLMConfig>('PUT', '/api/v1/llm-config', config);
}

// ==================== Admin - Service Keys ====================

export async function listServiceKeys() {
  return request<{ keys: ServiceKey[] }>('GET', '/api/v1/admin/service-keys');
}

export async function upsertServiceKey(input: ServiceKeyInput) {
  return request<ServiceKey>('PUT', '/api/v1/admin/service-keys', input);
}

export async function deleteServiceKey(provider: string) {
  return request<{ success: boolean }>('DELETE', `/api/v1/admin/service-keys/${provider}`);
}

// ==================== Fallback Chain ====================

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

// ==================== User LLM Keys ====================

export async function listUserServiceKeys() {
  return request<{ keys: UserServiceKey[] }>('GET', '/api/v1/llm/keys');
}

export async function upsertUserServiceKey(input: UserServiceKeyInput) {
  return request<UserServiceKey>('PUT', '/api/v1/llm/keys', input);
}

export async function deleteUserServiceKey(id: string) {
  return request<{ success: boolean }>('DELETE', `/api/v1/llm/keys/${id}`);
}

// ==================== LLM Providers ====================

export async function listLLMProviders() {
  return request<{ providers: LLMProvider[] }>('GET', '/api/v1/llm/providers');
}

// ==================== User Fallback Chain ====================

export async function getUserFallbackChain() {
  return request<{ chain: UserFallbackChainEntry[] }>('GET', '/api/v1/llm/chain');
}

export async function setUserFallbackChain(chain: UserFallbackChainEntryInput[]) {
  return request<{ chain: UserFallbackChainEntry[] }>('PUT', '/api/v1/llm/chain', { chain });
}

// ==================== Provider Models ====================

export async function listProviderModels(provider: string) {
  return request<{ models: ProviderModel[] }>('GET', `/api/v1/admin/models/${provider}`);
}

export async function listUserProviderModels(provider: string) {
  return request<{ models: ProviderModel[] }>('GET', `/api/v1/llm/models/${provider}`);
}

export async function validateModels(models: ModelValidationRequest[]) {
  return request<{ results: ModelValidationResult[] }>('POST', '/api/v1/admin/models/validate', { models });
}

// ==================== Subscription Tiers ====================

export async function listSubscriptionTiers() {
  return request<{ tiers: SubscriptionTier[] }>('GET', '/api/v1/admin/tiers');
}

export async function validateTiers(tiers: TierValidationRequest[]) {
  return request<{ results: TierValidationResult[] }>('POST', '/api/v1/admin/tiers/validate', { tiers });
}

export async function listTierLimits() {
  return request<{ tiers: TierLimits[] }>('GET', '/api/v1/pricing/tiers', undefined, false);
}

export async function syncTiers() {
  return request<{ message: string }>('POST', '/api/v1/admin/tiers/sync');
}

// ==================== Schema Catalog ====================

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

export async function createPlatformSchema(data: CreatePlatformSchemaInput) {
  return request<Schema>('POST', '/api/v1/admin/schemas', data);
}

// ==================== Saved Sites ====================

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

// ==================== Webhooks ====================

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

// ==================== Admin Analytics ====================

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

export async function getAdminJobResults(jobId: string) {
  return request<AdminJobResultsResponse>('GET', `/api/v1/admin/analytics/jobs/${jobId}/results`);
}
