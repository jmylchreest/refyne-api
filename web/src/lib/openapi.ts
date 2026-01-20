import { createOpenAPI } from 'fumadocs-openapi/server';

const apiUrl = process.env.NEXT_PUBLIC_API_URL || 'https://api.refyne.uk';
const openApiUrl = `${apiUrl}/openapi.json`;

// Log during build so we can see what URL is being fetched
console.log(`[OpenAPI] Fetching spec from: ${openApiUrl}`);
console.log(`[OpenAPI] NEXT_PUBLIC_API_URL=${process.env.NEXT_PUBLIC_API_URL || '(not set, using default)'}`);

export const openapi = createOpenAPI({
  input: [openApiUrl],
  disableCache: process.env.NODE_ENV === 'development',
});
