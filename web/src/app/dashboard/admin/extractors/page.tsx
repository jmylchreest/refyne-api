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
  ServiceKey,
  ServiceKeyInput,
  FallbackChainEntry,
  FallbackChainEntryInput,
  ModelValidationResult,
  SubscriptionTier,
} from '@/lib/api';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Badge } from '@/components/ui/badge';
import { ModelSelector } from '@/components/model-selector';
import { toast } from 'sonner';

const DEFAULT_MODELS: Record<string, string> = {
  openrouter: 'xiaomi/mimo-v2-flash:free',
  anthropic: 'claude-sonnet-4-5-20250514',
  openai: 'gpt-4o-mini',
  ollama: 'llama3.2',
};

const PROVIDER_LABELS: Record<string, string> = {
  openrouter: 'OpenRouter',
  anthropic: 'Anthropic',
  openai: 'OpenAI',
  ollama: 'Ollama',
};

const PROVIDERS = ['openrouter', 'anthropic', 'openai', 'ollama'] as const;

type ModelStatusMap = Map<string, ModelValidationResult>;

export default function ExtractorsPage() {
  const { user, isLoaded } = useUser();
  const [keys, setKeys] = useState<ServiceKey[]>([]);
  const [chain, setChain] = useState<FallbackChainEntry[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [editingProvider, setEditingProvider] = useState<string | null>(null);
  const [formData, setFormData] = useState<Partial<ServiceKeyInput>>({});
  const [chainDraft, setChainDraft] = useState<FallbackChainEntryInput[]>([]);
  const [isEditingChain, setIsEditingChain] = useState(false);
  const [savingChain, setSavingChain] = useState(false);
  const [modelStatuses, setModelStatuses] = useState<ModelStatusMap>(new Map());
  const [validatingModels, setValidatingModels] = useState(false);
  const [showApiKey, setShowApiKey] = useState<Record<string, boolean>>({});
  const [tiers, setTiers] = useState<SubscriptionTier[]>([]);
  const [selectedTier, setSelectedTier] = useState<string>('');
  const [chainsByTier, setChainsByTier] = useState<Record<string, FallbackChainEntry[]>>({});

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
      const [keysRes, tiersRes] = await Promise.all([
        listServiceKeys(),
        listSubscriptionTiers(),
      ]);
      setKeys(keysRes.keys || []);
      setTiers(tiersRes.tiers || []);

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
  }, [isLoaded, isSuperadmin]);

  useEffect(() => {
    if (selectedTier && isSuperadmin) {
      if (chainsByTier[selectedTier]) {
        setChain(chainsByTier[selectedTier]);
      } else {
        loadChainForTier(selectedTier);
      }
    }
  }, [selectedTier, isSuperadmin]);

  const handleSave = async (provider: 'openrouter' | 'anthropic' | 'openai') => {
    if (!formData.api_key) {
      toast.error('API key is required');
      return;
    }

    try {
      await upsertServiceKey({
        provider,
        api_key: formData.api_key,
        default_model: formData.default_model || DEFAULT_MODELS[provider],
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
      default_model: existing?.default_model || DEFAULT_MODELS[provider],
      is_enabled: existing?.is_enabled ?? true,
      api_key: '',
    });
  };

  const startEditingChain = () => {
    setChainDraft(chain.map(e => ({
      provider: e.provider as FallbackChainEntryInput['provider'],
      model: e.model,
      is_enabled: e.is_enabled,
    })));
    setIsEditingChain(true);
  };

  const cancelEditingChain = () => {
    setChainDraft([]);
    setIsEditingChain(false);
  };

  const saveChain = async () => {
    if (!selectedTier) {
      toast.error('Please select a tier first');
      return;
    }

    setSavingChain(true);
    try {
      const validationResult = await validateModels(
        chainDraft.map(e => ({ provider: e.provider, model: e.model }))
      );

      const notFoundModels = validationResult.results.filter(r => r.status === 'not_found');
      if (notFoundModels.length > 0) {
        const modelList = notFoundModels.map(m => `${m.provider}:${m.model}`).join(', ');
        const proceed = confirm(
          `The following models were not found in provider catalogs:\n\n${modelList}\n\nThey may still work if the model IDs are correct. Save anyway?`
        );
        if (!proceed) {
          setSavingChain(false);
          return;
        }
      }

      const result = await setFallbackChain(chainDraft, selectedTier);
      setChain(result.chain);
      setChainsByTier(prev => ({ ...prev, [selectedTier]: result.chain }));
      setIsEditingChain(false);
      setChainDraft([]);
      toast.success(`Extraction fallback chain saved for ${selectedTier}`);

      validateChainModels(result.chain);
    } catch (err) {
      const error = err as { error?: string };
      toast.error(error.error || 'Failed to save extraction fallback chain');
    } finally {
      setSavingChain(false);
    }
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

  const updateChainEntry = (index: number, updates: Partial<FallbackChainEntryInput>) => {
    setChainDraft(chainDraft.map((entry, i) =>
      i === index ? { ...entry, ...updates } : entry
    ));
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
                    <p>Model: <code className="bg-zinc-100 dark:bg-zinc-800 px-1 rounded">{key.default_model}</code></p>
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
                          placeholder={key ? '(unchanged)' : 'Enter API key...'}
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
                    <div>
                      <Label htmlFor={`${provider}-model`}>Default Model</Label>
                      <ModelSelector
                        provider={provider}
                        value={formData.default_model || DEFAULT_MODELS[provider]}
                        onValueChange={(model) => setFormData({ ...formData, default_model: model })}
                      />
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
          <div className="flex items-center justify-between">
            <div>
              <CardTitle>Extraction Fallback Chain</CardTitle>
              <CardDescription>
                Configure the order in which LLM providers are tried for extraction per subscription tier.
                The extraction service will try each entry in order until one succeeds.
              </CardDescription>
            </div>
            {!isEditingChain && selectedTier && (
              <Button variant="outline" onClick={startEditingChain}>
                Edit Chain
              </Button>
            )}
          </div>
        </CardHeader>
        <CardContent>
          {tiers.length > 0 && (
            <div className="space-y-4 mb-6">
              <div className="flex gap-2 border-b pb-4">
                {tiers.map((tier) => (
                  <Button
                    key={tier.id}
                    variant={selectedTier === tier.slug ? 'default' : 'outline'}
                    size="sm"
                    onClick={() => {
                      if (!isEditingChain) {
                        setSelectedTier(tier.slug);
                      }
                    }}
                    disabled={isEditingChain}
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
          {!isEditingChain ? (
            chain.length > 0 ? (
              <ol className="space-y-2">
                {chain.map((entry, index) => (
                  <li
                    key={entry.id}
                    className="flex items-center gap-3 p-3 border rounded-lg"
                  >
                    <span className="text-sm font-medium text-zinc-500 w-6">{index + 1}.</span>
                    <span className="font-medium">{PROVIDER_LABELS[entry.provider] || entry.provider}</span>
                    <code className="bg-zinc-100 dark:bg-zinc-800 px-2 py-1 rounded text-sm">
                      {entry.model}
                    </code>
                    {getModelStatusBadge(entry.provider, entry.model)}
                    <Badge variant={entry.is_enabled ? 'default' : 'secondary'}>
                      {entry.is_enabled ? 'Enabled' : 'Disabled'}
                    </Badge>
                  </li>
                ))}
              </ol>
            ) : (
              <div className="text-sm text-zinc-500 space-y-2">
                {selectedTier ? (
                  <p>No extraction fallback chain configured for <strong>{selectedTier}</strong>. Click &quot;Edit Chain&quot; to add one.</p>
                ) : (
                  <p>Select a tier above to view or configure its extraction fallback chain.</p>
                )}
              </div>
            )
          ) : (
            <div className="space-y-4">
              <div className="text-sm text-zinc-600 dark:text-zinc-400 mb-2">
                Editing chain for: <strong>{tiers.find(t => t.slug === selectedTier)?.name || selectedTier}</strong>
              </div>
              {chainDraft.map((entry, index) => (
                <div key={index} className="flex items-center gap-3 p-3 border rounded-lg">
                  <div className="flex flex-col gap-1">
                    <Button
                      variant="ghost"
                      size="sm"
                      className="h-6 w-6 p-0"
                      onClick={() => moveChainEntry(index, 'up')}
                      disabled={index === 0}
                    >
                      <ChevronUpIcon className="h-4 w-4" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      className="h-6 w-6 p-0"
                      onClick={() => moveChainEntry(index, 'down')}
                      disabled={index === chainDraft.length - 1}
                    >
                      <ChevronDownIcon className="h-4 w-4" />
                    </Button>
                  </div>
                  <span className="text-sm font-medium text-zinc-500 w-6">{index + 1}.</span>
                  <select
                    value={entry.provider}
                    onChange={(e) => {
                      const provider = e.target.value as FallbackChainEntryInput['provider'];
                      updateChainEntry(index, {
                        provider,
                        model: DEFAULT_MODELS[provider],
                      });
                    }}
                    className="border rounded px-2 py-1 bg-white dark:bg-zinc-900"
                  >
                    {PROVIDERS.map(p => (
                      <option key={p} value={p}>{PROVIDER_LABELS[p]}</option>
                    ))}
                  </select>
                  <div className="flex-1">
                    <ModelSelector
                      provider={entry.provider}
                      value={entry.model}
                      onValueChange={(model) => updateChainEntry(index, { model })}
                    />
                  </div>
                  <div className="flex items-center gap-2">
                    <input
                      type="checkbox"
                      checked={entry.is_enabled}
                      onChange={(e) => updateChainEntry(index, { is_enabled: e.target.checked })}
                      className="rounded"
                    />
                    <span className="text-sm">Enabled</span>
                  </div>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => removeChainEntry(index)}
                    className="text-red-500 hover:text-red-700"
                  >
                    <TrashIcon className="h-4 w-4" />
                  </Button>
                </div>
              ))}
              <div className="flex gap-2">
                <Button variant="outline" onClick={addChainEntry}>
                  Add Entry
                </Button>
              </div>
              <div className="flex gap-2 pt-4 border-t">
                <Button onClick={saveChain} disabled={savingChain}>
                  {savingChain ? 'Saving...' : 'Save Chain'}
                </Button>
                <Button variant="outline" onClick={cancelEditingChain}>
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

function ChevronUpIcon({ className }: { className?: string }) {
  return (
    <svg className={className} fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" d="m4.5 15.75 7.5-7.5 7.5 7.5" />
    </svg>
  );
}

function ChevronDownIcon({ className }: { className?: string }) {
  return (
    <svg className={className} fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" d="m19.5 8.25-7.5 7.5-7.5-7.5" />
    </svg>
  );
}

function TrashIcon({ className }: { className?: string }) {
  return (
    <svg className={className} fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" d="m14.74 9-.346 9m-4.788 0L9.26 9m9.968-3.21c.342.052.682.107 1.022.166m-1.022-.165L18.16 19.673a2.25 2.25 0 0 1-2.244 2.077H8.084a2.25 2.25 0 0 1-2.244-2.077L4.772 5.79m14.456 0a48.108 48.108 0 0 0-3.478-.397m-12 .562c.34-.059.68-.114 1.022-.165m0 0a48.11 48.11 0 0 1 3.478-.397m7.5 0v-.916c0-1.18-.91-2.164-2.09-2.201a51.964 51.964 0 0 0-3.32 0c-1.18.037-2.09 1.022-2.09 2.201v.916m7.5 0a48.667 48.667 0 0 0-7.5 0" />
    </svg>
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
