// API Types - All interfaces and type definitions for the Refyne API

// ==================== Error Types ====================

export interface ApiError {
  error: string;
  status: number;
  error_category?: string;  // Error classification (rate_limited, invalid_api_key, provider_error, bot_protection_detected, etc.)
  error_details?: string;   // Full error details (only for BYOK users)
  is_byok?: boolean;        // Whether user's own API key was used
  provider?: string;        // LLM provider (only for BYOK users)
  model?: string;           // LLM model (only for BYOK users)
  protection_type?: string;      // Type of bot protection detected (cloudflare_challenge, captcha, etc.)
  suggested_fetch_mode?: string; // Suggested fetch mode to bypass protection (usually 'dynamic')
}

// Helper to check if an error is a bot protection error that can be resolved with browser rendering
export function isBotProtectionError(error: ApiError): boolean {
  return error.error_category === 'bot_protection_detected';
}

// ==================== Extraction Types ====================

export interface ExtractResult {
  job_id: string;
  data: unknown;
  url: string;
  fetched_at: string;
  input_format: 'schema' | 'prompt';
  fetch_mode: 'static' | 'dynamic';
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
  fetch_mode_used?: string;  // Actual fetch mode used for this analysis (static or dynamic)
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

// ==================== Job Types ====================

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

export interface CreateCrawlJobInput {
  url: string;
  schema: object | string;
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
    fetch_mode?: 'auto' | 'static' | 'dynamic';
  };
  webhook_url?: string;
}

export type OutputFormat = 'json' | 'jsonl' | 'yaml';

// ==================== API Key Types ====================

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

// ==================== Usage Types ====================

export interface UsageSummary {
  total_jobs: number;
  total_charged_usd: number;
  byok_jobs: number;
}

// ==================== LLM Config Types ====================

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
  is_enabled: boolean;
  has_key: boolean;
  created_at: string;
  updated_at: string;
}

export interface ServiceKeyInput {
  provider: 'openrouter' | 'anthropic' | 'openai';
  api_key: string;
  is_enabled: boolean;
}

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

export interface ProviderModel {
  id: string;
  name: string;
  description?: string;
  is_free: boolean;
  context_size?: number;
}

// ==================== Fallback Chain Types ====================

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
  provider: string;
  model: string;
  temperature?: number;
  max_tokens?: number;
  is_enabled: boolean;
}

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
  provider: string;
  model: string;
  temperature?: number;
  max_tokens?: number;
  is_enabled: boolean;
}

// ==================== Model Validation Types ====================

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

// ==================== Tier Types ====================

export interface SubscriptionTier {
  id: string;
  name: string;
  slug: string;
  description?: string;
  is_default: boolean;
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

export interface TierLimits {
  name: string;
  display_name: string;
  monthly_extractions: number;
  max_concurrent_jobs: number;
  max_pages_per_crawl: number;
  requests_per_minute: number;
  credit_allocation_usd: number;
  credit_rollover_months: number;
}

// ==================== Schema Types ====================

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

export interface CreatePlatformSchemaInput {
  name: string;
  description?: string;
  category: string;
  schema_yaml: string;
  tags?: string[];
}

// ==================== Saved Site Types ====================

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
  analysis_result?: AnalyzeResult;
  default_schema_id?: string;
  crawl_options?: CrawlOptions;
  fetch_mode?: string;
}

// ==================== Webhook Types ====================

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

// ==================== Analytics Types ====================

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
  discovery_method?: string;
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

export interface AdminJobResultsResponse {
  job_id: string;
  download_url: string;
  expires_at: string;
}

// ==================== Debug Capture Types ====================

export interface DebugCaptureLLMRequest {
  provider: string;
  model: string;
  fetch_mode?: string;
  content_size: number;
  prompt_size: number;
  // Payload fields
  schema?: string;
  prompt?: string;
  page_content?: string;
  hints_applied?: Record<string, string>;
}

export interface DebugCaptureLLMResponse {
  input_tokens: number;
  output_tokens: number;
  duration_ms: number;
  success: boolean;
  error?: string;
  // Payload fields
  raw_output?: string;
}

export interface DebugCaptureEntry {
  id: string;
  url: string;
  timestamp: string;
  job_type: string;
  api_version?: string;
  request: DebugCaptureLLMRequest;
  response: DebugCaptureLLMResponse;
}

export interface JobDebugCaptureResponse {
  job_id: string;
  enabled: boolean;
  captures: DebugCaptureEntry[];
}
