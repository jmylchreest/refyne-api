'use client';

import dynamic from 'next/dynamic';
import type { ReactNode } from 'react';

// Dynamic imports with SSR disabled for Clerk-dependent components
// This prevents useAuth() from being called during static generation
const SiteHeader = dynamic(
  () => import('@/components/site-header').then(mod => mod.SiteHeader),
  { ssr: false }
);

const PlaygroundAuthSync = dynamic(
  () => import('@/components/playground-auth-sync').then(mod => mod.PlaygroundAuthSync),
  { ssr: false }
);

export function ClientProviders({ children }: { children: ReactNode }) {
  return (
    <>
      <PlaygroundAuthSync />
      <div className="flex flex-col min-h-screen">
        <SiteHeader />
        {children}
      </div>
    </>
  );
}
