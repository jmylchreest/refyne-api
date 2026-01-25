'use client';

import { useState, useEffect } from 'react';
import { useAuth } from '@clerk/nextjs';
import { useSearchParams } from 'next/navigation';
import {
  extract,
  analyze,
  createCrawlJob,
  listTierLimits,
  ExtractResult,
  AnalyzeResult,
  ApiError,
  TierLimits,
  isBotProtectionError,
} from '@/lib/api';
import { useCrawlStream, useCatalog, type CatalogCrawlOptions } from '@/lib/hooks';
import { toast } from 'sonner';
import { parseClerkFeatures, normalizeUrl, getErrorMessage } from '@/lib/utils';

// Components
import { ProgressAvatarDialog, defaultStages, AvatarStage } from '@/components/ui/progress-avatar';
import { CrawlModeSection, CrawlOptions, ExtractionMode } from '@/components/crawl-mode-section';
import {
  UrlInputSection,
  AnalysisSummary,
  SchemaEditor,
  ExtractionResults,
  yamlToJson,
  type FetchMode,
} from '@/components/extraction';
import {
  SaveSchemaDialog,
  OverwriteConfirmationDialog,
} from '@/components/dialogs';

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

export default function DashboardPage() {
  const { sessionClaims, getToken } = useAuth();
  const searchParams = useSearchParams();
  const features = parseClerkFeatures(sessionClaims?.fea as string | undefined);
  const canCrawl = features.includes('extraction_crawled');
  const canDynamic = features.includes('content_dynamic');

  // Tier limits for enforcing max pages
  const [tierLimits, setTierLimits] = useState<TierLimits[]>([]);
  const userTier = (sessionClaims?.tier as string) || 'free';
  const currentTierLimits = tierLimits.find(t => t.name === userTier);
  const maxPagesLimit = currentTierLimits?.max_pages_per_crawl || 0;

  // URL and extraction state
  const [url, setUrl] = useState('');
  const [schema, setSchema] = useState(defaultSchema);
  const [isLoading, setIsLoading] = useState(false);
  const [isAnalyzing, setIsAnalyzing] = useState(false);
  const [result, setResult] = useState<ExtractResult | null>(null);
  const [analysisResult, setAnalysisResult] = useState<AnalyzeResult | null>(null);

  // Fetch mode: 'auto' detects protection and retries with dynamic if needed
  const [fetchMode, setFetchMode] = useState<FetchMode>('auto');

  // Extraction mode state
  const [extractionMode, setExtractionMode] = useState<ExtractionMode>('single');
  const [crawlOptions, setCrawlOptions] = useState<CrawlOptions>({
    followSelector: '',
    followPattern: '',
    maxPages: 0,
    maxDepth: 1,
    useSitemap: false,
  });

  // Progress avatar state
  const [showProgressAvatar, setShowProgressAvatar] = useState(false);
  const [progressStage, setProgressStage] = useState('connecting');
  const [progressComplete, setProgressComplete] = useState(false);
  const [progressError, setProgressError] = useState(false);
  const [progressErrorMessage, setProgressErrorMessage] = useState('');
  const [progressMode, setProgressMode] = useState<'analyze' | 'extract'>('analyze');

  // Dialog state
  const [showSaveSchemaDialog, setShowSaveSchemaDialog] = useState(false);
  const [showSiteOverwriteDialog, setShowSiteOverwriteDialog] = useState(false);
  const [showSchemaOverwriteDialog, setShowSchemaOverwriteDialog] = useState(false);
  const [pendingSiteData, setPendingSiteData] = useState<{
    analysisResult: AnalyzeResult;
    crawlOptions: CatalogCrawlOptions;
    existingSiteId: string;
  } | null>(null);
  const [pendingSchemaData, setPendingSchemaData] = useState<{
    name: string;
    description: string;
    existingSchemaId: string;
  } | null>(null);

  // Analysis details collapsed state
  const [analysisExpanded, setAnalysisExpanded] = useState(false);

  // Copy state
  const [schemaCopied, setSchemaCopied] = useState(false);

  // Crawl stream hook
  const {
    crawlUrls,
    crawlFinalResult,
    crawlProgress,
    isCrawling,
    startStream,
    reset: resetCrawl,
  } = useCrawlStream({
    onComplete: (success, pageCount) => {
      if (success) {
        toast.success(`Crawl completed! ${pageCount} pages extracted.`);
      } else {
        toast.error('Crawl failed');
      }
      setIsLoading(false);
    },
    onError: (error) => {
      toast.error(error);
      setIsLoading(false);
    },
  });

  // Catalog hook
  const catalog = useCatalog({
    onSchemaLoaded: (schema) => {
      setSchema(schema.schema_yaml);
    },
    onSiteLoaded: (site) => {
      setUrl(site.url);
      if (site.analysis_result?.suggested_schema) {
        setSchema(site.analysis_result.suggested_schema);
      }
      setAnalysisResult(site.analysis_result || null);
      if (site.fetch_mode) {
        setFetchMode(site.fetch_mode as FetchMode);
      }
      if (site.crawl_options) {
        setCrawlOptions({
          followSelector: site.crawl_options.follow_selector || '',
          followPattern: site.crawl_options.follow_pattern || '',
          maxPages: site.crawl_options.max_pages || 0,
          maxDepth: site.crawl_options.max_depth || 1,
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
    },
  });

  // Update crawl options when analysis result has follow patterns
  useEffect(() => {
    if (analysisResult?.follow_patterns && analysisResult.follow_patterns.length > 0) {
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

  // Load tier limits on mount
  useEffect(() => {
    const loadTierLimits = async () => {
      try {
        const response = await listTierLimits();
        setTierLimits(response.tiers || []);
      } catch {
        // Tier limits are optional
      }
    };
    loadTierLimits();
  }, []);

  // Handle URL parameters from catalogue pages
  const { loadSiteById, loadSchemaById } = catalog;
  useEffect(() => {
    const siteId = searchParams.get('siteId');
    const schemaId = searchParams.get('schemaId');

    if (siteId) {
      loadSiteById(siteId);
    }
    if (schemaId) {
      loadSchemaById(schemaId);
    }
  }, [searchParams, loadSiteById, loadSchemaById]);

  const handleAnalyze = async () => {
    if (!url) {
      toast.error('Please enter a URL');
      return;
    }

    const normalizedUrl = normalizeUrl(url);

    setIsAnalyzing(true);
    setAnalysisResult(null);
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
      const resultPromise = analyze(normalizedUrl, 0, fetchMode);

      await progressPromise;
      setProgressStage('generating');

      const result = await resultPromise;
      setAnalysisResult(result);

      if (result.suggested_schema) {
        setSchema(result.suggested_schema);
      }

      if (result.recommended_fetch_mode) {
        setFetchMode(result.recommended_fetch_mode as FetchMode);
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
    resetCrawl();
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
            fetch_mode: fetchMode,
          },
        });

        toast.info(`Crawl job started - streaming progress...`);
        startStream(crawlResult.job_id, crawlOptions.maxPages, getToken);
        return;
      }

      if (extractionMode !== 'single' && !canCrawl) {
        toast.error('Crawl and Sitemap modes require a paid plan. Running single page extraction instead.');
        setExtractionMode('single');
      }

      const progressPromise = stageProgression();
      const resultPromise = extract(normalizedUrl, parsedSchema, undefined, fetchMode);

      await progressPromise;
      setProgressStage('generating');

      const extractResult = await resultPromise;
      setResult(extractResult);
      setProgressComplete(true);
      toast.success('Extraction completed');
    } catch (err) {
      const error = err as ApiError | { error?: string };

      if (isBotProtectionError(error as ApiError)) {
        const apiError = error as ApiError;
        const protectionType = apiError.protection_type || 'unknown';
        const suggestedMode = apiError.suggested_fetch_mode || 'dynamic';

        if (!canDynamic) {
          setProgressError(true);
          setProgressErrorMessage(
            `This site uses ${protectionType} protection. Browser rendering is required to access this content. ` +
            'Upgrade to a plan with browser rendering to extract from protected sites.'
          );
          toast.error('Browser rendering required - upgrade to access protected sites');
        } else if (fetchMode === 'static') {
          setProgressError(true);
          setProgressErrorMessage(
            `This site uses ${protectionType} protection. Retry with "${suggestedMode}" mode to bypass protection.`
          );
          toast.error(`Protection detected - retry with ${suggestedMode} mode`);
        } else {
          setProgressError(true);
          setProgressErrorMessage(getErrorMessage(error));
          toast.error(getErrorMessage(error));
        }
      } else {
        const errorMessage = getErrorMessage(error);
        setProgressError(true);
        setProgressErrorMessage(errorMessage);
        toast.error(errorMessage);
      }
    } finally {
      setIsLoading(false);
    }
  };

  const handleSaveSchema = async (name: string, description: string) => {
    const result = await catalog.saveSchema(name, description, schema);
    if (result.success) {
      setShowSaveSchemaDialog(false);
    } else if (result.existingSchema) {
      setPendingSchemaData({
        name,
        description,
        existingSchemaId: result.existingSchema.id,
      });
      setShowSaveSchemaDialog(false);
      setShowSchemaOverwriteDialog(true);
    }
  };

  const handleOverwriteSchema = async () => {
    if (!pendingSchemaData) return;
    await catalog.overwriteSchema(
      pendingSchemaData.existingSchemaId,
      pendingSchemaData.name,
      pendingSchemaData.description,
      schema
    );
    setShowSchemaOverwriteDialog(false);
    setPendingSchemaData(null);
  };

  const handleCreateNewSchema = async () => {
    if (!pendingSchemaData) return;
    // Create with a different name
    await catalog.saveSchema(
      `${pendingSchemaData.name} (copy)`,
      pendingSchemaData.description,
      schema
    );
    setShowSchemaOverwriteDialog(false);
    setPendingSchemaData(null);
  };

  const handleSaveSite = async () => {
    if (!url || !analysisResult) {
      toast.error('Analyze a URL first to save it');
      return;
    }

    const catalogCrawlOptions: CatalogCrawlOptions = {
      followSelector: crawlOptions.followSelector,
      followPattern: crawlOptions.followPattern,
      maxPages: crawlOptions.maxPages,
      maxDepth: crawlOptions.maxDepth,
      useSitemap: crawlOptions.useSitemap,
    };

    const result = await catalog.saveSite(url, analysisResult, catalogCrawlOptions);
    if (!result.success && result.existingSite) {
      setPendingSiteData({
        analysisResult,
        crawlOptions: catalogCrawlOptions,
        existingSiteId: result.existingSite.id,
      });
      setShowSiteOverwriteDialog(true);
    }
  };

  const handleOverwriteSite = async () => {
    if (!pendingSiteData) return;
    await catalog.overwriteSite(
      pendingSiteData.existingSiteId,
      url,
      pendingSiteData.analysisResult,
      pendingSiteData.crawlOptions
    );
    setShowSiteOverwriteDialog(false);
    setPendingSiteData(null);
  };

  const handleCreateNewSite = async () => {
    if (!pendingSiteData) return;
    // Create with modified URL (add timestamp)
    const modifiedUrl = `${url}#${Date.now()}`;
    await catalog.saveSite(modifiedUrl, pendingSiteData.analysisResult, pendingSiteData.crawlOptions);
    setShowSiteOverwriteDialog(false);
    setPendingSiteData(null);
  };

  const copySchema = async () => {
    await navigator.clipboard.writeText(schema);
    setSchemaCopied(true);
    setTimeout(() => setSchemaCopied(false), 2000);
  };

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
        onClose={() => {
          setShowProgressAvatar(false);
          setProgressComplete(false);
          setProgressError(false);
        }}
      />

      {/* SECTION 1: Target URL */}
      <div className="rounded-lg border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 shadow-sm">
        <div className="p-4">
          <UrlInputSection
            url={url}
            onUrlChange={setUrl}
            savedSites={catalog.savedSites}
            selectedSiteId={catalog.selectedSiteId}
            onSiteSelect={catalog.selectSite}
            onAnalyze={handleAnalyze}
            isAnalyzing={isAnalyzing}
            isLoading={isLoading}
          />
        </div>

        {/* Analysis Summary */}
        {analysisResult && (
          <AnalysisSummary
            analysisResult={analysisResult}
            fetchMode={fetchMode}
            onFetchModeChange={setFetchMode}
            canDynamic={canDynamic}
            expanded={analysisExpanded}
            onExpandedChange={setAnalysisExpanded}
            onSave={handleSaveSite}
          />
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
        maxPagesLimit={maxPagesLimit}
      />

      {/* SECTION 3: Schema Editor */}
      <SchemaEditor
        schema={schema}
        onSchemaChange={setSchema}
        schemas={catalog.schemas}
        selectedSchemaId={catalog.selectedSchemaId}
        onSchemaSelect={(id) => catalog.selectSchema(id)}
        onSave={() => {
          if (url) {
            setShowSaveSchemaDialog(true);
          } else {
            setShowSaveSchemaDialog(true);
          }
        }}
        onCopy={copySchema}
        isCopied={schemaCopied}
        onExtract={handleExtract}
        isLoading={isLoading}
        disabled={!url}
        extractionMode={extractionMode}
        canCrawl={canCrawl}
      />

      {/* SECTION 4: Results */}
      <ExtractionResults
        result={result}
        crawlFinalResult={crawlFinalResult}
        crawlUrls={crawlUrls}
        crawlProgress={crawlProgress}
        isCrawling={isCrawling}
      />

      {/* Save Schema Dialog */}
      <SaveSchemaDialog
        open={showSaveSchemaDialog}
        onOpenChange={setShowSaveSchemaDialog}
        onSave={handleSaveSchema}
        defaultName={url ? normalizeUrl(url) : ''}
      />

      {/* Site Overwrite Dialog */}
      <OverwriteConfirmationDialog
        open={showSiteOverwriteDialog}
        onOpenChange={setShowSiteOverwriteDialog}
        title="Site Already Exists"
        description="A saved site with this URL already exists. What would you like to do?"
        itemName={url}
        onOverwrite={handleOverwriteSite}
        onCreateNew={handleCreateNewSite}
      />

      {/* Schema Overwrite Dialog */}
      <OverwriteConfirmationDialog
        open={showSchemaOverwriteDialog}
        onOpenChange={setShowSchemaOverwriteDialog}
        title="Schema Already Exists"
        description="A schema with this name already exists. What would you like to do?"
        itemName={pendingSchemaData?.name || ''}
        onOverwrite={handleOverwriteSchema}
        onCreateNew={handleCreateNewSchema}
      />
    </div>
  );
}
