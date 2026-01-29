'use client';

import { useEffect, useState, useCallback } from 'react';
import { useUser } from '@clerk/nextjs';
import { Card, CardHeader } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Input } from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { ChevronLeft, ChevronRight, RefreshCw, Settings2, ExternalLink, Loader2, Eye } from 'lucide-react';
import Link from 'next/link';
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover';
import { Checkbox } from '@/components/ui/checkbox';
import { Label } from '@/components/ui/label';
import { toast } from 'sonner';
import {
  StatsCard,
  TrendsChart,
  ErrorsChart,
  DateRangePicker,
  ExportButton,
} from '@/components/analytics';
import {
  getAnalyticsOverview,
  getAnalyticsJobs,
  getAnalyticsErrors,
  getAnalyticsTrends,
  getAnalyticsUsers,
  getAdminJobResults,
  type AnalyticsOverview,
  type AnalyticsJob,
  type AnalyticsErrorSummary,
  type TrendDataPoint,
  type AnalyticsUserSummary,
} from '@/lib/api';

function formatDate(dateStr: string): string {
  const date = new Date(dateStr);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffMins = Math.floor(diffMs / 60000);
  const diffHours = Math.floor(diffMs / 3600000);
  const diffDays = Math.floor(diffMs / 86400000);

  if (diffMins < 1) return 'just now';
  if (diffMins < 60) return `${diffMins}m ago`;
  if (diffHours < 24) return `${diffHours}h ago`;
  if (diffDays < 7) return `${diffDays}d ago`;

  return date.toLocaleDateString('en-US', {
    month: 'short',
    day: 'numeric',
    year: date.getFullYear() !== now.getFullYear() ? 'numeric' : undefined,
  });
}

function formatCurrency(value: number | undefined | null): string {
  if (value == null) {
    return '$0.00';
  }
  if (value >= 1) {
    return `$${value.toFixed(2)}`;
  }
  return `$${value.toFixed(4)}`;
}

function formatTokens(value: number): string {
  if (value >= 1000000) {
    return `${(value / 1000000).toFixed(1)}M`;
  }
  if (value >= 1000) {
    return `${(value / 1000).toFixed(1)}K`;
  }
  return value.toString();
}

export default function AdminAnalyticsPage() {
  const { user, isLoaded } = useUser();
  const isSuperadmin = user?.publicMetadata?.global_superadmin === true;

  // Date range state
  const [startDate, setStartDate] = useState(() => {
    const date = new Date();
    date.setDate(date.getDate() - 7);
    return date.toISOString().split('T')[0];
  });
  const [endDate, setEndDate] = useState(() => {
    return new Date().toISOString().split('T')[0];
  });

  // Data state
  const [isLoading, setIsLoading] = useState(true);
  const [overview, setOverview] = useState<AnalyticsOverview | null>(null);
  const [trends, setTrends] = useState<TrendDataPoint[]>([]);
  const [errors, setErrors] = useState<AnalyticsErrorSummary | null>(null);
  const [jobs, setJobs] = useState<AnalyticsJob[]>([]);
  const [jobsTotal, setJobsTotal] = useState(0);
  const [users, setUsers] = useState<AnalyticsUserSummary[]>([]);
  const [usersTotal, setUsersTotal] = useState(0);

  // Filter state
  const [activeTab, setActiveTab] = useState('jobs');
  const [statusFilter, setStatusFilter] = useState<string>('');
  const [typeFilter, setTypeFilter] = useState<string>('');
  const [userIdFilter, setUserIdFilter] = useState('');
  const [jobsPage, setJobsPage] = useState(0);
  const [usersPage, setUsersPage] = useState(0);
  const [userSort, setUserSort] = useState('total_cost');
  const pageSize = 50;

  // Column visibility state with localStorage persistence
  type JobColumn = 'id' | 'user' | 'type' | 'status' | 'url' | 'cost' | 'llmCost' | 'tokens' | 'provider' | 'model' | 'discovery' | 'created' | 'view' | 'results';
  const STORAGE_KEY = 'admin-analytics-visible-columns';
  const defaultColumns: JobColumn[] = ['user', 'type', 'status', 'cost', 'llmCost', 'tokens', 'provider', 'discovery', 'created', 'view', 'results'];

  const [visibleColumns, setVisibleColumns] = useState<Set<JobColumn>>(() => {
    if (typeof window === 'undefined') return new Set(defaultColumns);
    try {
      const stored = localStorage.getItem(STORAGE_KEY);
      if (stored) {
        const parsed = JSON.parse(stored) as JobColumn[];
        return new Set(parsed);
      }
    } catch {
      // Ignore parse errors
    }
    return new Set(defaultColumns);
  });

  // Persist column visibility to localStorage
  useEffect(() => {
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify([...visibleColumns]));
    } catch {
      // Ignore storage errors
    }
  }, [visibleColumns]);

  const columnLabels: Record<JobColumn, string> = {
    id: 'Job ID',
    user: 'User',
    type: 'Type',
    status: 'Status',
    url: 'URL',
    cost: 'User Cost',
    llmCost: 'LLM Cost',
    tokens: 'Tokens',
    provider: 'Provider',
    model: 'Model',
    discovery: 'Discovery',
    created: 'Created',
    view: 'View',
    results: 'Results',
  };

  // State for tracking which job is currently loading results
  const [loadingResultsJobId, setLoadingResultsJobId] = useState<string | null>(null);

  const handleGetResults = async (jobId: string) => {
    setLoadingResultsJobId(jobId);
    try {
      const result = await getAdminJobResults(jobId);
      // Copy to clipboard
      await navigator.clipboard.writeText(result.download_url);
      toast.success('Results URL copied to clipboard');
    } catch (err) {
      const error = err as { error?: string; status?: number };
      if (error.status === 404) {
        toast.error('No results found for this job');
      } else {
        toast.error(error.error || 'Failed to get results URL');
      }
    } finally {
      setLoadingResultsJobId(null);
    }
  };

  const toggleColumn = (col: JobColumn) => {
    setVisibleColumns(prev => {
      const next = new Set(prev);
      if (next.has(col)) {
        next.delete(col);
      } else {
        next.add(col);
      }
      return next;
    });
  };

  const loadData = useCallback(async () => {
    setIsLoading(true);
    try {
      const [overviewRes, trendsRes, errorsRes] = await Promise.all([
        getAnalyticsOverview(startDate, endDate),
        getAnalyticsTrends(startDate, endDate, 'day'),
        getAnalyticsErrors(startDate, endDate),
      ]);

      setOverview(overviewRes);
      setTrends(trendsRes.trends || []);
      setErrors(errorsRes);
    } catch (err) {
      const error = err as { error?: string; status?: number };
      if (error.status === 403) {
        // Not admin
      } else {
        toast.error(error.error || 'Failed to load analytics');
      }
    } finally {
      setIsLoading(false);
    }
  }, [startDate, endDate]);

  const loadJobs = useCallback(async () => {
    try {
      const res = await getAnalyticsJobs({
        start_date: startDate,
        end_date: endDate,
        status: statusFilter || undefined,
        type: typeFilter || undefined,
        user_id: userIdFilter || undefined,
        limit: pageSize,
        offset: jobsPage * pageSize,
        sort: 'created_at',
        order: 'desc',
      });
      setJobs(res.jobs || []);
      setJobsTotal(res.total_count);
    } catch (err) {
      const error = err as { error?: string };
      toast.error(error.error || 'Failed to load jobs');
    }
  }, [startDate, endDate, statusFilter, typeFilter, userIdFilter, jobsPage]);

  const loadUsers = useCallback(async () => {
    try {
      const res = await getAnalyticsUsers({
        start_date: startDate,
        end_date: endDate,
        limit: pageSize,
        offset: usersPage * pageSize,
        sort: userSort,
        order: 'desc',
      });
      setUsers(res.users || []);
      setUsersTotal(res.total_count);
    } catch (err) {
      const error = err as { error?: string };
      toast.error(error.error || 'Failed to load users');
    }
  }, [startDate, endDate, usersPage, userSort]);

  useEffect(() => {
    if (isLoaded && isSuperadmin) {
      loadData();
    } else if (isLoaded) {
      setIsLoading(false);
    }
  }, [isLoaded, isSuperadmin, loadData]);

  useEffect(() => {
    if (isLoaded && isSuperadmin && activeTab === 'jobs') {
      loadJobs();
    }
  }, [isLoaded, isSuperadmin, activeTab, loadJobs]);

  useEffect(() => {
    if (isLoaded && isSuperadmin && activeTab === 'users') {
      loadUsers();
    }
  }, [isLoaded, isSuperadmin, activeTab, loadUsers]);

  const handleDateRangeChange = (newStart: string, newEnd: string) => {
    setStartDate(newStart);
    setEndDate(newEnd);
    setJobsPage(0);
    setUsersPage(0);
  };

  const handleRefresh = () => {
    loadData();
    if (activeTab === 'jobs') loadJobs();
    if (activeTab === 'users') loadUsers();
  };

  const getExportData = async () => {
    if (activeTab === 'jobs') {
      const res = await getAnalyticsJobs({
        start_date: startDate,
        end_date: endDate,
        status: statusFilter || undefined,
        type: typeFilter || undefined,
        user_id: userIdFilter || undefined,
        limit: 1000,
        offset: 0,
      });
      return { type: 'jobs' as const, data: res.jobs || [] };
    } else if (activeTab === 'users') {
      const res = await getAnalyticsUsers({
        start_date: startDate,
        end_date: endDate,
        limit: 1000,
        offset: 0,
        sort: userSort,
        order: 'desc',
      });
      return { type: 'users' as const, data: res.users || [] };
    } else {
      const res = await getAnalyticsTrends(startDate, endDate, 'day');
      return { type: 'trends' as const, data: res.trends || [] };
    }
  };

  if (!isLoaded || isLoading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-zinc-900 dark:border-white" />
      </div>
    );
  }

  if (!isSuperadmin) {
    return null;
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-xl font-semibold">Analytics</h2>
          <p className="text-sm text-zinc-500 dark:text-zinc-400">
            Usage history, costs, and errors across all users
          </p>
        </div>
        <div className="flex items-center gap-2">
          <DateRangePicker
            startDate={startDate}
            endDate={endDate}
            onChange={handleDateRangeChange}
          />
          <Button variant="outline" size="icon" onClick={handleRefresh}>
            <RefreshCw className="h-4 w-4" />
          </Button>
          <ExportButton getData={getExportData} filename={`analytics-${startDate}-${endDate}`} />
        </div>
      </div>

      {/* Stats Cards */}
      <div className="grid grid-cols-2 md:grid-cols-5 gap-4">
        <StatsCard
          label="Total Jobs"
          value={overview?.total_jobs ?? 0}
          subValue={`${overview?.completed_jobs ?? 0} completed`}
        />
        <StatsCard
          label="Total Cost"
          value={formatCurrency(overview?.total_cost_usd ?? 0)}
          subValue={`${overview?.platform_jobs ?? 0} platform / ${overview?.byok_jobs ?? 0} BYOK`}
        />
        <StatsCard
          label="Error Rate"
          value={`${(overview?.error_rate ?? 0).toFixed(1)}%`}
          subValue={`${overview?.failed_jobs ?? 0} failed`}
          trend={overview?.error_rate && overview.error_rate > 5 ? 'down' : 'neutral'}
        />
        <StatsCard
          label="Tokens"
          value={formatTokens((overview?.total_tokens_input ?? 0) + (overview?.total_tokens_output ?? 0))}
          subValue={`${formatTokens(overview?.total_tokens_input ?? 0)} in / ${formatTokens(overview?.total_tokens_output ?? 0)} out`}
        />
        <StatsCard
          label="Active Users"
          value={overview?.active_users ?? 0}
          subValue="unique users"
        />
      </div>

      {/* Charts */}
      <div className="grid md:grid-cols-2 gap-4">
        <TrendsChart data={trends} />
        <ErrorsChart data={errors?.by_category || []} />
      </div>

      {/* Data Tables */}
      <Card>
        <CardHeader className="pb-3">
          <Tabs value={activeTab} onValueChange={setActiveTab}>
            <TabsList>
              <TabsTrigger value="jobs">Usage History</TabsTrigger>
              <TabsTrigger value="errors">Errors</TabsTrigger>
              <TabsTrigger value="users">Users</TabsTrigger>
            </TabsList>

            <TabsContent value="jobs" className="mt-4 space-y-4">
              {/* Filters */}
              <div className="flex items-center gap-3">
                <Select value={statusFilter || 'all'} onValueChange={(v) => { setStatusFilter(v === 'all' ? '' : v); setJobsPage(0); }}>
                  <SelectTrigger className="w-[140px]">
                    <SelectValue placeholder="All statuses" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">All statuses</SelectItem>
                    <SelectItem value="completed">Completed</SelectItem>
                    <SelectItem value="failed">Failed</SelectItem>
                    <SelectItem value="pending">Pending</SelectItem>
                    <SelectItem value="running">Running</SelectItem>
                  </SelectContent>
                </Select>

                <Select value={typeFilter || 'all'} onValueChange={(v) => { setTypeFilter(v === 'all' ? '' : v); setJobsPage(0); }}>
                  <SelectTrigger className="w-[130px]">
                    <SelectValue placeholder="All types" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">All types</SelectItem>
                    <SelectItem value="extract">Extract</SelectItem>
                    <SelectItem value="crawl">Crawl</SelectItem>
                    <SelectItem value="analyze">Analyze</SelectItem>
                  </SelectContent>
                </Select>

                <Input
                  placeholder="Filter by user ID..."
                  value={userIdFilter}
                  onChange={(e) => { setUserIdFilter(e.target.value); setJobsPage(0); }}
                  className="w-[200px]"
                />

                <div className="flex-1" />

                {/* Column Selector */}
                <Popover>
                  <PopoverTrigger asChild>
                    <Button variant="outline" size="sm" className="gap-2">
                      <Settings2 className="h-4 w-4" />
                      Columns
                    </Button>
                  </PopoverTrigger>
                  <PopoverContent className="w-48" align="end">
                    <div className="space-y-2">
                      <p className="text-sm font-medium">Toggle columns</p>
                      {(Object.keys(columnLabels) as JobColumn[]).map((col) => (
                        <div key={col} className="flex items-center space-x-2">
                          <Checkbox
                            id={`col-${col}`}
                            checked={visibleColumns.has(col)}
                            onCheckedChange={() => toggleColumn(col)}
                          />
                          <Label htmlFor={`col-${col}`} className="text-sm cursor-pointer">
                            {columnLabels[col]}
                          </Label>
                        </div>
                      ))}
                    </div>
                  </PopoverContent>
                </Popover>
              </div>

              {/* Jobs Table */}
              <div className="rounded-md border overflow-x-auto">
                <Table>
                  <TableHeader>
                    <TableRow>
                      {visibleColumns.has('id') && <TableHead>Job ID</TableHead>}
                      {visibleColumns.has('user') && <TableHead>User</TableHead>}
                      {visibleColumns.has('type') && <TableHead>Type</TableHead>}
                      {visibleColumns.has('status') && <TableHead>Status</TableHead>}
                      {visibleColumns.has('url') && <TableHead>URL</TableHead>}
                      {visibleColumns.has('cost') && <TableHead>User Cost</TableHead>}
                      {visibleColumns.has('llmCost') && <TableHead>LLM Cost</TableHead>}
                      {visibleColumns.has('tokens') && <TableHead>Tokens</TableHead>}
                      {visibleColumns.has('provider') && <TableHead>Provider</TableHead>}
                      {visibleColumns.has('model') && <TableHead>Model</TableHead>}
                      {visibleColumns.has('discovery') && <TableHead>Discovery</TableHead>}
                      {visibleColumns.has('created') && <TableHead>Created</TableHead>}
                      {visibleColumns.has('view') && <TableHead>View</TableHead>}
                      {visibleColumns.has('results') && <TableHead>Results</TableHead>}
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {jobs.length === 0 ? (
                      <TableRow>
                        <TableCell colSpan={visibleColumns.size} className="text-center text-zinc-500">
                          No jobs found
                        </TableCell>
                      </TableRow>
                    ) : (
                      jobs.map((job) => (
                        <TableRow key={job.id}>
                          {visibleColumns.has('id') && (
                            <TableCell className="font-mono text-xs max-w-[100px] truncate" title={job.id}>
                              {job.id.substring(0, 8)}...
                            </TableCell>
                          )}
                          {visibleColumns.has('user') && (
                            <TableCell className="font-mono text-xs max-w-[120px] truncate" title={job.user_id}>
                              {job.user_id.substring(0, 12)}...
                            </TableCell>
                          )}
                          {visibleColumns.has('type') && (
                            <TableCell>
                              <Badge variant="outline">{job.type}</Badge>
                            </TableCell>
                          )}
                          {visibleColumns.has('status') && (
                            <TableCell>
                              <Badge
                                variant={
                                  job.status === 'completed' ? 'default' :
                                  job.status === 'failed' ? 'destructive' :
                                  'secondary'
                                }
                              >
                                {job.status}
                              </Badge>
                            </TableCell>
                          )}
                          {visibleColumns.has('url') && (
                            <TableCell className="font-mono text-xs max-w-[200px] truncate" title={job.url}>
                              {job.url}
                            </TableCell>
                          )}
                          {visibleColumns.has('cost') && (
                            <TableCell>
                              {job.is_byok ? (
                                <span className="text-zinc-400 text-xs">BYOK</span>
                              ) : (
                                formatCurrency(job.cost_usd)
                              )}
                            </TableCell>
                          )}
                          {visibleColumns.has('llmCost') && (
                            <TableCell className="text-sm">
                              {formatCurrency(job.llm_cost_usd)}
                            </TableCell>
                          )}
                          {visibleColumns.has('tokens') && (
                            <TableCell className="text-sm">
                              {formatTokens(job.tokens_input + job.tokens_output)}
                            </TableCell>
                          )}
                          {visibleColumns.has('provider') && (
                            <TableCell className="text-xs">
                              {job.provider || <span className="text-zinc-400">-</span>}
                            </TableCell>
                          )}
                          {visibleColumns.has('model') && (
                            <TableCell className="font-mono text-xs max-w-[150px] truncate" title={job.model || ''}>
                              {job.model || <span className="text-zinc-400">-</span>}
                            </TableCell>
                          )}
                          {visibleColumns.has('discovery') && (
                            <TableCell className="text-xs">
                              {job.discovery_method ? (
                                <Badge variant="outline" className="text-xs">
                                  {job.discovery_method}
                                </Badge>
                              ) : (
                                <span className="text-zinc-400">-</span>
                              )}
                            </TableCell>
                          )}
                          {visibleColumns.has('created') && (
                            <TableCell className="text-sm text-zinc-500">
                              {formatDate(job.created_at)}
                            </TableCell>
                          )}
                          {visibleColumns.has('view') && (
                            <TableCell>
                              <Link href={`/dashboard/jobs/${job.id}`}>
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  className="h-7 px-2 text-xs"
                                  title="View job details"
                                >
                                  <Eye className="h-3 w-3" />
                                </Button>
                              </Link>
                            </TableCell>
                          )}
                          {visibleColumns.has('results') && (
                            <TableCell>
                              {job.status === 'completed' ? (
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  className="h-7 px-2 text-xs"
                                  disabled={loadingResultsJobId === job.id}
                                  onClick={() => handleGetResults(job.id)}
                                  title="Copy results URL to clipboard"
                                >
                                  {loadingResultsJobId === job.id ? (
                                    <Loader2 className="h-3 w-3 animate-spin" />
                                  ) : (
                                    <ExternalLink className="h-3 w-3" />
                                  )}
                                </Button>
                              ) : (
                                <span className="text-zinc-400">-</span>
                              )}
                            </TableCell>
                          )}
                        </TableRow>
                      ))
                    )}
                  </TableBody>
                </Table>
              </div>

              {/* Pagination */}
              <div className="flex items-center justify-between">
                <p className="text-sm text-zinc-500">
                  Showing {jobsPage * pageSize + 1}-{Math.min((jobsPage + 1) * pageSize, jobsTotal)} of {jobsTotal}
                </p>
                <div className="flex items-center gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setJobsPage((p) => Math.max(0, p - 1))}
                    disabled={jobsPage === 0}
                  >
                    <ChevronLeft className="h-4 w-4" />
                    Prev
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setJobsPage((p) => p + 1)}
                    disabled={(jobsPage + 1) * pageSize >= jobsTotal}
                  >
                    Next
                    <ChevronRight className="h-4 w-4" />
                  </Button>
                </div>
              </div>
            </TabsContent>

            <TabsContent value="errors" className="mt-4 space-y-4">
              {/* Top Failing URLs */}
              <div>
                <h3 className="text-sm font-medium mb-2">Top Failing URLs</h3>
                <div className="rounded-md border">
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>URL</TableHead>
                        <TableHead className="text-right">Failures</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {(errors?.top_failing_urls || []).length === 0 ? (
                        <TableRow>
                          <TableCell colSpan={2} className="text-center text-zinc-500">
                            No failing URLs
                          </TableCell>
                        </TableRow>
                      ) : (
                        (errors?.top_failing_urls || []).map((url, idx) => (
                          <TableRow key={idx}>
                            <TableCell className="font-mono text-xs max-w-[400px] truncate" title={url.url}>
                              {url.url}
                            </TableCell>
                            <TableCell className="text-right">{url.count}</TableCell>
                          </TableRow>
                        ))
                      )}
                    </TableBody>
                  </Table>
                </div>
              </div>

              {/* Errors by Provider */}
              <div>
                <h3 className="text-sm font-medium mb-2">Errors by Provider/Model</h3>
                <div className="rounded-md border">
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>Provider</TableHead>
                        <TableHead>Model</TableHead>
                        <TableHead className="text-right">Errors</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {(errors?.by_provider || []).length === 0 ? (
                        <TableRow>
                          <TableCell colSpan={3} className="text-center text-zinc-500">
                            No provider errors
                          </TableCell>
                        </TableRow>
                      ) : (
                        (errors?.by_provider || []).map((p, idx) => (
                          <TableRow key={idx}>
                            <TableCell>{p.provider}</TableCell>
                            <TableCell className="font-mono text-xs">{p.model}</TableCell>
                            <TableCell className="text-right">{p.count}</TableCell>
                          </TableRow>
                        ))
                      )}
                    </TableBody>
                  </Table>
                </div>
              </div>
            </TabsContent>

            <TabsContent value="users" className="mt-4 space-y-4">
              {/* Sort Control */}
              <div className="flex items-center gap-3">
                <span className="text-sm text-zinc-500">Sort by:</span>
                <Select value={userSort} onValueChange={(v) => { setUserSort(v); setUsersPage(0); }}>
                  <SelectTrigger className="w-[150px]">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="total_cost">Total Cost</SelectItem>
                    <SelectItem value="total_jobs">Total Jobs</SelectItem>
                    <SelectItem value="failed_jobs">Failed Jobs</SelectItem>
                    <SelectItem value="total_tokens">Total Tokens</SelectItem>
                    <SelectItem value="last_active">Last Active</SelectItem>
                  </SelectContent>
                </Select>
              </div>

              {/* Users Table */}
              <div className="rounded-md border">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>User ID</TableHead>
                      <TableHead className="text-right">Jobs</TableHead>
                      <TableHead className="text-right">Completed</TableHead>
                      <TableHead className="text-right">Failed</TableHead>
                      <TableHead className="text-right">Cost</TableHead>
                      <TableHead className="text-right">Tokens</TableHead>
                      <TableHead>Last Active</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {users.length === 0 ? (
                      <TableRow>
                        <TableCell colSpan={7} className="text-center text-zinc-500">
                          No users found
                        </TableCell>
                      </TableRow>
                    ) : (
                      users.map((u) => (
                        <TableRow key={u.user_id}>
                          <TableCell className="font-mono text-xs max-w-[180px] truncate" title={u.user_id}>
                            {u.user_id}
                          </TableCell>
                          <TableCell className="text-right">{u.total_jobs}</TableCell>
                          <TableCell className="text-right text-green-600 dark:text-green-400">
                            {u.completed_jobs}
                          </TableCell>
                          <TableCell className="text-right text-red-600 dark:text-red-400">
                            {u.failed_jobs}
                          </TableCell>
                          <TableCell className="text-right">{formatCurrency(u.total_cost_usd)}</TableCell>
                          <TableCell className="text-right">{formatTokens(u.total_tokens)}</TableCell>
                          <TableCell className="text-sm text-zinc-500">
                            {u.last_active ? formatDate(u.last_active) : '-'}
                          </TableCell>
                        </TableRow>
                      ))
                    )}
                  </TableBody>
                </Table>
              </div>

              {/* Pagination */}
              <div className="flex items-center justify-between">
                <p className="text-sm text-zinc-500">
                  Showing {usersPage * pageSize + 1}-{Math.min((usersPage + 1) * pageSize, usersTotal)} of {usersTotal}
                </p>
                <div className="flex items-center gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setUsersPage((p) => Math.max(0, p - 1))}
                    disabled={usersPage === 0}
                  >
                    <ChevronLeft className="h-4 w-4" />
                    Prev
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setUsersPage((p) => p + 1)}
                    disabled={(usersPage + 1) * pageSize >= usersTotal}
                  >
                    Next
                    <ChevronRight className="h-4 w-4" />
                  </Button>
                </div>
              </div>
            </TabsContent>
          </Tabs>
        </CardHeader>
      </Card>
    </div>
  );
}
