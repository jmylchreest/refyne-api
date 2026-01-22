'use client';

import { useAuth } from '@clerk/clerk-react';
import { useEffect, useRef } from 'react';

// Storage key matches the security scheme name in OpenAPI
const AUTH_STORAGE_KEY = 'fumadocs-openapi-auth-Authorization';

/**
 * Syncs Clerk JWT to the playground authorization field.
 * Uses MutationObserver to catch dynamically rendered inputs.
 */
export function PlaygroundAuthSync() {
  const { getToken, isSignedIn } = useAuth();
  const observerRef = useRef<MutationObserver | null>(null);
  const tokenRef = useRef<string | null>(null);

  useEffect(() => {
    if (!isSignedIn) {
      localStorage.removeItem(AUTH_STORAGE_KEY);
      tokenRef.current = null;
      return;
    }

    async function fetchAndApplyToken() {
      try {
        const token = await getToken();
        if (!token) return;

        const bearerToken = `Bearer ${token}`;
        tokenRef.current = bearerToken;

        // Store in localStorage for fumadocs
        localStorage.setItem(AUTH_STORAGE_KEY, JSON.stringify(bearerToken));

        // Apply to any existing inputs
        applyTokenToInputs(bearerToken);
      } catch (error) {
        console.error('Failed to sync auth token:', error);
      }
    }

    function applyTokenToInputs(token: string) {
      // Find authorization inputs by various selectors
      const inputs = document.querySelectorAll<HTMLInputElement>([
        'input[name="header.Authorization"]',
        'input[name*="Authorization"]',
        'input[placeholder*="Bearer"]',
      ].join(', '));

      inputs.forEach(input => {
        // Only update if it has the default "Bearer " value or is empty
        if (input.value === 'Bearer ' || input.value === 'Bearer' || input.value === '') {
          // Use native setter to bypass React's controlled input
          const nativeSetter = Object.getOwnPropertyDescriptor(
            window.HTMLInputElement.prototype,
            'value'
          )?.set;

          if (nativeSetter) {
            nativeSetter.call(input, token);
            // Dispatch events to notify React
            input.dispatchEvent(new Event('input', { bubbles: true }));
            input.dispatchEvent(new Event('change', { bubbles: true }));
          }
        }
      });
    }

    // Set up MutationObserver to watch for new inputs
    observerRef.current = new MutationObserver((mutations) => {
      if (!tokenRef.current) return;

      for (const mutation of mutations) {
        if (mutation.type === 'childList' && mutation.addedNodes.length > 0) {
          // Small delay to let React finish rendering
          setTimeout(() => {
            if (tokenRef.current) {
              applyTokenToInputs(tokenRef.current);
            }
          }, 100);
          break;
        }
      }
    });

    // Start observing
    observerRef.current.observe(document.body, {
      childList: true,
      subtree: true,
    });

    // Initial fetch
    fetchAndApplyToken();

    return () => {
      observerRef.current?.disconnect();
    };
  }, [getToken, isSignedIn]);

  return null;
}
