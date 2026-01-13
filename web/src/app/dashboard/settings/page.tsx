'use client';

import { useEffect, useState, useCallback } from 'react';
import {
  getUsage,
  listUserServiceKeys,
  upsertUserServiceKey,
  deleteUserServiceKey,
  getUserFallbackChain,
  setUserFallbackChain,
  UsageSummary,
  UserServiceKey,
  UserFallbackChainEntry,
  UserFallbackChainEntryInput,
} from '@/lib/api';
import { useUser } from '@clerk/nextjs';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { ModelSelector } from '@/components/model-selector';
import { toast } from 'sonner';
import { Plus, Trash2, ChevronUp, ChevronDown, Eye, EyeOff } from 'lucide-react';

const LLM_PROVIDERS = [
  { value: 'openrouter', label: 'OpenRouter' },
  { value: 'anthropic', label: 'Anthropic' },
  { value: 'openai', label: 'OpenAI' },
  { value: 'ollama', label: 'Ollama' },
] as const;

const DEFAULT_MODELS: Record<string, string> = {
  openrouter: 'google/gemini-2.0-flash-001',
  anthropic: 'claude-sonnet-4-5-20250514',
  openai: 'gpt-4o-mini',
  ollama: 'llama3.2',
};

type Provider = typeof LLM_PROVIDERS[number]['value'];

export default function SettingsPage() {
  const { user } = useUser();
  const [isLoading, setIsLoading] = useState(true);
  const [usage, setUsage] = useState<UsageSummary | null>(null);

  // Provider keys state
  const [providerKeys, setProviderKeys] = useState<UserServiceKey[]>([]);
  const [editingProvider, setEditingProvider] = useState<string | null>(null);
  const [formData, setFormData] = useState<{ apiKey: string; baseUrl: string }>({ apiKey: '', baseUrl: '' });
  const [showApiKey, setShowApiKey] = useState(false);
  const [isSavingKey, setIsSavingKey] = useState(false);

  // Fallback chain state
  const [chain, setChain] = useState<UserFallbackChainEntry[]>([]);
  const [isEditingChain, setIsEditingChain] = useState(false);
  const [chainDraft, setChainDraft] = useState<UserFallbackChainEntryInput[]>([]);
  const [isSavingChain, setIsSavingChain] = useState(false);

  const loadData = useCallback(async () => {
    try {
      const [usageData, keysData, chainData] = await Promise.all([
        getUsage(),
        listUserServiceKeys(),
        getUserFallbackChain(),
      ]);
      setUsage(usageData);
      setProviderKeys(keysData.keys || []);
      setChain(chainData.chain || []);
    } catch (err) {
      const error = err as { error?: string };
      toast.error(error.error || 'Failed to load settings');
    } finally {
      setIsLoading(false);
    }
  }, []);

  useEffect(() => {
    loadData();
  }, [loadData]);

  const getProviderLabel = (provider: string) => {
    const p = LLM_PROVIDERS.find(lp => lp.value === provider);
    return p ? p.label : provider;
  };

  const getKeyForProvider = (provider: string) => providerKeys.find(k => k.provider === provider);
  const configuredProviders = new Set(providerKeys.map(k => k.provider));

  // Provider key handlers
  const startEditing = (provider: string) => {
    const existing = getKeyForProvider(provider);
    setEditingProvider(provider);
    setFormData({
      apiKey: '',
      baseUrl: existing?.base_url || '',
    });
    setShowApiKey(false);
  };

  const handleSaveKey = async (provider: Provider) => {
    setIsSavingKey(true);
    try {
      await upsertUserServiceKey({
        provider,
        api_key: formData.apiKey || undefined,
        base_url: formData.baseUrl || undefined,
        is_enabled: true,
      });
      toast.success(`${getProviderLabel(provider)} key saved`);
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
        provider: key.provider as Provider,
        is_enabled: !key.is_enabled,
      });
      loadData();
    } catch {
      toast.error('Failed to update');
    }
  };

  // Fallback chain handlers
  const startEditingChain = () => {
    setChainDraft(chain.map(e => ({
      provider: e.provider as Provider,
      model: e.model,
      is_enabled: e.is_enabled,
    })));
    setIsEditingChain(true);
  };

  const cancelEditingChain = () => {
    setChainDraft([]);
    setIsEditingChain(false);
  };

  const addChainEntry = () => {
    setChainDraft([...chainDraft, {
      provider: 'openrouter',
      model: DEFAULT_MODELS.openrouter,
      is_enabled: true,
    }]);
  };

  const removeChainEntry = (index: number) => {
    setChainDraft(chainDraft.filter((_, i) => i !== index));
  };

  const moveChainEntry = (index: number, direction: 'up' | 'down') => {
    const newIndex = direction === 'up' ? index - 1 : index + 1;
    if (newIndex < 0 || newIndex >= chainDraft.length) return;
    const newDraft = [...chainDraft];
    [newDraft[index], newDraft[newIndex]] = [newDraft[newIndex], newDraft[index]];
    setChainDraft(newDraft);
  };

  const updateChainEntry = (index: number, updates: Partial<UserFallbackChainEntryInput>) => {
    setChainDraft(chainDraft.map((entry, i) =>
      i === index ? { ...entry, ...updates } : entry
    ));
  };

  const handleSaveChain = async () => {
    setIsSavingChain(true);
    try {
      const result = await setUserFallbackChain(chainDraft);
      setChain(result.chain);
      setIsEditingChain(false);
      setChainDraft([]);
      toast.success('Fallback chain saved');
    } catch (err) {
      const error = err as { error?: string };
      toast.error(error.error || 'Failed to save chain');
    } finally {
      setIsSavingChain(false);
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
        <h1 className="text-2xl font-bold tracking-tight">Settings</h1>
        <p className="text-sm text-zinc-600 dark:text-zinc-400">
          Manage your account and LLM configuration.
        </p>
      </div>

      {/* Usage Summary */}
      {usage && (
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-base">Usage This Month</CardTitle>
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

      {/* Account Settings */}
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-base">Account</CardTitle>
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

      {/* LLM Provider Keys */}
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-base">LLM Provider Keys</CardTitle>
          <CardDescription className="text-xs">
            Configure API keys for BYOK mode. Your keys are encrypted at rest.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          {LLM_PROVIDERS.map((provider) => {
            const key = getKeyForProvider(provider.value);
            const isEditing = editingProvider === provider.value;

            return (
              <div key={provider.value} className="border rounded-md p-3">
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <span className="font-medium text-sm">{provider.label}</span>
                    {key ? (
                      <Badge variant={key.is_enabled ? 'default' : 'secondary'} className="text-xs">
                        {key.is_enabled ? 'Enabled' : 'Disabled'}
                      </Badge>
                    ) : (
                      <Badge variant="outline" className="text-xs">Not configured</Badge>
                    )}
                  </div>
                  {!isEditing && (
                    <div className="flex gap-1">
                      <Button variant="outline" size="sm" className="h-7 text-xs" onClick={() => startEditing(provider.value)}>
                        {key ? 'Update' : 'Configure'}
                      </Button>
                      {key && (
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

                {isEditing && (
                  <div className="mt-3 space-y-3">
                    <div className="space-y-1">
                      <Label className="text-xs">API Key</Label>
                      <div className="relative">
                        <Input
                          type={showApiKey ? 'text' : 'password'}
                          placeholder={key?.has_key ? '(unchanged)' : 'Enter API key...'}
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
                    {provider.value === 'ollama' && (
                      <div className="space-y-1">
                        <Label className="text-xs">Base URL</Label>
                        <Input
                          placeholder="http://localhost:11434"
                          value={formData.baseUrl}
                          onChange={(e) => setFormData({ ...formData, baseUrl: e.target.value })}
                          className="h-8 text-sm"
                        />
                      </div>
                    )}
                    <div className="flex gap-2">
                      <Button size="sm" className="h-7 text-xs" onClick={() => handleSaveKey(provider.value)} disabled={isSavingKey}>
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
          })}
        </CardContent>
      </Card>

      {/* Fallback Chain */}
      <Card>
        <CardHeader className="pb-3">
          <div className="flex items-center justify-between">
            <div>
              <CardTitle className="text-base">Fallback Chain</CardTitle>
              <CardDescription className="text-xs">
                Configure provider order for extractions. First successful provider wins.
              </CardDescription>
            </div>
            {!isEditingChain && (
              <Button variant="outline" size="sm" className="h-7 text-xs" onClick={startEditingChain}>
                Edit Chain
              </Button>
            )}
          </div>
        </CardHeader>
        <CardContent>
          {!isEditingChain ? (
            chain.length > 0 ? (
              <div className="space-y-1">
                {chain.map((entry, index) => (
                  <div
                    key={entry.id}
                    className={`flex items-center gap-2 p-2 border rounded text-sm ${
                      entry.is_enabled ? '' : 'opacity-50'
                    }`}
                  >
                    <span className="text-xs text-zinc-500 w-5">{index + 1}.</span>
                    <span className="font-medium">{getProviderLabel(entry.provider)}</span>
                    <code className="text-xs bg-zinc-100 dark:bg-zinc-800 px-1 rounded">{entry.model}</code>
                    {!configuredProviders.has(entry.provider) && (
                      <Badge variant="destructive" className="text-xs">No key</Badge>
                    )}
                    <Badge variant={entry.is_enabled ? 'default' : 'secondary'} className="text-xs ml-auto">
                      {entry.is_enabled ? 'On' : 'Off'}
                    </Badge>
                  </div>
                ))}
              </div>
            ) : (
              <p className="text-sm text-zinc-500 text-center py-4">
                No fallback chain configured. System defaults will be used.
              </p>
            )
          ) : (
            <div className="space-y-3">
              {chainDraft.map((entry, index) => (
                <div key={index} className="flex items-center gap-2 p-2 border rounded">
                  <div className="flex flex-col gap-0.5">
                    <Button
                      variant="ghost"
                      size="sm"
                      className="h-5 w-5 p-0"
                      onClick={() => moveChainEntry(index, 'up')}
                      disabled={index === 0}
                    >
                      <ChevronUp className="h-3 w-3" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      className="h-5 w-5 p-0"
                      onClick={() => moveChainEntry(index, 'down')}
                      disabled={index === chainDraft.length - 1}
                    >
                      <ChevronDown className="h-3 w-3" />
                    </Button>
                  </div>
                  <span className="text-xs text-zinc-500 w-5">{index + 1}.</span>
                  <select
                    value={entry.provider}
                    onChange={(e) => {
                      const provider = e.target.value as Provider;
                      updateChainEntry(index, {
                        provider,
                        model: DEFAULT_MODELS[provider],
                      });
                    }}
                    className="h-8 px-2 text-sm border rounded bg-white dark:bg-zinc-900"
                  >
                    {LLM_PROVIDERS.map(p => (
                      <option key={p.value} value={p.value}>{p.label}</option>
                    ))}
                  </select>
                  <div className="flex-1">
                    <ModelSelector
                      provider={entry.provider}
                      value={entry.model}
                      onValueChange={(model) => updateChainEntry(index, { model })}
                      useUserEndpoint
                    />
                  </div>
                  <label className="flex items-center gap-1 text-xs cursor-pointer">
                    <input
                      type="checkbox"
                      checked={entry.is_enabled}
                      onChange={(e) => updateChainEntry(index, { is_enabled: e.target.checked })}
                      className="rounded"
                    />
                    On
                  </label>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="h-7 px-2"
                    onClick={() => removeChainEntry(index)}
                  >
                    <Trash2 className="h-3 w-3 text-red-500" />
                  </Button>
                </div>
              ))}

              <div className="flex gap-2">
                <Button variant="outline" size="sm" className="h-7 text-xs" onClick={addChainEntry}>
                  <Plus className="h-3 w-3 mr-1" />
                  Add Entry
                </Button>
              </div>

              <div className="flex gap-2 pt-2 border-t">
                <Button size="sm" className="h-7 text-xs" onClick={handleSaveChain} disabled={isSavingChain}>
                  {isSavingChain ? 'Saving...' : 'Save Chain'}
                </Button>
                <Button variant="outline" size="sm" className="h-7 text-xs" onClick={cancelEditingChain}>
                  Cancel
                </Button>
              </div>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
