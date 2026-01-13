'use client';

import { cn } from '@/lib/utils';

interface SectionTabProps {
  label: string;
  variant?: 'default' | 'primary' | 'accent';
  className?: string;
}

/**
 * IDE-tab-style section label that sits above a container.
 * Styled like an editor tab with an angled edge on the right.
 */
export function SectionTab({ label, variant = 'default', className }: SectionTabProps) {
  const variants = {
    default: {
      bg: 'bg-zinc-100 dark:bg-zinc-800',
      text: 'text-zinc-600 dark:text-zinc-400',
      border: 'border-zinc-200 dark:border-zinc-700',
    },
    primary: {
      bg: 'bg-indigo-100 dark:bg-indigo-900/40',
      text: 'text-indigo-700 dark:text-indigo-300',
      border: 'border-indigo-200 dark:border-indigo-800',
    },
    accent: {
      bg: 'bg-emerald-100 dark:bg-emerald-900/40',
      text: 'text-emerald-700 dark:text-emerald-300',
      border: 'border-emerald-200 dark:border-emerald-800',
    },
  };

  const v = variants[variant];

  return (
    <div className={cn('relative inline-flex', className)}>
      {/* Main tab body */}
      <div
        className={cn(
          'relative flex items-center h-7 pl-3 pr-4 text-xs font-semibold uppercase tracking-wider',
          'border border-b-0 rounded-t-md',
          v.bg,
          v.text,
          v.border
        )}
        style={{
          clipPath: 'polygon(0 0, calc(100% - 12px) 0, 100% 100%, 0 100%)',
        }}
      >
        {label}
      </div>
      {/* Angled edge overlay - creates the slant */}
      <div
        className={cn(
          'absolute right-0 top-0 h-full w-4',
          v.bg
        )}
        style={{
          clipPath: 'polygon(0 0, 100% 100%, 0 100%)',
          transform: 'translateX(calc(100% - 12px))',
        }}
      />
    </div>
  );
}

/**
 * Container that pairs with SectionTab for a complete IDE-like section.
 * Use the tab prop to include an attached tab label.
 */
interface SectionContainerProps {
  tab?: string;
  tabVariant?: 'default' | 'primary' | 'accent';
  children: React.ReactNode;
  className?: string;
}

export function SectionContainer({
  tab,
  tabVariant = 'default',
  children,
  className,
}: SectionContainerProps) {
  const borderColors = {
    default: 'border-zinc-200 dark:border-zinc-700',
    primary: 'border-indigo-200 dark:border-indigo-800',
    accent: 'border-emerald-200 dark:border-emerald-800',
  };

  return (
    <div className="relative">
      {tab && (
        <div className="relative z-10">
          <SectionTab label={tab} variant={tabVariant} />
        </div>
      )}
      <div
        className={cn(
          'rounded-lg border bg-white dark:bg-zinc-900 shadow-sm',
          tab && 'rounded-tl-none -mt-px',
          tab ? borderColors[tabVariant] : 'border-zinc-200 dark:border-zinc-700',
          className
        )}
      >
        {children}
      </div>
    </div>
  );
}

/**
 * Simple inline label styled like the hero visualization labels.
 * All uppercase, muted styling.
 */
interface InlineLabelProps {
  children: React.ReactNode;
  variant?: 'muted' | 'accent';
  className?: string;
}

export function InlineLabel({ children, variant = 'muted', className }: InlineLabelProps) {
  return (
    <span
      className={cn(
        'text-xs uppercase tracking-wider font-medium',
        variant === 'muted' && 'text-zinc-500 dark:text-zinc-400',
        variant === 'accent' && 'text-indigo-600 dark:text-indigo-400',
        className
      )}
    >
      {children}
    </span>
  );
}
