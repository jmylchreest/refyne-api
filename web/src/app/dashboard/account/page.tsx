'use client';

import { useEffect, useState, useCallback } from 'react';
import { getUsage, UsageSummary } from '@/lib/api';
import { useUser, useClerk } from '@clerk/nextjs';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { toast } from 'sonner';
import { AlertTriangle } from 'lucide-react';

export default function AccountPage() {
  const { user } = useUser();
  const { signOut } = useClerk();
  const [isLoading, setIsLoading] = useState(true);
  const [usage, setUsage] = useState<UsageSummary | null>(null);
  const [showCloseDialog, setShowCloseDialog] = useState(false);
  const [isDeleting, setIsDeleting] = useState(false);
  const [confirmText, setConfirmText] = useState('');

  const loadData = useCallback(async () => {
    try {
      const usageData = await getUsage();
      setUsage(usageData);
    } catch (err) {
      const error = err as { error?: string };
      toast.error(error.error || 'Failed to load usage data');
    } finally {
      setIsLoading(false);
    }
  }, []);

  useEffect(() => {
    loadData();
  }, [loadData]);

  const handleCloseAccount = async () => {
    if (confirmText !== 'DELETE') {
      toast.error('Please type DELETE to confirm');
      return;
    }

    setIsDeleting(true);
    try {
      // Delete the Clerk user account (this will cascade delete our data via webhooks)
      await user?.delete();
      await signOut();
      toast.success('Account deleted successfully');
    } catch (err) {
      const error = err as { message?: string };
      toast.error(error.message || 'Failed to delete account');
    } finally {
      setIsDeleting(false);
      setShowCloseDialog(false);
    }
  };

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-zinc-900 dark:border-white" />
      </div>
    );
  }

  return (
    <div className="max-w-4xl space-y-6">
      <div>
        <h1 className="text-2xl font-bold tracking-tight">Account</h1>
        <p className="text-sm text-zinc-600 dark:text-zinc-400">
          Manage your account settings and view usage statistics.
        </p>
      </div>

      {/* Usage Summary */}
      {usage && (
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-base">Usage This Month</CardTitle>
            <CardDescription className="text-xs">
              Your extraction and crawl usage for the current billing period.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="grid grid-cols-3 gap-3">
              <div className="rounded-md border p-3">
                <p className="text-xs text-zinc-500">Total Jobs</p>
                <p className="text-xl font-bold">{usage.total_jobs}</p>
              </div>
              <div className="rounded-md border p-3">
                <p className="text-xs text-zinc-500">Total Cost</p>
                <p className="text-xl font-bold">${usage.total_charged_usd.toFixed(4)}</p>
              </div>
              <div className="rounded-md border p-3">
                <p className="text-xs text-zinc-500">BYOK Jobs</p>
                <p className="text-xl font-bold">{usage.byok_jobs}</p>
              </div>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Account Information */}
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-base">Account Information</CardTitle>
          <CardDescription className="text-xs">
            Your account details. To update your profile, click your avatar in the top right of this window and select &quot;Manage Account&quot;.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-2 gap-3">
            <div className="space-y-1">
              <Label className="text-xs">Email</Label>
              <Input value={user?.primaryEmailAddress?.emailAddress || ''} disabled className="h-8 text-sm" />
            </div>
            <div className="space-y-1">
              <Label className="text-xs">Name</Label>
              <Input value={user?.fullName || ''} disabled className="h-8 text-sm" />
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Danger Zone */}
      <Card className="border-red-200 dark:border-red-900/50">
        <CardHeader className="pb-3">
          <CardTitle className="text-base text-red-600 dark:text-red-400 flex items-center gap-2">
            <AlertTriangle className="h-4 w-4" />
            Danger Zone
          </CardTitle>
          <CardDescription className="text-xs">
            Irreversible actions that affect your account.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex items-center justify-between p-3 border border-red-200 dark:border-red-900/50 rounded-md">
            <div>
              <p className="text-sm font-medium">Delete Account</p>
              <p className="text-xs text-zinc-500">
                Permanently delete your account and all associated data including jobs, API keys, and configurations.
              </p>
            </div>
            <Button
              variant="destructive"
              size="sm"
              onClick={() => setShowCloseDialog(true)}
            >
              Delete Account
            </Button>
          </div>
        </CardContent>
      </Card>

      {/* Delete Confirmation Dialog */}
      <Dialog open={showCloseDialog} onOpenChange={setShowCloseDialog}>
        <DialogContent showCloseButton={false}>
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2 text-red-600">
              <AlertTriangle className="h-5 w-5" />
              Delete Account
            </DialogTitle>
            <DialogDescription asChild>
              <div className="space-y-3">
                <p>
                  This action cannot be undone. This will permanently delete your account and all associated data:
                </p>
                <ul className="list-disc list-inside text-sm space-y-1">
                  <li>All extraction and crawl job history</li>
                  <li>All API keys and configurations</li>
                  <li>All LLM provider keys and model settings</li>
                  <li>All webhook configurations</li>
                  <li>All saved schemas and sites</li>
                </ul>
                <div className="pt-2">
                  <Label className="text-xs font-medium">
                    Type <span className="font-mono font-bold">DELETE</span> to confirm:
                  </Label>
                  <Input
                    value={confirmText}
                    onChange={(e) => setConfirmText(e.target.value)}
                    placeholder="DELETE"
                    className="mt-1.5 h-8 text-sm"
                  />
                </div>
              </div>
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => { setShowCloseDialog(false); setConfirmText(''); }}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={handleCloseAccount}
              disabled={isDeleting || confirmText !== 'DELETE'}
            >
              {isDeleting ? 'Deleting...' : 'Yes, delete my account'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
