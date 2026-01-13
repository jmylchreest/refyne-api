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
import { Loader2, Sparkles, Save, BookOpen, Globe, Clock, ChevronDown, ChevronUp, Play, Layers, Lock, HelpCircle } from 'lucide-react';
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { ProgressAvatarDialog, defaultStages, AvatarStage } from '@/components/ui/progress-avatar';
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

// Tab component for the main editor section
function EditorTab({
  active,
  onClick,
  children,
  badge,
}: {
  active: boolean;
  onClick: () => void;
  children: React.ReactNode;
  badge?: string | number;
}) {
  return (
    <button
      onClick={onClick}
      className={cn(
        'px-4 py-2 text-sm font-medium border-b-2 transition-colors cursor-pointer',
        active
          ? 'border-zinc-900 dark:border-zinc-100 text-zinc-900 dark:text-zinc-100'
          : 'border-transparent text-zinc-500 dark:text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-300'
      )}
    >
      <span className="flex items-center gap-2">
        {children}
        {badge !== undefined && (
          <span className="px-1.5 py-0.5 text-xs bg-zinc-200 dark:bg-zinc-700 rounded">
            {badge}
          </span>
        )}
      </span>
    </button>
  );
}

// Streaming result item for crawl mode
interface CrawlResult {
  url: string;
  data: unknown;
  timestamp: Date;
}

// Helper to parse Clerk features from the "fea" claim
function parseClerkFeatures(feaClaim: string | undefined): string[] {
  if (!feaClaim) return [];
  return feaClaim.split(',').map(f => {
    const trimmed = f.trim();
    // Strip u: or o: prefix
    if (trimmed.startsWith('u:')) return trimmed.slice(2);
    if (trimmed.startsWith('o:')) return trimmed.slice(2);
    return trimmed;
  }).filter(Boolean);
}

export default function DashboardPage() {
  // Clerk auth for feature checks and token access
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

  // Editor tab state
  const [activeTab, setActiveTab] = useState<'schema' | 'result'>('schema');

  // Crawl mode state
  const [isCrawlMode, setIsCrawlMode] = useState(false);
  const [isCrawling, setIsCrawling] = useState(false); // Track active crawl for UI
  const [crawlOptions, setCrawlOptions] = useState({
    followSelector: '',
    followPattern: '',
    maxPages: 0, // 0 means no limit
    maxDepth: 1,
  });
  const [crawlResults, setCrawlResults] = useState<CrawlResult[]>([]);
  const [crawlProgress, setCrawlProgress] = useState({
    extracted: 0,
    maxPages: 0,
    status: 'pending' as 'pending' | 'running' | 'completed' | 'failed'
  });
  const resultsEndRef = useRef<HTMLDivElement>(null);

  // Progress avatar state (shared for analyze and extract)
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

  // Auto-switch to result tab when we get results
  useEffect(() => {
    if (result || crawlResults.length > 0) {
      setActiveTab('result');
    }
  }, [result, crawlResults.length]);

  // Auto-scroll to latest crawl result
  useEffect(() => {
    if (crawlResults.length > 0) {
      resultsEndRef.current?.scrollIntoView({ behavior: 'smooth' });
    }
  }, [crawlResults.length]);

  // Update crawl options when analysis result has follow patterns
  // Only auto-populate if no selector is currently set (fresh analysis, not loading saved site)
  useEffect(() => {
    if (analysisResult?.follow_patterns?.[0]?.pattern) {
      setCrawlOptions(prev => {
        // Don't overwrite if we already have a follow selector (loading saved site)
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
          // Load crawl options if present
          if (site.crawl_options) {
            setCrawlOptions({
              followSelector: site.crawl_options.follow_selector || '',
              followPattern: site.crawl_options.follow_pattern || '',
              maxPages: site.crawl_options.max_pages || 0,
              maxDepth: site.crawl_options.max_depth || 1,
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
      // Schema loading is optional, don't show error
    }
  };

  const loadSavedSites = async () => {
    try {
      const response = await listSavedSites();
      setSavedSites(response.sites || []);
    } catch {
      // Site loading is optional, don't show error
    }
  };

  // Normalize URL to ensure it has a scheme
  const normalizeUrl = (inputUrl: string): string => {
    const trimmed = inputUrl.trim();
    if (!trimmed) return trimmed;
    // Add https:// if no scheme present
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

    // Reset progress state
    setIsAnalyzing(true);
    setAnalysisResult(null);
    setShowProgressAvatar(true);
    setProgressMode('analyze');
    setProgressStage('connecting');
    setProgressComplete(false);
    setProgressError(false);
    setProgressErrorMessage('');

    // Simulate stage progression (in reality, this would come from SSE/WebSocket)
    const stageProgression = async () => {
      await new Promise(resolve => setTimeout(resolve, 800));
      setProgressStage('reading');
      await new Promise(resolve => setTimeout(resolve, 1200));
      setProgressStage('analyzing');
      await new Promise(resolve => setTimeout(resolve, 1000));
      setProgressStage('thinking');
    };

    try {
      // Start stage progression in parallel with the actual request
      const progressPromise = stageProgression();
      const resultPromise = analyze(normalizedUrl, 0);

      // Wait for both
      await progressPromise;
      setProgressStage('generating');

      const result = await resultPromise;
      setAnalysisResult(result);

      // Auto-populate the schema from analysis
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

    // Convert YAML to JSON for the API
    let parsedSchema;
    try {
      parsedSchema = yamlToJson(schema);
    } catch {
      toast.error('Invalid schema format');
      return;
    }

    // Reset state
    setIsLoading(true);
    setResult(null);
    setCrawlResults([]);
    setShowProgressAvatar(true);
    setProgressMode('extract');
    setProgressStage('connecting');
    setProgressComplete(false);
    setProgressError(false);
    setProgressErrorMessage('');

    // Simulate stage progression for extraction
    const stageProgression = async () => {
      await new Promise(resolve => setTimeout(resolve, 600));
      setProgressStage('reading');
      await new Promise(resolve => setTimeout(resolve, 1000));
      setProgressStage('analyzing');
    };

    try {
      if (isCrawlMode && canCrawl) {
        // Use crawl API for multi-page extraction
        setShowProgressAvatar(false); // Crawl jobs are async, stream via SSE
        setCrawlResults([]); // Clear previous results
        setIsCrawling(true); // Mark crawl as active for UI
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
          },
        });

        // Switch to results tab to show streaming results
        setActiveTab('result');
        toast.info(`Crawl job started - streaming results...`);

        // Connect to SSE stream for real-time results using fetch (supports auth headers)
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

        // Process SSE stream
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
              buffer = lines.pop() || ''; // Keep incomplete line in buffer

              // Parse SSE events - format is "event: type\ndata: json\n\n"
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
                  currentEvent = null; // Reset after processing data
                } else if (line === '') {
                  // Empty line marks end of event, reset
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
        // User doesn't have crawl feature - show upgrade message
        toast.error('Crawl requires a paid plan. Running single page extraction instead.');
        setIsCrawlMode(false);
      }

      // Start stage progression in parallel with the actual request
      const progressPromise = stageProgression();
      const resultPromise = extract(normalizedUrl, parsedSchema);

      // Wait for progress stages
      await progressPromise;
      setProgressStage('generating');

      // Wait for actual result
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
      // Load crawl options if present
      if (selected.crawl_options) {
        setCrawlOptions({
          followSelector: selected.crawl_options.follow_selector || '',
          followPattern: selected.crawl_options.follow_pattern || '',
          maxPages: selected.crawl_options.max_pages || 0,
          maxDepth: selected.crawl_options.max_depth || 1,
        });
      } else {
        // Reset to defaults if no crawl options saved
        setCrawlOptions({
          followSelector: '',
          followPattern: '',
          maxPages: 0,
          maxDepth: 1,
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

    // Check if a schema with this name already exists (only user's own schemas, not platform)
    const existingSchema = schemas.find(s => s.name === newSchemaName && !s.is_platform);
    if (existingSchema) {
      // Show confirmation dialog
      setExistingSchemaToOverwrite(existingSchema);
      setPendingSchemaData(schemaData);
      setShowSaveSchemaDialog(false);
      setShowSchemaOverwriteDialog(true);
      return;
    }

    // No existing schema, create new
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

    // Build analysis result with only the fields the backend expects
    // (strip out sample_data and any other extra fields)
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

    // Check if a site with this URL already exists
    const existingSite = savedSites.find(s => s.url === normalizedUrl);
    if (existingSite) {
      // Show confirmation dialog
      setExistingSiteToOverwrite(existingSite);
      setPendingSiteData(siteData);
      setShowSiteOverwriteDialog(true);
      return;
    }

    // No existing site, create new
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

  // Simple YAML to JSON converter for basic schemas
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
        // Array item
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
          // Could be start of an array or nested object
          arrayKey = currentKey;
        } else {
          result[currentKey] = parseValue(value);
        }
      } else if (inArray && trimmed) {
        // Continuation of array item
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
    <div className="flex flex-col h-full">
      <div className="mb-6">
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

      {/* URL Input Section */}
      <div className="rounded-lg border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 shadow-sm mb-4">
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

        {/* Compact Analysis Summary */}
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

      {/* Main Editor Section - Schema/Result Tabs */}
      <div className="flex-1 rounded-lg border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 shadow-sm flex flex-col min-h-0">
        {/* Tab Header */}
        <div className="flex items-center justify-between border-b border-zinc-200 dark:border-zinc-800 px-2">
          <div className="flex">
            <EditorTab active={activeTab === 'schema'} onClick={() => setActiveTab('schema')}>
              Schema
            </EditorTab>
            {hasResults && (
              <EditorTab
                active={activeTab === 'result'}
                onClick={() => setActiveTab('result')}
                badge={isCrawlMode && crawlResults.length > 0 ? crawlResults.length : undefined}
              >
                Result
              </EditorTab>
            )}
          </div>
          <div className="flex items-center gap-2 py-2 pr-2">
            {activeTab === 'schema' && (
              <>
                <Select value={selectedSchemaId} onValueChange={handleSchemaSelect}>
                  <SelectTrigger className="h-7 w-[130px] text-xs">
                    <BookOpen className="h-3 w-3 mr-1.5 shrink-0" />
                    <span className="truncate">
                      <SelectValue placeholder="Schemas" />
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
                  className="h-7 text-xs"
                >
                  <Save className="h-3 w-3 mr-1" />
                  Save
                </Button>
              </>
            )}
            {/* Mode toggle */}
            <button
              onClick={() => setIsCrawlMode(!isCrawlMode)}
              className={cn(
                'h-7 px-2 text-xs font-medium rounded-md transition-colors cursor-pointer flex items-center gap-1',
                isCrawlMode
                  ? 'bg-amber-100 dark:bg-amber-900/30 text-amber-700 dark:text-amber-400 border border-amber-300 dark:border-amber-700'
                  : 'text-zinc-500 dark:text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-300 hover:bg-zinc-100 dark:hover:bg-zinc-800'
              )}
              title={
                !canCrawl
                  ? 'Crawl requires a paid plan'
                  : isCrawlMode
                    ? 'Crawl mode enabled - will follow links'
                    : 'Click to enable crawl mode'
              }
            >
              <Layers className="h-3 w-3" />
              <span>Crawl</span>
              {!canCrawl && <Lock className="h-3 w-3 text-zinc-400" />}
              {canCrawl && isCrawlMode && (
                <span className="ml-0.5 w-1.5 h-1.5 rounded-full bg-amber-500 animate-pulse" />
              )}
            </button>
            {activeTab === 'schema' && (
              <Button
                size="sm"
                onClick={handleExtract}
                disabled={isLoading || !url}
                className={cn('h-7', isCrawlMode && 'bg-amber-600 hover:bg-amber-700')}
              >
                {isLoading ? (
                  <Loader2 className="h-3 w-3 animate-spin mr-1" />
                ) : (
                  <Play className="h-3 w-3 mr-1" />
                )}
                {isCrawlMode ? 'Crawl & Extract' : 'Extract'}
              </Button>
            )}
            {activeTab === 'result' && result && (
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
        </div>

        {/* Crawl Options Section */}
        {isCrawlMode && canCrawl && activeTab === 'schema' && (
          <TooltipProvider>
            <div className="border-b border-zinc-200 dark:border-zinc-800 bg-amber-50 dark:bg-amber-900/10 px-4 py-4">
              <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
                {/* Link Selection - stacked vertically, takes 2 columns */}
                <div className="lg:col-span-2 space-y-4">
                  <div className="space-y-1">
                    <div className="flex items-center gap-1.5">
                      <Label htmlFor="follow-selector" className="text-sm text-zinc-700 dark:text-zinc-300">
                        CSS Selectors
                      </Label>
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <HelpCircle className="h-3.5 w-3.5 text-zinc-400 cursor-help" />
                        </TooltipTrigger>
                        <TooltipContent side="right" className="max-w-[280px]">
                          <p>CSS selectors to find links to follow when crawling. The crawler will extract href values from matching elements.</p>
                          <p className="mt-1 text-zinc-400">Example: <code className="text-xs">a.product-link</code></p>
                        </TooltipContent>
                      </Tooltip>
                    </div>
                    <Textarea
                      id="follow-selector"
                      placeholder={"a.product-link\na[href*='/product/']\na[href*='/item/']"}
                      value={crawlOptions.followSelector}
                      onChange={(e) => setCrawlOptions(prev => ({ ...prev, followSelector: e.target.value }))}
                      className="min-h-[80px] text-sm font-mono resize-none"
                    />
                    <p className="text-xs text-zinc-500">One selector per line, or comma-separated</p>
                  </div>

                  <div className="space-y-1">
                    <div className="flex items-center gap-1.5">
                      <Label htmlFor="follow-pattern" className="text-sm text-zinc-700 dark:text-zinc-300">
                        URL Filter (regex)
                      </Label>
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <HelpCircle className="h-3.5 w-3.5 text-zinc-400 cursor-help" />
                        </TooltipTrigger>
                        <TooltipContent side="right" className="max-w-[280px]">
                          <p>Regex patterns to filter which discovered URLs to crawl. Only URLs matching these patterns will be followed.</p>
                          <p className="mt-1 text-zinc-400">Leave empty to follow all discovered links.</p>
                          <p className="mt-1 text-zinc-400">Example: <code className="text-xs">/product/.*</code></p>
                        </TooltipContent>
                      </Tooltip>
                    </div>
                    <Textarea
                      id="follow-pattern"
                      placeholder={"/product/.*\n/item/.*\n/category/.*/.*"}
                      value={crawlOptions.followPattern}
                      onChange={(e) => setCrawlOptions(prev => ({ ...prev, followPattern: e.target.value }))}
                      className="min-h-[80px] text-sm font-mono resize-none"
                    />
                    <p className="text-xs text-zinc-500">One pattern per line (combined with |). Optional filter.</p>
                  </div>
                </div>

                {/* Right column: Limits then Suggested Selectors */}
                <div className="space-y-4">
                  {/* Crawl Limits - now first */}
                  <div className="space-y-2">
                    <div className="flex items-center gap-1.5">
                      <div className="text-xs font-medium text-amber-700 dark:text-amber-400 uppercase tracking-wide">
                        Limits
                      </div>
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <HelpCircle className="h-3 w-3 text-amber-600/60 cursor-help" />
                        </TooltipTrigger>
                        <TooltipContent side="right" className="max-w-[240px]">
                          <p>Control how many pages are crawled. Max pages is the total limit (0 = no limit), max depth controls how many levels deep from the starting URL.</p>
                        </TooltipContent>
                      </Tooltip>
                    </div>
                    <div className="grid grid-cols-2 gap-3">
                      <div className="space-y-1">
                        <Label htmlFor="max-pages" className="text-xs text-zinc-600 dark:text-zinc-400">
                          Max Pages (0 = no limit)
                        </Label>
                        <Input
                          id="max-pages"
                          type="number"
                          min={0}
                          value={crawlOptions.maxPages}
                          onChange={(e) => setCrawlOptions(prev => ({ ...prev, maxPages: parseInt(e.target.value) || 0 }))}
                          className="h-8 text-sm"
                        />
                      </div>
                      <div className="space-y-1">
                        <Label htmlFor="max-depth" className="text-xs text-zinc-600 dark:text-zinc-400">
                          Max Depth
                        </Label>
                        <Input
                          id="max-depth"
                          type="number"
                          min={1}
                          max={5}
                          value={crawlOptions.maxDepth}
                          onChange={(e) => setCrawlOptions(prev => ({ ...prev, maxDepth: parseInt(e.target.value) || 1 }))}
                          className="h-8 text-sm"
                        />
                      </div>
                    </div>
                  </div>

                  {/* Suggested CSS Selectors - now second */}
                  {analysisResult?.follow_patterns && analysisResult.follow_patterns.length > 0 && (
                    <div className="space-y-2">
                      <div className="flex items-center gap-1.5">
                        <div className="text-xs font-medium text-amber-700 dark:text-amber-400 uppercase tracking-wide">
                          Suggested Selectors
                        </div>
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <HelpCircle className="h-3 w-3 text-amber-600/60 cursor-help" />
                          </TooltipTrigger>
                          <TooltipContent side="right" className="max-w-[240px]">
                            <p>CSS selectors detected from the page analysis. Click to add them to the CSS Selectors field above.</p>
                          </TooltipContent>
                        </Tooltip>
                      </div>
                      <div className="space-y-1.5">
                        {analysisResult.follow_patterns.map((fp, i) => (
                          <button
                            key={i}
                            type="button"
                            onClick={() => {
                              // Add to CSS selector, detecting format (comma vs newline)
                              setCrawlOptions(prev => {
                                if (!prev.followSelector.trim()) {
                                  return { ...prev, followSelector: fp.pattern };
                                }
                                // Detect separator: if content has newlines use newline, else use comma
                                const hasNewlines = prev.followSelector.includes('\n');
                                const separator = hasNewlines ? '\n' : ', ';
                                return {
                                  ...prev,
                                  followSelector: `${prev.followSelector}${separator}${fp.pattern}`
                                };
                              });
                            }}
                            className="w-full px-2 py-1.5 text-xs bg-white dark:bg-zinc-800 border border-zinc-200 dark:border-zinc-700 rounded-md hover:bg-amber-100 dark:hover:bg-amber-900/30 hover:border-amber-300 dark:hover:border-amber-700 cursor-pointer font-mono text-left transition-colors"
                            title={`Click to add: ${fp.description}`}
                          >
                            <div className="truncate">{fp.pattern}</div>
                            {fp.description && (
                              <div className="text-[10px] text-zinc-400 truncate mt-0.5">{fp.description}</div>
                            )}
                          </button>
                        ))}
                      </div>
                    </div>
                  )}
                </div>
              </div>
            </div>
          </TooltipProvider>
        )}

        {/* Tab Content */}
        <div className="flex-1 min-h-0 overflow-hidden">
          {activeTab === 'schema' && (
            <form onSubmit={handleExtract} className="h-full flex flex-col">
              <Textarea
                placeholder="Enter your extraction schema in YAML format..."
                value={schema}
                onChange={(e) => setSchema(e.target.value)}
                className="flex-1 font-mono text-sm border-0 rounded-none resize-none focus-visible:ring-0 focus-visible:ring-offset-0"
                disabled={isLoading}
              />
            </form>
          )}

          {activeTab === 'result' && (
            <div className="h-full flex flex-col overflow-hidden">
              {isCrawling || crawlResults.length > 0 ? (
                // Streaming crawl results
                <div className="h-full flex flex-col overflow-hidden">
                  {/* Crawl Progress Header */}
                  <div className="px-4 py-3 border-b border-zinc-200 dark:border-zinc-800 bg-zinc-50 dark:bg-zinc-800/50 shrink-0">
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
                                <span className="text-zinc-400">pages extracted</span>
                              </span>
                            </div>
                          </>
                        ) : (
                          <div className="flex items-center gap-2 text-sm font-medium text-green-600 dark:text-green-400">
                            <span>Crawl complete</span>
                            <span className="text-zinc-400">-</span>
                            <span className="text-zinc-600 dark:text-zinc-400">
                              {crawlResults.length} pages extracted
                            </span>
                          </div>
                        )}
                      </div>
                      {/* Progress bar - only show determinate progress when maxPages > 0 */}
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
                  {/* Results List */}
                  <div className="flex-1 overflow-auto p-4 space-y-3">
                    {crawlResults.map((item, idx) => (
                      <div
                        key={idx}
                        className="rounded-lg border border-zinc-200 dark:border-zinc-800 overflow-hidden"
                      >
                        <div className="px-3 py-2 bg-zinc-50 dark:bg-zinc-800/50 border-b border-zinc-200 dark:border-zinc-800 flex items-center justify-between">
                          <span className="text-xs font-mono text-zinc-500 truncate">
                            {item.url}
                          </span>
                          <span className="text-xs text-zinc-400 shrink-0 ml-2">
                            {item.timestamp.toLocaleTimeString()}
                          </span>
                        </div>
                        <pre className="p-3 text-sm text-zinc-300 bg-zinc-950 overflow-auto max-h-[200px]">
                          {JSON.stringify(item.data, null, 2)}
                        </pre>
                      </div>
                    ))}
                    {isCrawling && crawlResults.length === 0 && (
                      <div className="text-center py-8 text-zinc-400">
                        <Loader2 className="h-6 w-6 animate-spin mx-auto mb-2" />
                        <p>Waiting for first extraction result...</p>
                      </div>
                    )}
                    <div ref={resultsEndRef} />
                  </div>
                </div>
              ) : result ? (
                // Single extraction result - fixed to fill vertical space
                <div className="h-full flex flex-col overflow-hidden">
                  <div className="px-4 py-2 border-b border-zinc-200 dark:border-zinc-800 bg-zinc-50 dark:bg-zinc-800/50 shrink-0">
                    <span className="text-xs font-mono text-zinc-500">{result.url}</span>
                  </div>
                  <div className="flex-1 overflow-auto bg-zinc-950 min-h-0">
                    <pre className="p-4 text-sm text-zinc-300 min-h-full">
                      {JSON.stringify(result.data, null, 2)}
                    </pre>
                  </div>
                </div>
              ) : (
                <div className="h-full flex items-center justify-center text-zinc-400">
                  <p>Run an extraction to see results</p>
                </div>
              )}
            </div>
          )}
        </div>
      </div>

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
