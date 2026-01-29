'use client';

import { useState, useEffect } from 'react';
import { format } from 'date-fns';
import {
  getJobDebugCapture,
  JobDebugCaptureResponse,
  DebugCaptureEntry,
} from '@/lib/api';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import {
  Loader2,
  Copy,
  Check,
  ChevronRight,
  CheckCircle2,
  XCircle,
  FileCode,
  Zap,
  DollarSign,
  Clock,
  Key,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import { toast } from 'sonner';

// Props can be either jobId (fetch from API) or data (direct data)
interface JobInspectorProps {
  jobId?: string;
  data?: JobDebugCaptureResponse;
}

function formatTokens(count: number): string {
  if (count >= 1000000) return `${(count / 1000000).toFixed(1)}M`;
  if (count >= 1000) return `${(count / 1000).toFixed(1)}k`;
  return count.toString();
}

function truncateUrl(url: string, maxLength = 40) {
  try {
    const parsed = new URL(url);
    const path = parsed.pathname.length > maxLength - 10
      ? parsed.pathname.slice(0, maxLength - 10) + '...'
      : parsed.pathname;
    return parsed.hostname + path;
  } catch {
    return url.length > maxLength ? url.slice(0, maxLength) + '...' : url;
  }
}

export function JobInspector({ jobId, data: initialData }: JobInspectorProps) {
  const [capture, setCapture] = useState<JobDebugCaptureResponse | null>(initialData || null);
  const [isLoading, setIsLoading] = useState(!initialData && !!jobId);
  const [error, setError] = useState<string | null>(null);
  const [selectedCapture, setSelectedCapture] = useState<DebugCaptureEntry | null>(
    initialData?.captures?.[0] || null
  );
  const [copied, setCopied] = useState<string | null>(null);

  // Fetch from API only if jobId is provided and no initial data
  useEffect(() => {
    if (initialData) {
      setCapture(initialData);
      if (initialData.captures?.length > 0) {
        setSelectedCapture(initialData.captures[0]);
      }
      setIsLoading(false);
      return;
    }

    if (!jobId) {
      setIsLoading(false);
      return;
    }

    const loadCapture = async () => {
      setIsLoading(true);
      setError(null);
      try {
        const data = await getJobDebugCapture(jobId);
        setCapture(data);
        // Auto-select first capture if available
        if (data.captures.length > 0) {
          setSelectedCapture(data.captures[0]);
        }
      } catch (err) {
        const errorMsg = err instanceof Error ? err.message : 'Failed to load debug capture';
        setError(errorMsg);
      } finally {
        setIsLoading(false);
      }
    };

    loadCapture();
  }, [jobId, initialData]);

  const copyToClipboard = async (text: string, label: string) => {
    await navigator.clipboard.writeText(text);
    setCopied(label);
    toast.success(`${label} copied to clipboard`);
    setTimeout(() => setCopied(null), 2000);
  };

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-full">
        <Loader2 className="h-6 w-6 animate-spin text-zinc-400" />
        <span className="ml-2 text-sm text-zinc-400">Loading debug data...</span>
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex items-center justify-center h-full p-4">
        <div className="text-center">
          <XCircle className="h-8 w-8 text-red-400 mx-auto mb-2" />
          <p className="text-sm text-red-400">{error}</p>
        </div>
      </div>
    );
  }

  if (!capture || !capture.enabled || capture.captures.length === 0) {
    return (
      <div className="flex items-center justify-center h-full p-4">
        <div className="text-center">
          <FileCode className="h-8 w-8 text-zinc-400 mx-auto mb-2" />
          <p className="text-sm text-zinc-500">No debug captures available for this job.</p>
          <p className="text-xs text-zinc-400 mt-1">
            Enable debug capture when creating the job to inspect LLM requests.
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="flex h-full">
      {/* Left Navigator */}
      <div className="w-56 border-r border-zinc-200 dark:border-zinc-800 flex flex-col">
        {/* Job Summary Header */}
        <div className="p-3 border-b border-zinc-200 dark:border-zinc-800 space-y-2">
          <div className="flex items-center justify-between">
            <span className="text-xs font-medium text-zinc-500">
              {capture.captures.length} request{capture.captures.length !== 1 ? 's' : ''}
            </span>
            {capture.is_byok && (
              <Badge variant="outline" className="text-xs bg-amber-50 text-amber-700 border-amber-200">
                <Key className="h-3 w-3 mr-1" />
                BYOK
              </Badge>
            )}
          </div>
          {/* Summary Stats */}
          {(capture.total_tokens_in || capture.total_cost_usd || capture.total_duration_ms) && (
            <div className="grid grid-cols-2 gap-1 text-xs">
              {capture.total_tokens_in !== undefined && (
                <div className="text-zinc-500">
                  <span className="font-medium">{formatTokens(capture.total_tokens_in + (capture.total_tokens_out || 0))}</span> tokens
                </div>
              )}
              {capture.total_cost_usd !== undefined && capture.total_cost_usd > 0 && (
                <div className="text-zinc-500">
                  <span className="font-medium">${capture.total_cost_usd.toFixed(4)}</span>
                </div>
              )}
              {capture.total_duration_ms !== undefined && (
                <div className="text-zinc-500">
                  <span className="font-medium">{(capture.total_duration_ms / 1000).toFixed(1)}s</span> total
                </div>
              )}
            </div>
          )}
          {capture.api_version && (
            <div className="text-xs text-zinc-400">
              API {capture.api_version}
            </div>
          )}
        </div>
        <div className="flex-1 overflow-auto">
          <div className="p-1">
            {capture.captures.map((entry) => (
              <button
                key={entry.id}
                onClick={() => setSelectedCapture(entry)}
                className={cn(
                  "w-full text-left px-2 py-1.5 rounded text-xs flex items-center gap-1.5 transition-colors",
                  selectedCapture?.id === entry.id
                    ? "bg-zinc-100 dark:bg-zinc-800"
                    : "hover:bg-zinc-50 dark:hover:bg-zinc-800/50"
                )}
              >
                {entry.response.success ? (
                  <CheckCircle2 className="h-3 w-3 text-green-500 shrink-0" />
                ) : (
                  <XCircle className="h-3 w-3 text-red-500 shrink-0" />
                )}
                <span className="truncate">{truncateUrl(entry.url, 25)}</span>
              </button>
            ))}
          </div>
        </div>
      </div>

      {/* Right Detail View */}
      <div className="flex-1 flex flex-col min-w-0 min-h-0">
        {selectedCapture ? (
          <div className="flex-1 flex flex-col min-h-0 overflow-auto">
            <div className="p-4 space-y-4">
              {/* URL Header */}
              <div className="flex items-center gap-2">
                {selectedCapture.response.success ? (
                  <CheckCircle2 className="h-4 w-4 text-green-500 shrink-0" />
                ) : (
                  <XCircle className="h-4 w-4 text-red-500 shrink-0" />
                )}
                <a
                  href={selectedCapture.url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="text-sm font-medium text-blue-600 dark:text-blue-400 hover:underline truncate"
                >
                  {selectedCapture.url}
                </a>
              </div>

              {/* Metadata Grid */}
              <div className="grid grid-cols-3 gap-3 text-sm">
                <div className="space-y-1">
                  <div className="flex items-center gap-1.5">
                    <Zap className="h-3 w-3 text-zinc-400" />
                    <span className="text-xs text-zinc-500">Provider</span>
                  </div>
                  <Badge variant="outline" className="font-mono text-xs">
                    {selectedCapture.request.provider}
                  </Badge>
                </div>
                <div className="space-y-1">
                  <div className="flex items-center gap-1.5">
                    <ChevronRight className="h-3 w-3 text-zinc-400" />
                    <span className="text-xs text-zinc-500">Model</span>
                  </div>
                  <Badge variant="outline" className="font-mono text-xs">
                    {selectedCapture.request.model}
                  </Badge>
                </div>
                {selectedCapture.api_version && (
                  <div className="space-y-1">
                    <span className="text-xs text-zinc-500">API Version</span>
                    <Badge variant="outline" className="font-mono text-xs">
                      {selectedCapture.api_version}
                    </Badge>
                  </div>
                )}
                <div className="space-y-1">
                  <span className="text-xs text-zinc-500">Input Tokens</span>
                  <div className="font-medium">{formatTokens(selectedCapture.response.input_tokens)}</div>
                </div>
                <div className="space-y-1">
                  <span className="text-xs text-zinc-500">Output Tokens</span>
                  <div className="font-medium">{formatTokens(selectedCapture.response.output_tokens)}</div>
                </div>
                <div className="space-y-1">
                  <div className="flex items-center gap-1.5">
                    <Clock className="h-3 w-3 text-zinc-400" />
                    <span className="text-xs text-zinc-500">Duration</span>
                  </div>
                  <div className="font-medium">{(selectedCapture.response.duration_ms / 1000).toFixed(2)}s</div>
                </div>
                {selectedCapture.response.cost_usd !== undefined && selectedCapture.response.cost_usd > 0 && (
                  <div className="space-y-1">
                    <div className="flex items-center gap-1.5">
                      <DollarSign className="h-3 w-3 text-zinc-400" />
                      <span className="text-xs text-zinc-500">Cost</span>
                    </div>
                    <div className="font-medium">${selectedCapture.response.cost_usd.toFixed(4)}</div>
                  </div>
                )}
                <div className="space-y-1">
                  <span className="text-xs text-zinc-500">Content Size</span>
                  <div className="font-medium">{formatTokens(selectedCapture.request.content_size)} chars</div>
                </div>
                {selectedCapture.request.temperature !== undefined && (
                  <div className="space-y-1">
                    <span className="text-xs text-zinc-500">Temperature</span>
                    <div className="font-medium">{selectedCapture.request.temperature}</div>
                  </div>
                )}
                {selectedCapture.request.max_tokens !== undefined && selectedCapture.request.max_tokens > 0 && (
                  <div className="space-y-1">
                    <span className="text-xs text-zinc-500">Max Tokens</span>
                    <div className="font-medium">{formatTokens(selectedCapture.request.max_tokens)}</div>
                  </div>
                )}
                {selectedCapture.is_byok && (
                  <div className="space-y-1">
                    <span className="text-xs text-zinc-500">Key Type</span>
                    <Badge variant="outline" className="text-xs bg-amber-50 text-amber-700 border-amber-200">
                      BYOK
                    </Badge>
                  </div>
                )}
                {selectedCapture.request.is_retry && (
                  <div className="space-y-1">
                    <span className="text-xs text-zinc-500">Retry</span>
                    <Badge variant="outline" className="text-xs bg-orange-50 text-orange-700 border-orange-200">
                      Retry
                    </Badge>
                  </div>
                )}
                {selectedCapture.request.fallback_position !== undefined && selectedCapture.request.fallback_position > 0 && (
                  <div className="space-y-1">
                    <span className="text-xs text-zinc-500">Fallback</span>
                    <Badge variant="outline" className="text-xs">
                      Position {selectedCapture.request.fallback_position}
                    </Badge>
                  </div>
                )}
              </div>

              {/* Error if failed */}
              {selectedCapture.response.error && (
                <div className="p-3 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded text-sm text-red-600 dark:text-red-400">
                  <span className="font-medium">Error: </span>
                  {selectedCapture.response.error}
                </div>
              )}

              {/* Hints Applied */}
              {selectedCapture.request.hints_applied && Object.keys(selectedCapture.request.hints_applied).length > 0 && (
                <div className="space-y-2">
                  <div className="flex items-center justify-between">
                    <span className="text-sm font-medium text-zinc-500">Hints Applied</span>
                  </div>
                  <div className="bg-zinc-50 dark:bg-zinc-900 rounded-lg p-3 space-y-2">
                    {Object.entries(selectedCapture.request.hints_applied).map(([key, value]) => (
                      <div key={key} className="text-xs">
                        <span className="font-medium text-zinc-600 dark:text-zinc-400">{key}:</span>
                        <span className="ml-2 text-zinc-500">{value}</span>
                      </div>
                    ))}
                  </div>
                </div>
              )}

              {/* Raw Content / Prompt */}
              {(selectedCapture.request.page_content || selectedCapture.request.prompt) && (
                <div className="space-y-2">
                  <div className="flex items-center justify-between">
                    <span className="text-sm font-medium text-zinc-500">
                      {selectedCapture.request.prompt ? 'Prompt' : 'Page Content'}
                    </span>
                    <Button
                      variant="ghost"
                      size="sm"
                      className="h-7 text-xs"
                      onClick={() => copyToClipboard(
                        selectedCapture.request.prompt || selectedCapture.request.page_content || '',
                        'Content'
                      )}
                    >
                      {copied === 'Content' ? (
                        <Check className="h-3.5 w-3.5 mr-1 text-green-500" />
                      ) : (
                        <Copy className="h-3.5 w-3.5 mr-1" />
                      )}
                      Copy
                    </Button>
                  </div>
                  <div className="bg-zinc-950 rounded-lg">
                    <pre className="p-3 text-xs text-zinc-300 font-mono whitespace-pre-wrap break-all">
                      {selectedCapture.request.prompt || selectedCapture.request.page_content}
                    </pre>
                  </div>
                </div>
              )}

              {/* Schema */}
              {selectedCapture.request.schema && (
                <div className="space-y-2">
                  <div className="flex items-center justify-between">
                    <span className="text-sm font-medium text-zinc-500">Schema</span>
                    <Button
                      variant="ghost"
                      size="sm"
                      className="h-7 text-xs"
                      onClick={() => copyToClipboard(selectedCapture.request.schema || '', 'Schema')}
                    >
                      {copied === 'Schema' ? (
                        <Check className="h-3.5 w-3.5 mr-1 text-green-500" />
                      ) : (
                        <Copy className="h-3.5 w-3.5 mr-1" />
                      )}
                      Copy
                    </Button>
                  </div>
                  <div className="bg-zinc-950 rounded-lg">
                    <pre className="p-3 text-xs text-zinc-300 font-mono whitespace-pre-wrap">
                      {selectedCapture.request.schema}
                    </pre>
                  </div>
                </div>
              )}

              {/* LLM Raw Output */}
              {selectedCapture.response.raw_output && (
                <div className="space-y-2">
                  <div className="flex items-center justify-between">
                    <span className="text-sm font-medium text-zinc-500">LLM Raw Output</span>
                    <Button
                      variant="ghost"
                      size="sm"
                      className="h-7 text-xs"
                      onClick={() => copyToClipboard(selectedCapture.response.raw_output || '', 'Raw Output')}
                    >
                      {copied === 'Raw Output' ? (
                        <Check className="h-3.5 w-3.5 mr-1 text-green-500" />
                      ) : (
                        <Copy className="h-3.5 w-3.5 mr-1" />
                      )}
                      Copy
                    </Button>
                  </div>
                  <div className="bg-zinc-950 rounded-lg">
                    <pre className="p-3 text-xs text-zinc-300 font-mono whitespace-pre-wrap">
                      {(() => {
                        try {
                          return JSON.stringify(JSON.parse(selectedCapture.response.raw_output || ''), null, 2);
                        } catch {
                          return selectedCapture.response.raw_output;
                        }
                      })()}
                    </pre>
                  </div>
                </div>
              )}

              {/* Timestamp */}
              <div className="text-xs text-zinc-400 pt-2 border-t border-zinc-200 dark:border-zinc-800">
                Captured at {format(new Date(selectedCapture.timestamp), 'PPpp')}
              </div>
            </div>
          </div>
        ) : (
          <div className="flex items-center justify-center h-full text-zinc-400">
            <p className="text-sm">Select a request to view details</p>
          </div>
        )}
      </div>
    </div>
  );
}
