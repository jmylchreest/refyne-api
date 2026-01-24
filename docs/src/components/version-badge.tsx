'use client';

import { latestVersion, versions } from '@/lib/versions';
import Link from 'next/link';

export function VersionBadge() {
  const hasMultipleVersions = versions.length > 1;

  return (
    <div className="mb-4 flex items-center justify-between rounded-lg border bg-fd-card/50 px-3 py-2">
      <div className="flex items-center gap-2">
        <span className="text-xs text-fd-muted-foreground">Version</span>
        <span className="rounded bg-fd-primary/10 px-2 py-0.5 text-xs font-medium text-fd-primary">
          {latestVersion.name.replace('Latest ', '').replace(/[()]/g, '')}
        </span>
      </div>
      {hasMultipleVersions && (
        <Link
          href="/versions"
          className="text-xs text-fd-muted-foreground hover:text-fd-foreground"
        >
          All versions
        </Link>
      )}
    </div>
  );
}
