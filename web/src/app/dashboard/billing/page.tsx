'use client';

import { PricingTable } from '@clerk/nextjs';

export default function BillingPage() {
  return (
    <div className="max-w-5xl space-y-8">
      <div>
        <h1 className="text-3xl font-bold tracking-tight">Billing</h1>
        <p className="mt-2 text-zinc-600 dark:text-zinc-400">
          Manage your subscription and billing information.
        </p>
      </div>

      {/* Clerk Billing PricingTable */}
      <PricingTable
        newSubscriptionRedirectUrl="/dashboard/billing"
        checkoutProps={{
          appearance: {
            elements: {
              // Position drawer below the fixed header with even spacing
              // Header is 4rem, add 0.75rem padding top and bottom
              drawerRoot: {
                top: 'calc(4rem + 0.75rem)',
                height: 'calc(100vh - 4rem - 1.5rem)',
              },
              drawerContent: {
                top: 'calc(4rem + 0.75rem)',
                maxHeight: 'calc(100vh - 4rem - 1.5rem)',
                borderRadius: '0.5rem',
              },
              drawerBackdrop: {
                top: '4rem',
              },
            },
          },
        }}
        fallback={
          <div className="flex items-center justify-center py-12">
            <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-zinc-900 dark:border-white" />
          </div>
        }
      />
    </div>
  );
}
