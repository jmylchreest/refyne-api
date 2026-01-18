'use client';

import { useState, useEffect, useRef } from 'react';
import yaml from 'js-yaml';
import { useAuth } from '@clerk/nextjs';
import { useSearchParams } from 'next/navigation';
import {
  extract,
  analyze,
  createCrawlJob,
  getJobResults,
  getJobResultsRaw,
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
  ApiError,
  OutputFormat,
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
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { toast } from 'sonner';
import { Loader2, Sparkles, Save, BookOpen, Globe, Clock, ChevronDown, ChevronUp, Play, Copy, Check, AlertCircle, CheckCircle2 } from 'lucide-react';
import { ProgressAvatarDialog, defaultStages, AvatarStage } from '@/components/ui/progress-avatar';
import { CrawlModeSection, CrawlOptions, ExtractionMode } from '@/components/crawl-mode-section';
import { cn, parseClerkFeatures } from '@/lib/utils';

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

// Crawl progress tracking - URLs with status (merging done by backend)
interface CrawlProgressUrl {
  url: string;
  timestamp: Date;
  error?: string;
  errorCategory?: string;
  errorDetails?: string; // Full error details (BYOK users only)
}

// Get user-friendly error message based on error category
function getErrorMessage(error: ApiError | { error?: string }): string {
  const apiError = error as ApiError;

  // If we have an error category, provide a more helpful message
  if (apiError.error_category) {
    const categoryMessages: Record<string, string> = {
      invalid_api_key: apiError.is_byok
        ? 'Your API key is invalid. Please check your LLM provider settings.'
        : 'Authentication error. Please try again.',
      insufficient_credits: 'Insufficient credits. Please add credits to continue.',
      quota_exceeded: 'You have exceeded your usage quota. Please upgrade your plan or wait for your quota to reset.',
      feature_disabled: 'This feature is not available on your current plan.',
      rate_limited: 'Too many requests. Please wait a moment and try again.',
      quota_exhausted: 'Free tier quota exhausted. Please upgrade or try again later.',
      provider_unavailable: apiError.is_byok
        ? `The ${apiError.provider || 'LLM'} provider is currently unavailable. Please check their status or try a different provider.`
        : 'The extraction service is temporarily unavailable. Please try again later.',
      model_unavailable: apiError.is_byok
        ? `The model ${apiError.model || ''} is unavailable. Please try a different model.`
        : 'The extraction model is temporarily unavailable. Please try again later.',
      provider_error: apiError.is_byok
        ? `Error from ${apiError.provider || 'provider'}: ${apiError.error}${apiError.error_details ? `\n\nDetails: ${apiError.error_details}` : ''}`
        : 'A temporary error occurred. Please try again.',
      extraction_error: apiError.error || 'Extraction failed. Please check your schema and try again.',
    };

    return categoryMessages[apiError.error_category] || apiError.error || 'An error occurred';
  }

  // Fallback to the error message
  return apiError.error || 'An error occurred';
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

  // Extraction mode state
  const [extractionMode, setExtractionMode] = useState<ExtractionMode>('single');
  const [isCrawling, setIsCrawling] = useState(false);
  const [crawlOptions, setCrawlOptions] = useState<CrawlOptions>({
    followSelector: '',
    followPattern: '',
    maxPages: 0,
    maxDepth: 1,
    useSitemap: false,
  });
  const [crawlJobId, setCrawlJobId] = useState<string | null>(null);
  const [crawlUrls, setCrawlUrls] = useState<CrawlProgressUrl[]>([]);
  const [crawlFinalResult, setCrawlFinalResult] = useState<Record<string, unknown> | null>(null);
  const [crawlProgress, setCrawlProgress] = useState({
    extracted: 0,
    urlsQueued: 0, // Total URLs queued for processing (from sitemap or link discovery)
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

  // Copy state
  const [schemaCopied, setSchemaCopied] = useState(false);
  const [resultsCopied, setResultsCopied] = useState(false);

  // Output format state
  const [outputFormat, setOutputFormat] = useState<OutputFormat>('json');
  const [formattedOutput, setFormattedOutput] = useState<string>('');

  // Auto-scroll to latest crawl URL
  useEffect(() => {
    if (crawlUrls.length > 0) {
      resultsEndRef.current?.scrollIntoView({ behavior: 'smooth' });
    }
  }, [crawlUrls.length]);

  // Format results when output format changes or results update
  useEffect(() => {
    const formatResults = async () => {
      // Determine the data to format
      let dataToFormat: unknown = null;
      if (crawlFinalResult) {
        dataToFormat = crawlFinalResult;
      } else if (result) {
        dataToFormat = result.data;
      }

      if (!dataToFormat) {
        setFormattedOutput('');
        return;
      }

      if (outputFormat === 'json') {
        setFormattedOutput(JSON.stringify(dataToFormat, null, 2));
      } else if (outputFormat === 'jsonl') {
        // JSONL: one line per item
        if (Array.isArray(dataToFormat)) {
          setFormattedOutput(dataToFormat.map(item => JSON.stringify(item)).join('\n'));
        } else if (typeof dataToFormat === 'object' && dataToFormat !== null) {
          // For objects with items array, output each item as a line
          const obj = dataToFormat as Record<string, unknown>;
          if (Array.isArray(obj.items)) {
            setFormattedOutput(obj.items.map(item => JSON.stringify(item)).join('\n'));
          } else {
            // Single object as single line
            setFormattedOutput(JSON.stringify(dataToFormat));
          }
        }
      } else if (outputFormat === 'yaml') {
        // Use js-yaml to convert to YAML
        try {
          setFormattedOutput(yaml.dump(dataToFormat, { indent: 2, lineWidth: -1 }));
        } catch {
          setFormattedOutput('# Error converting to YAML\n' + JSON.stringify(dataToFormat, null, 2));
        }
      }
    };

    formatResults();
  }, [outputFormat, crawlFinalResult, result]);

  // Update crawl options when analysis result has follow patterns
  // Auto-populate with ALL suggested selectors, replacing any existing ones
  useEffect(() => {
    if (analysisResult?.follow_patterns && analysisResult.follow_patterns.length > 0) {
      // Combine all suggested patterns into the selector field
      const allPatterns = analysisResult.follow_patterns
        .map(fp => fp.pattern)
        .filter(Boolean)
        .join('\n');

      if (allPatterns) {
        setCrawlOptions(prev => ({
          ...prev,
          followSelector: allPatterns,
        }));
      }
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
    // Clear existing selectors so they get replaced by analysis results
    setCrawlOptions(prev => ({
      ...prev,
      followSelector: '',
      followPattern: '',
    }));
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
      const error = err as ApiError | { error?: string };
      const errorMessage = getErrorMessage(error);
      setProgressError(true);
      setProgressErrorMessage(errorMessage);
      toast.error(errorMessage);
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
    setCrawlUrls([]);
    setCrawlFinalResult(null);
    setCrawlJobId(null);
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
      if (extractionMode !== 'single' && canCrawl) {
        setShowProgressAvatar(false);
        setCrawlUrls([]);
        setCrawlFinalResult(null);
        setIsCrawling(true);
        setCrawlProgress({
          extracted: 0,
          urlsQueued: 0,
          maxPages: crawlOptions.maxPages,
          status: 'running'
        });

        // In sitemap mode, don't pass follow_selector (no link following)
        const isSitemapMode = extractionMode === 'sitemap';

        const crawlResult = await createCrawlJob({
          url: normalizedUrl,
          schema: parsedSchema,
          options: {
            follow_selector: isSitemapMode ? undefined : (crawlOptions.followSelector || undefined),
            follow_pattern: crawlOptions.followPattern || undefined,
            max_pages: crawlOptions.maxPages,
            max_depth: isSitemapMode ? 0 : crawlOptions.maxDepth,
            same_domain_only: true,
            extract_from_seeds: true,
            use_sitemap: isSitemapMode,
          },
        });

        const jobId = crawlResult.job_id;
        setCrawlJobId(jobId);
        toast.info(`Crawl job started - streaming progress...`);

        const apiBase = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080';
        const token = await getToken();

        const streamResponse = await fetch(
          `${apiBase}/api/v1/jobs/${jobId}/stream`,
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
                      // Track URL with error status for progress display
                      setCrawlUrls(prev => [...prev, {
                        url: data.url,
                        timestamp: new Date(),
                        error: data.error_message,
                        errorCategory: data.error_category,
                        errorDetails: data.error_details, // Full details for BYOK users
                      }]);
                      setCrawlProgress(prev => ({
                        ...prev,
                        extracted: prev.extracted + 1
                      }));
                    } else if (currentEvent === 'status') {
                      setCrawlProgress(prev => ({
                        ...prev,
                        extracted: data.page_count || prev.extracted,
                        urlsQueued: data.urls_queued || prev.urlsQueued
                      }));
                    } else if (currentEvent === 'complete') {
                      setIsCrawling(false);
                      setCrawlProgress(prev => ({
                        ...prev,
                        status: data.status === 'completed' ? 'completed' : 'failed',
                        extracted: data.page_count || prev.extracted,
                        urlsQueued: data.urls_queued || prev.urlsQueued
                      }));

                      if (data.status === 'completed') {
                        // Fetch merged results from backend
                        try {
                          const resultsResponse = await getJobResults(jobId, true);
                          if (resultsResponse.merged) {
                            setCrawlFinalResult(resultsResponse.merged);
                          }
                          toast.success(`Crawl completed! ${data.page_count} pages extracted.`);
                        } catch (fetchErr) {
                          console.error('Failed to fetch merged results:', fetchErr);
                          toast.error('Crawl completed but failed to fetch merged results');
                        }
                      } else {
                        toast.error(`Crawl failed: ${data.error || 'Unknown error'}`);
                      }
                      setIsLoading(false);
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

      if (extractionMode !== 'single' && !canCrawl) {
        toast.error('Crawl and Sitemap modes require a paid plan. Running single page extraction instead.');
        setExtractionMode('single');
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
      const error = err as ApiError | { error?: string };
      const errorMessage = getErrorMessage(error);
      setProgressError(true);
      setProgressErrorMessage(errorMessage);
      setIsCrawling(false);
      setCrawlProgress(prev => ({ ...prev, status: 'failed' }));
      toast.error(errorMessage);
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

  // Convert YAML schema to JSON using js-yaml library
  const yamlToJson = (yamlStr: string): object => {
    const parsed = yaml.load(yamlStr);
    if (typeof parsed !== 'object' || parsed === null) {
      throw new Error('Invalid YAML: must be an object');
    }
    return parsed as object;
  };

  const getTotalTime = () => {
    if (!result) return null;
    const total = (result.metadata.fetch_duration_ms || 0) + (result.metadata.extract_duration_ms || 0);
    return total > 1000 ? `${(total / 1000).toFixed(1)}s` : `${total}ms`;
  };

  const copySchema = async () => {
    await navigator.clipboard.writeText(schema);
    setSchemaCopied(true);
    setTimeout(() => setSchemaCopied(false), 2000);
  };

  const copyResults = async () => {
    await navigator.clipboard.writeText(formattedOutput);
    setResultsCopied(true);
    setTimeout(() => setResultsCopied(false), 2000);
  };

  const hasResults = result !== null || crawlFinalResult !== null || isCrawling;

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
                onKeyDown={(e) => {
                  if (e.key === 'Enter' && !isLoading && !isAnalyzing && url) {
                    e.preventDefault();
                    handleAnalyze();
                  }
                }}
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
        extractionMode={extractionMode}
        onModeChange={setExtractionMode}
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
              onClick={() => {
                // Pre-populate schema name with normalized URL
                if (url && !newSchemaName) {
                  setNewSchemaName(normalizeUrl(url));
                }
                setShowSaveSchemaDialog(true);
              }}
              className="h-8 text-xs"
            >
              <Save className="h-3 w-3 mr-1" />
              Save
            </Button>
            <Button
              variant="ghost"
              size="icon"
              onClick={copySchema}
              className="h-8 w-8"
              title="Copy schema"
            >
              {schemaCopied ? (
                <Check className="h-3 w-3 text-green-500" />
              ) : (
                <Copy className="h-3 w-3" />
              )}
            </Button>
          </div>
          <Button
            size="sm"
            onClick={handleExtract}
            disabled={isLoading || !url}
            className={cn(
              'h-8',
              extractionMode === 'crawl' && canCrawl && 'bg-amber-600 hover:bg-amber-700',
              extractionMode === 'sitemap' && canCrawl && 'bg-emerald-600 hover:bg-emerald-700'
            )}
          >
            {isLoading ? (
              <Loader2 className="h-3 w-3 animate-spin mr-1" />
            ) : (
              <Play className="h-3 w-3 mr-1" />
            )}
            {extractionMode === 'crawl' && canCrawl
              ? 'Crawl & Extract'
              : extractionMode === 'sitemap' && canCrawl
                ? 'Sitemap Extract'
                : 'Extract'}
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
              {!isCrawling && !crawlFinalResult && result && (
                <span className="text-xs text-zinc-500">{result.url}</span>
              )}
            </div>
            <div className="flex items-center gap-3 text-xs text-zinc-500">
              {result && (
                <>
                  <span className="flex items-center gap-1">
                    <Clock className="h-3 w-3" />
                    {getTotalTime()}
                  </span>
                  {result.usage.cost_usd && result.usage.cost_usd > 0 && (
                    <span>${result.usage.cost_usd.toFixed(4)}</span>
                  )}
                </>
              )}
              {(result || crawlFinalResult) && (
                <>
                  <Select value={outputFormat} onValueChange={(v) => setOutputFormat(v as OutputFormat)}>
                    <SelectTrigger className="h-7 w-[80px] text-xs">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="json">JSON</SelectItem>
                      <SelectItem value="jsonl">JSONL</SelectItem>
                      <SelectItem value="yaml">YAML</SelectItem>
                    </SelectContent>
                  </Select>
                  <Button
                    variant="ghost"
                    size="icon"
                    onClick={copyResults}
                    className="h-8 w-8"
                    title="Copy results"
                  >
                    {resultsCopied ? (
                      <Check className="h-3 w-3 text-green-500" />
                    ) : (
                      <Copy className="h-3 w-3" />
                    )}
                  </Button>
                </>
              )}
            </div>
          </div>

          {/* Results Content */}
          <div className="flex-1 overflow-hidden flex flex-col min-h-0">
            {isCrawling || crawlFinalResult ? (
              // Crawl results
              <>
                {/* Crawl Progress */}
                {(() => {
                  const successCount = crawlUrls.filter(u => !u.error).length;
                  const failedCount = crawlUrls.filter(u => u.error).length;
                  return (
                    <div className="px-4 py-2 border-b border-zinc-200 dark:border-zinc-800 bg-zinc-50 dark:bg-zinc-800/50 shrink-0">
                      <div className="flex items-center justify-between">
                        {isCrawling ? (
                          <div className="flex items-center gap-3 text-sm font-medium">
                            <Loader2 className="h-4 w-4 animate-spin text-amber-500" />
                            <span>Crawling...</span>
                            <span className="flex items-center gap-1 text-green-600">
                              <CheckCircle2 className="h-3.5 w-3.5" />
                              {successCount}
                            </span>
                            {failedCount > 0 && (
                              <span className="flex items-center gap-1 text-red-500">
                                <AlertCircle className="h-3.5 w-3.5" />
                                {failedCount}
                              </span>
                            )}
                          </div>
                        ) : (
                          <div className="flex items-center gap-3 text-sm font-medium">
                            <span className="text-green-600 dark:text-green-400">Crawl complete</span>
                            <span className="flex items-center gap-1 text-green-600">
                              <CheckCircle2 className="h-3.5 w-3.5" />
                              {successCount}
                            </span>
                            {failedCount > 0 && (
                              <span className="flex items-center gap-1 text-red-500">
                                <AlertCircle className="h-3.5 w-3.5" />
                                {failedCount} failed
                              </span>
                            )}
                          </div>
                        )}
                        {(() => {
                          const total = crawlProgress.maxPages > 0 ? crawlProgress.maxPages : crawlProgress.urlsQueued;
                          const current = crawlProgress.extracted;
                          const showCount = total > 0;
                          const progressPercent = showCount ? Math.min(100, (current / total) * 100) : 0;

                          return (
                            <div className="flex items-center gap-2">
                              {showCount && (
                                <span className="text-xs text-zinc-500 dark:text-zinc-400 tabular-nums">
                                  {current}/{total}
                                </span>
                              )}
                              <div className="w-32 h-1.5 bg-zinc-200 dark:bg-zinc-700 rounded-full overflow-hidden">
                                {showCount ? (
                                  <div
                                    className={cn(
                                      "h-full transition-all duration-300 rounded-full",
                                      isCrawling ? "bg-amber-500" : "bg-green-500"
                                    )}
                                    style={{ width: `${progressPercent}%` }}
                                  />
                                ) : isCrawling ? (
                                  <div className="h-full w-full bg-amber-500 animate-pulse rounded-full" />
                                ) : (
                                  <div className="h-full w-full bg-green-500 rounded-full" />
                                )}
                              </div>
                            </div>
                          );
                        })()}
                      </div>
                    </div>
                  );
                })()}

                {/* Final Merged Results from Backend */}
                {crawlFinalResult ? (
                  <>
                    <div className="flex-1 overflow-auto bg-zinc-950 min-h-0">
                      <pre className="p-4 text-sm text-zinc-300 min-h-full">
                        {formattedOutput}
                      </pre>
                    </div>
                    {crawlUrls.length > 0 && (
                      <details className="border-t border-zinc-200 dark:border-zinc-800 bg-zinc-50 dark:bg-zinc-900 shrink-0">
                        <summary className="px-4 py-2 text-xs text-zinc-500 cursor-pointer hover:text-zinc-700 dark:hover:text-zinc-300">
                          Source pages ({crawlUrls.length})
                        </summary>
                        <div className="px-4 pb-2 space-y-1.5 max-h-48 overflow-auto">
                          {crawlUrls.map((item, idx) => (
                            <div key={idx} className="text-xs">
                              <div className="flex items-center gap-2">
                                {item.error ? (
                                  <AlertCircle className="h-3.5 w-3.5 text-red-500 shrink-0" />
                                ) : (
                                  <CheckCircle2 className="h-3.5 w-3.5 text-green-500 shrink-0" />
                                )}
                                <a
                                  href={item.url}
                                  target="_blank"
                                  rel="noopener noreferrer"
                                  className={cn(
                                    "font-mono truncate hover:underline",
                                    item.error ? "text-red-400 hover:text-red-300" : "text-zinc-400 hover:text-zinc-200"
                                  )}
                                >
                                  {item.url}
                                </a>
                              </div>
                              {item.error && (
                                item.errorDetails ? (
                                  <Tooltip>
                                    <TooltipTrigger asChild>
                                      <div className="ml-5.5 mt-0.5 text-red-400/80 text-[11px] leading-tight cursor-help underline decoration-dotted decoration-red-400/50">
                                        {item.error}
                                      </div>
                                    </TooltipTrigger>
                                    <TooltipContent side="bottom" className="max-w-md text-left font-mono text-[10px] whitespace-pre-wrap">
                                      {item.errorDetails}
                                    </TooltipContent>
                                  </Tooltip>
                                ) : (
                                  <div className="ml-5.5 mt-0.5 text-red-400/80 text-[11px] leading-tight">
                                    {item.error}
                                  </div>
                                )
                              )}
                            </div>
                          ))}
                        </div>
                      </details>
                    )}
                  </>
                ) : isCrawling ? (
                  <div className="flex-1 flex flex-col min-h-0">
                    {/* Show URLs being processed during crawl */}
                    {crawlUrls.length > 0 ? (
                      <div className="flex-1 overflow-auto bg-zinc-950 min-h-0">
                        <div className="p-4 space-y-1.5">
                          {crawlUrls.map((item, idx) => (
                            <div key={idx} className="text-xs">
                              <div className="flex items-center gap-2">
                                {item.error ? (
                                  <AlertCircle className="h-3.5 w-3.5 text-red-500 shrink-0" />
                                ) : (
                                  <CheckCircle2 className="h-3.5 w-3.5 text-green-500 shrink-0" />
                                )}
                                <a
                                  href={item.url}
                                  target="_blank"
                                  rel="noopener noreferrer"
                                  className={cn(
                                    "font-mono truncate hover:underline",
                                    item.error ? "text-red-400 hover:text-red-300" : "text-zinc-400 hover:text-zinc-200"
                                  )}
                                >
                                  {item.url}
                                </a>
                              </div>
                              {item.error && (
                                item.errorDetails ? (
                                  <Tooltip>
                                    <TooltipTrigger asChild>
                                      <div className="ml-5.5 mt-0.5 text-red-400/80 text-[11px] leading-tight cursor-help underline decoration-dotted decoration-red-400/50">
                                        {item.error}
                                      </div>
                                    </TooltipTrigger>
                                    <TooltipContent side="bottom" className="max-w-md text-left font-mono text-[10px] whitespace-pre-wrap">
                                      {item.errorDetails}
                                    </TooltipContent>
                                  </Tooltip>
                                ) : (
                                  <div className="ml-5.5 mt-0.5 text-red-400/80 text-[11px] leading-tight">
                                    {item.error}
                                  </div>
                                )
                              )}
                            </div>
                          ))}
                        </div>
                      </div>
                    ) : (
                      <div className="flex-1 flex items-center justify-center text-zinc-400">
                        <div className="text-center">
                          <Loader2 className="h-6 w-6 animate-spin mx-auto mb-2" />
                          <p>Waiting for first extraction result...</p>
                        </div>
                      </div>
                    )}
                  </div>
                ) : null}
                <div ref={resultsEndRef} />
              </>
            ) : result ? (
              // Single extraction result
              <div className="flex-1 overflow-auto bg-zinc-950 min-h-0">
                <pre className="p-4 text-sm text-zinc-300 min-h-full">
                  {formattedOutput}
                </pre>
              </div>
            ) : null}
          </div>
        </div>
      )}

      {/* Save Schema Dialog */}
      <Dialog open={showSaveSchemaDialog} onOpenChange={(open) => {
        if (!open) {
          setNewSchemaName('');
          setNewSchemaDescription('');
        }
        setShowSaveSchemaDialog(open);
      }}>
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
