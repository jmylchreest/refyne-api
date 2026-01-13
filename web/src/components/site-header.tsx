'use client';

import Link from 'next/link';
import { SignedIn, SignedOut, UserButton } from '@clerk/nextjs';
import { RefyneLogo } from '@/components/refyne-logo';
import { HeaderThemeToggle } from '@/components/header-theme-toggle';
import { cn } from '@/lib/utils';
import { useEffect, useState } from 'react';

interface SiteHeaderProps {
  fixed?: boolean;
}

// Pill-style button matching the theme toggle design
function PillButton({
  children,
  variant = 'default'
}: {
  children: React.ReactNode;
  variant?: 'default' | 'muted';
}) {
  return (
    <span className={cn(
      "flex items-center gap-1.5 rounded-full px-3 py-1 text-sm font-medium transition-all cursor-pointer",
      variant === 'default'
        ? "bg-secondary text-foreground hover:bg-secondary/80"
        : "text-foreground/50 hover:text-foreground/70 hover:bg-secondary/50"
    )}>
      {children}
    </span>
  );
}

export function SiteHeader({ fixed = false }: SiteHeaderProps) {
  // Prevent hydration mismatch by deferring auth UI until mounted
  const [mounted, setMounted] = useState(false);

  useEffect(() => {
    setMounted(true);
  }, []);

  return (
    <header className={cn(
      "border-b border-zinc-200/50 dark:border-zinc-800/50 backdrop-blur-sm bg-zinc-50/80 dark:bg-zinc-950/80 z-50",
      fixed ? "fixed top-0 left-0 right-0" : "sticky top-0"
    )}>
      <div className="relative flex h-16 items-center justify-between px-4">
        <Link href="/" className="flex items-baseline">
          <RefyneLogo size="md" />
          <span className="font-[family-name:var(--font-code)] text-[10px] font-semibold text-indigo-500 -translate-y-2 ml-0.5">
            BETA
          </span>
        </Link>
        <nav className="hidden md:flex items-center gap-8 absolute left-1/2 -translate-x-1/2">
          <Link href="/#capabilities" className="text-sm text-zinc-600 hover:text-zinc-900 dark:text-zinc-400 dark:hover:text-white transition-colors">
            Capabilities
          </Link>
          <Link href="/#pricing" className="text-sm text-zinc-600 hover:text-zinc-900 dark:text-zinc-400 dark:hover:text-white transition-colors">
            Pricing
          </Link>
          <Link href="/docs" className="text-sm text-zinc-600 hover:text-zinc-900 dark:text-zinc-400 dark:hover:text-white transition-colors">
            Docs
          </Link>
        </nav>
        <div className="flex items-center gap-2">
          <HeaderThemeToggle />
          {mounted && (
            <>
              <SignedOut>
                <Link href="/sign-up">
                  <PillButton>Sign Up</PillButton>
                </Link>
                <Link href="/sign-in">
                  <PillButton variant="muted">Log In</PillButton>
                </Link>
              </SignedOut>
              <SignedIn>
                <Link href="/dashboard">
                  <PillButton>Dashboard</PillButton>
                </Link>
                <UserButton afterSignOutUrl="/" />
              </SignedIn>
            </>
          )}
        </div>
      </div>
    </header>
  );
}
