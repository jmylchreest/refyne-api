import { DocsLayout } from 'fumadocs-ui/layouts/docs';
import { RootProvider } from 'fumadocs-ui/provider/next';
import type { ReactNode } from 'react';
import { source } from '@/lib/source';
import { ClientProviders } from '@/components/client-providers';
import { VersionBadge } from '@/components/version-badge';

export default function Layout({ children }: { children: ReactNode }) {
  return (
    <RootProvider>
      <ClientProviders>
        <DocsLayout
          tree={source.pageTree}
          nav={{
            enabled: false,
          }}
          sidebar={{
            defaultOpenLevel: 1,
            banner: <VersionBadge />,
          }}
          themeSwitch={{
            enabled: false,
          }}
        >
          {children}
        </DocsLayout>
      </ClientProviders>
    </RootProvider>
  );
}
