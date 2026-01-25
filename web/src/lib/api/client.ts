// API Client - Base request function and authentication handling

import type { ApiError } from './types';

const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080';

// Token getter function - will be set by the auth hook
let getToken: (() => Promise<string | null>) | null = null;

export function setTokenGetter(getter: () => Promise<string | null>) {
  getToken = getter;
}

export function getTokenGetter() {
  return getToken;
}

export async function request<T>(
  method: string,
  path: string,
  body?: unknown,
  requireAuth = true
): Promise<T> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
  };

  if (requireAuth && getToken) {
    const token = await getToken();
    if (token) {
      headers['Authorization'] = `Bearer ${token}`;
    }
  }

  const response = await fetch(`${API_BASE_URL}${path}`, {
    method,
    headers,
    body: body ? JSON.stringify(body) : undefined,
  });

  if (!response.ok) {
    // Provide helpful error messages for common HTTP errors
    const statusMessages: Record<number, string> = {
      400: 'Invalid request - please check your input',
      401: 'Authentication required - please sign in',
      403: 'Access denied - you may not have permission for this action',
      404: 'Resource not found',
      408: 'Request timed out - please try again',
      429: 'Too many requests - please wait and try again',
      500: 'Server error - please try again later',
      502: 'Service temporarily unavailable - the extraction provider may be down',
      503: 'Service unavailable - please try again later',
      504: 'Request timed out - extraction took too long. Try a simpler schema or check if the URL is accessible',
    };

    const error = await response.json().catch(() => ({}));
    const errorMessage = error.error || error.message || statusMessages[response.status] || `Request failed (${response.status})`;
    throw { error: errorMessage, status: response.status, ...error } as ApiError;
  }

  return response.json();
}

export { API_BASE_URL };
