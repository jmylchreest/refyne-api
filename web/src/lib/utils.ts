import { clsx, type ClassValue } from "clsx"
import { twMerge } from "tailwind-merge"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

/**
 * Parse Clerk Commerce features from the JWT "fea" claim.
 * The claim format is comma-separated features with optional prefixes:
 * - "u:" prefix = user-level feature
 * - "o:" prefix = org-level feature
 * - no prefix = direct feature name
 *
 * @param feaClaim - The raw "fea" claim from Clerk session claims
 * @returns Array of feature names (prefixes stripped)
 */
export function parseClerkFeatures(feaClaim: string | undefined): string[] {
  if (!feaClaim) return [];
  return feaClaim.split(',').map(f => {
    const trimmed = f.trim();
    if (trimmed.startsWith('u:')) return trimmed.slice(2);
    if (trimmed.startsWith('o:')) return trimmed.slice(2);
    return trimmed;
  }).filter(Boolean);
}

/**
 * Check if a specific feature is enabled in the Clerk Commerce features.
 *
 * @param feaClaim - The raw "fea" claim from Clerk session claims
 * @param feature - The feature name to check for
 * @returns true if the feature is enabled
 */
export function hasClerkFeature(feaClaim: string | undefined, feature: string): boolean {
  return parseClerkFeatures(feaClaim).includes(feature);
}
