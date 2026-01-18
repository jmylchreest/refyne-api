'use client';

import { usePathname } from 'next/navigation';
import Link from 'next/link';
import { useUser } from '@clerk/nextjs';
import { cn } from '@/lib/utils';
import { Card, CardContent } from '@/components/ui/card';

const adminTabs = [
  { name: 'Extractors', href: '/dashboard/admin/extractors' },
  { name: 'Schema Catalog', href: '/dashboard/admin/schemas' },
  { name: 'System', href: '/dashboard/admin/system' },
  { name: 'Debug', href: '/dashboard/admin/debug' },
];

export default function AdminLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const pathname = usePathname();
  const { user, isLoaded } = useUser();

  const isSuperadmin = user?.publicMetadata?.global_superadmin === true;

  if (!isLoaded) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-zinc-900 dark:border-white" />
      </div>
    );
  }

  if (!isSuperadmin) {
    return (
      <div className="max-w-4xl">
        <Card>
          <CardContent className="flex flex-col items-center justify-center py-12">
            <p className="text-zinc-500 dark:text-zinc-400 mb-4">Access Denied</p>
            <p className="text-sm text-zinc-400 dark:text-zinc-500">
              This page is only accessible to global superadmins.
            </p>
          </CardContent>
        </Card>
      </div>
    );
  }

  return (
    <div className="max-w-5xl">
      <div className="mb-8">
        <h1 className="text-3xl font-bold tracking-tight">Admin Settings</h1>
        <p className="mt-2 text-zinc-600 dark:text-zinc-400">
          Configure system-wide settings, LLM providers, and platform schemas.
        </p>
      </div>

      {/* Admin sub-navigation tabs */}
      <div className="border-b border-zinc-200 dark:border-zinc-800 mb-6">
        <nav className="flex gap-4">
          {adminTabs.map((tab) => {
            const isActive = pathname === tab.href || pathname.startsWith(tab.href + '/');
            return (
              <Link
                key={tab.name}
                href={tab.href}
                className={cn(
                  'px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors',
                  isActive
                    ? 'border-zinc-900 text-zinc-900 dark:border-white dark:text-white'
                    : 'border-transparent text-zinc-500 hover:text-zinc-700 hover:border-zinc-300 dark:text-zinc-400 dark:hover:text-zinc-300'
                )}
              >
                {tab.name}
              </Link>
            );
          })}
        </nav>
      </div>

      {children}
    </div>
  );
}
