'use client';

import { Label } from '@/components/ui/label';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { HelpCircle, Layers, FileText, Link2, Lock } from 'lucide-react';
import { cn } from '@/lib/utils';

export type ExtractionMode = 'single' | 'crawl' | 'sitemap';

export interface CrawlOptions {
  followSelector: string;
  followPattern: string;
  maxPages: number;
  maxDepth: number;
  useSitemap: boolean; // Derived from mode, kept for API compatibility
}

export interface FollowPattern {
  pattern: string;
  description?: string;
}

interface CrawlModeSectionProps {
  extractionMode: ExtractionMode;
  onModeChange: (mode: ExtractionMode) => void;
  crawlOptions: CrawlOptions;
  onOptionsChange: (options: CrawlOptions) => void;
  suggestedSelectors?: FollowPattern[];
  canCrawl: boolean;
  disabled?: boolean;
  /** Maximum pages allowed by the user's tier (0 = unlimited) */
  maxPagesLimit?: number;
}

export function CrawlModeSection({
  extractionMode,
  onModeChange,
  crawlOptions,
  onOptionsChange,
  suggestedSelectors,
  canCrawl,
  disabled = false,
  maxPagesLimit = 0,
}: CrawlModeSectionProps) {
  const updateOption = <K extends keyof CrawlOptions>(key: K, value: CrawlOptions[K]) => {
    onOptionsChange({ ...crawlOptions, [key]: value });
  };

  const addSelector = (selector: string) => {
    if (!crawlOptions.followSelector.trim()) {
      updateOption('followSelector', selector);
      return;
    }
    // Detect separator: if content has newlines use newline, else use comma
    const hasNewlines = crawlOptions.followSelector.includes('\n');
    const separator = hasNewlines ? '\n' : ', ';
    updateOption('followSelector', `${crawlOptions.followSelector}${separator}${selector}`);
  };

  const handleModeChange = (mode: ExtractionMode) => {
    if (mode !== 'single' && !canCrawl) return;
    onModeChange(mode);
    // Update useSitemap flag for API compatibility
    if (mode === 'sitemap') {
      onOptionsChange({ ...crawlOptions, useSitemap: true });
    } else {
      onOptionsChange({ ...crawlOptions, useSitemap: false });
    }
  };

  return (
    <TooltipProvider>
      <div className="rounded-lg border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 shadow-sm">
        {/* Mode Toggle Header */}
        <div className="p-4 border-b border-zinc-200 dark:border-zinc-800">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <Layers className="h-4 w-4 text-zinc-500" />
              <span className="text-sm font-medium">Extraction Mode</span>
            </div>

            {/* Three-Mode Toggle Buttons */}
            <div className="flex rounded-lg border border-zinc-200 dark:border-zinc-700 p-0.5 bg-zinc-100 dark:bg-zinc-800">
              <button
                type="button"
                onClick={() => handleModeChange('single')}
                disabled={disabled}
                className={cn(
                  'px-3 py-1.5 text-sm font-medium rounded-md transition-all cursor-pointer',
                  extractionMode === 'single'
                    ? 'bg-white dark:bg-zinc-900 text-zinc-900 dark:text-zinc-100 shadow-sm'
                    : 'text-zinc-500 dark:text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-300',
                  disabled && 'opacity-50 cursor-not-allowed'
                )}
              >
                Single
              </button>
              <button
                type="button"
                onClick={() => handleModeChange('crawl')}
                disabled={disabled || !canCrawl}
                className={cn(
                  'px-3 py-1.5 text-sm font-medium rounded-md transition-all flex items-center gap-1.5',
                  extractionMode === 'crawl'
                    ? 'bg-amber-500 text-white shadow-sm'
                    : 'text-zinc-500 dark:text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-300 cursor-pointer',
                  (!canCrawl || disabled) && 'opacity-50 cursor-not-allowed'
                )}
              >
                <Link2 className="h-3.5 w-3.5" />
                Crawl
                {!canCrawl && <Lock className="h-3 w-3" />}
              </button>
              <button
                type="button"
                onClick={() => handleModeChange('sitemap')}
                disabled={disabled || !canCrawl}
                className={cn(
                  'px-3 py-1.5 text-sm font-medium rounded-md transition-all flex items-center gap-1.5',
                  extractionMode === 'sitemap'
                    ? 'bg-emerald-500 text-white shadow-sm'
                    : 'text-zinc-500 dark:text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-300 cursor-pointer',
                  (!canCrawl || disabled) && 'opacity-50 cursor-not-allowed'
                )}
              >
                <FileText className="h-3.5 w-3.5" />
                Sitemap
                {!canCrawl && <Lock className="h-3 w-3" />}
              </button>
            </div>
          </div>

          {!canCrawl && (
            <p className="mt-2 text-xs text-zinc-500">
              Crawl and Sitemap modes require a paid plan to extract from multiple pages.
            </p>
          )}
        </div>

        {/* Crawl Mode Options */}
        {extractionMode === 'crawl' && canCrawl && (
          <div className="p-4 bg-amber-50/50 dark:bg-amber-900/10 space-y-6">
            {/* Link Discovery Section */}
            <div className="space-y-4">
              <div className="flex items-center gap-2">
                <h3 className="text-sm font-medium text-amber-800 dark:text-amber-300">
                  Link Discovery
                </h3>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <HelpCircle className="h-3.5 w-3.5 text-amber-600/60 cursor-help" />
                  </TooltipTrigger>
                  <TooltipContent side="right" className="max-w-[280px]">
                    <p>Configure how the crawler finds and follows links from the seed URL. Use CSS selectors to identify link elements and patterns to filter URLs.</p>
                  </TooltipContent>
                </Tooltip>
              </div>

              {/* CSS Selectors */}
              <div className="space-y-2">
                <div className="flex items-center gap-1.5">
                  <Label htmlFor="follow-selector" className="text-sm text-zinc-700 dark:text-zinc-300">
                    CSS Selectors
                  </Label>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <HelpCircle className="h-3.5 w-3.5 text-zinc-400 cursor-help" />
                    </TooltipTrigger>
                    <TooltipContent side="right" className="max-w-[280px]">
                      <p>CSS selectors to find links to follow. The crawler extracts href values from matching elements.</p>
                      <p className="mt-1 text-zinc-400">Example: <code className="text-xs">a.product-link</code></p>
                    </TooltipContent>
                  </Tooltip>
                </div>
                <Textarea
                  id="follow-selector"
                  placeholder={"main a[href*='/blog/']\narticle a[href*='/post/']"}
                  value={crawlOptions.followSelector}
                  onChange={(e) => updateOption('followSelector', e.target.value)}
                  disabled={disabled}
                  className="min-h-[72px] text-sm font-mono resize-none"
                />
                <p className="text-xs text-zinc-500">CSS selectors for links to follow (one per line or comma-separated)</p>
              </div>

              {/* URL Pattern Filter */}
              <div className="space-y-2">
                <div className="flex items-center gap-1.5">
                  <Label htmlFor="follow-pattern" className="text-sm text-zinc-700 dark:text-zinc-300">
                    URL Filter (optional)
                  </Label>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <HelpCircle className="h-3.5 w-3.5 text-zinc-400 cursor-help" />
                    </TooltipTrigger>
                    <TooltipContent side="right" className="max-w-[280px]">
                      <p>Only crawl URLs containing these patterns. Leave empty to follow all discovered links.</p>
                    </TooltipContent>
                  </Tooltip>
                </div>
                <Input
                  id="follow-pattern"
                  placeholder="e.g., /blog/ or /article/"
                  value={crawlOptions.followPattern}
                  onChange={(e) => updateOption('followPattern', e.target.value)}
                  disabled={disabled}
                  className="text-sm font-mono"
                />
                <p className="text-xs text-zinc-500">Optional: Only follow URLs containing this pattern</p>
              </div>

              {/* Suggested Selectors */}
              {suggestedSelectors && suggestedSelectors.length > 0 && (
                <div className="space-y-2">
                  <div className="flex items-center gap-1.5">
                    <span className="text-xs font-medium text-amber-700 dark:text-amber-400 uppercase tracking-wide">
                      Suggested Selectors
                    </span>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <HelpCircle className="h-3 w-3 text-amber-600/60 cursor-help" />
                      </TooltipTrigger>
                      <TooltipContent side="right" className="max-w-[240px]">
                        <p>CSS selectors detected from the page analysis. Click to add them.</p>
                      </TooltipContent>
                    </Tooltip>
                  </div>
                  <div className="flex flex-wrap gap-2">
                    {suggestedSelectors.map((fp, i) => (
                      <button
                        key={i}
                        type="button"
                        onClick={() => addSelector(fp.pattern)}
                        disabled={disabled}
                        className="px-2 py-1 text-xs bg-white dark:bg-zinc-800 border border-zinc-200 dark:border-zinc-700 rounded-md hover:bg-amber-100 dark:hover:bg-amber-900/30 hover:border-amber-300 dark:hover:border-amber-700 cursor-pointer font-mono transition-colors disabled:opacity-50"
                        title={fp.description || `Add: ${fp.pattern}`}
                      >
                        {fp.pattern}
                      </button>
                    ))}
                  </div>
                </div>
              )}
            </div>

            {/* Limits Section */}
            <div className="space-y-3">
              <div className="flex items-center gap-2">
                <h3 className="text-sm font-medium text-amber-800 dark:text-amber-300">
                  Limits
                </h3>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <HelpCircle className="h-3.5 w-3.5 text-amber-600/60 cursor-help" />
                  </TooltipTrigger>
                  <TooltipContent side="right" className="max-w-[240px]">
                    <p>Control crawl scope. Max pages is the total limit (0 = no limit). Max depth controls how many link levels from the starting URL.</p>
                  </TooltipContent>
                </Tooltip>
              </div>

              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-1.5">
                  <Label htmlFor="max-pages" className="text-xs text-zinc-600 dark:text-zinc-400">
                    Max Pages {maxPagesLimit > 0 ? `(limit: ${maxPagesLimit})` : '(0 = no limit)'}
                  </Label>
                  <Input
                    id="max-pages"
                    type="number"
                    min={0}
                    max={maxPagesLimit > 0 ? maxPagesLimit : undefined}
                    value={crawlOptions.maxPages}
                    onChange={(e) => {
                      let value = parseInt(e.target.value) || 0;
                      // Clamp to tier limit if set
                      if (maxPagesLimit > 0 && value > maxPagesLimit) {
                        value = maxPagesLimit;
                      }
                      updateOption('maxPages', value);
                    }}
                    disabled={disabled}
                    className="h-9"
                  />
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="max-depth" className="text-xs text-zinc-600 dark:text-zinc-400">
                    Max Depth
                  </Label>
                  <Input
                    id="max-depth"
                    type="number"
                    min={1}
                    max={5}
                    value={crawlOptions.maxDepth}
                    onChange={(e) => updateOption('maxDepth', parseInt(e.target.value) || 1)}
                    disabled={disabled}
                    className="h-9"
                  />
                </div>
              </div>
            </div>
          </div>
        )}

        {/* Sitemap Mode Options */}
        {extractionMode === 'sitemap' && canCrawl && (
          <div className="p-4 bg-emerald-50/50 dark:bg-emerald-900/10 space-y-4">
            <div className="flex items-center gap-2">
              <h3 className="text-sm font-medium text-emerald-800 dark:text-emerald-300">
                Sitemap Extraction
              </h3>
              <Tooltip>
                <TooltipTrigger asChild>
                  <HelpCircle className="h-3.5 w-3.5 text-emerald-600/60 cursor-help" />
                </TooltipTrigger>
                <TooltipContent side="right" className="max-w-[280px]">
                  <p>Extract from all URLs listed in the site&apos;s sitemap.xml. Each URL is processed independently without following links.</p>
                </TooltipContent>
              </Tooltip>
            </div>

            <p className="text-sm text-emerald-700 dark:text-emerald-400">
              Discovers URLs from sitemap.xml and extracts from each page independently. No link following - only processes sitemap URLs.
            </p>

            {/* URL Pattern Filter */}
            <div className="space-y-2">
              <div className="flex items-center gap-1.5">
                <Label htmlFor="sitemap-pattern" className="text-sm text-zinc-700 dark:text-zinc-300">
                  URL Filter (optional)
                </Label>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <HelpCircle className="h-3.5 w-3.5 text-zinc-400 cursor-help" />
                  </TooltipTrigger>
                  <TooltipContent side="right" className="max-w-[280px]">
                    <p>Only process sitemap URLs containing this pattern. Leave empty to extract from all sitemap URLs.</p>
                    <p className="mt-1 text-zinc-400">Example: <code className="text-xs">/products/</code></p>
                  </TooltipContent>
                </Tooltip>
              </div>
              <Input
                id="sitemap-pattern"
                placeholder="e.g., /blog/ or /article/"
                value={crawlOptions.followPattern}
                onChange={(e) => updateOption('followPattern', e.target.value)}
                disabled={disabled}
                className="text-sm font-mono"
              />
              <p className="text-xs text-zinc-500">Optional: Only extract from sitemap URLs containing this pattern</p>
            </div>

            {/* Max Pages Limit */}
            <div className="space-y-1.5">
              <Label htmlFor="sitemap-max-pages" className="text-xs text-zinc-600 dark:text-zinc-400">
                Max Pages {maxPagesLimit > 0 ? `(limit: ${maxPagesLimit})` : '(0 = no limit)'}
              </Label>
              <Input
                id="sitemap-max-pages"
                type="number"
                min={0}
                max={maxPagesLimit > 0 ? maxPagesLimit : undefined}
                value={crawlOptions.maxPages}
                onChange={(e) => {
                  let value = parseInt(e.target.value) || 0;
                  // Clamp to tier limit if set
                  if (maxPagesLimit > 0 && value > maxPagesLimit) {
                    value = maxPagesLimit;
                  }
                  updateOption('maxPages', value);
                }}
                disabled={disabled}
                className="h-9 w-32"
              />
            </div>
          </div>
        )}
      </div>
    </TooltipProvider>
  );
}
