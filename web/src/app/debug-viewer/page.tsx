'use client';

import { useState, useCallback } from 'react';
import { JobDebugCaptureResponse } from '@/lib/api';
import { JobInspector } from '@/components/job-inspector';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Upload, FileJson, AlertCircle, X } from 'lucide-react';
import { cn } from '@/lib/utils';

export default function DebugViewerPage() {
  const [debugData, setDebugData] = useState<JobDebugCaptureResponse | null>(null);
  const [fileName, setFileName] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [isDragging, setIsDragging] = useState(false);

  const parseDebugFile = useCallback((content: string, name: string) => {
    try {
      const parsed = JSON.parse(content);

      // Validate it looks like a debug capture file
      if (!parsed.captures || !Array.isArray(parsed.captures)) {
        throw new Error('Invalid debug capture file: missing captures array');
      }

      // Handle both API response format and raw S3 format
      const data: JobDebugCaptureResponse = {
        job_id: parsed.job_id || parsed.JobID || 'unknown',
        job_type: parsed.job_type || parsed.JobType,
        api_version: parsed.api_version || parsed.APIVersion,
        is_byok: parsed.is_byok || parsed.IsBYOK,
        enabled: parsed.enabled !== false,
        total_requests: parsed.total_requests || parsed.TotalRequests || parsed.captures.length,
        total_tokens_in: parsed.total_tokens_in || parsed.TotalTokensIn,
        total_tokens_out: parsed.total_tokens_out || parsed.TotalTokensOut,
        total_cost_usd: parsed.total_cost_usd || parsed.TotalCostUSD,
        total_duration_ms: parsed.total_duration_ms || parsed.TotalDurationMs,
        captures: parsed.captures.map((c: any) => ({
          id: c.id || c.ID,
          url: c.url || c.URL,
          timestamp: c.timestamp || c.Timestamp,
          job_type: c.job_type || c.JobType,
          api_version: c.api_version || c.APIVersion,
          sequence: c.sequence || c.Sequence,
          is_byok: c.is_byok || c.IsBYOK,
          request: {
            provider: c.request?.metadata?.provider || c.request?.Provider || c.Request?.Metadata?.Provider,
            model: c.request?.metadata?.model || c.request?.Model || c.Request?.Metadata?.Model,
            fetch_mode: c.request?.metadata?.fetch_mode || c.request?.FetchMode || c.Request?.Metadata?.FetchMode,
            content_size: c.request?.metadata?.content_size || c.request?.ContentSize || c.Request?.Metadata?.ContentSize || 0,
            prompt_size: c.request?.metadata?.prompt_size || c.request?.PromptSize || c.Request?.Metadata?.PromptSize || 0,
            temperature: c.request?.metadata?.temperature || c.Request?.Metadata?.Temperature,
            max_tokens: c.request?.metadata?.max_tokens || c.Request?.Metadata?.MaxTokens,
            json_mode: c.request?.metadata?.json_mode || c.Request?.Metadata?.JSONMode,
            fallback_position: c.request?.metadata?.fallback_position || c.Request?.Metadata?.FallbackPosition,
            is_retry: c.request?.metadata?.is_retry || c.Request?.Metadata?.IsRetry,
            system_prompt: c.request?.payload?.system_prompt || c.Request?.Payload?.SystemPrompt,
            user_prompt: c.request?.payload?.user_prompt || c.Request?.Payload?.UserPrompt,
            schema: c.request?.payload?.schema || c.Request?.Payload?.Schema,
            prompt: c.request?.payload?.prompt || c.Request?.Payload?.Prompt,
            page_content: c.request?.payload?.page_content || c.Request?.Payload?.PageContent,
            hints_applied: c.request?.payload?.hints_applied || c.request?.payload?.hints || c.Request?.Payload?.Hints,
          },
          response: {
            input_tokens: c.response?.metadata?.input_tokens || c.response?.InputTokens || c.Response?.Metadata?.InputTokens || 0,
            output_tokens: c.response?.metadata?.output_tokens || c.response?.OutputTokens || c.Response?.Metadata?.OutputTokens || 0,
            duration_ms: c.response?.metadata?.duration_ms || c.response?.DurationMs || c.Response?.Metadata?.DurationMs || 0,
            success: c.response?.metadata?.success !== false && c.Response?.Metadata?.Success !== false,
            error: c.response?.metadata?.error || c.Response?.Metadata?.Error,
            error_category: c.response?.metadata?.error_category || c.Response?.Metadata?.ErrorCategory,
            cost_usd: c.response?.metadata?.cost_usd || c.Response?.Metadata?.CostUSD,
            raw_output: c.response?.payload?.raw_output || c.Response?.Payload?.RawOutput,
            parsed_output: c.response?.payload?.parsed_output || c.Response?.Payload?.ParsedOutput,
            parse_error: c.response?.payload?.parse_error || c.Response?.Payload?.ParseError,
          },
        })),
      };

      setDebugData(data);
      setFileName(name);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to parse debug file');
      setDebugData(null);
      setFileName(null);
    }
  }, []);

  const handleFileSelect = useCallback((file: File) => {
    const reader = new FileReader();
    reader.onload = (e) => {
      const content = e.target?.result as string;
      parseDebugFile(content, file.name);
    };
    reader.onerror = () => {
      setError('Failed to read file');
    };
    reader.readAsText(file);
  }, [parseDebugFile]);

  const handleDrop = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    setIsDragging(false);

    const file = e.dataTransfer.files[0];
    if (file && file.type === 'application/json') {
      handleFileSelect(file);
    } else {
      setError('Please drop a JSON file');
    }
  }, [handleFileSelect]);

  const handleDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    setIsDragging(true);
  }, []);

  const handleDragLeave = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    setIsDragging(false);
  }, []);

  const handleInputChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (file) {
      handleFileSelect(file);
    }
  }, [handleFileSelect]);

  const clearFile = useCallback(() => {
    setDebugData(null);
    setFileName(null);
    setError(null);
  }, []);

  return (
    <div className="min-h-screen bg-zinc-50 dark:bg-zinc-950">
      <div className="container mx-auto py-8 px-4">
        <div className="mb-6">
          <h1 className="text-2xl font-bold text-zinc-900 dark:text-zinc-100">Debug Capture Viewer</h1>
          <p className="text-sm text-zinc-500 mt-1">
            Load and inspect debug capture files to analyze LLM requests and responses.
          </p>
        </div>

        {!debugData ? (
          <Card className="max-w-2xl mx-auto">
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <FileJson className="h-5 w-5" />
                Load Debug File
              </CardTitle>
              <CardDescription>
                Drop a debug capture JSON file here or click to browse.
                Debug files can be downloaded from the job inspector in the dashboard.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div
                onDrop={handleDrop}
                onDragOver={handleDragOver}
                onDragLeave={handleDragLeave}
                className={cn(
                  "border-2 border-dashed rounded-lg p-12 text-center transition-colors",
                  isDragging
                    ? "border-blue-500 bg-blue-50 dark:bg-blue-950/20"
                    : "border-zinc-200 dark:border-zinc-800 hover:border-zinc-300 dark:hover:border-zinc-700"
                )}
              >
                <Upload className={cn(
                  "h-12 w-12 mx-auto mb-4",
                  isDragging ? "text-blue-500" : "text-zinc-400"
                )} />
                <p className="text-sm text-zinc-600 dark:text-zinc-400 mb-4">
                  Drag and drop a debug capture JSON file here
                </p>
                <label>
                  <input
                    type="file"
                    accept=".json,application/json"
                    onChange={handleInputChange}
                    className="hidden"
                  />
                  <Button variant="outline" asChild>
                    <span className="cursor-pointer">Browse Files</span>
                  </Button>
                </label>
              </div>

              {error && (
                <div className="mt-4 p-3 bg-red-50 dark:bg-red-950/20 border border-red-200 dark:border-red-800 rounded-lg flex items-start gap-2">
                  <AlertCircle className="h-4 w-4 text-red-500 mt-0.5 shrink-0" />
                  <p className="text-sm text-red-600 dark:text-red-400">{error}</p>
                </div>
              )}
            </CardContent>
          </Card>
        ) : (
          <div className="space-y-4">
            {/* File info bar */}
            <div className="flex items-center justify-between bg-white dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-800 rounded-lg px-4 py-2">
              <div className="flex items-center gap-3">
                <FileJson className="h-4 w-4 text-zinc-500" />
                <span className="text-sm font-medium">{fileName}</span>
                {debugData.job_id && debugData.job_id !== 'unknown' && (
                  <span className="text-xs text-zinc-500">Job: {debugData.job_id}</span>
                )}
              </div>
              <Button variant="ghost" size="sm" onClick={clearFile}>
                <X className="h-4 w-4 mr-1" />
                Close
              </Button>
            </div>

            {/* Inspector */}
            <Card className="overflow-hidden">
              <div className="h-[calc(100vh-220px)] min-h-[500px]">
                <JobInspector data={debugData} />
              </div>
            </Card>
          </div>
        )}
      </div>
    </div>
  );
}
