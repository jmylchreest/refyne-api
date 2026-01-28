'use client';

import { cn } from '@/lib/utils';

export type FormatType = 'yaml' | 'json';

interface FormatToggleProps {
  value: FormatType;
  onChange: (format: FormatType) => void;
  disabled?: boolean;
  className?: string;
}

export function FormatToggle({
  value,
  onChange,
  disabled = false,
  className,
}: FormatToggleProps) {
  return (
    <div
      className={cn(
        'inline-flex items-center rounded-md bg-zinc-100 dark:bg-zinc-800 p-0.5',
        disabled && 'opacity-50 pointer-events-none',
        className
      )}
    >
      <button
        type="button"
        onClick={() => onChange('yaml')}
        className={cn(
          'px-2.5 py-1 text-xs font-medium rounded transition-all duration-150',
          value === 'yaml'
            ? 'bg-white dark:bg-zinc-700 text-zinc-900 dark:text-zinc-100 shadow-sm'
            : 'text-zinc-500 dark:text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-300'
        )}
        disabled={disabled}
      >
        YAML
      </button>
      <button
        type="button"
        onClick={() => onChange('json')}
        className={cn(
          'px-2.5 py-1 text-xs font-medium rounded transition-all duration-150',
          value === 'json'
            ? 'bg-white dark:bg-zinc-700 text-zinc-900 dark:text-zinc-100 shadow-sm'
            : 'text-zinc-500 dark:text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-300'
        )}
        disabled={disabled}
      >
        JSON
      </button>
    </div>
  );
}
