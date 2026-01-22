'use client';

import { ClerkProvider as BaseClerkProvider } from '@clerk/clerk-react';
import type { ReactNode } from 'react';

const PUBLISHABLE_KEY = process.env.NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY;

export function ClerkProvider({ children }: { children: ReactNode }) {
  if (!PUBLISHABLE_KEY) {
    // In development without Clerk key, just render children
    return <>{children}</>;
  }

  return (
    <BaseClerkProvider publishableKey={PUBLISHABLE_KEY}>
      {children}
    </BaseClerkProvider>
  );
}
