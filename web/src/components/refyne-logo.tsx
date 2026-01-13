'use client';

import { cn } from '@/lib/utils';

interface RefyneLogoProps {
  className?: string;
  size?: 'sm' | 'md' | 'lg' | 'xl';
}

/**
 * Inline text component for rendering "Refyne" with proper cursive styling.
 * Use this anywhere you need the Refyne brand name in text.
 * Rendered in Victor Mono italic for distinctive branding.
 */
export function RefyneText({ className }: { className?: string }) {
  return (
    <span className={cn('font-bold refyne-cursive', className)}>
      Refyne
    </span>
  );
}

/**
 * Full logo component - just the styled "Refyne" text.
 * Use this for headers, navigation, and prominent branding.
 */
export function RefyneLogo({ className, size = 'md' }: RefyneLogoProps) {
  const sizeClasses = {
    sm: 'text-lg',
    md: 'text-xl',
    lg: 'text-2xl',
    xl: 'text-4xl',
  };

  return (
    <span className={cn('font-bold tracking-tight refyne-cursive', sizeClasses[size], className)}>
      Refyne
    </span>
  );
}
