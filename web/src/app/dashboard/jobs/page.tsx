'use client';

import { useEffect, useState, useRef, useCallback } from 'react';
import { listJobs, getJob, getJobResults, getJobDownloadUrl, Job } from '@/lib/api';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { toast } from 'sonner';
import { Download, Loader2, Eye, ChevronDown, ChevronUp } from 'lucide-react';

function formatDate(dateString: string) {
  return new Date(dateString).toLocaleString();
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

export default function JobsPage() {
  const [jobs, setJobs] = useState<Job[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [isDownloading, setIsDownloading] = useState(false);
  const [isLoadingResults, setIsLoadingResults] = useState(false);
  const [selectedJob, setSelectedJob] = useState<Job | null>(null);
  const [mergedResults, setMergedResults] = useState<Record<string, unknown> | null>(null);
  const [showResults, setShowResults] = useState(false);
  const jobsRef = useRef<Job[]>([]);

  const loadJobs = useCallback(async () => {
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
        j => j.status === 'pending' || j.status === 'running'
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
      setShowResults(false);
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
      const result = await getJobDownloadUrl(jobId);
      // Open download URL in new tab
      window.open(result.download_url, '_blank');
      toast.success('Download started');
    } catch (err) {
      const error = err as { error?: string; status?: number };
      if (error.status === 404) {
        toast.error('Results not available - storage may not be configured');
      } else if (error.status === 503) {
        toast.error('Result storage is not configured');
      } else {
        toast.error(error.error || 'Failed to get download URL');
      }
    } finally {
      setIsDownloading(false);
    }
  };

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-zinc-900 dark:border-white" />
      </div>
    );
  }

  return (
    <div className="max-w-6xl">
      <div className="mb-8 flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Jobs</h1>
          <p className="mt-2 text-zinc-600 dark:text-zinc-400">
            View your extraction and crawl job history.
          </p>
        </div>
        <Button variant="outline" onClick={loadJobs}>
          Refresh
        </Button>
      </div>

      {jobs.length === 0 ? (
        <Card>
          <CardContent className="flex flex-col items-center justify-center py-12">
            <p className="text-zinc-500 dark:text-zinc-400 mb-4">No jobs yet</p>
            <p className="text-sm text-zinc-400 dark:text-zinc-500">
              Start an extraction or crawl to see your job history here.
            </p>
          </CardContent>
        </Card>
      ) : (
        <div className="grid gap-6 lg:grid-cols-2">
          <Card>
            <CardHeader>
              <CardTitle>Job History</CardTitle>
              <CardDescription>{jobs.length} job(s)</CardDescription>
            </CardHeader>
            <CardContent className="space-y-2">
              {jobs.map((job) => (
                <button
                  key={job.id}
                  onClick={() => viewJobDetails(job.id)}
                  className={`w-full text-left rounded-lg border p-4 transition-colors hover:bg-zinc-50 dark:hover:bg-zinc-900 ${
                    selectedJob?.id === job.id
                      ? 'border-zinc-900 dark:border-zinc-100'
                      : 'border-zinc-200 dark:border-zinc-800'
                  }`}
                >
                  <div className="flex items-center justify-between mb-2">
                    <Badge className={getStatusColor(job.status)}>
                      {job.status}
                    </Badge>
                    <span className="text-xs text-zinc-500">
                      {formatDate(job.created_at)}
                    </span>
                  </div>
                  <p className="font-mono text-sm truncate">{job.url}</p>
                  <div className="mt-2 flex gap-4 text-xs text-zinc-500">
                    <span>{job.type}</span>
                    <span>{job.page_count} page(s)</span>
                    {job.cost_credits > 0 && (
                      <span>${(job.cost_credits / 100).toFixed(4)}</span>
                    )}
                  </div>
                </button>
              ))}
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Job Details</CardTitle>
              <CardDescription>
                {selectedJob ? `Job ${selectedJob.id}` : 'Select a job to view details'}
              </CardDescription>
            </CardHeader>
            <CardContent>
              {selectedJob ? (
                <div className="space-y-4">
                  <div className="grid grid-cols-2 gap-4 text-sm">
                    <div>
                      <p className="text-zinc-500 dark:text-zinc-400">Status</p>
                      <Badge className={getStatusColor(selectedJob.status)}>
                        {selectedJob.status}
                      </Badge>
                    </div>
                    <div>
                      <p className="text-zinc-500 dark:text-zinc-400">Type</p>
                      <p className="font-medium capitalize">{selectedJob.type}</p>
                    </div>
                    <div>
                      <p className="text-zinc-500 dark:text-zinc-400">Pages</p>
                      <p className="font-medium">{selectedJob.page_count}</p>
                    </div>
                    <div>
                      <p className="text-zinc-500 dark:text-zinc-400">Cost</p>
                      <p className="font-medium">
                        {selectedJob.cost_credits > 0
                          ? `$${(selectedJob.cost_credits / 100).toFixed(4)}`
                          : '-'}
                      </p>
                    </div>
                    <div>
                      <p className="text-zinc-500 dark:text-zinc-400">Input Tokens</p>
                      <p className="font-medium">{selectedJob.token_usage_input.toLocaleString()}</p>
                    </div>
                    <div>
                      <p className="text-zinc-500 dark:text-zinc-400">Output Tokens</p>
                      <p className="font-medium">{selectedJob.token_usage_output.toLocaleString()}</p>
                    </div>
                  </div>
                  <div>
                    <p className="text-zinc-500 dark:text-zinc-400 text-sm">URL</p>
                    <p className="font-mono text-sm break-all">{selectedJob.url}</p>
                  </div>
                  <div className="grid grid-cols-2 gap-4 text-sm">
                    <div>
                      <p className="text-zinc-500 dark:text-zinc-400">Created</p>
                      <p className="font-medium">{formatDate(selectedJob.created_at)}</p>
                    </div>
                    {selectedJob.started_at && (
                      <div>
                        <p className="text-zinc-500 dark:text-zinc-400">Started</p>
                        <p className="font-medium">{formatDate(selectedJob.started_at)}</p>
                      </div>
                    )}
                    {selectedJob.completed_at && (
                      <div>
                        <p className="text-zinc-500 dark:text-zinc-400">Completed</p>
                        <p className="font-medium">{formatDate(selectedJob.completed_at)}</p>
                      </div>
                    )}
                  </div>
                  {selectedJob.error_message && (
                    <div>
                      <p className="text-zinc-500 dark:text-zinc-400 text-sm">Error</p>
                      <p className="text-red-600 dark:text-red-400 text-sm">{selectedJob.error_message}</p>
                    </div>
                  )}
                  {selectedJob.status === 'completed' && (
                    <div className="pt-2 border-t border-zinc-200 dark:border-zinc-800 space-y-2">
                      <div className="flex gap-2">
                        <Button
                          variant="default"
                          size="sm"
                          onClick={() => showResults ? setShowResults(false) : loadJobResults(selectedJob.id)}
                          disabled={isLoadingResults}
                          className="flex-1"
                        >
                          {isLoadingResults ? (
                            <>
                              <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                              Loading...
                            </>
                          ) : showResults ? (
                            <>
                              <ChevronUp className="h-4 w-4 mr-2" />
                              Hide Results
                            </>
                          ) : (
                            <>
                              <Eye className="h-4 w-4 mr-2" />
                              View Results
                            </>
                          )}
                        </Button>
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => handleDownloadResults(selectedJob.id)}
                          disabled={isDownloading}
                          title="Download as JSON file"
                        >
                          {isDownloading ? (
                            <Loader2 className="h-4 w-4 animate-spin" />
                          ) : (
                            <Download className="h-4 w-4" />
                          )}
                        </Button>
                      </div>
                      {showResults && mergedResults && (
                        <div className="rounded-lg border border-zinc-200 dark:border-zinc-800 overflow-hidden">
                          <div className="px-3 py-2 bg-zinc-100 dark:bg-zinc-800 border-b border-zinc-200 dark:border-zinc-700 flex items-center justify-between">
                            <span className="text-xs font-medium text-zinc-600 dark:text-zinc-400">
                              Merged results from {selectedJob.page_count} page{selectedJob.page_count !== 1 ? 's' : ''}
                            </span>
                          </div>
                          <div className="max-h-[400px] overflow-auto bg-zinc-950">
                            <pre className="p-3 text-xs text-zinc-300 font-mono">
                              {JSON.stringify(mergedResults, null, 2)}
                            </pre>
                          </div>
                        </div>
                      )}
                      {showResults && !mergedResults && !isLoadingResults && (
                        <div className="rounded-lg border border-zinc-200 dark:border-zinc-800 p-4 text-center text-sm text-zinc-500">
                          No results found for this job
                        </div>
                      )}
                    </div>
                  )}
                </div>
              ) : (
                <div className="flex items-center justify-center h-48 text-zinc-400">
                  <p>Select a job to view details</p>
                </div>
              )}
            </CardContent>
          </Card>
        </div>
      )}
    </div>
  );
}
