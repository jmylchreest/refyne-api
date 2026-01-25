'use client';

import { useState, useEffect, useRef } from 'react';
import yaml from 'js-yaml';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import {
  Loader2,
  Copy,
  Check,
  Clock,
  AlertCircle,
  CheckCircle2,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import type { ExtractResult, OutputFormat } from '@/lib/api';
import type { CrawlProgressUrl, CrawlProgress } from '@/lib/hooks';

interface ExtractionResultsProps {
  result: ExtractResult | null;
  crawlFinalResult: Record<string, unknown> | null;
  crawlUrls: CrawlProgressUrl[];
  crawlProgress: CrawlProgress;
  isCrawling: boolean;
}

export function ExtractionResults({
  result,
  crawlFinalResult,
  crawlUrls,
  crawlProgress,
  isCrawling,
}: ExtractionResultsProps) {
  const [outputFormat, setOutputFormat] = useState<OutputFormat>('json');
  const [formattedOutput, setFormattedOutput] = useState<string>('');
  const [resultsCopied, setResultsCopied] = useState(false);
  const resultsEndRef = useRef<HTMLDivElement>(null);

  // Auto-scroll to latest crawl URL
  useEffect(() => {
    if (crawlUrls.length > 0) {
      resultsEndRef.current?.scrollIntoView({ behavior: 'smooth' });
    }
  }, [crawlUrls.length]);

  // Format results when output format changes or results update
  useEffect(() => {
    const formatResults = async () => {
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
        if (Array.isArray(dataToFormat)) {
          setFormattedOutput(
            dataToFormat.map((item) => JSON.stringify(item)).join('\n')
          );
        } else if (typeof dataToFormat === 'object' && dataToFormat !== null) {
          const obj = dataToFormat as Record<string, unknown>;
          if (Array.isArray(obj.items)) {
            setFormattedOutput(
              obj.items.map((item) => JSON.stringify(item)).join('\n')
            );
          } else {
            setFormattedOutput(JSON.stringify(dataToFormat));
          }
        }
      } else if (outputFormat === 'yaml') {
        try {
          setFormattedOutput(
            yaml.dump(dataToFormat, { indent: 2, lineWidth: -1 })
          );
        } catch {
          setFormattedOutput(
            '# Error converting to YAML\n' +
              JSON.stringify(dataToFormat, null, 2)
          );
        }
      }
    };

    formatResults();
  }, [outputFormat, crawlFinalResult, result]);

  const copyResults = async () => {
    await navigator.clipboard.writeText(formattedOutput);
    setResultsCopied(true);
    setTimeout(() => setResultsCopied(false), 2000);
  };

  const getTotalTime = () => {
    if (!result) return null;
    const total =
      (result.metadata.fetch_duration_ms || 0) +
      (result.metadata.extract_duration_ms || 0);
    return total > 1000 ? `${(total / 1000).toFixed(1)}s` : `${total}ms`;
  };

  const hasResults =
    result !== null || crawlFinalResult !== null || isCrawling;

  if (!hasResults) return null;

  const successCount = crawlUrls.filter((u) => !u.error).length;
  const failedCount = crawlUrls.filter((u) => u.error).length;

  return (
    <div className="rounded-lg border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 shadow-sm flex flex-col min-h-[300px] max-h-[500px]">
      {/* Results Header */}
      <div className="flex items-center justify-between border-b border-zinc-200 dark:border-zinc-800 px-4 py-2 shrink-0">
        <div className="flex items-center gap-3">
          <span className="text-sm font-medium">Results</span>
          {!isCrawling && !crawlFinalResult && result && (
            <>
              <Badge
                variant="secondary"
                className={`text-[10px] px-1.5 py-0 ${
                  result.input_format === 'prompt'
                    ? 'bg-violet-100 dark:bg-violet-900/30 text-violet-700 dark:text-violet-300'
                    : 'bg-zinc-100 dark:bg-zinc-800 text-zinc-600 dark:text-zinc-400'
                }`}
              >
                {result.input_format === 'prompt' ? 'PROMPT' : 'SCHEMA'}
              </Badge>
              <span className="text-xs text-zinc-500">{result.url}</span>
            </>
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
              <Select
                value={outputFormat}
                onValueChange={(v) => setOutputFormat(v as OutputFormat)}
              >
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
            <CrawlProgressBar
              isCrawling={isCrawling}
              successCount={successCount}
              failedCount={failedCount}
              crawlProgress={crawlProgress}
            />

            {/* Final Merged Results from Backend */}
            {crawlFinalResult ? (
              <>
                <div className="flex-1 overflow-auto bg-zinc-950 min-h-0">
                  <pre className="p-4 text-sm text-zinc-300 min-h-full">
                    {formattedOutput}
                  </pre>
                </div>
                {crawlUrls.length > 0 && (
                  <CrawlUrlList crawlUrls={crawlUrls} collapsed />
                )}
              </>
            ) : isCrawling ? (
              <div className="flex-1 flex flex-col min-h-0">
                {crawlUrls.length > 0 ? (
                  <CrawlUrlList crawlUrls={crawlUrls} />
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
  );
}

// Crawl progress bar component
function CrawlProgressBar({
  isCrawling,
  successCount,
  failedCount,
  crawlProgress,
}: {
  isCrawling: boolean;
  successCount: number;
  failedCount: number;
  crawlProgress: CrawlProgress;
}) {
  const total =
    crawlProgress.maxPages > 0
      ? crawlProgress.maxPages
      : crawlProgress.urlsQueued;
  const current = crawlProgress.extracted;
  const showCount = total > 0;
  const progressPercent = showCount ? Math.min(100, (current / total) * 100) : 0;

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
            <span className="text-green-600 dark:text-green-400">
              Crawl complete
            </span>
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
                  'h-full transition-all duration-300 rounded-full',
                  isCrawling ? 'bg-amber-500' : 'bg-green-500'
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
      </div>
    </div>
  );
}

// Crawl URL list component
function CrawlUrlList({
  crawlUrls,
  collapsed = false,
}: {
  crawlUrls: CrawlProgressUrl[];
  collapsed?: boolean;
}) {
  if (collapsed) {
    return (
      <details className="border-t border-zinc-200 dark:border-zinc-800 bg-zinc-50 dark:bg-zinc-900 shrink-0">
        <summary className="px-4 py-2 text-xs text-zinc-500 cursor-pointer hover:text-zinc-700 dark:hover:text-zinc-300">
          Source pages ({crawlUrls.length})
        </summary>
        <div className="px-4 pb-2 space-y-1.5 max-h-48 overflow-auto">
          {crawlUrls.map((item, idx) => (
            <CrawlUrlItem key={idx} item={item} />
          ))}
        </div>
      </details>
    );
  }

  return (
    <div className="flex-1 overflow-auto bg-zinc-950 min-h-0">
      <div className="p-4 space-y-1.5">
        {crawlUrls.map((item, idx) => (
          <CrawlUrlItem key={idx} item={item} />
        ))}
      </div>
    </div>
  );
}

// Individual crawl URL item
function CrawlUrlItem({ item }: { item: CrawlProgressUrl }) {
  return (
    <div className="text-xs">
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
            'font-mono truncate hover:underline',
            item.error
              ? 'text-red-400 hover:text-red-300'
              : 'text-zinc-400 hover:text-zinc-200'
          )}
        >
          {item.url}
        </a>
      </div>
      {item.error &&
        (item.errorDetails ? (
          <Tooltip>
            <TooltipTrigger asChild>
              <div className="ml-5.5 mt-0.5 text-red-400/80 text-[11px] leading-tight cursor-help underline decoration-dotted decoration-red-400/50">
                {item.error}
              </div>
            </TooltipTrigger>
            <TooltipContent
              side="bottom"
              className="max-w-md text-left font-mono text-[10px] whitespace-pre-wrap"
            >
              {item.errorDetails}
            </TooltipContent>
          </Tooltip>
        ) : (
          <div className="ml-5.5 mt-0.5 text-red-400/80 text-[11px] leading-tight">
            {item.error}
          </div>
        ))}
    </div>
  );
}
