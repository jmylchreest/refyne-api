'use client';

import { Label } from '@/components/ui/label';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { Badge } from '@/components/ui/badge';
import { Checkbox } from '@/components/ui/checkbox';
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { HelpCircle, Layers, FileText, Lock } from 'lucide-react';
import { cn } from '@/lib/utils';

export interface CrawlOptions {
  followSelector: string;
  followPattern: string;
  maxPages: number;
  maxDepth: number;
  useSitemap: boolean;
}

export interface FollowPattern {
  pattern: string;
  description?: string;
}

interface CrawlModeSectionProps {
  isCrawlMode: boolean;
  onModeChange: (crawlMode: boolean) => void;
  crawlOptions: CrawlOptions;
  onOptionsChange: (options: CrawlOptions) => void;
  suggestedSelectors?: FollowPattern[];
  canCrawl: boolean;
  disabled?: boolean;
}

export function CrawlModeSection({
  isCrawlMode,
  onModeChange,
  crawlOptions,
  onOptionsChange,
  suggestedSelectors,
  canCrawl,
  disabled = false,
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

            {/* Mode Toggle Buttons */}
            <div className="flex rounded-lg border border-zinc-200 dark:border-zinc-700 p-0.5 bg-zinc-100 dark:bg-zinc-800">
              <button
                type="button"
                onClick={() => onModeChange(false)}
                disabled={disabled}
                className={cn(
                  'px-4 py-1.5 text-sm font-medium rounded-md transition-all cursor-pointer',
                  !isCrawlMode
                    ? 'bg-white dark:bg-zinc-900 text-zinc-900 dark:text-zinc-100 shadow-sm'
                    : 'text-zinc-500 dark:text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-300',
                  disabled && 'opacity-50 cursor-not-allowed'
                )}
              >
                Single Page
              </button>
              <button
                type="button"
                onClick={() => canCrawl && onModeChange(true)}
                disabled={disabled || !canCrawl}
                className={cn(
                  'px-4 py-1.5 text-sm font-medium rounded-md transition-all flex items-center gap-1.5',
                  isCrawlMode
                    ? 'bg-amber-500 text-white shadow-sm'
                    : 'text-zinc-500 dark:text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-300 cursor-pointer',
                  (!canCrawl || disabled) && 'opacity-50 cursor-not-allowed'
                )}
              >
                Crawl
                {!canCrawl && <Lock className="h-3 w-3" />}
              </button>
            </div>
          </div>

          {!canCrawl && (
            <p className="mt-2 text-xs text-zinc-500">
              Crawl mode requires a paid plan to extract from multiple pages.
            </p>
          )}
        </div>

        {/* Crawl Options - Only shown when in crawl mode */}
        {isCrawlMode && canCrawl && (
          <div className="p-4 bg-amber-50/50 dark:bg-amber-900/10 space-y-6">
            {/* URL Discovery Section */}
            <div className="space-y-4">
              <div className="flex items-center gap-2">
                <h3 className="text-sm font-medium text-amber-800 dark:text-amber-300">
                  URL Discovery
                </h3>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <HelpCircle className="h-3.5 w-3.5 text-amber-600/60 cursor-help" />
                  </TooltipTrigger>
                  <TooltipContent side="right" className="max-w-[280px]">
                    <p>Choose how the crawler discovers URLs to extract from. You can use the site&apos;s sitemap, CSS selectors to find links, or URL patterns to filter.</p>
                  </TooltipContent>
                </Tooltip>
              </div>

              {/* Sitemap Option */}
              <div className="flex items-start gap-3 p-3 rounded-lg border border-amber-200 dark:border-amber-800/50 bg-white dark:bg-zinc-900">
                <Checkbox
                  id="use-sitemap"
                  checked={crawlOptions.useSitemap}
                  onCheckedChange={(checked) => updateOption('useSitemap', checked === true)}
                  disabled={disabled}
                  className="mt-0.5"
                />
                <div className="flex-1">
                  <label
                    htmlFor="use-sitemap"
                    className="text-sm font-medium cursor-pointer flex items-center gap-2"
                  >
                    <FileText className="h-4 w-4 text-amber-600" />
                    Discover from Sitemap
                  </label>
                  <p className="text-xs text-zinc-500 mt-1">
                    Fetch sitemap.xml to discover all indexable URLs. Best for comprehensive site extraction.
                  </p>
                </div>
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
                  placeholder={"a.product-link\na[href*='/product/']\na[href*='/item/']"}
                  value={crawlOptions.followSelector}
                  onChange={(e) => updateOption('followSelector', e.target.value)}
                  disabled={disabled}
                  className="min-h-[72px] text-sm font-mono resize-none"
                />
                <p className="text-xs text-zinc-500">One selector per line, or comma-separated</p>
              </div>

              {/* URL Pattern Filter */}
              <div className="space-y-2">
                <div className="flex items-center gap-1.5">
                  <Label htmlFor="follow-pattern" className="text-sm text-zinc-700 dark:text-zinc-300">
                    URL Filter (regex)
                  </Label>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <HelpCircle className="h-3.5 w-3.5 text-zinc-400 cursor-help" />
                    </TooltipTrigger>
                    <TooltipContent side="right" className="max-w-[280px]">
                      <p>Regex patterns to filter discovered URLs. Only matching URLs will be crawled.</p>
                      <p className="mt-1 text-zinc-400">Leave empty to follow all discovered links.</p>
                    </TooltipContent>
                  </Tooltip>
                </div>
                <Textarea
                  id="follow-pattern"
                  placeholder={"/product/.*\n/item/.*\n/category/.*/.*"}
                  value={crawlOptions.followPattern}
                  onChange={(e) => updateOption('followPattern', e.target.value)}
                  disabled={disabled}
                  className="min-h-[72px] text-sm font-mono resize-none"
                />
                <p className="text-xs text-zinc-500">One pattern per line (combined with |). Optional filter.</p>
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
                    Max Pages (0 = no limit)
                  </Label>
                  <Input
                    id="max-pages"
                    type="number"
                    min={0}
                    value={crawlOptions.maxPages}
                    onChange={(e) => updateOption('maxPages', parseInt(e.target.value) || 0)}
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
      </div>
    </TooltipProvider>
  );
}
