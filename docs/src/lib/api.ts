// Simplified API client for docs site (only public endpoints)
const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL || 'https://api.refyne.uk';

// Pricing / Tier Limits (public endpoint)
export interface TierLimits {
  name: string;
  display_name: string;
  monthly_extractions: number;      // 0 = unlimited
  max_concurrent_jobs: number;      // 0 = unlimited
  max_pages_per_crawl: number;      // 0 = unlimited
  requests_per_minute: number;      // 0 = unlimited
  credit_allocation_usd: number;    // Monthly USD credit for premium models (0 = none)
  credit_rollover_months: number;   // -1 = never expires, 0 = current period, N = N additional periods
}

export async function listTierLimits(): Promise<{ tiers: TierLimits[] }> {
  const response = await fetch(`${API_BASE_URL}/api/v1/pricing/tiers`, {
    headers: {
      'Content-Type': 'application/json',
    },
  });

  if (!response.ok) {
    throw new Error(`Failed to fetch tier limits: ${response.status}`);
  }

  return response.json();
}
