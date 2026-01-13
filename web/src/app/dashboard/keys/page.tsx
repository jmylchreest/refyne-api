'use client';

import { useEffect, useState } from 'react';
import { listApiKeys, createApiKey, revokeApiKey, ApiKey, ApiKeyWithSecret } from '@/lib/api';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog';
import { toast } from 'sonner';

function formatDate(dateString: string) {
  return new Date(dateString).toLocaleDateString();
}

export default function KeysPage() {
  const [keys, setKeys] = useState<ApiKey[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [isCreating, setIsCreating] = useState(false);
  const [newKeyName, setNewKeyName] = useState('');
  const [newKey, setNewKey] = useState<ApiKeyWithSecret | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);

  const loadKeys = async () => {
    try {
      const { keys: keyList } = await listApiKeys();
      setKeys(keyList || []);
    } catch (err) {
      const error = err as { error?: string };
      toast.error(error.error || 'Failed to load API keys');
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    loadKeys();
  }, []);

  const handleCreateKey = async () => {
    if (!newKeyName.trim()) {
      toast.error('Please enter a name for the API key');
      return;
    }

    setIsCreating(true);
    try {
      const key = await createApiKey(newKeyName);
      setNewKey(key);
      setNewKeyName('');
      loadKeys();
    } catch (err) {
      const error = err as { error?: string };
      toast.error(error.error || 'Failed to create API key');
    } finally {
      setIsCreating(false);
    }
  };

  const handleRevokeKey = async (id: string) => {
    if (!confirm('Are you sure you want to revoke this API key? This action cannot be undone.')) {
      return;
    }

    try {
      await revokeApiKey(id);
      toast.success('API key revoked');
      loadKeys();
    } catch (err) {
      const error = err as { error?: string };
      toast.error(error.error || 'Failed to revoke API key');
    }
  };

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text);
    toast.success('Copied to clipboard');
  };

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-zinc-900 dark:border-white" />
      </div>
    );
  }

  return (
    <div className="max-w-4xl">
      <div className="mb-8 flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">API Keys</h1>
          <p className="mt-2 text-zinc-600 dark:text-zinc-400">
            Manage your API keys for programmatic access.
          </p>
        </div>
        <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
          <DialogTrigger asChild>
            <Button onClick={() => { setNewKey(null); setDialogOpen(true); }}>
              Create API Key
            </Button>
          </DialogTrigger>
          <DialogContent>
            {newKey ? (
              <>
                <DialogHeader>
                  <DialogTitle>API Key Created</DialogTitle>
                  <DialogDescription>
                    Copy your API key now. You won&apos;t be able to see it again.
                  </DialogDescription>
                </DialogHeader>
                <div className="space-y-4 py-4">
                  <div className="rounded-lg bg-zinc-950 p-4">
                    <code className="text-sm text-green-400 break-all">{newKey.key}</code>
                  </div>
                  <Button
                    variant="outline"
                    className="w-full"
                    onClick={() => copyToClipboard(newKey.key)}
                  >
                    Copy to Clipboard
                  </Button>
                </div>
                <DialogFooter>
                  <Button onClick={() => { setNewKey(null); setDialogOpen(false); }}>
                    Done
                  </Button>
                </DialogFooter>
              </>
            ) : (
              <>
                <DialogHeader>
                  <DialogTitle>Create API Key</DialogTitle>
                  <DialogDescription>
                    Give your API key a descriptive name so you can identify it later.
                  </DialogDescription>
                </DialogHeader>
                <div className="space-y-4 py-4">
                  <div className="space-y-2">
                    <Label htmlFor="keyName">Name</Label>
                    <Input
                      id="keyName"
                      placeholder="e.g., Production Server"
                      value={newKeyName}
                      onChange={(e) => setNewKeyName(e.target.value)}
                      disabled={isCreating}
                    />
                  </div>
                </div>
                <DialogFooter>
                  <Button variant="outline" onClick={() => setDialogOpen(false)}>
                    Cancel
                  </Button>
                  <Button onClick={handleCreateKey} disabled={isCreating}>
                    {isCreating ? 'Creating...' : 'Create'}
                  </Button>
                </DialogFooter>
              </>
            )}
          </DialogContent>
        </Dialog>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Your API Keys</CardTitle>
          <CardDescription>
            Use these keys to authenticate API requests. Keep them secret.
          </CardDescription>
        </CardHeader>
        <CardContent>
          {keys.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-12">
              <p className="text-zinc-500 dark:text-zinc-400 mb-4">No API keys yet</p>
              <p className="text-sm text-zinc-400 dark:text-zinc-500">
                Create an API key to get started with the API.
              </p>
            </div>
          ) : (
            <div className="space-y-4">
              {keys.map((key) => (
                <div
                  key={key.id}
                  className="flex items-center justify-between rounded-lg border border-zinc-200 dark:border-zinc-800 p-4"
                >
                  <div className="space-y-1">
                    <div className="flex items-center gap-2">
                      <p className="font-medium">{key.name}</p>
                      {key.scopes && key.scopes.length > 0 && (
                        <div className="flex gap-1">
                          {key.scopes.map((scope) => (
                            <Badge key={scope} variant="secondary" className="text-xs">
                              {scope}
                            </Badge>
                          ))}
                        </div>
                      )}
                    </div>
                    <p className="font-mono text-sm text-zinc-500">
                      {key.key_prefix}...
                    </p>
                    <div className="flex gap-4 text-xs text-zinc-400">
                      <span>Created {formatDate(key.created_at)}</span>
                      {key.last_used_at && (
                        <span>Last used {formatDate(key.last_used_at)}</span>
                      )}
                      {key.expires_at && (
                        <span>Expires {formatDate(key.expires_at)}</span>
                      )}
                    </div>
                  </div>
                  <Button
                    variant="destructive"
                    size="sm"
                    onClick={() => handleRevokeKey(key.id)}
                  >
                    Revoke
                  </Button>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      <Card className="mt-6">
        <CardHeader>
          <CardTitle>Usage</CardTitle>
          <CardDescription>
            How to use your API key
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="rounded-lg bg-zinc-950 p-4 overflow-auto">
            <pre className="text-sm text-zinc-300">
{`curl -X POST https://api.refyne.uk/api/v1/extract \\
  -H "Authorization: Bearer rf_your_api_key" \\
  -H "Content-Type: application/json" \\
  -d '{
    "url": "https://example.com/product",
    "schema": {
      "name": "Product",
      "fields": [
        {"name": "title", "type": "string"},
        {"name": "price", "type": "number"}
      ]
    }
  }'`}
            </pre>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
