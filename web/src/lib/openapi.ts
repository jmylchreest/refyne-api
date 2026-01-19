import { createOpenAPI } from 'fumadocs-openapi/server';

const apiUrl = process.env.NEXT_PUBLIC_API_URL || 'https://api.refyne.uk';

export const openapi = createOpenAPI({
  input: [`${apiUrl}/openapi.json`],
  disableCache: process.env.NODE_ENV === 'development',
});
