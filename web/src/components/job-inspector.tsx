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
} from 'lucide-react';
import { cn } from '@/lib/utils';
import { toast } from 'sonner';

interface JobInspectorProps {
  jobId: string;
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

export function JobInspector({ jobId }: JobInspectorProps) {
  const [capture, setCapture] = useState<JobDebugCaptureResponse | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selectedCapture, setSelectedCapture] = useState<DebugCaptureEntry | null>(null);
  const [copied, setCopied] = useState<string | null>(null);

  useEffect(() => {
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
  }, [jobId]);

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
      <div className="w-48 border-r border-zinc-200 dark:border-zinc-800 flex flex-col">
        <div className="p-2 border-b border-zinc-200 dark:border-zinc-800">
          <span className="text-xs font-medium text-zinc-500">
            {capture.captures.length} request{capture.captures.length !== 1 ? 's' : ''}
          </span>
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
          <div className="flex-1 flex flex-col min-h-0 overflow-hidden">
            <div className="p-4 space-y-4 flex flex-col flex-1 min-h-0">
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
              <div className="grid grid-cols-2 gap-3 text-sm">
                <div className="space-y-2">
                  <div className="flex items-center gap-2">
                    <Zap className="h-3.5 w-3.5 text-zinc-400" />
                    <span className="text-zinc-500">Provider</span>
                  </div>
                  <Badge variant="outline" className="font-mono">
                    {selectedCapture.request.provider}
                  </Badge>
                </div>
                <div className="space-y-2">
                  <div className="flex items-center gap-2">
                    <ChevronRight className="h-3.5 w-3.5 text-zinc-400" />
                    <span className="text-zinc-500">Model</span>
                  </div>
                  <Badge variant="outline" className="font-mono">
                    {selectedCapture.request.model}
                  </Badge>
                </div>
                <div className="space-y-2">
                  <div className="text-zinc-500">Input Tokens</div>
                  <span className="font-medium">{formatTokens(selectedCapture.response.input_tokens)}</span>
                </div>
                <div className="space-y-2">
                  <div className="text-zinc-500">Output Tokens</div>
                  <span className="font-medium">{formatTokens(selectedCapture.response.output_tokens)}</span>
                </div>
                <div className="space-y-2">
                  <div className="text-zinc-500">Duration</div>
                  <span className="font-medium">{(selectedCapture.response.duration_ms / 1000).toFixed(2)}s</span>
                </div>
                <div className="space-y-2">
                  <div className="text-zinc-500">Content Size</div>
                  <span className="font-medium">{formatTokens(selectedCapture.request.content_size)} chars</span>
                </div>
                {selectedCapture.api_version && (
                  <div className="space-y-2">
                    <div className="text-zinc-500">API Version</div>
                    <Badge variant="outline" className="font-mono text-xs">
                      {selectedCapture.api_version}
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
                <div className="flex flex-col flex-1 min-h-0">
                  <div className="flex items-center justify-between mb-2 shrink-0">
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
                  <div className="bg-zinc-950 rounded-lg flex-1 min-h-0 overflow-auto">
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
                  <div className="bg-zinc-950 rounded-lg max-h-48 overflow-auto">
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
                  <div className="bg-zinc-950 rounded-lg max-h-48 overflow-auto">
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
              <div className="text-xs text-zinc-400 pt-2 border-t border-zinc-200 dark:border-zinc-800 shrink-0">
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
