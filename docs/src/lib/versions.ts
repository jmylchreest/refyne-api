/**
 * Documentation versioning configuration.
 *
 * Versions are managed automatically:
 * - 'latest' always points to the current docs in content/docs/
 * - Tagged versions are created by the CI workflow on release
 */

export interface DocVersion {
  name: string;       // Display name (e.g., "v0.1.6", "Latest")
  slug: string;       // URL slug (e.g., "v0.1.6", "latest")
  path: string;       // Content path relative to content/ (e.g., "docs", "versions/v0.1.6")
  isLatest?: boolean; // Whether this is the latest version
}

// This file is auto-updated by the release workflow
// Do not edit manually - changes will be overwritten
export const versions: DocVersion[] = [
  {
    name: 'Latest (v0.1.6)',
    slug: 'latest',
    path: 'docs',
    isLatest: true,
  },
  // Versioned snapshots will be added here by the release workflow
  // Example:
  // {
  //   name: 'v0.1.5',
  //   slug: 'v0.1.5',
  //   path: 'versions/v0.1.5',
  // },
];

export const latestVersion = versions.find(v => v.isLatest) || versions[0];

export function getVersionBySlug(slug: string): DocVersion | undefined {
  return versions.find(v => v.slug === slug);
}

export function isLatestVersion(slug: string): boolean {
  return slug === 'latest' || slug === latestVersion.slug;
}
