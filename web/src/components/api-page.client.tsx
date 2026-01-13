'use client';

import { defineClientConfig } from 'fumadocs-openapi/ui/client';

// Storage key matches the security scheme name in OpenAPI
const AUTH_STORAGE_KEY = 'fumadocs-openapi-auth-Authorization';

export default defineClientConfig({
  playground: {
    requestTimeout: 30,
    // Transform auth inputs to inject stored token as default value
    transformAuthInputs: (fields) => {
      if (typeof window === 'undefined') return fields;

      const stored = localStorage.getItem(AUTH_STORAGE_KEY);
      if (!stored) return fields;

      try {
        const token = JSON.parse(stored);
        if (typeof token !== 'string' || !token.startsWith('Bearer ')) return fields;

        return fields.map(field => {
          // Match HTTP bearer auth fields
          if (field.fieldName === 'header.Authorization' &&
              field.original?.type === 'http') {
            return {
              ...field,
              defaultValue: token,
            };
          }
          return field;
        });
      } catch {
        return fields;
      }
    },
  },
});
