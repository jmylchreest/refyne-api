'use client';

import { useEffect, useState, useRef, useCallback } from 'react';
import yaml from 'js-yaml';
import { formatDistanceToNow, formatDistance, format } from 'date-fns';
import {
  listJobs,
  getJob,
  getJobResults,
  getJobWebhookDeliveries,
  Job,
  JobWebhookDelivery,
  OutputFormat,
} from '@/lib/api';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { toast } from 'sonner';
import {
  Download,
  Loader2,
  Eye,
  Copy,
  Check,
  PanelLeftClose,
  PanelLeft,
  RefreshCw,
  Clock,
  Coins,
  FileText,
  Webhook,
  CheckCircle2,
  XCircle,
  AlertCircle,
  Timer,
  Search,
} from 'lucide-react';
import { JobInspector } from '@/components/job-inspector';
import { cn } from '@/lib/utils';

// Format helpers using date-fns
function formatRelativeTime(dateString: string) {
  return formatDistanceToNow(new Date(dateString), { addSuffix: false });
}

function formatDateTime(dateString: string) {
  return format(new Date(dateString), 'PPpp');
}

function formatDuration(startDate: string, endDate: string) {
  return formatDistance(new Date(startDate), new Date(endDate));
}

function formatTokens(count: number): string {
  if (count >= 1000000) return `${(count / 1000000).toFixed(1)}M`;
  if (count >= 1000) return `${(count / 1000).toFixed(1)}k`;
  return count.toString();
}

function truncateUrl(url: string) {
  try {
    const parsed = new URL(url);
    const path =
      parsed.pathname.length > 18
        ? parsed.pathname.slice(0, 18) + '...'
        : parsed.pathname;
    return parsed.hostname + path;
  } catch {
    return url.slice(0, 30) + '...';
  }
}

function getStatusColor(status: Job['status']) {
  switch (status) {
    case 'pending':
      return 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-400';
    case 'running':
      return 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400';
    case 'completed':
      return 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400';
    case 'failed':
      return 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400';
    case 'cancelled':
      return 'bg-zinc-100 text-zinc-800 dark:bg-zinc-900/30 dark:text-zinc-400';
    default:
      return 'bg-zinc-100 text-zinc-800 dark:bg-zinc-900/30 dark:text-zinc-400';
  }
}

function getStatusDotColor(status: Job['status']) {
  switch (status) {
    case 'pending':
      return 'bg-yellow-500';
    case 'running':
      return 'bg-blue-500 animate-pulse';
    case 'completed':
      return 'bg-green-500';
    case 'failed':
      return 'bg-red-500';
    case 'cancelled':
      return 'bg-zinc-400';
    default:
      return 'bg-zinc-400';
  }
}

function getWebhookStatusIcon(status: JobWebhookDelivery['status']) {
  switch (status) {
    case 'success':
      return <CheckCircle2 className="h-3.5 w-3.5 text-green-500" />;
    case 'failed':
      return <XCircle className="h-3.5 w-3.5 text-red-500" />;
    case 'retrying':
      return <AlertCircle className="h-3.5 w-3.5 text-yellow-500" />;
    case 'pending':
      return <Clock className="h-3.5 w-3.5 text-zinc-400" />;
    default:
      return <Clock className="h-3.5 w-3.5 text-zinc-400" />;
  }
}

interface JobListItemProps {
  job: Job;
  index: number;
  isSelected: boolean;
  collapsed: boolean;
  onClick: () => void;
}

function JobListItem({ job, index, isSelected, collapsed, onClick }: JobListItemProps) {
  if (collapsed) {
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <button
            onClick={onClick}
            className={cn(
              'w-full flex items-center justify-between px-2 py-1.5 rounded transition-colors',
              isSelected
                ? 'bg-zinc-100 dark:bg-zinc-800'
                : 'hover:bg-zinc-50 dark:hover:bg-zinc-900'
            )}
          >
            <span className={cn('w-2 h-2 rounded-full shrink-0', getStatusDotColor(job.status))} />
            <span className="text-[10px] text-zinc-400 tabular-nums">{index + 1}</span>
          </button>
        </TooltipTrigger>
        <TooltipContent side="right" className="text-xs">
          <p className="font-medium">{truncateUrl(job.url)}</p>
          <p className="text-zinc-400">
            {job.type} - {job.page_count} pages
          </p>
        </TooltipContent>
      </Tooltip>
    );
  }

  return (
    <button
      onClick={onClick}
      className={cn(
        'w-full text-left rounded-md p-2 transition-colors',
        isSelected ? 'bg-zinc-100 dark:bg-zinc-800' : 'hover:bg-zinc-50 dark:hover:bg-zinc-900'
      )}
    >
      <div className="flex items-center gap-2 mb-1">
        <span className={cn('w-2 h-2 rounded-full shrink-0', getStatusDotColor(job.status))} />
        <span className="text-xs font-medium truncate">{truncateUrl(job.url)}</span>
      </div>
      <div className="flex items-center gap-1.5 text-[11px] text-zinc-500 pl-4">
        <span>{job.type}</span>
        <span>-</span>
        <span>{job.page_count} pg</span>
        <span className="ml-auto">{formatRelativeTime(job.created_at)}</span>
      </div>
    </button>
  );
}

interface StatItemProps {
  icon: React.ReactNode;
  label: string;
  value: string | React.ReactNode;
  subValue?: string;
}

function StatItem({ icon, label, value, subValue }: StatItemProps) {
  return (
    <div className="flex items-start gap-2">
      <div className="text-zinc-400 mt-0.5">{icon}</div>
      <div>
        <p className="text-[11px] text-zinc-500 dark:text-zinc-400">{label}</p>
        <p className="text-sm font-medium">{value}</p>
        {subValue && <p className="text-[11px] text-zinc-400">{subValue}</p>}
      </div>
    </div>
  );
}

export default function JobsPage() {
  const [jobs, setJobs] = useState<Job[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [isRefreshing, setIsRefreshing] = useState(false);
  const [isDownloading, setIsDownloading] = useState(false);
  const [isLoadingResults, setIsLoadingResults] = useState(false);
  const [selectedJob, setSelectedJob] = useState<Job | null>(null);
  const [mergedResults, setMergedResults] = useState<Record<string, unknown> | null>(null);
  const [showResults, setShowResults] = useState(false);
  const [webhookDeliveries, setWebhookDeliveries] = useState<JobWebhookDelivery[]>([]);
  const jobsRef = useRef<Job[]>([]);

  // Format state
  const [outputFormat, setOutputFormat] = useState<OutputFormat>('json');
  const [formattedOutput, setFormattedOutput] = useState<string>('');
  const [resultsCopied, setResultsCopied] = useState(false);
  const [listCollapsed, setListCollapsed] = useState(false);

  // Load collapsed state from localStorage on mount
  useEffect(() => {
    const saved = localStorage.getItem('jobs-list-collapsed');
    if (saved !== null) {
      setListCollapsed(saved === 'true');
    }
  }, []);

  // Persist collapsed state to localStorage
  useEffect(() => {
    localStorage.setItem('jobs-list-collapsed', String(listCollapsed));
  }, [listCollapsed]);

  // Format output when format or results change
  useEffect(() => {
    if (!mergedResults) {
      setFormattedOutput('');
      return;
    }
    if (outputFormat === 'json') {
      setFormattedOutput(JSON.stringify(mergedResults, null, 2));
    } else if (outputFormat === 'jsonl') {
      const items = (mergedResults as { items?: unknown[] }).items;
      if (Array.isArray(items)) {
        setFormattedOutput(items.map((item) => JSON.stringify(item)).join('\n'));
      } else {
        setFormattedOutput(JSON.stringify(mergedResults));
      }
    } else if (outputFormat === 'yaml') {
      try {
        setFormattedOutput(yaml.dump(mergedResults, { indent: 2, lineWidth: -1 }));
      } catch {
        setFormattedOutput('# Error converting to YAML\n' + JSON.stringify(mergedResults, null, 2));
      }
    }
  }, [outputFormat, mergedResults]);

  const loadJobs = useCallback(async (showRefreshState = false) => {
    if (showRefreshState) setIsRefreshing(true);
    try {
      const { jobs: jobList } = await listJobs();
      const newJobs = jobList || [];
      setJobs(newJobs);
      jobsRef.current = newJobs;
    } catch (err) {
      const error = err as { error?: string };
      toast.error(error.error || 'Failed to load jobs');
    } finally {
      setIsLoading(false);
      setIsRefreshing(false);
    }
  }, []);

  // Initial load
  useEffect(() => {
    loadJobs();
  }, [loadJobs]);

  // Poll for updates every 5 seconds if there are pending/running jobs
  useEffect(() => {
    const interval = setInterval(() => {
      const hasPendingJobs = jobsRef.current.some(
        (j) => j.status === 'pending' || j.status === 'running'
      );
      if (hasPendingJobs) {
        loadJobs();
      }
    }, 5000);
    return () => clearInterval(interval);
  }, [loadJobs]);

  const viewJobDetails = async (jobId: string) => {
    try {
      const job = await getJob(jobId);
      setSelectedJob(job);
      setMergedResults(null);
      setFormattedOutput('');
      setShowResults(false);
      setWebhookDeliveries([]);

      // Load webhook deliveries in background
      getJobWebhookDeliveries(jobId)
        .then((res) => setWebhookDeliveries(res.deliveries || []))
        .catch(() => setWebhookDeliveries([]));
    } catch (err) {
      const error = err as { error?: string };
      toast.error(error.error || 'Failed to load job details');
    }
  };

  const loadJobResults = async (jobId: string) => {
    setIsLoadingResults(true);
    try {
      const response = await getJobResults(jobId, true);
      setMergedResults(response.merged || null);
      setShowResults(true);
    } catch (err) {
      const error = err as { error?: string };
      toast.error(error.error || 'Failed to load results');
    } finally {
      setIsLoadingResults(false);
    }
  };

  const handleDownloadResults = async (jobId: string) => {
    setIsDownloading(true);
    try {
      let content = formattedOutput;

      if (!content) {
        const response = await getJobResults(jobId, true);
        const results = response.merged || {};

        if (outputFormat === 'json') {
          content = JSON.stringify(results, null, 2);
        } else if (outputFormat === 'jsonl') {
          const items = (results as { items?: unknown[] }).items;
          if (Array.isArray(items)) {
            content = items.map((item) => JSON.stringify(item)).join('\n');
          } else {
            content = JSON.stringify(results);
          }
        } else if (outputFormat === 'yaml') {
          content = yaml.dump(results, { indent: 2, lineWidth: -1 });
        }
      }

      const extensions: Record<OutputFormat, string> = {
        json: '.json',
        jsonl: '.jsonl',
        yaml: '.yaml',
      };
      const ext = extensions[outputFormat];

      const mimeTypes: Record<OutputFormat, string> = {
        json: 'application/json',
        jsonl: 'application/x-ndjson',
        yaml: 'application/yaml',
      };
      const blob = new Blob([content], { type: mimeTypes[outputFormat] });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `job-${jobId}-results${ext}`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);

      toast.success(`Downloaded as ${outputFormat.toUpperCase()}`);
    } catch (err) {
      const error = err as { error?: string };
      toast.error(error.error || 'Failed to download results');
    } finally {
      setIsDownloading(false);
    }
  };

  const copyResults = async () => {
    await navigator.clipboard.writeText(formattedOutput);
    setResultsCopied(true);
    setTimeout(() => setResultsCopied(false), 2000);
  };

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-zinc-900 dark:border-white" />
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full gap-4">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Jobs</h1>
          <p className="mt-2 text-zinc-600 dark:text-zinc-400">
            View your extraction and crawl job history.
          </p>
        </div>
        <Button variant="outline" onClick={() => loadJobs(true)} disabled={isRefreshing}>
          {isRefreshing ? (
            <Loader2 className="h-4 w-4 animate-spin" />
          ) : (
            <RefreshCw className="h-4 w-4" />
          )}
          <span className="ml-2">Refresh</span>
        </Button>
      </div>

      {/* Main content */}
      {jobs.length === 0 ? (
        <div className="flex-1 rounded-lg border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 flex items-center justify-center">
          <div className="text-center">
            <p className="text-zinc-500 dark:text-zinc-400 mb-2">No jobs yet</p>
            <p className="text-sm text-zinc-400 dark:text-zinc-500">
              Start an extraction or crawl to see your job history here.
            </p>
          </div>
        </div>
      ) : (
        <div className="flex-1 flex gap-4 min-h-0">
          {/* Collapsible Job List */}
          <div
            className={cn(
              'rounded-lg border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 flex flex-col transition-all duration-200 shrink-0',
              listCollapsed ? 'w-14' : 'w-72'
            )}
          >
            {/* List header */}
            <div className="p-2 border-b border-zinc-200 dark:border-zinc-800 flex items-center justify-between">
              {!listCollapsed && (
                <div className="flex items-center gap-2">
                  <span className="text-sm font-medium">Jobs</span>
                  <span className="text-xs text-zinc-500">{jobs.length}</span>
                </div>
              )}
              <Button
                variant="ghost"
                size="icon"
                className={cn('h-7 w-7', listCollapsed && 'mx-auto')}
                onClick={() => setListCollapsed(!listCollapsed)}
                title={listCollapsed ? 'Expand job list' : 'Collapse job list'}
              >
                {listCollapsed ? (
                  <PanelLeft className="h-4 w-4" />
                ) : (
                  <PanelLeftClose className="h-4 w-4" />
                )}
              </Button>
            </div>

            {/* Job list */}
            <div className="flex-1 overflow-auto p-1.5 space-y-0.5">
              {jobs.map((job, index) => (
                <JobListItem
                  key={job.id}
                  job={job}
                  index={index}
                  isSelected={selectedJob?.id === job.id}
                  collapsed={listCollapsed}
                  onClick={() => viewJobDetails(job.id)}
                />
              ))}
            </div>
          </div>

          {/* Job Details */}
          <div className="flex-1 rounded-lg border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 flex flex-col min-h-0 overflow-hidden">
            {selectedJob ? (
              <>
                {/* Job metadata header */}
                <div className="p-4 border-b border-zinc-200 dark:border-zinc-800 shrink-0">
                  <div className="flex items-start justify-between gap-4 mb-3">
                    <div className="min-w-0 flex-1">
                      <p className="font-mono text-sm break-all">{selectedJob.url}</p>
                    </div>
                    <div className="flex items-center gap-2 shrink-0">
                      <Badge variant="outline" className="font-mono text-xs text-zinc-500 dark:text-zinc-400">
                        {selectedJob.id}
                      </Badge>
                      <Badge className={cn(getStatusColor(selectedJob.status))}>
                        {selectedJob.status}
                      </Badge>
                    </div>
                  </div>

                  {/* Stats grid */}
                  <div className="grid grid-cols-2 sm:grid-cols-4 lg:grid-cols-6 gap-4">
                    <StatItem
                      icon={<FileText className="h-4 w-4" />}
                      label="Type"
                      value={<span className="capitalize">{selectedJob.type}</span>}
                    />
                    <StatItem
                      icon={<FileText className="h-4 w-4" />}
                      label="Pages"
                      value={selectedJob.page_count.toString()}
                      subValue={
                        selectedJob.urls_queued > 0
                          ? `of ${selectedJob.urls_queued} queued`
                          : undefined
                      }
                    />
                    <StatItem
                      icon={<Coins className="h-4 w-4" />}
                      label="Cost"
                      value={
                        selectedJob.cost_usd > 0 ? `$${selectedJob.cost_usd.toFixed(4)}` : 'Free'
                      }
                    />
                    <StatItem
                      icon={<FileText className="h-4 w-4" />}
                      label="Tokens"
                      value={formatTokens(
                        selectedJob.token_usage_input + selectedJob.token_usage_output
                      )}
                      subValue={`${formatTokens(selectedJob.token_usage_input)} in / ${formatTokens(selectedJob.token_usage_output)} out`}
                    />
                    <StatItem
                      icon={<Clock className="h-4 w-4" />}
                      label="Created"
                      value={formatRelativeTime(selectedJob.created_at) + ' ago'}
                      subValue={formatDateTime(selectedJob.created_at)}
                    />
                    {selectedJob.started_at && selectedJob.completed_at && (
                      <StatItem
                        icon={<Timer className="h-4 w-4" />}
                        label="Duration"
                        value={formatDuration(selectedJob.started_at, selectedJob.completed_at)}
                      />
                    )}
                  </div>

                  {/* Webhook deliveries */}
                  {webhookDeliveries.length > 0 && (
                    <div className="mt-4 pt-3 border-t border-zinc-200 dark:border-zinc-800">
                      <div className="flex items-center gap-2 mb-2">
                        <Webhook className="h-4 w-4 text-zinc-400" />
                        <span className="text-sm font-medium">Webhook Deliveries</span>
                      </div>
                      <div className="space-y-1.5">
                        {webhookDeliveries.map((delivery) => (
                          <div
                            key={delivery.id}
                            className="flex items-center gap-2 text-xs bg-zinc-50 dark:bg-zinc-800/50 rounded px-2 py-1.5"
                          >
                            {getWebhookStatusIcon(delivery.status)}
                            <span className="font-medium">{delivery.event_type}</span>
                            <span className="text-zinc-400 truncate flex-1">{delivery.url}</span>
                            {delivery.status_code && (
                              <span
                                className={cn(
                                  'text-[10px] px-1.5 py-0.5 rounded',
                                  delivery.status_code >= 200 && delivery.status_code < 300
                                    ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400'
                                    : 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400'
                                )}
                              >
                                {delivery.status_code}
                              </span>
                            )}
                            {delivery.response_time_ms && (
                              <span className="text-zinc-400">{delivery.response_time_ms}ms</span>
                            )}
                            {delivery.status === 'retrying' && (
                              <span className="text-yellow-600 dark:text-yellow-400">
                                Attempt {delivery.attempt_number}/{delivery.max_attempts}
                              </span>
                            )}
                          </div>
                        ))}
                      </div>
                    </div>
                  )}

                  {/* Error message */}
                  {selectedJob.error_message && (
                    <div className="mt-3 p-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded text-sm text-red-600 dark:text-red-400">
                      {selectedJob.error_category && (
                        <span className="font-medium">[{selectedJob.error_category}] </span>
                      )}
                      {selectedJob.error_message}
                    </div>
                  )}
                </div>

                {/* Results/Inspector section for completed jobs */}
                {selectedJob.status === 'completed' && (
                  <Tabs key={selectedJob.id} defaultValue="results" className="flex-1 flex flex-col min-h-0">
                    {/* Tab header */}
                    <div className="px-4 py-2 border-b border-zinc-200 dark:border-zinc-800 flex items-center justify-between shrink-0">
                      <TabsList className="h-8">
                        <TabsTrigger value="results" className="text-xs px-3">
                          <FileText className="h-3.5 w-3.5 mr-1.5" />
                          Results
                        </TabsTrigger>
                        {selectedJob.capture_debug && (
                          <TabsTrigger value="inspector" className="text-xs px-3">
                            <Search className="h-3.5 w-3.5 mr-1.5" />
                            Inspector
                          </TabsTrigger>
                        )}
                      </TabsList>
                      <div className="flex items-center gap-2">
                        <Button
                          variant="outline"
                          size="sm"
                          className="h-8 text-xs"
                          onClick={() => loadJobResults(selectedJob.id)}
                          disabled={isLoadingResults || showResults}
                        >
                          {isLoadingResults ? (
                            <Loader2 className="h-3.5 w-3.5 mr-1.5 animate-spin" />
                          ) : (
                            <Eye className="h-3.5 w-3.5 mr-1.5" />
                          )}
                          Load Results
                        </Button>
                        <Select
                          value={outputFormat}
                          onValueChange={(v) => setOutputFormat(v as OutputFormat)}
                          disabled={!showResults}
                        >
                          <SelectTrigger className={cn("h-8 w-[80px] text-xs", !showResults && "opacity-50")}>
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
                          className={cn("h-8 w-8", !showResults && "opacity-50")}
                          onClick={copyResults}
                          disabled={!showResults}
                          title="Copy to clipboard"
                        >
                          {resultsCopied ? (
                            <Check className="h-4 w-4 text-green-500" />
                          ) : (
                            <Copy className="h-4 w-4" />
                          )}
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon"
                          className="h-8 w-8"
                          onClick={() => handleDownloadResults(selectedJob.id)}
                          disabled={isDownloading}
                          title={`Download as ${outputFormat.toUpperCase()}`}
                        >
                          {isDownloading ? (
                            <Loader2 className="h-4 w-4 animate-spin" />
                          ) : (
                            <Download className="h-4 w-4" />
                          )}
                        </Button>
                      </div>
                    </div>

                    {/* Results tab content */}
                    <TabsContent value="results" className="flex-1 overflow-auto min-h-0 m-0">
                      {!showResults ? (
                        <div className="flex items-center justify-center h-full text-zinc-400">
                          <p className="text-sm">Click &quot;Load Results&quot; to view extracted data</p>
                        </div>
                      ) : formattedOutput ? (
                        <div className="bg-zinc-950 h-full">
                          <pre className="p-4 text-sm text-zinc-300 font-mono whitespace-pre-wrap break-all">
                            {formattedOutput}
                          </pre>
                        </div>
                      ) : (
                        <div className="flex items-center justify-center h-full text-zinc-500">
                          No results found
                        </div>
                      )}
                    </TabsContent>

                    {/* Inspector tab content */}
                    {selectedJob.capture_debug && (
                      <TabsContent value="inspector" className="flex-1 min-h-0 m-0">
                        <JobInspector jobId={selectedJob.id} />
                      </TabsContent>
                    )}
                  </Tabs>
                )}

                {/* For running jobs */}
                {selectedJob.status === 'running' && (
                  <div className="flex-1 flex items-center justify-center">
                    <div className="text-center">
                      <Loader2 className="h-8 w-8 animate-spin mx-auto mb-3 text-blue-500" />
                      <p className="text-sm text-zinc-500">Job is running...</p>
                      <p className="text-xs text-zinc-400 mt-1">
                        {selectedJob.page_count} pages processed
                        {selectedJob.urls_queued > 0 && ` of ${selectedJob.urls_queued} queued`}
                      </p>
                    </div>
                  </div>
                )}

                {/* For pending jobs */}
                {selectedJob.status === 'pending' && (
                  <div className="flex-1 flex items-center justify-center">
                    <div className="text-center">
                      <div className="w-8 h-8 rounded-full bg-yellow-100 dark:bg-yellow-900/30 flex items-center justify-center mx-auto mb-3">
                        <div className="w-3 h-3 rounded-full bg-yellow-500" />
                      </div>
                      <p className="text-sm text-zinc-500">Job is pending...</p>
                    </div>
                  </div>
                )}

                {/* For failed jobs */}
                {selectedJob.status === 'failed' && !selectedJob.error_message && (
                  <div className="flex-1 flex items-center justify-center p-4">
                    <div className="text-center">
                      <div className="w-8 h-8 rounded-full bg-red-100 dark:bg-red-900/30 flex items-center justify-center mx-auto mb-3">
                        <div className="w-3 h-3 rounded-full bg-red-500" />
                      </div>
                      <p className="text-sm text-zinc-500">Job failed</p>
                    </div>
                  </div>
                )}
              </>
            ) : (
              <div className="flex items-center justify-center h-full text-zinc-400">
                <p>Select a job to view details</p>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
