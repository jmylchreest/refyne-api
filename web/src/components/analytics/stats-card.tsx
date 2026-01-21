'use client';

import { Card, CardContent } from '@/components/ui/card';
import { cn } from '@/lib/utils';

interface StatsCardProps {
  label: string;
  value: string | number;
  subValue?: string;
  trend?: 'up' | 'down' | 'neutral';
  className?: string;
}

export function StatsCard({ label, value, subValue, trend, className }: StatsCardProps) {
  return (
    <Card className={cn('', className)}>
      <CardContent className="p-4">
        <p className="text-xs font-medium text-zinc-500 dark:text-zinc-400 uppercase tracking-wide">
          {label}
        </p>
        <p className="mt-1 text-2xl font-bold text-zinc-900 dark:text-white">
          {typeof value === 'number' ? value.toLocaleString() : value}
        </p>
        {subValue && (
          <p className={cn(
            'mt-1 text-xs',
            trend === 'up' && 'text-green-600 dark:text-green-400',
            trend === 'down' && 'text-red-600 dark:text-red-400',
            trend === 'neutral' && 'text-zinc-500 dark:text-zinc-400',
            !trend && 'text-zinc-500 dark:text-zinc-400'
          )}>
            {subValue}
          </p>
        )}
      </CardContent>
    </Card>
  );
}
