import { clsx, type ClassValue } from "clsx"
import { twMerge } from "tailwind-merge"
import { formatDistanceToNow, formatDistance, format } from 'date-fns';
import type { ApiError } from './api/types';

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

// ==================== Date/Time Formatters ====================

export function formatRelativeTime(dateString: string) {
  return formatDistanceToNow(new Date(dateString), { addSuffix: false });
}

export function formatDateTime(dateString: string) {
  return format(new Date(dateString), 'PPpp');
}

export function formatDuration(startDate: string, endDate: string) {
  return formatDistance(new Date(startDate), new Date(endDate));
}

// ==================== Number Formatters ====================

export function formatTokens(count: number): string {
  if (count >= 1000000) return `${(count / 1000000).toFixed(1)}M`;
  if (count >= 1000) return `${(count / 1000).toFixed(1)}k`;
  return count.toString();
}

export function formatCurrency(amount: number, decimals = 4): string {
  return `$${amount.toFixed(decimals)}`;
}

// ==================== URL Helpers ====================

export function truncateUrl(url: string, maxLength = 30): string {
  try {
    const parsed = new URL(url);
    const path =
      parsed.pathname.length > 18
        ? parsed.pathname.slice(0, 18) + '...'
        : parsed.pathname;
    return parsed.hostname + path;
  } catch {
    return url.length > maxLength ? url.slice(0, maxLength) + '...' : url;
  }
}

export function normalizeUrl(inputUrl: string): string {
  const trimmed = inputUrl.trim();
  if (!trimmed) return trimmed;
  if (!trimmed.match(/^https?:\/\//i)) {
    return `https://${trimmed}`;
  }
  return trimmed;
}

export function getHostname(url: string): string {
  try {
    return new URL(url).hostname;
  } catch {
    return url;
  }
}

// ==================== Status Colors ====================

export type JobStatus = 'pending' | 'running' | 'completed' | 'failed' | 'cancelled';

export function getStatusColor(status: JobStatus): string {
  switch (status) {
    case 'pending':
      return 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-400';
    case 'running':
      return 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400';
    case 'completed':
      return 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400';
    case 'failed':
      return 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400';
    case 'cancelled':
      return 'bg-zinc-100 text-zinc-800 dark:bg-zinc-900/30 dark:text-zinc-400';
    default:
      return 'bg-zinc-100 text-zinc-800 dark:bg-zinc-900/30 dark:text-zinc-400';
  }
}

export function getStatusDotColor(status: JobStatus): string {
  switch (status) {
    case 'pending':
      return 'bg-yellow-500';
    case 'running':
      return 'bg-blue-500 animate-pulse';
    case 'completed':
      return 'bg-green-500';
    case 'failed':
      return 'bg-red-500';
    case 'cancelled':
      return 'bg-zinc-400';
    default:
      return 'bg-zinc-400';
  }
}

// ==================== Error Handling ====================

export function getErrorMessage(error: ApiError | { error?: string }): string {
  const apiError = error as ApiError;

  // If we have an error category, provide a more helpful message
  if (apiError.error_category) {
    const categoryMessages: Record<string, string> = {
      invalid_api_key: apiError.is_byok
        ? 'Your API key is invalid. Please check your LLM provider settings.'
        : 'Authentication error. Please try again.',
      insufficient_credits: 'Insufficient credits. Please add credits to continue.',
      quota_exceeded: 'You have exceeded your usage quota. Please upgrade your plan or wait for your quota to reset.',
      feature_disabled: 'This feature is not available on your current plan.',
      rate_limited: 'Too many requests. Please wait a moment and try again.',
      quota_exhausted: 'Free tier quota exhausted. Please upgrade or try again later.',
      provider_unavailable: apiError.is_byok
        ? `The ${apiError.provider || 'LLM'} provider is currently unavailable. Please check their status or try a different provider.`
        : 'The extraction service is temporarily unavailable. Please try again later.',
      model_unavailable: apiError.is_byok
        ? `The model ${apiError.model || ''} is unavailable. Please try a different model.`
        : 'The extraction model is temporarily unavailable. Please try again later.',
      provider_error: apiError.is_byok
        ? `Error from ${apiError.provider || 'provider'}: ${apiError.error}${apiError.error_details ? `\n\nDetails: ${apiError.error_details}` : ''}`
        : 'A temporary error occurred. Please try again.',
      extraction_error: apiError.error || 'Extraction failed. Please check your schema and try again.',
    };

    return categoryMessages[apiError.error_category] || apiError.error || 'An error occurred';
  }

  // Fallback to the error message
  return apiError.error || 'An error occurred';
}

/**
 * Parse Clerk Commerce features from the JWT claims.
 *
 * Features are collected from two sources (in priority order):
 * 1. public_metadata.feature_overrides (admin overrides - highest priority)
 * 2. "fea" claim from Clerk Commerce (comma-separated with u:/o: prefixes)
 *
 * The fea claim format is comma-separated features with optional prefixes:
 * - "u:" prefix = user-level feature
 * - "o:" prefix = org-level feature
 * - no prefix = direct feature name
 *
 * @param feaClaim - The raw "fea" claim from Clerk session claims
 * @param publicMetadata - Optional public_metadata from session claims
 * @returns Array of feature names (prefixes stripped, deduped)
 */
export function parseClerkFeatures(
  feaClaim: string | undefined,
  publicMetadata?: Record<string, unknown>
): string[] {
  const seen = new Set<string>();
  const features: string[] = [];

  // Add admin feature overrides first (highest priority)
  if (publicMetadata?.feature_overrides) {
    const overrides = publicMetadata.feature_overrides;
    if (Array.isArray(overrides)) {
      for (const f of overrides) {
        if (typeof f === 'string' && f && !seen.has(f)) {
          features.push(f);
          seen.add(f);
        }
      }
    }
  }

  // Add Clerk Commerce features (strip prefixes)
  if (feaClaim) {
    for (const f of feaClaim.split(',')) {
      let feature = f.trim();
      if (feature.startsWith('u:')) feature = feature.slice(2);
      else if (feature.startsWith('o:')) feature = feature.slice(2);

      if (feature && !seen.has(feature)) {
        features.push(feature);
        seen.add(feature);
      }
    }
  }

  return features;
}

/**
 * Check if a specific feature is enabled in the Clerk Commerce features.
 *
 * @param feaClaim - The raw "fea" claim from Clerk session claims
 * @param feature - The feature name to check for
 * @param publicMetadata - Optional public_metadata from session claims
 * @returns true if the feature is enabled
 */
export function hasClerkFeature(
  feaClaim: string | undefined,
  feature: string,
  publicMetadata?: Record<string, unknown>
): boolean {
  return parseClerkFeatures(feaClaim, publicMetadata).includes(feature);
}

/**
 * Parse Clerk tier from the JWT claims.
 *
 * Tier is determined from (in priority order):
 * 1. public_metadata.tier_override (admin override for specific users)
 * 2. "pla" claim from Clerk Commerce (e.g., "u:tier_v1_free" or "o:tier_v1_pro")
 * 3. public_metadata.subscription.tier (legacy)
 * 4. Default "free"
 *
 * @param plaClaim - The raw "pla" (plan) claim from Clerk session claims
 * @param publicMetadata - Optional public_metadata from session claims
 * @returns The tier name (prefixes stripped)
 */
export function parseClerkTier(
  plaClaim: string | undefined,
  publicMetadata?: Record<string, unknown>
): string {
  // Check for admin tier override first (highest priority)
  if (publicMetadata?.tier_override) {
    const override = publicMetadata.tier_override;
    if (typeof override === 'string' && override) {
      return override;
    }
  }

  // Check Clerk Commerce plan claim (strip u: or o: prefix)
  if (plaClaim) {
    let tier = plaClaim.trim();
    if (tier.startsWith('u:')) tier = tier.slice(2);
    else if (tier.startsWith('o:')) tier = tier.slice(2);
    if (tier) return tier;
  }

  // Fall back to public_metadata.subscription.tier (legacy)
  if (publicMetadata?.subscription) {
    const sub = publicMetadata.subscription;
    if (typeof sub === 'object' && sub !== null) {
      const tier = (sub as Record<string, unknown>).tier;
      if (typeof tier === 'string' && tier) {
        return tier;
      }
    }
  }

  return 'free';
}
