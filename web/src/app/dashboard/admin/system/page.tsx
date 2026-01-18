'use client';

import { useState } from 'react';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { syncTiers } from '@/lib/api';
import { RefreshCw, CheckCircle, AlertCircle } from 'lucide-react';

export default function AdminSystemPage() {
  const [syncing, setSyncing] = useState(false);
  const [syncResult, setSyncResult] = useState<{ success: boolean; message: string } | null>(null);

  const handleSyncTiers = async () => {
    setSyncing(true);
    setSyncResult(null);

    try {
      const result = await syncTiers();
      setSyncResult({ success: true, message: result.message });
    } catch (err) {
      const error = err as { error?: string };
      setSyncResult({ success: false, message: error.error || 'Failed to sync tiers' });
    } finally {
      setSyncing(false);
    }
  };

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle>Clerk Commerce Sync</CardTitle>
          <CardDescription>
            Manually sync tier metadata from Clerk Commerce. This updates tier display names
            and visibility based on the public availability setting in Clerk.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-center gap-4">
            <Button onClick={handleSyncTiers} disabled={syncing}>
              <RefreshCw className={`h-4 w-4 mr-2 ${syncing ? 'animate-spin' : ''}`} />
              {syncing ? 'Syncing...' : 'Sync Tiers from Clerk'}
            </Button>
          </div>

          {syncResult && (
            <Alert variant={syncResult.success ? 'default' : 'destructive'}>
              {syncResult.success ? (
                <CheckCircle className="h-4 w-4" />
              ) : (
                <AlertCircle className="h-4 w-4" />
              )}
              <AlertDescription>{syncResult.message}</AlertDescription>
            </Alert>
          )}

          <div className="text-sm text-zinc-500 dark:text-zinc-400 space-y-2">
            <p>
              <strong>When to use:</strong> After updating plan visibility or names in Clerk Commerce,
              use this to immediately reflect changes without restarting the API.
            </p>
            <p>
              <strong>Automatic sync:</strong> This also happens automatically on API startup and
              when Clerk sends <code className="text-xs bg-zinc-100 dark:bg-zinc-800 px-1 py-0.5 rounded">plan.created</code> or{' '}
              <code className="text-xs bg-zinc-100 dark:bg-zinc-800 px-1 py-0.5 rounded">plan.updated</code> webhooks.
            </p>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
