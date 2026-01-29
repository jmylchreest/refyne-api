'use client';

import { useEffect } from 'react';
import Link from 'next/link';
import { VersionInfo } from '@/components/version-info';

export default function Error({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    // Log the error to an error reporting service
    console.error(error);
  }, [error]);

  return (
    <div className="min-h-screen bg-zinc-50 dark:bg-zinc-950 flex flex-col items-center justify-center px-4">
      <div className="text-center">
        <h1 className="text-6xl font-bold text-zinc-900 dark:text-white mb-4">Error</h1>
        <h2 className="text-xl font-medium text-zinc-600 dark:text-zinc-400 mb-6">
          Something went wrong
        </h2>
        <p className="text-zinc-500 dark:text-zinc-500 mb-2 max-w-md">
          An unexpected error occurred. Please try again.
        </p>
        {error.digest && (
          <p className="text-xs text-zinc-400 dark:text-zinc-600 mb-8 font-mono">
            Error ID: {error.digest}
          </p>
        )}
        <div className="flex gap-4 justify-center">
          <button
            onClick={() => reset()}
            className="inline-flex items-center justify-center rounded-md bg-zinc-900 dark:bg-white px-4 py-2 text-sm font-medium text-white dark:text-zinc-900 hover:bg-zinc-800 dark:hover:bg-zinc-100 transition-colors"
          >
            Try again
          </button>
          <Link
            href="/"
            className="inline-flex items-center justify-center rounded-md border border-zinc-300 dark:border-zinc-700 px-4 py-2 text-sm font-medium text-zinc-700 dark:text-zinc-300 hover:bg-zinc-100 dark:hover:bg-zinc-800 transition-colors"
          >
            Go home
          </Link>
        </div>
      </div>

      {/* Version info at bottom center */}
      <div className="absolute bottom-4 left-0 right-0 flex justify-center">
        <VersionInfo showBuildTime />
      </div>
    </div>
  );
}
