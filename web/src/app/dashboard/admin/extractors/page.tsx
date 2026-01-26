'use client';

import { useEffect, useState } from 'react';
import { useUser } from '@clerk/nextjs';
import {
  listServiceKeys,
  upsertServiceKey,
  deleteServiceKey,
  getFallbackChain,
  setFallbackChain,
  validateModels,
  listSubscriptionTiers,
  listLLMProviders,
  ServiceKey,
  ServiceKeyInput,
  FallbackChainEntry,
  ModelValidationResult,
  SubscriptionTier,
  LLMProvider,
} from '@/lib/api';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Badge } from '@/components/ui/badge';
import { FallbackChainEditor, ChainEntry, SavedChainEntry } from '@/components/fallback-chain-editor';
import { toast } from 'sonner';

const PROVIDER_LABELS: Record<string, string> = {
  openrouter: 'OpenRouter',
  anthropic: 'Anthropic',
  openai: 'OpenAI',
  ollama: 'Ollama',
};

type ModelStatusMap = Map<string, ModelValidationResult>;

export default function ExtractorsPage() {
  const { user, isLoaded } = useUser();
  const [keys, setKeys] = useState<ServiceKey[]>([]);
  const [chain, setChain] = useState<FallbackChainEntry[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [editingProvider, setEditingProvider] = useState<string | null>(null);
  const [formData, setFormData] = useState<Partial<ServiceKeyInput>>({});
  const [modelStatuses, setModelStatuses] = useState<ModelStatusMap>(new Map());
  const [validatingModels, setValidatingModels] = useState(false);
  const [showApiKey, setShowApiKey] = useState<Record<string, boolean>>({});
  const [tiers, setTiers] = useState<SubscriptionTier[]>([]);
  const [selectedTier, setSelectedTier] = useState<string>('');
  const [chainsByTier, setChainsByTier] = useState<Record<string, FallbackChainEntry[]>>({});
  const [availableProviders, setAvailableProviders] = useState<LLMProvider[]>([]);

  const isSuperadmin = user?.publicMetadata?.global_superadmin === true;

  const validateChainModels = async (entries: FallbackChainEntry[]) => {
    if (entries.length === 0) return;

    setValidatingModels(true);
    try {
      const result = await validateModels(
        entries.map(e => ({ provider: e.provider, model: e.model }))
      );

      const statusMap = new Map<string, ModelValidationResult>();
      for (const r of result.results) {
        statusMap.set(`${r.provider}:${r.model}`, r);
      }
      setModelStatuses(statusMap);
    } catch (err) {
      console.error('Failed to validate models:', err);
    } finally {
      setValidatingModels(false);
    }
  };

  const loadData = async () => {
    try {
      const [keysRes, tiersRes, providersRes] = await Promise.all([
        listServiceKeys(),
        listSubscriptionTiers(),
        listLLMProviders(),
      ]);
      setKeys(keysRes.keys || []);
      setTiers(tiersRes.tiers || []);
      setAvailableProviders(providersRes.providers || []);

      if (!selectedTier && tiersRes.tiers && tiersRes.tiers.length > 0) {
        setSelectedTier(tiersRes.tiers[0].slug);
      }
    } catch (err) {
      const error = err as { error?: string; status?: number };
      if (error.status === 403) {
        // Not admin - this is expected
      } else {
        toast.error(error.error || 'Failed to load admin data');
      }
    } finally {
      setIsLoading(false);
    }
  };

  const loadChainForTier = async (tier: string) => {
    try {
      const chainRes = await getFallbackChain(tier);
      const tierChain = chainRes.chain || [];
      setChainsByTier(prev => ({ ...prev, [tier]: tierChain }));
      setChain(tierChain);

      if (tierChain.length > 0) {
        validateChainModels(tierChain);
      }
    } catch (err) {
      console.error('Failed to load chain for tier:', tier, err);
    }
  };

  useEffect(() => {
    if (isLoaded && isSuperadmin) {
      loadData();
    } else if (isLoaded) {
      setIsLoading(false);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isLoaded, isSuperadmin]);

  useEffect(() => {
    if (selectedTier && isSuperadmin) {
      if (chainsByTier[selectedTier]) {
        setChain(chainsByTier[selectedTier]);
      } else {
        loadChainForTier(selectedTier);
      }
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedTier, isSuperadmin]);

  const handleSave = async (provider: 'openrouter' | 'anthropic' | 'openai') => {
    const existingKey = keys.find(k => k.provider === provider);

    // API key is required for new keys, optional for updates
    if (!existingKey && !formData.api_key) {
      toast.error('API key is required for new service keys');
      return;
    }

    try {
      await upsertServiceKey({
        provider,
        api_key: formData.api_key || '', // Empty string preserves existing key on backend
        is_enabled: formData.is_enabled ?? true,
      });
      toast.success(`${PROVIDER_LABELS[provider]} key saved`);
      setEditingProvider(null);
      setFormData({});
      loadData();
    } catch (err) {
      const error = err as { error?: string };
      toast.error(error.error || 'Failed to save service key');
    }
  };

  const handleDelete = async (provider: string) => {
    if (!confirm(`Delete ${PROVIDER_LABELS[provider]} service key?`)) return;

    try {
      await deleteServiceKey(provider);
      toast.success(`${PROVIDER_LABELS[provider]} key deleted`);
      loadData();
    } catch (err) {
      const error = err as { error?: string };
      toast.error(error.error || 'Failed to delete service key');
    }
  };

  const startEditing = (provider: string) => {
    const existing = keys.find(k => k.provider === provider);
    setEditingProvider(provider);
    setFormData({
      is_enabled: existing?.is_enabled ?? true,
      api_key: '',
    });
  };

  const handleSaveChain = async (entries: ChainEntry[]) => {
    if (!selectedTier) {
      toast.error('Please select a tier first');
      throw new Error('No tier selected');
    }

    // Validate models before saving
    const validationResult = await validateModels(
      entries.map(e => ({ provider: e.provider, model: e.model }))
    );

    const notFoundModels = validationResult.results.filter(r => r.status === 'not_found');
    if (notFoundModels.length > 0) {
      const modelList = notFoundModels.map(m => `${m.provider}:${m.model}`).join(', ');
      const proceed = confirm(
        `The following models were not found in provider catalogs:\n\n${modelList}\n\nThey may still work if the model IDs are correct. Save anyway?`
      );
      if (!proceed) {
        throw new Error('User cancelled');
      }
    }

    try {
      const result = await setFallbackChain(
        entries.map(e => ({
          provider: e.provider,
          model: e.model,
          temperature: e.temperature,
          max_tokens: e.max_tokens,
          is_enabled: e.is_enabled,
        })),
        selectedTier
      );
      setChain(result.chain);
      setChainsByTier(prev => ({ ...prev, [selectedTier]: result.chain }));
      toast.success(`Extraction fallback chain saved for ${selectedTier}`);
      validateChainModels(result.chain);
    } catch (err) {
      const error = err as { error?: string };
      toast.error(error.error || 'Failed to save extraction fallback chain');
      throw err;
    }
  };

  const getModelStatusBadge = (provider: string, model: string) => {
    const key = `${provider}:${model}`;
    const status = modelStatuses.get(key);

    if (validatingModels) {
      return <Badge variant="outline" className="text-xs">Checking...</Badge>;
    }

    if (!status) return null;

    switch (status.status) {
      case 'valid':
        return <Badge variant="default" className="text-xs bg-green-600">Valid</Badge>;
      case 'not_found':
        return (
          <Badge variant="destructive" className="text-xs" title={status.message}>
            Not Found
          </Badge>
        );
      case 'deprecated':
        return (
          <Badge variant="secondary" className="text-xs bg-yellow-500 text-black" title={status.message}>
            Deprecated
          </Badge>
        );
      case 'unknown':
        return (
          <Badge variant="outline" className="text-xs" title={status.message}>
            Unknown
          </Badge>
        );
      default:
        return null;
    }
  };

  if (!isLoaded || isLoading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-zinc-900 dark:border-white" />
      </div>
    );
  }

  if (!isSuperadmin) {
    return null;
  }

  const getKeyForProvider = (provider: string) => keys.find(k => k.provider === provider);

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle>Service API Keys</CardTitle>
          <CardDescription>
            These keys are used for users on the free tier or without their own API keys (BYOK).
            Keys are encrypted at rest.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-6">
          {(['openrouter', 'anthropic', 'openai'] as const).map((provider) => {
            const key = getKeyForProvider(provider);
            const isEditing = editingProvider === provider;

            return (
              <div key={provider} className="border rounded-lg p-4">
                <div className="flex items-center justify-between mb-4">
                  <div className="flex items-center gap-3">
                    <h3 className="font-medium">{PROVIDER_LABELS[provider]}</h3>
                    {key ? (
                      <Badge variant={key.is_enabled ? 'default' : 'secondary'}>
                        {key.is_enabled ? 'Enabled' : 'Disabled'}
                      </Badge>
                    ) : (
                      <Badge variant="outline">Not configured</Badge>
                    )}
                  </div>
                  {!isEditing && (
                    <div className="flex gap-2">
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => startEditing(provider)}
                      >
                        {key ? 'Update' : 'Configure'}
                      </Button>
                      {key && (
                        <Button
                          variant="destructive"
                          size="sm"
                          onClick={() => handleDelete(provider)}
                        >
                          Delete
                        </Button>
                      )}
                    </div>
                  )}
                </div>

                {key && !isEditing && (
                  <div className="text-sm text-zinc-500 space-y-1">
                    <p>Updated: {new Date(key.updated_at).toLocaleString()}</p>
                  </div>
                )}

                {isEditing && (
                  <div className="space-y-4">
                    <div>
                      <Label htmlFor={`${provider}-key`}>API Key</Label>
                      <div className="relative">
                        <Input
                          id={`${provider}-key`}
                          type={showApiKey[provider] ? 'text' : 'password'}
                          placeholder={key ? '(leave empty to keep existing key)' : 'Enter API key...'}
                          value={formData.api_key || ''}
                          onChange={(e) => setFormData({ ...formData, api_key: e.target.value })}
                          className="pr-10"
                        />
                        <button
                          type="button"
                          onClick={() => setShowApiKey({ ...showApiKey, [provider]: !showApiKey[provider] })}
                          className="absolute right-2 top-1/2 -translate-y-1/2 text-zinc-500 hover:text-zinc-700 dark:hover:text-zinc-300"
                          tabIndex={-1}
                        >
                          {showApiKey[provider] ? (
                            <EyeOffIcon className="h-5 w-5" />
                          ) : (
                            <EyeIcon className="h-5 w-5" />
                          )}
                        </button>
                      </div>
                    </div>
                    <div className="flex items-center gap-2">
                      <input
                        type="checkbox"
                        id={`${provider}-enabled`}
                        checked={formData.is_enabled ?? true}
                        onChange={(e) => setFormData({ ...formData, is_enabled: e.target.checked })}
                        className="rounded"
                      />
                      <Label htmlFor={`${provider}-enabled`}>Enabled</Label>
                    </div>
                    <div className="flex gap-2">
                      <Button onClick={() => handleSave(provider)}>Save</Button>
                      <Button
                        variant="outline"
                        onClick={() => {
                          setEditingProvider(null);
                          setFormData({});
                          setShowApiKey({});
                        }}
                      >
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

      <Card>
        <CardHeader>
          <CardTitle>Extraction Fallback Chain</CardTitle>
          <CardDescription>
            Configure the order in which LLM providers are tried for extraction per subscription tier.
            The extraction service will try each entry in order until one succeeds.
            Click the settings icon on each entry to configure temperature and max tokens.
          </CardDescription>
        </CardHeader>
        <CardContent>
          {tiers.length > 0 && (
            <div className="space-y-4 mb-6">
              <div className="flex gap-2 flex-wrap border-b pb-4">
                {tiers.map((tier) => (
                  <Button
                    key={tier.id}
                    variant={selectedTier === tier.slug ? 'default' : 'outline'}
                    size="sm"
                    onClick={() => setSelectedTier(tier.slug)}
                    title={`ID: ${tier.id}`}
                  >
                    {tier.name}
                    {chainsByTier[tier.slug]?.length > 0 && (
                      <Badge variant="secondary" className="ml-2 text-xs">
                        {chainsByTier[tier.slug].length}
                      </Badge>
                    )}
                  </Button>
                ))}
              </div>
              {selectedTier && (() => {
                const tier = tiers.find(t => t.slug === selectedTier);
                if (!tier) return null;
                return (
                  <div className="bg-zinc-50 dark:bg-zinc-900 rounded-lg p-3 text-sm">
                    <div className="flex items-center gap-4">
                      <div>
                        <span className="text-zinc-500">Tier:</span>{' '}
                        <span className="font-medium">{tier.name}</span>
                        {tier.is_default && (
                          <Badge variant="outline" className="ml-2 text-xs">Default</Badge>
                        )}
                      </div>
                      <div>
                        <span className="text-zinc-500">Slug:</span>{' '}
                        <code className="bg-zinc-200 dark:bg-zinc-800 px-1 rounded text-xs">{tier.slug}</code>
                      </div>
                      <div>
                        <span className="text-zinc-500">ID:</span>{' '}
                        <code className="bg-zinc-200 dark:bg-zinc-800 px-1 rounded text-xs">{tier.id}</code>
                      </div>
                    </div>
                    {tier.description && (
                      <div className="mt-2 text-zinc-500">{tier.description}</div>
                    )}
                  </div>
                );
              })()}
            </div>
          )}
          {tiers.length === 0 && (
            <div className="text-sm text-zinc-500 mb-4">
              No subscription tiers found. Configure tiers in Clerk Commerce and set CLERK_SECRET_KEY.
            </div>
          )}
          {selectedTier ? (
            <FallbackChainEditor
              chain={chain as SavedChainEntry[]}
              onSave={handleSaveChain}
              getModelStatusBadge={getModelStatusBadge}
              availableProviders={availableProviders}
            />
          ) : (
            <div className="text-sm text-zinc-500 text-center py-4">
              Select a tier above to view or configure its extraction fallback chain.
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function EyeIcon({ className }: { className?: string }) {
  return (
    <svg className={className} fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" d="M2.036 12.322a1.012 1.012 0 0 1 0-.639C3.423 7.51 7.36 4.5 12 4.5c4.638 0 8.573 3.007 9.963 7.178.07.207.07.431 0 .639C20.577 16.49 16.64 19.5 12 19.5c-4.638 0-8.573-3.007-9.963-7.178Z" />
      <path strokeLinecap="round" strokeLinejoin="round" d="M15 12a3 3 0 1 1-6 0 3 3 0 0 1 6 0Z" />
    </svg>
  );
}

function EyeOffIcon({ className }: { className?: string }) {
  return (
    <svg className={className} fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" d="M3.98 8.223A10.477 10.477 0 0 0 1.934 12C3.226 16.338 7.244 19.5 12 19.5c.993 0 1.953-.138 2.863-.395M6.228 6.228A10.451 10.451 0 0 1 12 4.5c4.756 0 8.773 3.162 10.065 7.498a10.522 10.522 0 0 1-4.293 5.774M6.228 6.228 3 3m3.228 3.228 3.65 3.65m7.894 7.894L21 21m-3.228-3.228-3.65-3.65m0 0a3 3 0 1 0-4.243-4.243m4.242 4.242L9.88 9.88" />
    </svg>
  );
}
