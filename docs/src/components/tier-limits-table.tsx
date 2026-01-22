'use client';

import { useEffect, useState } from 'react';
import { listTierLimits, TierLimits } from '@/lib/api';

function formatLimit(value: number, suffix?: string): string {
  if (value === 0) return 'Unlimited';
  return suffix ? `${value.toLocaleString()}${suffix}` : value.toLocaleString();
}

function formatCurrency(value: number): string {
  if (value === 0) return '-';
  return `$${value.toFixed(2)}`;
}

function formatRollover(months: number): string {
  if (months === -1) return 'Never expires';
  if (months === 0) return 'Current period';
  return `${months} month${months > 1 ? 's' : ''}`;
}

export function TierLimitsTable() {
  const [tiers, setTiers] = useState<TierLimits[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    async function fetchLimits() {
      try {
        const response = await listTierLimits();
        setTiers(response.tiers || []);
      } catch (err) {
        setError('Failed to load tier limits');
        console.error('Failed to fetch tier limits:', err);
      } finally {
        setLoading(false);
      }
    }
    fetchLimits();
  }, []);

  if (loading) {
    return (
      <div className="flex items-center justify-center py-8">
        <div className="animate-spin rounded-full h-6 w-6 border-b-2 border-zinc-500" />
      </div>
    );
  }

  if (error) {
    return (
      <div className="rounded-lg border border-red-200 dark:border-red-900 bg-red-50 dark:bg-red-950/20 p-4 text-sm text-red-600 dark:text-red-400">
        {error}
      </div>
    );
  }

  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-zinc-200 dark:border-zinc-800">
            <th className="text-left py-3 px-4 font-medium text-zinc-900 dark:text-zinc-100">Limit</th>
            {tiers.map((tier) => (
              <th key={tier.name} className="text-center py-3 px-4 font-medium text-zinc-900 dark:text-zinc-100">
                {tier.display_name}
              </th>
            ))}
          </tr>
        </thead>
        <tbody className="divide-y divide-zinc-200 dark:divide-zinc-800">
          <tr>
            <td className="py-3 px-4 text-zinc-600 dark:text-zinc-400">Monthly Extractions</td>
            {tiers.map((tier) => (
              <td key={tier.name} className="text-center py-3 px-4 font-mono text-zinc-900 dark:text-zinc-100">
                {formatLimit(tier.monthly_extractions)}
              </td>
            ))}
          </tr>
          <tr>
            <td className="py-3 px-4 text-zinc-600 dark:text-zinc-400">Max Pages per Crawl</td>
            {tiers.map((tier) => (
              <td key={tier.name} className="text-center py-3 px-4 font-mono text-zinc-900 dark:text-zinc-100">
                {formatLimit(tier.max_pages_per_crawl)}
              </td>
            ))}
          </tr>
          <tr>
            <td className="py-3 px-4 text-zinc-600 dark:text-zinc-400">Concurrent Jobs</td>
            {tiers.map((tier) => (
              <td key={tier.name} className="text-center py-3 px-4 font-mono text-zinc-900 dark:text-zinc-100">
                {formatLimit(tier.max_concurrent_jobs)}
              </td>
            ))}
          </tr>
          <tr>
            <td className="py-3 px-4 text-zinc-600 dark:text-zinc-400">API Rate Limit</td>
            {tiers.map((tier) => (
              <td key={tier.name} className="text-center py-3 px-4 font-mono text-zinc-900 dark:text-zinc-100">
                {formatLimit(tier.requests_per_minute, '/min')}
              </td>
            ))}
          </tr>
          <tr>
            <td className="py-3 px-4 text-zinc-600 dark:text-zinc-400">Monthly Credit</td>
            {tiers.map((tier) => (
              <td key={tier.name} className="text-center py-3 px-4 font-mono text-zinc-900 dark:text-zinc-100">
                {formatCurrency(tier.credit_allocation_usd)}
              </td>
            ))}
          </tr>
          <tr>
            <td className="py-3 px-4 text-zinc-600 dark:text-zinc-400">Credit Rollover</td>
            {tiers.map((tier) => (
              <td key={tier.name} className="text-center py-3 px-4 font-mono text-zinc-900 dark:text-zinc-100">
                {tier.credit_allocation_usd > 0 ? formatRollover(tier.credit_rollover_months) : '-'}
              </td>
            ))}
          </tr>
        </tbody>
      </table>
    </div>
  );
}
