import { docs } from '../../.source/server';
import { loader, multiple, type LoaderPlugin } from 'fumadocs-core/source';
import { openapiPlugin, openapiSource } from 'fumadocs-openapi/server';
import { openapi } from './openapi';

// Dynamic OpenAPI source (no file generation needed)
const openapiPages = await openapiSource(openapi, {
  baseDir: 'api-reference',
  groupBy: 'tag',
});

// Acronyms that should always be fully capitalized
const ACRONYMS = new Set(['api', 'llm', 'url', 'json', 'jsonl', 'yaml', 'http', 'sdk', 'id', 'rest', 'html', 'css', 'sql', 'sse']);

// Special case words that need specific casing (not simple acronyms)
const SPECIAL_CASES: Record<string, string> = {
  'openapi': 'OpenAPI',
};

/**
 * Check if a word needs special casing (acronym or special case)
 */
function needsSpecialCasing(word: string): boolean {
  const lower = word.toLowerCase();
  if (ACRONYMS.has(lower)) return true;
  if (SPECIAL_CASES[lower]) return true;
  // Check if it's a plural form (e.g., "sdks" -> "sdk", "apis" -> "api")
  if (lower.endsWith('s') && ACRONYMS.has(lower.slice(0, -1))) return true;
  return false;
}

/**
 * Convert a word to its proper form (uppercase for acronyms, special case, or title case)
 * Handles plural acronyms: "sdks" -> "SDKs", "apis" -> "APIs"
 */
function toProperCase(word: string): string {
  const lower = word.toLowerCase();

  // Check for special case first
  if (SPECIAL_CASES[lower]) {
    return SPECIAL_CASES[lower];
  }

  // Check for exact acronym match
  if (ACRONYMS.has(lower)) {
    return word.toUpperCase();
  }

  // Check for plural acronym (e.g., "sdks" -> "SDKs")
  if (lower.endsWith('s') && ACRONYMS.has(lower.slice(0, -1))) {
    return lower.slice(0, -1).toUpperCase() + 's';
  }

  // Title case: capitalize first letter
  return word.charAt(0).toUpperCase() + word.slice(1).toLowerCase();
}

/**
 * Convert a slugified string to proper title case with acronym handling.
 * e.g., "api-keys" -> "API Keys", "llm-chain" -> "LLM Chain"
 */
function toDisplayName(slug: string): string {
  return slug.split('-').map(toProperCase).join(' ');
}

// Plugin to fix folder and page names in the page tree
const folderNamePlugin: LoaderPlugin = {
  name: 'folder-display-names',
  transformPageTree: {
    folder(node, folderPath) {
      // Get the last segment of the folder path (the folder name)
      const folderName = folderPath.split('/').pop() || '';
      if (folderName) {
        return { ...node, name: toDisplayName(folderName) };
      }
      return node;
    },
    file(node) {
      // Transform page names that contain words which should be acronyms
      // Only transform if the name is a simple string (not already a React element)
      if (typeof node.name === 'string') {
        const words = node.name.split(/[\s-]+/);
        const hasAcronymWord = words.some(word => needsSpecialCasing(word));
        if (hasAcronymWord) {
          // Re-process the name to apply proper acronym casing
          const newName = words
            .map(word => {
              if (needsSpecialCasing(word)) {
                return toProperCase(word);
              }
              return word; // Keep original casing for non-acronym words
            })
            .join(' ');
          if (newName !== node.name) {
            return { ...node, name: newName };
          }
        }
      }
      return node;
    },
  },
};

export const source = loader({
  baseUrl: '/', // Docs served from root on docs.refyne.uk
  source: multiple({
    docs: docs.toFumadocsSource(),
    openapi: openapiPages,
  }),
  plugins: [openapiPlugin(), folderNamePlugin],
});
