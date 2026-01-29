'use client';

import { cn } from '@/lib/utils';

interface VersionInfoProps {
  className?: string;
  showBuildTime?: boolean;
}

export function VersionInfo({ className, showBuildTime = false }: VersionInfoProps) {
  const commit = process.env.NEXT_PUBLIC_GIT_COMMIT || 'dev';
  const tag = process.env.NEXT_PUBLIC_GIT_TAG;
  const buildTime = process.env.NEXT_PUBLIC_BUILD_TIME;

  // Display tag if available, otherwise commit hash
  const version = tag || commit;

  return (
    <div
      className={cn(
        'text-xs text-zinc-400 dark:text-zinc-600 select-none',
        className
      )}
      title={showBuildTime && buildTime ? `Built: ${new Date(buildTime).toLocaleString()}` : undefined}
    >
      {version}
    </div>
  );
}
