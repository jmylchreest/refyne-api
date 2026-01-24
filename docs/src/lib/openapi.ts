import { createOpenAPI } from 'fumadocs-openapi/server';
import * as fs from 'fs';
import * as path from 'path';

// Check for local openapi.json first (from CI artifact), fall back to API URL
const localSpecPath = path.join(process.cwd(), 'openapi.json');
const hasLocalSpec = fs.existsSync(localSpecPath);

// API URL (localhost:8080 for local dev)
const apiUrl = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080';
const openApiUrl = `${apiUrl}/openapi.json`;

// Use local file in CI, fetch from API in development
const specSource = hasLocalSpec ? localSpecPath : openApiUrl;

console.log(`[OpenAPI] Using spec from: ${specSource}`);
console.log(`[OpenAPI] Local file exists: ${hasLocalSpec}`);

export const openapi = createOpenAPI({
  input: [specSource],
  disableCache: process.env.NODE_ENV === 'development',
});
