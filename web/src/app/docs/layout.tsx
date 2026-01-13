import { DocsLayout } from 'fumadocs-ui/layouts/docs';
import { RootProvider } from 'fumadocs-ui/provider/next';
import type { ReactNode } from 'react';
import { source } from '@/lib/source';
import { SiteHeader } from '@/components/site-header';
import { PlaygroundAuthSync } from '@/components/playground-auth-sync';

export default function Layout({ children }: { children: ReactNode }) {
  return (
    <RootProvider>
      <PlaygroundAuthSync />
      <div className="flex flex-col min-h-screen">
        <SiteHeader />
        <DocsLayout
          tree={source.pageTree}
          nav={{
            enabled: false,
          }}
          sidebar={{
            defaultOpenLevel: 1,
          }}
          themeSwitch={{
            enabled: false,
          }}
        >
          {children}
        </DocsLayout>
      </div>
    </RootProvider>
  );
}
