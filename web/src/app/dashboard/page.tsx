'use client';

import { useState, useEffect, useRef } from 'react';
import { useAuth } from '@clerk/nextjs';
import { useSearchParams } from 'next/navigation';
import {
  extract,
  analyze,
  createCrawlJob,
  listSchemas,
  listSavedSites,
  createSavedSite,
  updateSavedSite,
  getSavedSite,
  createSchema,
  updateSchema,
  getSchema,
  ExtractResult,
  AnalyzeResult,
  Schema,
  SavedSite,
} from '@/lib/api';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Badge } from '@/components/ui/badge';
import { toast } from 'sonner';
import { Loader2, Sparkles, Save, BookOpen, Globe, Clock, ChevronDown, ChevronUp, Play } from 'lucide-react';
import { ProgressAvatarDialog, defaultStages, AvatarStage } from '@/components/ui/progress-avatar';
import { CrawlModeSection, CrawlOptions } from '@/components/crawl-mode-section';
import { cn } from '@/lib/utils';

// Extraction-specific stages
const extractionStages: AvatarStage[] = [
  { id: 'connecting', label: 'Connecting...', frames: [] },
  { id: 'reading', label: 'Fetching page...', frames: [] },
  { id: 'analyzing', label: 'Processing content...', frames: [] },
  { id: 'generating', label: 'Extracting data...', frames: [] },
];

const defaultSchema = `name: Product
description: E-commerce product data
fields:
  - name: title
    type: string
    description: Product name
  - name: price
    type: number
    description: Current price
  - name: description
    type: string
    description: Product description`;

// Streaming result item for crawl mode
interface CrawlResult {
  url: string;
  data: unknown;
  timestamp: Date;
}

// Deep merge utility for combining crawl results
// eslint-disable-next-line @typescript-eslint/no-explicit-any
function deepMergeResults(target: Record<string, any>, source: Record<string, any>): Record<string, any> {
  const result = { ...target };

  for (const key of Object.keys(source)) {
    const sourceVal = source[key];
    const targetVal = result[key];

    if (sourceVal === null || sourceVal === undefined) {
      continue;
    }

    if (targetVal === null || targetVal === undefined) {
      result[key] = sourceVal;
      continue;
    }

    if (Array.isArray(sourceVal) && Array.isArray(targetVal)) {
      const combined = [...targetVal, ...sourceVal];
      result[key] = dedupeArray(combined);
    } else if (typeof sourceVal === 'object' && typeof targetVal === 'object' && !Array.isArray(sourceVal)) {
      result[key] = deepMergeResults(targetVal, sourceVal);
    }
  }

  return result;
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
function dedupeArray(arr: any[]): any[] {
  const seen = new Set<string>();
  return arr.filter(item => {
    const key = typeof item === 'object' ? JSON.stringify(item) : String(item);
    if (seen.has(key)) return false;
    seen.add(key);
    return true;
  });
}

function getMergedCrawlResult(results: CrawlResult[]): Record<string, unknown> {
  let merged: Record<string, unknown> = {};
  for (const result of results) {
    if (result.data && typeof result.data === 'object' && !Array.isArray(result.data)) {
      merged = deepMergeResults(merged, result.data as Record<string, unknown>);
    }
  }
  return merged;
}

function parseClerkFeatures(feaClaim: string | undefined): string[] {
  if (!feaClaim) return [];
  return feaClaim.split(',').map(f => {
    const trimmed = f.trim();
    if (trimmed.startsWith('u:')) return trimmed.slice(2);
    if (trimmed.startsWith('o:')) return trimmed.slice(2);
    return trimmed;
  }).filter(Boolean);
}

export default function DashboardPage() {
  const { sessionClaims, getToken } = useAuth();
  const searchParams = useSearchParams();
  const features = parseClerkFeatures(sessionClaims?.fea as string | undefined);
  const canCrawl = features.includes('extraction_crawled');

  // URL and extraction state
  const [url, setUrl] = useState('');
  const [schema, setSchema] = useState(defaultSchema);
  const [isLoading, setIsLoading] = useState(false);
  const [isAnalyzing, setIsAnalyzing] = useState(false);
  const [result, setResult] = useState<ExtractResult | null>(null);
  const [analysisResult, setAnalysisResult] = useState<AnalyzeResult | null>(null);

  // Crawl mode state
  const [isCrawlMode, setIsCrawlMode] = useState(false);
  const [isCrawling, setIsCrawling] = useState(false);
  const [crawlOptions, setCrawlOptions] = useState<CrawlOptions>({
    followSelector: '',
    followPattern: '',
    maxPages: 0,
    maxDepth: 1,
    useSitemap: false,
  });
  const [crawlResults, setCrawlResults] = useState<CrawlResult[]>([]);
  const [crawlProgress, setCrawlProgress] = useState({
    extracted: 0,
    maxPages: 0,
    status: 'pending' as 'pending' | 'running' | 'completed' | 'failed'
  });
  const resultsEndRef = useRef<HTMLDivElement>(null);

  // Progress avatar state
  const [showProgressAvatar, setShowProgressAvatar] = useState(false);
  const [progressStage, setProgressStage] = useState('connecting');
  const [progressComplete, setProgressComplete] = useState(false);
  const [progressError, setProgressError] = useState(false);
  const [progressErrorMessage, setProgressErrorMessage] = useState('');
  const [progressMode, setProgressMode] = useState<'analyze' | 'extract'>('analyze');

  // Catalog state
  const [schemas, setSchemas] = useState<Schema[]>([]);
  const [savedSites, setSavedSites] = useState<SavedSite[]>([]);
  const [selectedSchemaId, setSelectedSchemaId] = useState<string>('');
  const [selectedSiteId, setSelectedSiteId] = useState<string>('');

  // Dialog state
  const [showSaveSchemaDialog, setShowSaveSchemaDialog] = useState(false);
  const [newSchemaName, setNewSchemaName] = useState('');
  const [newSchemaDescription, setNewSchemaDescription] = useState('');

  // Site overwrite dialog state
  const [showSiteOverwriteDialog, setShowSiteOverwriteDialog] = useState(false);
  const [existingSiteToOverwrite, setExistingSiteToOverwrite] = useState<SavedSite | null>(null);
  const [pendingSiteData, setPendingSiteData] = useState<{
    url: string;
    name: string;
    analysis_result: AnalyzeResult;
    fetch_mode: string;
  } | null>(null);

  // Schema overwrite dialog state
  const [showSchemaOverwriteDialog, setShowSchemaOverwriteDialog] = useState(false);
  const [existingSchemaToOverwrite, setExistingSchemaToOverwrite] = useState<Schema | null>(null);
  const [pendingSchemaData, setPendingSchemaData] = useState<{
    name: string;
    description: string;
    schema_yaml: string;
    visibility: 'public' | 'private';
  } | null>(null);

  // Analysis details collapsed state
  const [analysisExpanded, setAnalysisExpanded] = useState(false);

  // Auto-scroll to latest crawl result
  useEffect(() => {
    if (crawlResults.length > 0) {
      resultsEndRef.current?.scrollIntoView({ behavior: 'smooth' });
    }
  }, [crawlResults.length]);

  // Update crawl options when analysis result has follow patterns
  useEffect(() => {
    if (analysisResult?.follow_patterns?.[0]?.pattern) {
      setCrawlOptions(prev => {
        if (prev.followSelector) {
          return prev;
        }
        return {
          ...prev,
          followSelector: analysisResult.follow_patterns[0].pattern,
        };
      });
    }
  }, [analysisResult]);

  // Load schemas and saved sites on mount
  useEffect(() => {
    loadSchemas();
    loadSavedSites();
  }, []);

  // Handle URL parameters from catalogue pages
  useEffect(() => {
    const siteId = searchParams.get('siteId');
    const schemaId = searchParams.get('schemaId');

    const loadFromParams = async () => {
      if (siteId) {
        try {
          const site = await getSavedSite(siteId);
          setUrl(site.url);
          if (site.analysis_result?.suggested_schema) {
            setSchema(site.analysis_result.suggested_schema);
          }
          setAnalysisResult(site.analysis_result || null);
          setSelectedSiteId(site.id);
          if (site.crawl_options) {
            setCrawlOptions({
              followSelector: site.crawl_options.follow_selector || '',
              followPattern: site.crawl_options.follow_pattern || '',
              maxPages: site.crawl_options.max_pages || 0,
              maxDepth: site.crawl_options.max_depth || 1,
              useSitemap: false,
            });
          }
        } catch {
          toast.error('Failed to load site');
        }
      }

      if (schemaId) {
        try {
          const schemaData = await getSchema(schemaId);
          setSchema(schemaData.schema_yaml);
          setSelectedSchemaId(schemaData.id);
        } catch {
          toast.error('Failed to load schema');
        }
      }
    };

    if (siteId || schemaId) {
      loadFromParams();
    }
  }, [searchParams]);

  const loadSchemas = async () => {
    try {
      const response = await listSchemas();
      setSchemas(response.schemas || []);
    } catch {
      // Schema loading is optional
    }
  };

  const loadSavedSites = async () => {
    try {
      const response = await listSavedSites();
      setSavedSites(response.sites || []);
    } catch {
      // Site loading is optional
    }
  };

  const normalizeUrl = (inputUrl: string): string => {
    const trimmed = inputUrl.trim();
    if (!trimmed) return trimmed;
    if (!trimmed.match(/^https?:\/\//i)) {
      return `https://${trimmed}`;
    }
    return trimmed;
  };

  const handleAnalyze = async () => {
    if (!url) {
      toast.error('Please enter a URL');
      return;
    }

    const normalizedUrl = normalizeUrl(url);

    setIsAnalyzing(true);
    setAnalysisResult(null);
    setShowProgressAvatar(true);
    setProgressMode('analyze');
    setProgressStage('connecting');
    setProgressComplete(false);
    setProgressError(false);
    setProgressErrorMessage('');

    const stageProgression = async () => {
      await new Promise(resolve => setTimeout(resolve, 800));
      setProgressStage('reading');
      await new Promise(resolve => setTimeout(resolve, 1200));
      setProgressStage('analyzing');
      await new Promise(resolve => setTimeout(resolve, 1000));
      setProgressStage('thinking');
    };

    try {
      const progressPromise = stageProgression();
      const resultPromise = analyze(normalizedUrl, 0);

      await progressPromise;
      setProgressStage('generating');

      const result = await resultPromise;
      setAnalysisResult(result);

      if (result.suggested_schema) {
        setSchema(result.suggested_schema);
      }

      setProgressComplete(true);
      toast.success('Analysis completed');
    } catch (err) {
      const error = err as { error?: string };
      setProgressError(true);
      setProgressErrorMessage(error.error || 'Analysis failed');
      toast.error(error.error || 'Analysis failed');
    } finally {
      setIsAnalyzing(false);
    }
  };

  const handleCloseProgressAvatar = () => {
    setShowProgressAvatar(false);
    setProgressComplete(false);
    setProgressError(false);
  };

  const handleExtract = async (e: React.FormEvent) => {
    e.preventDefault();

    if (!url) {
      toast.error('Please enter a URL');
      return;
    }

    const normalizedUrl = normalizeUrl(url);

    let parsedSchema;
    try {
      parsedSchema = yamlToJson(schema);
    } catch {
      toast.error('Invalid schema format');
      return;
    }

    setIsLoading(true);
    setResult(null);
    setCrawlResults([]);
    setShowProgressAvatar(true);
    setProgressMode('extract');
    setProgressStage('connecting');
    setProgressComplete(false);
    setProgressError(false);
    setProgressErrorMessage('');

    const stageProgression = async () => {
      await new Promise(resolve => setTimeout(resolve, 600));
      setProgressStage('reading');
      await new Promise(resolve => setTimeout(resolve, 1000));
      setProgressStage('analyzing');
    };

    try {
      if (isCrawlMode && canCrawl) {
        setShowProgressAvatar(false);
        setCrawlResults([]);
        setIsCrawling(true);
        setCrawlProgress({
          extracted: 0,
          maxPages: crawlOptions.maxPages,
          status: 'running'
        });

        const crawlResult = await createCrawlJob({
          url: normalizedUrl,
          schema: parsedSchema,
          options: {
            follow_selector: crawlOptions.followSelector || undefined,
            follow_pattern: crawlOptions.followPattern || undefined,
            max_pages: crawlOptions.maxPages,
            max_depth: crawlOptions.maxDepth,
            same_domain_only: true,
            extract_from_seeds: true,
            use_sitemap: crawlOptions.useSitemap,
          },
        });

        toast.info(`Crawl job started - streaming results...`);

        const apiBase = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080';
        const token = await getToken();

        const streamResponse = await fetch(
          `${apiBase}/api/v1/jobs/${crawlResult.job_id}/stream`,
          {
            headers: {
              'Authorization': `Bearer ${token}`,
              'Accept': 'text/event-stream',
            },
          }
        );

        if (!streamResponse.ok || !streamResponse.body) {
          setIsLoading(false);
          setIsCrawling(false);
          toast.error('Failed to connect to crawl stream');
          return;
        }

        const reader = streamResponse.body.getReader();
        const decoder = new TextDecoder();
        let buffer = '';

        const processStream = async () => {
          try {
            while (true) {
              const { done, value } = await reader.read();
              if (done) break;

              buffer += decoder.decode(value, { stream: true });
              const lines = buffer.split('\n');
              buffer = lines.pop() || '';

              let currentEvent: string | null = null;
              for (let i = 0; i < lines.length; i++) {
                const line = lines[i];

                if (line.startsWith('event: ')) {
                  currentEvent = line.slice(7).trim();
                } else if (line.startsWith('data: ') && currentEvent) {
                  const dataStr = line.slice(6);
                  try {
                    const data = JSON.parse(dataStr);

                    if (currentEvent === 'result') {
                      setCrawlResults(prev => [...prev, {
                        url: data.url,
                        data: data.data,
                        timestamp: new Date(),
                      }]);
                      setCrawlProgress(prev => ({
                        ...prev,
                        extracted: prev.extracted + 1
                      }));
                    } else if (currentEvent === 'status') {
                      setCrawlProgress(prev => ({
                        ...prev,
                        extracted: data.page_count || prev.extracted
                      }));
                    } else if (currentEvent === 'complete') {
                      setIsLoading(false);
                      setIsCrawling(false);
                      setCrawlProgress(prev => ({
                        ...prev,
                        status: data.status === 'completed' ? 'completed' : 'failed',
                        extracted: data.page_count || prev.extracted
                      }));
                      if (data.status === 'completed') {
                        toast.success(`Crawl completed! ${data.page_count} pages extracted.`);
                      } else {
                        toast.error(`Crawl failed: ${data.error || 'Unknown error'}`);
                      }
                      return;
                    }
                  } catch {
                    // Skip malformed JSON
                  }
                  currentEvent = null;
                } else if (line === '') {
                  currentEvent = null;
                }
              }
            }
          } catch (err) {
            console.error('Stream error:', err);
            toast.error('Connection to crawl stream lost');
          } finally {
            setIsLoading(false);
            setIsCrawling(false);
          }
        };

        processStream();
        return;
      }

      if (isCrawlMode && !canCrawl) {
        toast.error('Crawl requires a paid plan. Running single page extraction instead.');
        setIsCrawlMode(false);
      }

      const progressPromise = stageProgression();
      const resultPromise = extract(normalizedUrl, parsedSchema);

      await progressPromise;
      setProgressStage('generating');

      const extractResult = await resultPromise;
      setResult(extractResult);
      setProgressComplete(true);
      toast.success('Extraction completed');
    } catch (err) {
      const error = err as { error?: string };
      setProgressError(true);
      setProgressErrorMessage(error.error || 'Extraction failed');
      toast.error(error.error || 'Extraction failed');
    } finally {
      setIsLoading(false);
    }
  };

  const handleSchemaSelect = (schemaId: string) => {
    setSelectedSchemaId(schemaId);
    const selected = schemas.find(s => s.id === schemaId);
    if (selected) {
      setSchema(selected.schema_yaml);
    }
  };

  const handleSiteSelect = (siteId: string) => {
    setSelectedSiteId(siteId);
    const selected = savedSites.find(s => s.id === siteId);
    if (selected) {
      setUrl(selected.url);
      if (selected.analysis_result?.suggested_schema) {
        setSchema(selected.analysis_result.suggested_schema);
      }
      setAnalysisResult(selected.analysis_result || null);
      if (selected.crawl_options) {
        setCrawlOptions({
          followSelector: selected.crawl_options.follow_selector || '',
          followPattern: selected.crawl_options.follow_pattern || '',
          maxPages: selected.crawl_options.max_pages || 0,
          maxDepth: selected.crawl_options.max_depth || 1,
          useSitemap: false,
        });
      } else {
        setCrawlOptions({
          followSelector: '',
          followPattern: '',
          maxPages: 0,
          maxDepth: 1,
          useSitemap: false,
        });
      }
    }
  };

  const handleSaveSchema = async () => {
    if (!newSchemaName) {
      toast.error('Please enter a schema name');
      return;
    }

    const schemaData = {
      name: newSchemaName,
      description: newSchemaDescription,
      schema_yaml: schema,
      visibility: 'private' as const,
    };

    const existingSchema = schemas.find(s => s.name === newSchemaName && !s.is_platform);
    if (existingSchema) {
      setExistingSchemaToOverwrite(existingSchema);
      setPendingSchemaData(schemaData);
      setShowSaveSchemaDialog(false);
      setShowSchemaOverwriteDialog(true);
      return;
    }

    try {
      await createSchema(schemaData);
      toast.success('Schema saved');
      setShowSaveSchemaDialog(false);
      setNewSchemaName('');
      setNewSchemaDescription('');
      loadSchemas();
    } catch (err) {
      const error = err as { error?: string };
      toast.error(error.error || 'Failed to save schema');
    }
  };

  const handleOverwriteSchema = async () => {
    if (!existingSchemaToOverwrite || !pendingSchemaData) return;

    try {
      await updateSchema(existingSchemaToOverwrite.id, pendingSchemaData);
      toast.success('Schema updated');
      loadSchemas();
    } catch (err) {
      const error = err as { error?: string };
      toast.error(error.error || 'Failed to update schema');
    } finally {
      setShowSchemaOverwriteDialog(false);
      setExistingSchemaToOverwrite(null);
      setPendingSchemaData(null);
      setNewSchemaName('');
      setNewSchemaDescription('');
    }
  };

  const handleCreateNewSchema = async () => {
    if (!pendingSchemaData) return;

    try {
      await createSchema(pendingSchemaData);
      toast.success('Schema saved');
      loadSchemas();
    } catch (err) {
      const error = err as { error?: string };
      toast.error(error.error || 'Failed to save schema');
    } finally {
      setShowSchemaOverwriteDialog(false);
      setExistingSchemaToOverwrite(null);
      setPendingSchemaData(null);
      setNewSchemaName('');
      setNewSchemaDescription('');
    }
  };

  const handleSaveSite = async () => {
    if (!url || !analysisResult) {
      toast.error('Analyze a URL first to save it');
      return;
    }

    const normalizedUrl = normalizeUrl(url);
    let hostname = normalizedUrl;
    try {
      hostname = new URL(normalizedUrl).hostname;
    } catch {
      // Use the URL as-is if parsing fails
    }

    const cleanAnalysisResult = {
      site_summary: analysisResult.site_summary,
      page_type: analysisResult.page_type,
      detected_elements: analysisResult.detected_elements,
      suggested_schema: analysisResult.suggested_schema,
      follow_patterns: analysisResult.follow_patterns,
      sample_links: analysisResult.sample_links,
      recommended_fetch_mode: analysisResult.recommended_fetch_mode,
    };

    const siteData = {
      url: normalizedUrl,
      name: hostname,
      analysis_result: cleanAnalysisResult,
      fetch_mode: analysisResult.recommended_fetch_mode,
      crawl_options: {
        follow_selector: crawlOptions.followSelector || undefined,
        follow_pattern: crawlOptions.followPattern || undefined,
        max_pages: crawlOptions.maxPages,
        max_depth: crawlOptions.maxDepth,
      },
    };

    const existingSite = savedSites.find(s => s.url === normalizedUrl);
    if (existingSite) {
      setExistingSiteToOverwrite(existingSite);
      setPendingSiteData(siteData);
      setShowSiteOverwriteDialog(true);
      return;
    }

    try {
      await createSavedSite(siteData);
      toast.success('Site saved');
      loadSavedSites();
    } catch (err) {
      const error = err as { error?: string };
      toast.error(error.error || 'Failed to save site');
    }
  };

  const handleOverwriteSite = async () => {
    if (!existingSiteToOverwrite || !pendingSiteData) return;

    try {
      await updateSavedSite(existingSiteToOverwrite.id, pendingSiteData);
      toast.success('Site updated');
      loadSavedSites();
    } catch (err) {
      const error = err as { error?: string };
      toast.error(error.error || 'Failed to update site');
    } finally {
      setShowSiteOverwriteDialog(false);
      setExistingSiteToOverwrite(null);
      setPendingSiteData(null);
    }
  };

  const handleCreateNewSite = async () => {
    if (!pendingSiteData) return;

    try {
      await createSavedSite(pendingSiteData);
      toast.success('Site saved');
      loadSavedSites();
    } catch (err) {
      const error = err as { error?: string };
      toast.error(error.error || 'Failed to save site');
    } finally {
      setShowSiteOverwriteDialog(false);
      setExistingSiteToOverwrite(null);
      setPendingSiteData(null);
    }
  };

  const yamlToJson = (yaml: string): object => {
    const lines = yaml.split('\n');
    const result: Record<string, unknown> = {};
    let currentKey = '';
    let currentArray: Record<string, unknown>[] = [];
    let inArray = false;
    let arrayKey = '';

    for (const line of lines) {
      const trimmed = line.trim();
      if (!trimmed || trimmed.startsWith('#')) continue;

      if (trimmed.startsWith('- ')) {
        const content = trimmed.substring(2);
        if (content.includes(':')) {
          const [key, ...valueParts] = content.split(':');
          const value = valueParts.join(':').trim();
          if (inArray) {
            const lastItem = currentArray[currentArray.length - 1] || {};
            lastItem[key.trim()] = parseValue(value);
            if (currentArray.length === 0) {
              currentArray.push(lastItem);
            }
          }
        } else {
          currentArray.push({});
        }
        if (!inArray) {
          inArray = true;
        }
      } else if (trimmed.includes(':')) {
        if (inArray && arrayKey) {
          result[arrayKey] = currentArray;
          currentArray = [];
          inArray = false;
        }

        const [key, ...valueParts] = trimmed.split(':');
        const value = valueParts.join(':').trim();
        currentKey = key.trim();

        if (!value) {
          arrayKey = currentKey;
        } else {
          result[currentKey] = parseValue(value);
        }
      } else if (inArray && trimmed) {
        const [key, ...valueParts] = trimmed.split(':');
        const value = valueParts.join(':').trim();
        const lastItem = currentArray[currentArray.length - 1];
        if (lastItem) {
          lastItem[key.trim()] = parseValue(value);
        }
      }
    }

    if (inArray && arrayKey) {
      result[arrayKey] = currentArray;
    }

    return result;
  };

  const parseValue = (value: string): unknown => {
    if (!value) return '';
    if (value === 'true') return true;
    if (value === 'false') return false;
    if (!isNaN(Number(value))) return Number(value);
    return value.replace(/^["']|["']$/g, '');
  };

  const getTotalTime = () => {
    if (!result) return null;
    const total = (result.metadata.fetch_duration_ms || 0) + (result.metadata.extract_duration_ms || 0);
    return total > 1000 ? `${(total / 1000).toFixed(1)}s` : `${total}ms`;
  };

  const hasResults = result !== null || crawlResults.length > 0 || isCrawling;

  return (
    <div className="flex flex-col h-full gap-4">
      {/* Page Header */}
      <div>
        <h1 className="text-3xl font-bold tracking-tight">Extract Data</h1>
        <p className="mt-2 text-zinc-600 dark:text-zinc-400">
          Analyze URLs and extract structured data using LLM-powered extraction.
        </p>
      </div>

      {/* Progress Avatar Dialog */}
      <ProgressAvatarDialog
        open={showProgressAvatar}
        stages={progressMode === 'analyze' ? defaultStages : extractionStages}
        currentStage={progressStage}
        isComplete={progressComplete}
        isError={progressError}
        errorMessage={progressErrorMessage}
        onClose={handleCloseProgressAvatar}
      />

      {/* SECTION 1: Target URL */}
      <div className="rounded-lg border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 shadow-sm">
        <div className="p-4">
          <div className="flex flex-col sm:flex-row gap-3">
            <div className="flex-1 flex gap-2">
              <Input
                type="url"
                placeholder="https://example.com/product"
                value={url}
                onChange={(e) => setUrl(e.target.value)}
                disabled={isLoading || isAnalyzing}
                className="flex-1 font-mono text-sm"
              />
              <Select value={selectedSiteId} onValueChange={handleSiteSelect}>
                <SelectTrigger className="w-[140px]">
                  <Globe className="h-4 w-4 mr-2 shrink-0" />
                  <span className="truncate">
                    <SelectValue placeholder="Sites" />
                  </span>
                </SelectTrigger>
                <SelectContent>
                  {savedSites.map((s) => (
                    <SelectItem key={s.id} value={s.id}>
                      {s.name || s.domain}
                    </SelectItem>
                  ))}
                  {savedSites.length === 0 && (
                    <SelectItem value="none" disabled>
                      No saved sites
                    </SelectItem>
                  )}
                </SelectContent>
              </Select>
              <Button
                variant="secondary"
                onClick={handleAnalyze}
                disabled={isLoading || isAnalyzing || !url}
                className="shrink-0"
              >
                {isAnalyzing ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <Sparkles className="h-4 w-4" />
                )}
                <span className="ml-2 hidden sm:inline">Analyze</span>
              </Button>
            </div>
          </div>
        </div>

        {/* Analysis Summary */}
        {analysisResult && (
          <div className="border-t border-zinc-200 dark:border-zinc-800 px-4 py-3 bg-zinc-50 dark:bg-zinc-900/50">
            <div className="flex items-start justify-between gap-4">
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2 mb-1">
                  <Sparkles className="h-4 w-4 text-amber-500 shrink-0" />
                  <span className="text-sm font-medium truncate">
                    {analysisResult.page_type} page
                  </span>
                  <Badge variant="outline" className="text-xs shrink-0">
                    {analysisResult.recommended_fetch_mode}
                  </Badge>
                  <span className="text-xs text-zinc-500 shrink-0">
                    {analysisResult.detected_elements.length} fields detected
                  </span>
                </div>
                <p className="text-sm text-zinc-600 dark:text-zinc-400 line-clamp-1">
                  {analysisResult.site_summary}
                </p>
                {analysisExpanded && (
                  <div className="mt-3 flex flex-wrap gap-1.5">
                    {analysisResult.detected_elements.map((elem, i) => (
                      <Badge key={i} variant="secondary" className="text-xs font-normal">
                        {elem.name}
                        <span className="text-zinc-400 ml-1">({elem.type})</span>
                      </Badge>
                    ))}
                  </div>
                )}
              </div>
              <div className="flex items-center gap-2 shrink-0">
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => setAnalysisExpanded(!analysisExpanded)}
                  className="h-8 w-8 p-0"
                >
                  {analysisExpanded ? (
                    <ChevronUp className="h-4 w-4" />
                  ) : (
                    <ChevronDown className="h-4 w-4" />
                  )}
                </Button>
                <Button variant="outline" size="sm" onClick={handleSaveSite}>
                  <Save className="h-4 w-4 mr-1" />
                  Save
                </Button>
              </div>
            </div>
          </div>
        )}
      </div>

      {/* SECTION 2: Extraction Mode */}
      <CrawlModeSection
        isCrawlMode={isCrawlMode}
        onModeChange={setIsCrawlMode}
        crawlOptions={crawlOptions}
        onOptionsChange={setCrawlOptions}
        suggestedSelectors={analysisResult?.follow_patterns}
        canCrawl={canCrawl}
        disabled={isLoading || isAnalyzing}
      />

      {/* SECTION 3: Schema Editor */}
      <div className="flex-1 rounded-lg border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 shadow-sm flex flex-col min-h-0">
        {/* Schema Header */}
        <div className="flex items-center justify-between border-b border-zinc-200 dark:border-zinc-800 px-4 py-2">
          <div className="flex items-center gap-3">
            <span className="text-sm font-medium">Schema</span>
            <Select value={selectedSchemaId} onValueChange={handleSchemaSelect}>
              <SelectTrigger className="h-8 w-[160px] text-xs">
                <BookOpen className="h-3 w-3 mr-1.5 shrink-0" />
                <span className="truncate">
                  <SelectValue placeholder="Load schema..." />
                </span>
              </SelectTrigger>
              <SelectContent>
                {schemas.map((s) => (
                  <SelectItem key={s.id} value={s.id}>
                    <div className="flex items-center gap-2">
                      {s.name}
                      {s.is_platform && (
                        <Badge variant="secondary" className="text-xs">
                          Platform
                        </Badge>
                      )}
                    </div>
                  </SelectItem>
                ))}
                {schemas.length === 0 && (
                  <SelectItem value="none" disabled>
                    No schemas available
                  </SelectItem>
                )}
              </SelectContent>
            </Select>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setShowSaveSchemaDialog(true)}
              className="h-8 text-xs"
            >
              <Save className="h-3 w-3 mr-1" />
              Save
            </Button>
          </div>
          <Button
            size="sm"
            onClick={handleExtract}
            disabled={isLoading || !url}
            className={cn('h-8', isCrawlMode && canCrawl && 'bg-amber-600 hover:bg-amber-700')}
          >
            {isLoading ? (
              <Loader2 className="h-3 w-3 animate-spin mr-1" />
            ) : (
              <Play className="h-3 w-3 mr-1" />
            )}
            {isCrawlMode && canCrawl ? 'Crawl & Extract' : 'Extract'}
          </Button>
        </div>

        {/* Schema Editor */}
        <form onSubmit={handleExtract} className="flex-1 min-h-0 flex flex-col">
          <Textarea
            placeholder="Enter your extraction schema in YAML format..."
            value={schema}
            onChange={(e) => setSchema(e.target.value)}
            className="flex-1 font-mono text-sm border-0 rounded-none resize-none focus-visible:ring-0 focus-visible:ring-offset-0"
            disabled={isLoading}
          />
        </form>
      </div>

      {/* SECTION 4: Results (shown when available) */}
      {hasResults && (
        <div className="rounded-lg border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 shadow-sm flex flex-col min-h-[300px] max-h-[500px]">
          {/* Results Header */}
          <div className="flex items-center justify-between border-b border-zinc-200 dark:border-zinc-800 px-4 py-2 shrink-0">
            <div className="flex items-center gap-3">
              <span className="text-sm font-medium">Results</span>
              {isCrawling || crawlResults.length > 0 ? (
                <Badge variant={isCrawling ? 'default' : 'secondary'} className="text-xs">
                  {crawlResults.length} pages
                </Badge>
              ) : result && (
                <span className="text-xs text-zinc-500">{result.url}</span>
              )}
            </div>
            {result && (
              <div className="flex items-center gap-3 text-xs text-zinc-500">
                <span className="flex items-center gap-1">
                  <Clock className="h-3 w-3" />
                  {getTotalTime()}
                </span>
                {result.usage.cost_usd && result.usage.cost_usd > 0 && (
                  <span>${result.usage.cost_usd.toFixed(4)}</span>
                )}
              </div>
            )}
          </div>

          {/* Results Content */}
          <div className="flex-1 overflow-hidden flex flex-col min-h-0">
            {isCrawling || crawlResults.length > 0 ? (
              // Crawl results
              <>
                {/* Crawl Progress */}
                <div className="px-4 py-2 border-b border-zinc-200 dark:border-zinc-800 bg-zinc-50 dark:bg-zinc-800/50 shrink-0">
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-4">
                      {isCrawling ? (
                        <>
                          <div className="flex items-center gap-2 text-sm font-medium">
                            <Loader2 className="h-4 w-4 animate-spin text-amber-500" />
                            <span>Crawling...</span>
                          </div>
                          <div className="flex items-center gap-4 text-sm text-zinc-500">
                            <span className="flex items-center gap-1">
                              <span className="font-medium text-zinc-700 dark:text-zinc-300">
                                {crawlProgress.extracted}
                              </span>
                              {crawlProgress.maxPages > 0 && (
                                <>
                                  <span>/</span>
                                  <span>{crawlProgress.maxPages}</span>
                                </>
                              )}
                              <span className="text-zinc-400">pages</span>
                            </span>
                          </div>
                        </>
                      ) : (
                        <div className="text-sm font-medium text-green-600 dark:text-green-400">
                          Crawl complete
                        </div>
                      )}
                    </div>
                    {crawlProgress.maxPages > 0 ? (
                      <div className="w-32 h-1.5 bg-zinc-200 dark:bg-zinc-700 rounded-full overflow-hidden">
                        <div
                          className={cn(
                            "h-full transition-all duration-300 rounded-full",
                            isCrawling ? "bg-amber-500" : "bg-green-500"
                          )}
                          style={{
                            width: `${Math.min(100, (crawlProgress.extracted / crawlProgress.maxPages) * 100)}%`
                          }}
                        />
                      </div>
                    ) : isCrawling ? (
                      <div className="w-32 h-1.5 bg-zinc-200 dark:bg-zinc-700 rounded-full overflow-hidden">
                        <div className="h-full w-full bg-amber-500 animate-pulse rounded-full" />
                      </div>
                    ) : (
                      <div className="w-32 h-1.5 bg-green-500 rounded-full" />
                    )}
                  </div>
                </div>

                {/* Merged Results */}
                {crawlResults.length > 0 ? (
                  <>
                    <div className="flex-1 overflow-auto bg-zinc-950 min-h-0">
                      <pre className="p-4 text-sm text-zinc-300 min-h-full">
                        {JSON.stringify(getMergedCrawlResult(crawlResults), null, 2)}
                      </pre>
                    </div>
                    <details className="border-t border-zinc-200 dark:border-zinc-800 bg-zinc-50 dark:bg-zinc-900 shrink-0">
                      <summary className="px-4 py-2 text-xs text-zinc-500 cursor-pointer hover:text-zinc-700 dark:hover:text-zinc-300">
                        Source pages ({crawlResults.length})
                      </summary>
                      <div className="px-4 pb-2 space-y-1 max-h-32 overflow-auto">
                        {crawlResults.map((item, idx) => (
                          <div key={idx} className="text-xs font-mono text-zinc-400 truncate">
                            {item.url}
                          </div>
                        ))}
                      </div>
                    </details>
                  </>
                ) : isCrawling ? (
                  <div className="flex-1 flex items-center justify-center text-zinc-400">
                    <div className="text-center">
                      <Loader2 className="h-6 w-6 animate-spin mx-auto mb-2" />
                      <p>Waiting for first extraction result...</p>
                    </div>
                  </div>
                ) : null}
                <div ref={resultsEndRef} />
              </>
            ) : result ? (
              // Single extraction result
              <div className="flex-1 overflow-auto bg-zinc-950 min-h-0">
                <pre className="p-4 text-sm text-zinc-300 min-h-full">
                  {JSON.stringify(result.data, null, 2)}
                </pre>
              </div>
            ) : null}
          </div>
        </div>
      )}

      {/* Save Schema Dialog */}
      <Dialog open={showSaveSchemaDialog} onOpenChange={setShowSaveSchemaDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Save Schema</DialogTitle>
            <DialogDescription>
              Save this schema to your catalog for future use.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <div className="space-y-2">
              <Label htmlFor="schema-name">Name</Label>
              <Input
                id="schema-name"
                placeholder="My Product Schema"
                value={newSchemaName}
                onChange={(e) => setNewSchemaName(e.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="schema-description">Description (optional)</Label>
              <Textarea
                id="schema-description"
                placeholder="Schema for extracting product data..."
                value={newSchemaDescription}
                onChange={(e) => setNewSchemaDescription(e.target.value)}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowSaveSchemaDialog(false)}>
              Cancel
            </Button>
            <Button onClick={handleSaveSchema}>Save Schema</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Site Overwrite Confirmation Dialog */}
      <Dialog open={showSiteOverwriteDialog} onOpenChange={(open) => {
        if (!open) {
          setShowSiteOverwriteDialog(false);
          setExistingSiteToOverwrite(null);
          setPendingSiteData(null);
        }
      }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Site Already Exists</DialogTitle>
            <DialogDescription>
              A saved site with this URL already exists. What would you like to do?
            </DialogDescription>
          </DialogHeader>
          <div className="py-4">
            <div className="rounded-md bg-zinc-100 dark:bg-zinc-800 p-3">
              <p className="text-sm font-medium text-zinc-700 dark:text-zinc-300">
                Existing site:
              </p>
              <p className="text-sm text-zinc-500 dark:text-zinc-400 font-mono mt-1 truncate">
                {existingSiteToOverwrite?.url}
              </p>
              {existingSiteToOverwrite?.name && (
                <p className="text-xs text-zinc-400 mt-1">
                  Saved as: {existingSiteToOverwrite.name}
                </p>
              )}
            </div>
          </div>
          <DialogFooter className="flex-col sm:flex-row gap-2">
            <Button
              variant="outline"
              onClick={() => {
                setShowSiteOverwriteDialog(false);
                setExistingSiteToOverwrite(null);
                setPendingSiteData(null);
              }}
            >
              Cancel
            </Button>
            <Button variant="secondary" onClick={handleCreateNewSite}>
              Create New
            </Button>
            <Button onClick={handleOverwriteSite}>
              Overwrite Existing
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Schema Overwrite Confirmation Dialog */}
      <Dialog open={showSchemaOverwriteDialog} onOpenChange={(open) => {
        if (!open) {
          setShowSchemaOverwriteDialog(false);
          setExistingSchemaToOverwrite(null);
          setPendingSchemaData(null);
        }
      }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Schema Already Exists</DialogTitle>
            <DialogDescription>
              A schema with this name already exists. What would you like to do?
            </DialogDescription>
          </DialogHeader>
          <div className="py-4">
            <div className="rounded-md bg-zinc-100 dark:bg-zinc-800 p-3">
              <p className="text-sm font-medium text-zinc-700 dark:text-zinc-300">
                Existing schema:
              </p>
              <p className="text-sm text-zinc-500 dark:text-zinc-400 font-medium mt-1">
                {existingSchemaToOverwrite?.name}
              </p>
              {existingSchemaToOverwrite?.description && (
                <p className="text-xs text-zinc-400 mt-1">
                  {existingSchemaToOverwrite.description}
                </p>
              )}
            </div>
          </div>
          <DialogFooter className="flex-col sm:flex-row gap-2">
            <Button
              variant="outline"
              onClick={() => {
                setShowSchemaOverwriteDialog(false);
                setExistingSchemaToOverwrite(null);
                setPendingSchemaData(null);
              }}
            >
              Cancel
            </Button>
            <Button variant="secondary" onClick={handleCreateNewSchema}>
              Create New
            </Button>
            <Button onClick={handleOverwriteSchema}>
              Overwrite Existing
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
