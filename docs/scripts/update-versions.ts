#!/usr/bin/env npx tsx

/**
 * Script to update documentation versions on release.
 *
 * This script:
 * 1. Reads the current version from git tags or input
 * 2. Updates the versions.ts configuration file
 * 3. Creates a versioned snapshot of the documentation
 *
 * Usage:
 *   npx tsx scripts/update-versions.ts [version]
 *
 * If no version is provided, it reads from GITHUB_REF (for CI) or latest git tag.
 */

import { execSync } from 'child_process';
import * as fs from 'fs';
import * as path from 'path';

const DOCS_ROOT = path.resolve(__dirname, '..');
const VERSIONS_FILE = path.join(DOCS_ROOT, 'src/lib/versions.ts');
const CONTENT_DIR = path.join(DOCS_ROOT, 'content');
const VERSIONS_DIR = path.join(CONTENT_DIR, 'versions');

interface DocVersion {
  name: string;
  slug: string;
  path: string;
  isLatest?: boolean;
}

function getVersion(): string {
  // Check command line argument
  if (process.argv[2]) {
    return process.argv[2].replace(/^v/, '');
  }

  // Check GITHUB_REF environment variable (for CI)
  const githubRef = process.env.GITHUB_REF;
  if (githubRef && githubRef.startsWith('refs/tags/v')) {
    return githubRef.replace('refs/tags/v', '');
  }

  // Fall back to latest git tag
  try {
    const tag = execSync('git describe --tags --abbrev=0', { encoding: 'utf8' }).trim();
    return tag.replace(/^v/, '');
  } catch {
    console.error('Could not determine version. Please provide version as argument.');
    process.exit(1);
  }
}

function parseVersions(): DocVersion[] {
  const content = fs.readFileSync(VERSIONS_FILE, 'utf8');

  // Extract versions array using regex
  const match = content.match(/export const versions: DocVersion\[\] = \[([\s\S]*?)\];/);
  if (!match) {
    console.error('Could not parse versions from', VERSIONS_FILE);
    return [];
  }

  // Parse the versions - this is a simplified parser
  const versionsStr = match[1];
  const versions: DocVersion[] = [];

  // Match each version object
  const objRegex = /\{\s*name:\s*['"]([^'"]+)['"]\s*,\s*slug:\s*['"]([^'"]+)['"]\s*,\s*path:\s*['"]([^'"]+)['"]\s*(?:,\s*isLatest:\s*(true|false))?\s*\}/g;
  let objMatch;
  while ((objMatch = objRegex.exec(versionsStr)) !== null) {
    versions.push({
      name: objMatch[1],
      slug: objMatch[2],
      path: objMatch[3],
      isLatest: objMatch[4] === 'true',
    });
  }

  return versions;
}

function writeVersions(versions: DocVersion[]): void {
  const content = `/**
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
${versions.map(v => `  {
    name: '${v.name}',
    slug: '${v.slug}',
    path: '${v.path}',${v.isLatest ? '\n    isLatest: true,' : ''}
  },`).join('\n')}
];

export const latestVersion = versions.find(v => v.isLatest) || versions[0];

export function getVersionBySlug(slug: string): DocVersion | undefined {
  return versions.find(v => v.slug === slug);
}

export function isLatestVersion(slug: string): boolean {
  return slug === 'latest' || slug === latestVersion.slug;
}
`;

  fs.writeFileSync(VERSIONS_FILE, content);
  console.log(`Updated ${VERSIONS_FILE}`);
}

function copyDirectory(src: string, dest: string): void {
  if (!fs.existsSync(dest)) {
    fs.mkdirSync(dest, { recursive: true });
  }

  const entries = fs.readdirSync(src, { withFileTypes: true });

  for (const entry of entries) {
    const srcPath = path.join(src, entry.name);
    const destPath = path.join(dest, entry.name);

    if (entry.isDirectory()) {
      copyDirectory(srcPath, destPath);
    } else {
      fs.copyFileSync(srcPath, destPath);
    }
  }
}

function createVersionSnapshot(version: string): void {
  const srcDir = path.join(CONTENT_DIR, 'docs');
  const destDir = path.join(VERSIONS_DIR, `v${version}`);

  if (fs.existsSync(destDir)) {
    console.log(`Version snapshot v${version} already exists, skipping...`);
    return;
  }

  console.log(`Creating version snapshot: ${srcDir} -> ${destDir}`);
  copyDirectory(srcDir, destDir);
  console.log(`Created version snapshot at ${destDir}`);
}

function main(): void {
  const version = getVersion();
  console.log(`Processing version: v${version}`);

  // Parse existing versions
  const versions = parseVersions();

  // Check if this version already exists
  const existingVersion = versions.find(v => v.slug === `v${version}` || v.name.includes(version));
  if (existingVersion && !existingVersion.isLatest) {
    console.log(`Version v${version} already exists in versions list`);
    return;
  }

  // Update latest version name
  const latestIdx = versions.findIndex(v => v.isLatest);
  if (latestIdx >= 0) {
    versions[latestIdx] = {
      ...versions[latestIdx],
      name: `Latest (v${version})`,
    };
  }

  // Add versioned snapshot to the list (if not already there)
  const snapshotExists = versions.some(v => v.slug === `v${version}`);
  if (!snapshotExists) {
    // Insert after latest
    const insertIdx = latestIdx >= 0 ? latestIdx + 1 : 0;
    versions.splice(insertIdx, 0, {
      name: `v${version}`,
      slug: `v${version}`,
      path: `versions/v${version}`,
    });

    // Create the snapshot directory
    createVersionSnapshot(version);
  }

  // Write updated versions
  writeVersions(versions);

  console.log('Version update complete!');
}

main();
