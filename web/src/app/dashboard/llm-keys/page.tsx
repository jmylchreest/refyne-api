'use client';

import { useEffect, useState, useCallback } from 'react';
import {
  listUserServiceKeys,
  upsertUserServiceKey,
  deleteUserServiceKey,
  listLLMProviders,
  UserServiceKey,
  LLMProvider,
  UserServiceKeyInput,
} from '@/lib/api';
import { useAuth } from '@clerk/nextjs';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { toast } from 'sonner';
import { Trash2, Eye, EyeOff, Lock } from 'lucide-react';
import { parseClerkFeatures } from '@/lib/utils';

export default function LLMKeysPage() {
  const { sessionClaims } = useAuth();
  const [isLoading, setIsLoading] = useState(true);

  // Check if BYOK feature is enabled for this user
  const features = parseClerkFeatures(
    sessionClaims?.fea as string | undefined,
    sessionClaims?.public_metadata as Record<string, unknown> | undefined
  );
  const byokEnabled = features.includes('provider_byok');

  // Dynamic providers from API
  const [providers, setProviders] = useState<LLMProvider[]>([]);

  // Provider keys state
  const [providerKeys, setProviderKeys] = useState<UserServiceKey[]>([]);
  const [editingProvider, setEditingProvider] = useState<string | null>(null);
  const [formData, setFormData] = useState<{ apiKey: string; baseUrl: string }>({ apiKey: '', baseUrl: '' });
  const [showApiKey, setShowApiKey] = useState(false);
  const [isSavingKey, setIsSavingKey] = useState(false);

  const loadData = useCallback(async () => {
    try {
      const [keysData, providersData] = await Promise.all([
        listUserServiceKeys(),
        listLLMProviders(),
      ]);
      setProviderKeys(keysData.keys || []);
      setProviders(providersData.providers || []);
    } catch (err) {
      const error = err as { error?: string };
      toast.error(error.error || 'Failed to load provider data');
    } finally {
      setIsLoading(false);
    }
  }, []);

  useEffect(() => {
    loadData();
  }, [loadData]);

  const getProviderLabel = (providerName: string) => {
    const p = providers.find(lp => lp.name === providerName);
    return p ? p.display_name : providerName;
  };

  const getKeyForProvider = (providerName: string) => providerKeys.find(k => k.provider === providerName);

  // Provider key handlers
  const startEditing = (providerName: string) => {
    if (!byokEnabled) return;
    const existing = getKeyForProvider(providerName);
    setEditingProvider(providerName);
    setFormData({
      apiKey: '',
      baseUrl: existing?.base_url || '',
    });
    setShowApiKey(false);
  };

  const handleSaveKey = async (providerName: string) => {
    setIsSavingKey(true);
    try {
      await upsertUserServiceKey({
        provider: providerName as UserServiceKeyInput['provider'],
        api_key: formData.apiKey || undefined,
        base_url: formData.baseUrl || undefined,
        is_enabled: true,
      });
      toast.success(`${getProviderLabel(providerName)} key saved`);
      setEditingProvider(null);
      setFormData({ apiKey: '', baseUrl: '' });
      loadData();
    } catch (err) {
      const error = err as { error?: string };
      toast.error(error.error || 'Failed to save key');
    } finally {
      setIsSavingKey(false);
    }
  };

  const handleDeleteKey = async (key: UserServiceKey) => {
    if (!confirm(`Delete ${getProviderLabel(key.provider)} API key?`)) return;
    try {
      await deleteUserServiceKey(key.id);
      toast.success(`${getProviderLabel(key.provider)} key deleted`);
      loadData();
    } catch (err) {
      const error = err as { error?: string };
      toast.error(error.error || 'Failed to delete key');
    }
  };

  const handleToggleKey = async (key: UserServiceKey) => {
    try {
      await upsertUserServiceKey({
        provider: key.provider as UserServiceKeyInput['provider'],
        is_enabled: !key.is_enabled,
      });
      toast.success(`${getProviderLabel(key.provider)} ${key.is_enabled ? 'disabled' : 'enabled'}`);
      loadData();
    } catch {
      toast.error('Failed to update');
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
        <h1 className="text-2xl font-bold tracking-tight">LLM Provider Keys</h1>
        <p className="text-sm text-zinc-600 dark:text-zinc-400">
          Configure your own API keys for LLM providers (BYOK). Your keys are encrypted at rest.
        </p>
      </div>

      {/* Feature Gate Alert */}
      {!byokEnabled && (
        <Alert>
          <Lock className="h-4 w-4" />
          <AlertDescription className="text-sm">
            <span className="font-medium">BYOK (Bring Your Own Key)</span> is available on paid plans.
            With BYOK, you can use your own API keys to avoid per-extraction charges and get direct access to your preferred LLM providers.{' '}
            <a href="/dashboard/billing" className="underline font-medium hover:text-zinc-900 dark:hover:text-zinc-100">
              Upgrade to enable this feature.
            </a>
            {providerKeys.length > 0 && (
              <span className="block mt-2 text-zinc-500">
                Your saved keys will be restored when you upgrade.
              </span>
            )}
          </AlertDescription>
        </Alert>
      )}

      {/* Provider Keys */}
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-base">Provider API Keys</CardTitle>
          <CardDescription className="text-xs">
            Add your API keys to use your own LLM provider accounts. Keys can be individually enabled or disabled.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          {providers.length === 0 ? (
            <p className="text-sm text-zinc-500">No providers available.</p>
          ) : (
            providers.map((provider) => {
              const key = getKeyForProvider(provider.name);
              const isEditing = editingProvider === provider.name;

              return (
                <div
                  key={provider.name}
                  className={`border rounded-md p-3 transition-opacity ${!byokEnabled ? 'opacity-60' : ''}`}
                >
                  <div className="flex items-center justify-between">
                    <div className="flex-1">
                      <div className="flex items-center gap-2">
                        <span className="font-medium text-sm">{provider.display_name}</span>
                        {key ? (
                          <Badge variant={key.is_enabled ? 'default' : 'secondary'} className="text-xs">
                            {key.is_enabled ? 'Enabled' : 'Disabled'}
                          </Badge>
                        ) : (
                          <Badge variant="outline" className="text-xs">Not configured</Badge>
                        )}
                      </div>
                      <p className="text-xs text-zinc-500 mt-0.5">{provider.description}</p>
                    </div>
                    {!isEditing && (
                      <div className="flex gap-1">
                        <Button
                          variant="outline"
                          size="sm"
                          className="h-7 text-xs"
                          onClick={() => startEditing(provider.name)}
                          disabled={!byokEnabled}
                        >
                          {key ? 'Update' : 'Configure'}
                        </Button>
                        {key && byokEnabled && (
                          <>
                            <Button
                              variant="ghost"
                              size="sm"
                              className="h-7 text-xs"
                              onClick={() => handleToggleKey(key)}
                            >
                              {key.is_enabled ? 'Disable' : 'Enable'}
                            </Button>
                            <Button
                              variant="ghost"
                              size="sm"
                              className="h-7 px-2"
                              onClick={() => handleDeleteKey(key)}
                            >
                              <Trash2 className="h-3 w-3 text-red-500" />
                            </Button>
                          </>
                        )}
                      </div>
                    )}
                  </div>

                  {key && !isEditing && key.base_url && (
                    <p className="text-xs text-zinc-500 mt-1">Base URL: {key.base_url}</p>
                  )}

                  {isEditing && byokEnabled && (
                    <div className="mt-3 space-y-3">
                      {provider.requires_key && (
                        <div className="space-y-1">
                          <Label className="text-xs">API Key</Label>
                          <div className="relative">
                            <Input
                              type={showApiKey ? 'text' : 'password'}
                              placeholder={key?.has_key ? '(unchanged)' : (provider.key_placeholder || 'Enter API key...')}
                              value={formData.apiKey}
                              onChange={(e) => setFormData({ ...formData, apiKey: e.target.value })}
                              className="h-8 text-sm pr-8"
                            />
                            <button
                              type="button"
                              onClick={() => setShowApiKey(!showApiKey)}
                              className="absolute right-2 top-1/2 -translate-y-1/2 text-zinc-500 hover:text-zinc-700"
                            >
                              {showApiKey ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                            </button>
                          </div>
                        </div>
                      )}
                      {provider.base_url_hint && (
                        <div className="space-y-1">
                          <Label className="text-xs">Base URL</Label>
                          <Input
                            placeholder={provider.base_url_hint}
                            value={formData.baseUrl}
                            onChange={(e) => setFormData({ ...formData, baseUrl: e.target.value })}
                            className="h-8 text-sm"
                          />
                        </div>
                      )}
                      <div className="flex gap-2">
                        <Button size="sm" className="h-7 text-xs" onClick={() => handleSaveKey(provider.name)} disabled={isSavingKey}>
                          {isSavingKey ? 'Saving...' : 'Save'}
                        </Button>
                        <Button variant="outline" size="sm" className="h-7 text-xs" onClick={() => setEditingProvider(null)}>
                          Cancel
                        </Button>
                      </div>
                    </div>
                  )}
                </div>
              );
            })
          )}
        </CardContent>
      </Card>

      {/* Help Section */}
      {providers.length > 0 && (
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-base">Getting API Keys</CardTitle>
          </CardHeader>
          <CardContent className="text-sm space-y-2 text-zinc-600 dark:text-zinc-400">
            {providers.map((provider) => (
              <p key={provider.name}>
                <strong>{provider.display_name}:</strong>{' '}
                {provider.docs_url ? (
                  <>
                    Visit <a href={provider.docs_url} target="_blank" rel="noopener noreferrer" className="underline">{new URL(provider.docs_url).hostname}</a>
                    {provider.requires_key ? ' to create an API key.' : '. No API key required.'}
                  </>
                ) : (
                  provider.requires_key ? 'API key required.' : 'No API key required.'
                )}
              </p>
            ))}
          </CardContent>
        </Card>
      )}
    </div>
  );
}
