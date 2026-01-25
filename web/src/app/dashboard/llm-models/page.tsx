'use client';

import { useEffect, useState, useCallback } from 'react';
import {
  listUserServiceKeys,
  getUserFallbackChain,
  setUserFallbackChain,
  listLLMProviders,
  UserServiceKey,
  UserFallbackChainEntry,
  LLMProvider,
} from '@/lib/api';
import { useAuth } from '@clerk/nextjs';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { FallbackChainEditor, ChainEntry, SavedChainEntry } from '@/components/fallback-chain-editor';
import { toast } from 'sonner';
import { Lock, Info } from 'lucide-react';
import { parseClerkFeatures } from '@/lib/utils';
import { RefyneText } from '@/components/refyne-logo';

export default function LLMModelsPage() {
  const { sessionClaims } = useAuth();
  const [isLoading, setIsLoading] = useState(true);

  // Check features enabled for this user
  const features = parseClerkFeatures(
    sessionClaims?.fea as string | undefined,
    sessionClaims?.public_metadata as Record<string, unknown> | undefined
  );
  const modelsCustomEnabled = features.includes('models_custom');
  const byokEnabled = features.includes('provider_byok');

  // Provider keys state (to show which providers are configured)
  const [providerKeys, setProviderKeys] = useState<UserServiceKey[]>([]);
  // Fallback chain state
  const [chain, setChain] = useState<UserFallbackChainEntry[]>([]);
  // Available providers based on user features
  const [availableProviders, setAvailableProviders] = useState<LLMProvider[]>([]);

  const loadData = useCallback(async () => {
    try {
      const [keysData, chainData, providersData] = await Promise.all([
        listUserServiceKeys(),
        getUserFallbackChain(),
        listLLMProviders(),
      ]);
      setProviderKeys(keysData.keys || []);
      setChain(chainData.chain || []);
      setAvailableProviders(providersData.providers || []);
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

  const configuredProviders = new Set(
    providerKeys.filter(k => k.is_enabled).map(k => k.provider)
  );

  // Fallback chain save handler
  const handleSaveChain = async (entries: ChainEntry[]) => {
    try {
      const result = await setUserFallbackChain(entries.map(e => ({
        provider: e.provider,
        model: e.model,
        temperature: e.temperature,
        max_tokens: e.max_tokens,
        is_enabled: e.is_enabled,
      })));
      setChain(result.chain);
      toast.success('Fallback chain saved');
    } catch (err) {
      const error = err as { error?: string };
      toast.error(error.error || 'Failed to save chain');
      throw err; // Re-throw so the component knows save failed
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
        <h1 className="text-2xl font-bold tracking-tight">LLM Models</h1>
        <p className="text-sm text-zinc-600 dark:text-zinc-400">
          Configure your model fallback chain for extractions. Models are tried in order until one succeeds.
        </p>
      </div>

      {/* Feature Gate Alert */}
      {!modelsCustomEnabled && (
        <Alert>
          <Lock className="h-4 w-4" />
          <AlertDescription className="text-sm">
            <span className="font-medium">Custom Model Selection</span> is available on paid plans.
            Configure your own model fallback chain to control which models are used for extractions.{' '}
            <a href="/dashboard/billing" className="underline font-medium hover:text-zinc-900 dark:hover:text-zinc-100">
              Upgrade to enable this feature.
            </a>
            {chain.length > 0 && (
              <span className="block mt-2 text-zinc-500">
                Your saved configuration will be restored when you upgrade.
              </span>
            )}
          </AlertDescription>
        </Alert>
      )}

      {/* Info about key usage */}
      {modelsCustomEnabled && !byokEnabled && (
        <Alert>
          <Info className="h-4 w-4" />
          <AlertDescription className="text-sm">
            Your custom model chain will use <span className="font-medium">system API keys</span>.
            To use your own API keys, enable{' '}
            <a href="/dashboard/llm-keys" className="underline font-medium hover:text-zinc-900 dark:hover:text-zinc-100">
              BYOK (Bring Your Own Key)
            </a>{' '}
            on your account.
          </AlertDescription>
        </Alert>
      )}

      {/* Fallback Chain */}
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-base">Model Fallback Chain</CardTitle>
          <CardDescription className="text-xs">
            Configure the order of models to try for extractions. The first successful model wins.
            Click the settings icon on each entry to configure temperature and max tokens.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className={!modelsCustomEnabled ? 'opacity-60 pointer-events-none' : ''}>
            <FallbackChainEditor
              chain={chain as SavedChainEntry[]}
              configuredProviders={configuredProviders}
              onSave={handleSaveChain}
              useUserEndpoint
              compact
              disabled={!modelsCustomEnabled}
              availableProviders={availableProviders}
            />
          </div>
        </CardContent>
      </Card>

      {/* Help Section */}
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-base">How the Fallback Chain Works</CardTitle>
        </CardHeader>
        <CardContent className="text-sm space-y-3 text-zinc-600 dark:text-zinc-400">
          <p>
            When you submit an extraction request, <RefyneText /> tries each model in your fallback chain in order:
          </p>
          <ol className="list-decimal list-inside space-y-1 pl-2">
            <li>The first model in your chain is tried</li>
            <li>If it fails (rate limit, error, etc.), the next model is tried</li>
            <li>This continues until a model succeeds or all models fail</li>
          </ol>
          <p className="pt-2">
            <strong>Tips:</strong>
          </p>
          <ul className="list-disc list-inside space-y-1 pl-2">
            <li>Put your preferred (usually fastest or cheapest) model first</li>
            <li>Add backup models in case your primary is rate-limited</li>
            <li>Use the settings icon to adjust temperature for more creative or deterministic outputs</li>
            <li>Disable entries temporarily without removing them from your chain</li>
          </ul>
        </CardContent>
      </Card>
    </div>
  );
}
