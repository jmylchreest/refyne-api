'use client';

import { useState } from 'react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Badge } from '@/components/ui/badge';
import { ModelSelector } from '@/components/model-selector';
import { getModelDefaults } from '@/lib/model-defaults';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import { ChevronUp, ChevronDown, Trash2, Plus, Settings2, AlertCircle, AlertTriangle } from 'lucide-react';
import type { LLMProvider } from '@/lib/api';

const DEFAULT_MODELS: Record<string, string> = {
  openrouter: 'google/gemini-2.0-flash-001',
  anthropic: 'claude-sonnet-4-5-20250514',
  openai: 'gpt-4o-mini',
  ollama: 'llama3.2',
  helicone: 'gpt-4o-mini',
};

export interface ChainEntry {
  provider: string;
  model: string;
  temperature?: number;
  max_tokens?: number;
  is_enabled: boolean;
}

export interface SavedChainEntry extends ChainEntry {
  id: string;
  position: number;
  created_at: string;
  updated_at: string;
}

interface FallbackChainEditorProps {
  /** Current saved chain entries (readonly display mode) */
  chain: SavedChainEntry[];
  /** Whether a key is configured for each provider (for validation) */
  configuredProviders?: Set<string>;
  /** Called when save is requested with the new chain */
  onSave: (entries: ChainEntry[]) => Promise<void>;
  /** Whether to use user endpoint for model selector (default: false) */
  useUserEndpoint?: boolean;
  /** Optional model status badges (for admin view) */
  getModelStatusBadge?: (provider: string, model: string) => React.ReactNode;
  /** Compact mode for smaller display */
  compact?: boolean;
  /** Disable editing (show read-only view with locked state) */
  disabled?: boolean;
  /** Available providers from API (based on user features) */
  availableProviders?: LLMProvider[];
}

export function FallbackChainEditor({
  chain,
  configuredProviders,
  onSave,
  useUserEndpoint = false,
  getModelStatusBadge,
  compact = false,
  disabled = false,
  availableProviders = [],
}: FallbackChainEditorProps) {
  const [isEditing, setIsEditing] = useState(false);
  const [draft, setDraft] = useState<ChainEntry[]>([]);
  const [isSaving, setIsSaving] = useState(false);
  const [expandedSettings, setExpandedSettings] = useState<Set<number>>(new Set());

  // Create a set of available provider names for quick lookup
  const availableProviderNames = new Set(availableProviders.map(p => p.name));

  const getProviderLabel = (providerName: string) => {
    const p = availableProviders.find(lp => lp.name === providerName);
    return p ? p.display_name : providerName;
  };

  const isProviderAvailable = (providerName: string) => {
    return availableProviders.length === 0 || availableProviderNames.has(providerName);
  };

  const getProviderUnavailableReason = (providerName: string) => {
    // If no providers loaded yet, don't show as unavailable
    if (availableProviders.length === 0) return null;
    if (availableProviderNames.has(providerName)) return null;
    return `The ${providerName} provider is not available on your current plan. Upgrade to re-enable this entry.`;
  };

  const isProviderDecommissioned = (providerName: string) => {
    const p = availableProviders.find(lp => lp.name === providerName);
    return p?.status === 'decommissioned';
  };

  const getDecommissionInfo = (providerName: string) => {
    const p = availableProviders.find(lp => lp.name === providerName);
    if (p?.status !== 'decommissioned') return null;
    return {
      note: p.decommission_note || 'This provider has been decommissioned.',
      successor: p.successor_provider,
    };
  };

  const startEditing = () => {
    // Filter out entries for unavailable providers when starting to edit
    const editableEntries = chain
      .filter(e => isProviderAvailable(e.provider))
      .map(e => ({
        provider: e.provider,
        model: e.model,
        temperature: e.temperature,
        max_tokens: e.max_tokens,
        is_enabled: e.is_enabled,
      }));
    setDraft(editableEntries);
    setExpandedSettings(new Set());
    setIsEditing(true);
  };

  const cancelEditing = () => {
    setDraft([]);
    setExpandedSettings(new Set());
    setIsEditing(false);
  };

  const addEntry = () => {
    // Use the first available provider, fallback to openrouter
    const defaultProvider = availableProviders.length > 0 ? availableProviders[0].name : 'openrouter';
    const defaultModel = DEFAULT_MODELS[defaultProvider] || ''; // Empty if no known default
    setDraft([...draft, {
      provider: defaultProvider,
      model: defaultModel,
      temperature: undefined, // Use defaults
      max_tokens: undefined,
      is_enabled: true,
    }]);
  };

  const removeEntry = (index: number) => {
    setDraft(draft.filter((_, i) => i !== index));
    setExpandedSettings(prev => {
      const next = new Set(prev);
      next.delete(index);
      // Adjust indices for entries after the removed one
      const adjusted = new Set<number>();
      next.forEach(i => {
        if (i > index) adjusted.add(i - 1);
        else adjusted.add(i);
      });
      return adjusted;
    });
  };

  const moveEntry = (index: number, direction: 'up' | 'down') => {
    const newIndex = direction === 'up' ? index - 1 : index + 1;
    if (newIndex < 0 || newIndex >= draft.length) return;
    const newDraft = [...draft];
    [newDraft[index], newDraft[newIndex]] = [newDraft[newIndex], newDraft[index]];
    setDraft(newDraft);
    // Update expanded settings indices
    setExpandedSettings(prev => {
      const next = new Set<number>();
      prev.forEach(i => {
        if (i === index) next.add(newIndex);
        else if (i === newIndex) next.add(index);
        else next.add(i);
      });
      return next;
    });
  };

  const updateEntry = (index: number, updates: Partial<ChainEntry>) => {
    setDraft(draft.map((entry, i) => {
      if (i !== index) return entry;

      // If provider changed, reset to default model and clear custom settings
      if (updates.provider && updates.provider !== entry.provider) {
        const newProvider = updates.provider;
        return {
          ...entry,
          provider: newProvider,
          model: DEFAULT_MODELS[newProvider] || '', // Empty if no known default, ModelSelector will show available models
          temperature: undefined,
          max_tokens: undefined,
        };
      }

      return { ...entry, ...updates };
    }));
  };

  const toggleSettings = (index: number) => {
    setExpandedSettings(prev => {
      const next = new Set(prev);
      if (next.has(index)) {
        next.delete(index);
      } else {
        next.add(index);
      }
      return next;
    });
  };

  const handleSave = async () => {
    setIsSaving(true);
    try {
      await onSave(draft);
      setIsEditing(false);
      setDraft([]);
      setExpandedSettings(new Set());
    } finally {
      setIsSaving(false);
    }
  };

  const getEffectiveSettings = (entry: ChainEntry) => {
    const defaults = getModelDefaults(entry.provider, entry.model);
    return {
      temperature: entry.temperature ?? defaults.temperature,
      maxTokens: entry.max_tokens ?? defaults.maxTokens,
      isCustomTemp: entry.temperature !== undefined,
      isCustomTokens: entry.max_tokens !== undefined,
    };
  };

  // Read-only display
  if (!isEditing) {
    return (
      <TooltipProvider>
        <div className="space-y-2">
          <div className="flex items-center justify-between mb-2">
            <span className="text-sm text-zinc-500">
              {chain.length === 0 ? 'No chain configured' : `${chain.length} entries`}
            </span>
            <Button
              variant="outline"
              size="sm"
              className={compact ? "h-7 text-xs" : ""}
              onClick={startEditing}
              disabled={disabled}
            >
              Edit Chain
            </Button>
          </div>

          {chain.length > 0 && (
            <div className="space-y-1">
              {chain.map((entry, index) => {
                const settings = getEffectiveSettings(entry);
                const providerAvailable = isProviderAvailable(entry.provider);
                const unavailableReason = getProviderUnavailableReason(entry.provider);

                return (
                  <div
                    key={entry.id}
                    className={`flex items-center gap-2 p-2 border rounded text-sm ${
                      !providerAvailable ? 'opacity-50 bg-zinc-50 dark:bg-zinc-900/50' :
                      entry.is_enabled ? '' : 'opacity-50'
                    }`}
                  >
                    <span className="text-xs text-zinc-500 w-5">{index + 1}.</span>
                    <span className="font-medium">{getProviderLabel(entry.provider)}</span>
                    <code className="text-xs bg-zinc-100 dark:bg-zinc-800 px-1 rounded truncate max-w-[200px]">
                      {entry.model}
                    </code>
                    {getModelStatusBadge?.(entry.provider, entry.model)}
                    {isProviderDecommissioned(entry.provider) && (() => {
                      const info = getDecommissionInfo(entry.provider);
                      return (
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <Badge variant="outline" className="text-xs gap-1 cursor-help text-orange-600 border-orange-300">
                              <AlertTriangle className="h-3 w-3" />
                              Decommissioned
                            </Badge>
                          </TooltipTrigger>
                          <TooltipContent className="max-w-[250px]">
                            <p>{info?.note}</p>
                            {info?.successor && (
                              <p className="mt-1 font-medium">Consider migrating to: {info.successor}</p>
                            )}
                          </TooltipContent>
                        </Tooltip>
                      );
                    })()}
                    {!providerAvailable && unavailableReason && (
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <Badge variant="outline" className="text-xs gap-1 cursor-help text-amber-600 border-amber-300">
                            <AlertCircle className="h-3 w-3" />
                            Unavailable
                          </Badge>
                        </TooltipTrigger>
                        <TooltipContent className="max-w-[250px]">
                          <p>{unavailableReason}</p>
                        </TooltipContent>
                      </Tooltip>
                    )}
                    {providerAvailable && configuredProviders && !configuredProviders.has(entry.provider) && (
                      <Badge variant="destructive" className="text-xs">No key</Badge>
                    )}
                    <span className="text-xs text-zinc-400 ml-auto">
                      T:{settings.temperature} / {settings.maxTokens}tok
                    </span>
                    <Badge variant={entry.is_enabled && providerAvailable ? 'default' : 'secondary'} className="text-xs">
                      {providerAvailable ? (entry.is_enabled ? 'On' : 'Off') : 'Skipped'}
                    </Badge>
                  </div>
                );
              })}
            </div>
          )}

          {chain.length === 0 && (
            <p className="text-sm text-zinc-500 text-center py-4">
              No fallback chain configured. System defaults will be used.
            </p>
          )}
        </div>
      </TooltipProvider>
    );
  }

  // Editing mode
  return (
    <div className="space-y-3">
      {draft.map((entry, index) => {
        const defaults = getModelDefaults(entry.provider, entry.model);
        const isExpanded = expandedSettings.has(index);

        return (
          <div key={index} className="border rounded-lg p-3 space-y-2">
            <div className="flex items-center gap-2">
              <div className="flex flex-col gap-0.5">
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-5 w-5 p-0"
                  onClick={() => moveEntry(index, 'up')}
                  disabled={index === 0}
                >
                  <ChevronUp className="h-3 w-3" />
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-5 w-5 p-0"
                  onClick={() => moveEntry(index, 'down')}
                  disabled={index === draft.length - 1}
                >
                  <ChevronDown className="h-3 w-3" />
                </Button>
              </div>

              <span className="text-xs text-zinc-500 w-5">{index + 1}.</span>

              <select
                value={entry.provider}
                onChange={(e) => updateEntry(index, { provider: e.target.value })}
                className="h-8 px-2 text-sm border rounded bg-white dark:bg-zinc-900"
              >
                {availableProviders
                  .filter(p => p.status !== 'decommissioned' || p.name === entry.provider)
                  .map(p => (
                    <option key={p.name} value={p.name}>
                      {p.display_name}{p.status === 'decommissioned' ? ' (Decommissioned)' : ''}
                    </option>
                  ))}
              </select>

              <div className="flex-1 min-w-0">
                <ModelSelector
                  provider={entry.provider}
                  value={entry.model}
                  onValueChange={(model) => updateEntry(index, { model })}
                  useUserEndpoint={useUserEndpoint}
                />
              </div>

              <Button
                variant="ghost"
                size="sm"
                className={`h-7 px-2 ${isExpanded ? 'bg-zinc-100 dark:bg-zinc-800' : ''}`}
                onClick={() => toggleSettings(index)}
                title="Model settings"
              >
                <Settings2 className="h-3 w-3" />
              </Button>

              <label className="flex items-center gap-1 text-xs cursor-pointer">
                <input
                  type="checkbox"
                  checked={entry.is_enabled}
                  onChange={(e) => updateEntry(index, { is_enabled: e.target.checked })}
                  className="rounded"
                />
                On
              </label>

              <Button
                variant="ghost"
                size="sm"
                className="h-7 px-2"
                onClick={() => removeEntry(index)}
              >
                <Trash2 className="h-3 w-3 text-red-500" />
              </Button>
            </div>

            {isExpanded && (
              <div className="ml-12 pl-2 border-l-2 border-zinc-200 dark:border-zinc-700 space-y-2">
                <div className="grid grid-cols-2 gap-3">
                  <div className="space-y-1">
                    <Label className="text-xs flex items-center gap-2">
                      Temperature
                      <span className="text-zinc-400 font-normal">
                        (default: {defaults.temperature})
                      </span>
                    </Label>
                    <Input
                      type="number"
                      step="0.05"
                      min="0"
                      max="1"
                      placeholder={String(defaults.temperature)}
                      value={entry.temperature ?? ''}
                      onChange={(e) => updateEntry(index, {
                        temperature: e.target.value ? parseFloat(e.target.value) : undefined
                      })}
                      className="h-8 text-sm"
                    />
                  </div>
                  <div className="space-y-1">
                    <Label className="text-xs flex items-center gap-2">
                      Max Tokens
                      <span className="text-zinc-400 font-normal">
                        (default: {defaults.maxTokens})
                      </span>
                    </Label>
                    <Input
                      type="number"
                      step="1024"
                      min="1024"
                      max="32768"
                      placeholder={String(defaults.maxTokens)}
                      value={entry.max_tokens ?? ''}
                      onChange={(e) => updateEntry(index, {
                        max_tokens: e.target.value ? parseInt(e.target.value) : undefined
                      })}
                      className="h-8 text-sm"
                    />
                  </div>
                </div>
                <p className="text-xs text-zinc-500">
                  Leave blank to use recommended defaults for this model.
                </p>
              </div>
            )}
          </div>
        );
      })}

      <div className="flex gap-2">
        <Button variant="outline" size="sm" className={compact ? "h-7 text-xs" : ""} onClick={addEntry}>
          <Plus className="h-3 w-3 mr-1" />
          Add Entry
        </Button>
      </div>

      <div className="flex gap-2 pt-2 border-t">
        <Button size="sm" className={compact ? "h-7 text-xs" : ""} onClick={handleSave} disabled={isSaving}>
          {isSaving ? 'Saving...' : 'Save Chain'}
        </Button>
        <Button variant="outline" size="sm" className={compact ? "h-7 text-xs" : ""} onClick={cancelEditing}>
          Cancel
        </Button>
      </div>
    </div>
  );
}
