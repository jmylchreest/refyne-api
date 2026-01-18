'use client';

import { useEffect, useState } from 'react';
import {
  listWebhooks,
  createWebhook,
  updateWebhook,
  deleteWebhook,
  listWebhookDeliveries,
  Webhook,
  WebhookInput,
  WebhookDelivery,
  WebhookHeader,
} from '@/lib/api';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Switch } from '@/components/ui/switch';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { toast } from 'sonner';

const WEBHOOK_EVENTS = [
  { value: '*', label: 'All Events' },
  { value: 'job.started', label: 'Job Started' },
  { value: 'job.completed', label: 'Job Completed' },
  { value: 'job.failed', label: 'Job Failed' },
  { value: 'job.progress', label: 'Job Progress' },
  { value: 'extract.success', label: 'Extract Success' },
  { value: 'extract.failed', label: 'Extract Failed' },
];

function formatDate(dateString: string) {
  return new Date(dateString).toLocaleString();
}

function formatDuration(ms: number | undefined) {
  if (ms === undefined) return '-';
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(2)}s`;
}

function getStatusColor(status: string) {
  switch (status) {
    case 'success':
      return 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200';
    case 'failed':
      return 'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200';
    case 'retrying':
      return 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200';
    case 'pending':
    default:
      return 'bg-zinc-100 text-zinc-800 dark:bg-zinc-800 dark:text-zinc-200';
  }
}

interface WebhookFormData {
  name: string;
  url: string;
  secret: string;
  events: string[];
  headers: WebhookHeader[];
  is_active: boolean;
}

const emptyFormData: WebhookFormData = {
  name: '',
  url: '',
  secret: '',
  events: ['*'],
  headers: [],
  is_active: true,
};

export default function WebhooksPage() {
  const [webhooks, setWebhooks] = useState<Webhook[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [isSaving, setIsSaving] = useState(false);

  // Dialog state
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingWebhook, setEditingWebhook] = useState<Webhook | null>(null);
  const [formData, setFormData] = useState<WebhookFormData>(emptyFormData);

  // Deliveries state
  const [deliveriesDialogOpen, setDeliveriesDialogOpen] = useState(false);
  const [selectedWebhook, setSelectedWebhook] = useState<Webhook | null>(null);
  const [deliveries, setDeliveries] = useState<WebhookDelivery[]>([]);
  const [loadingDeliveries, setLoadingDeliveries] = useState(false);

  const loadWebhooks = async () => {
    try {
      const { webhooks: webhookList } = await listWebhooks();
      setWebhooks(webhookList || []);
    } catch (err) {
      const error = err as { error?: string };
      toast.error(error.error || 'Failed to load webhooks');
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    loadWebhooks();
  }, []);

  const handleOpenCreate = () => {
    setEditingWebhook(null);
    setFormData(emptyFormData);
    setDialogOpen(true);
  };

  const handleOpenEdit = (webhook: Webhook) => {
    setEditingWebhook(webhook);
    setFormData({
      name: webhook.name,
      url: webhook.url,
      secret: '', // Don't populate secret - user must re-enter if they want to change it
      events: webhook.events,
      headers: webhook.headers || [],
      is_active: webhook.is_active,
    });
    setDialogOpen(true);
  };

  const handleSave = async () => {
    if (!formData.name.trim()) {
      toast.error('Please enter a name for the webhook');
      return;
    }
    if (!formData.url.trim()) {
      toast.error('Please enter a URL for the webhook');
      return;
    }

    setIsSaving(true);
    try {
      const input: WebhookInput = {
        name: formData.name,
        url: formData.url,
        events: formData.events,
        headers: formData.headers.filter(h => h.name.trim()),
        is_active: formData.is_active,
      };

      // Only include secret if provided (for new webhook or to update existing)
      if (formData.secret.trim()) {
        input.secret = formData.secret;
      }

      if (editingWebhook) {
        await updateWebhook(editingWebhook.id, input);
        toast.success('Webhook updated');
      } else {
        await createWebhook(input);
        toast.success('Webhook created');
      }

      setDialogOpen(false);
      loadWebhooks();
    } catch (err) {
      const error = err as { error?: string };
      toast.error(error.error || 'Failed to save webhook');
    } finally {
      setIsSaving(false);
    }
  };

  const handleDelete = async (id: string) => {
    if (!confirm('Are you sure you want to delete this webhook? This action cannot be undone.')) {
      return;
    }

    try {
      await deleteWebhook(id);
      toast.success('Webhook deleted');
      loadWebhooks();
    } catch (err) {
      const error = err as { error?: string };
      toast.error(error.error || 'Failed to delete webhook');
    }
  };

  const handleViewDeliveries = async (webhook: Webhook) => {
    setSelectedWebhook(webhook);
    setDeliveriesDialogOpen(true);
    setLoadingDeliveries(true);

    try {
      const { deliveries: deliveryList } = await listWebhookDeliveries(webhook.id);
      setDeliveries(deliveryList || []);
    } catch (err) {
      const error = err as { error?: string };
      toast.error(error.error || 'Failed to load deliveries');
    } finally {
      setLoadingDeliveries(false);
    }
  };

  const addHeader = () => {
    setFormData(prev => ({
      ...prev,
      headers: [...prev.headers, { name: '', value: '' }],
    }));
  };

  const updateHeader = (index: number, field: 'name' | 'value', value: string) => {
    setFormData(prev => ({
      ...prev,
      headers: prev.headers.map((h, i) => i === index ? { ...h, [field]: value } : h),
    }));
  };

  const removeHeader = (index: number) => {
    setFormData(prev => ({
      ...prev,
      headers: prev.headers.filter((_, i) => i !== index),
    }));
  };

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-zinc-900 dark:border-white" />
      </div>
    );
  }

  return (
    <div className="max-w-4xl">
      <div className="mb-8 flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Webhooks</h1>
          <p className="mt-2 text-zinc-600 dark:text-zinc-400">
            Configure webhooks to receive notifications when jobs complete or fail.
          </p>
        </div>
        <Button onClick={handleOpenCreate}>
          Create Webhook
        </Button>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Your Webhooks</CardTitle>
          <CardDescription>
            Webhooks send HTTP POST requests to your server when events occur.
          </CardDescription>
        </CardHeader>
        <CardContent>
          {webhooks.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-12">
              <p className="text-zinc-500 dark:text-zinc-400 mb-4">No webhooks configured</p>
              <p className="text-sm text-zinc-400 dark:text-zinc-500">
                Create a webhook to receive notifications for job events.
              </p>
            </div>
          ) : (
            <div className="space-y-4">
              {webhooks.map((webhook) => (
                <div
                  key={webhook.id}
                  className="flex items-start justify-between rounded-lg border border-zinc-200 dark:border-zinc-800 p-4"
                >
                  <div className="space-y-2 flex-1">
                    <div className="flex items-center gap-2">
                      <p className="font-medium">{webhook.name}</p>
                      <Badge variant={webhook.is_active ? 'default' : 'secondary'}>
                        {webhook.is_active ? 'Active' : 'Inactive'}
                      </Badge>
                      {webhook.has_secret && (
                        <Badge variant="outline" className="text-xs">
                          Signed
                        </Badge>
                      )}
                    </div>
                    <p className="text-sm text-zinc-500 dark:text-zinc-400 font-mono break-all">
                      {webhook.url}
                    </p>
                    <div className="flex flex-wrap gap-1">
                      {webhook.events.map((event) => (
                        <Badge key={event} variant="secondary" className="text-xs">
                          {event === '*' ? 'All Events' : event}
                        </Badge>
                      ))}
                    </div>
                    <div className="flex gap-4 text-xs text-zinc-400">
                      <span>Created {formatDate(webhook.created_at)}</span>
                      <span>Updated {formatDate(webhook.updated_at)}</span>
                    </div>
                  </div>
                  <div className="flex gap-2 ml-4">
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => handleViewDeliveries(webhook)}
                    >
                      Deliveries
                    </Button>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => handleOpenEdit(webhook)}
                    >
                      Edit
                    </Button>
                    <Button
                      variant="destructive"
                      size="sm"
                      onClick={() => handleDelete(webhook.id)}
                    >
                      Delete
                    </Button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      <Card className="mt-6">
        <CardHeader>
          <CardTitle>Webhook Payload</CardTitle>
          <CardDescription>
            Example of the payload your endpoint will receive
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="rounded-lg bg-zinc-950 p-4 overflow-auto">
            <pre className="text-sm text-zinc-300">
{`{
  "event": "job.completed",
  "timestamp": "2024-01-15T10:30:00Z",
  "job_id": "01HQXYZ...",
  "data": {
    "status": "completed",
    "page_count": 25,
    "results": [...],
    "cost_usd": 0.0234
  }
}`}
            </pre>
          </div>
          <div className="mt-4 text-sm text-zinc-500 dark:text-zinc-400">
            <p className="mb-2">
              <strong>Headers included:</strong>
            </p>
            <ul className="list-disc list-inside space-y-1">
              <li><code className="bg-zinc-100 dark:bg-zinc-800 px-1 rounded">Content-Type: application/json</code></li>
              <li><code className="bg-zinc-100 dark:bg-zinc-800 px-1 rounded">X-Refyne-Event: job.completed</code></li>
              <li><code className="bg-zinc-100 dark:bg-zinc-800 px-1 rounded">X-Refyne-Delivery: {"<delivery-id>"}</code></li>
              <li><code className="bg-zinc-100 dark:bg-zinc-800 px-1 rounded">X-Refyne-Signature: {"<HMAC-SHA256 signature>"}</code> (if secret configured)</li>
            </ul>
          </div>
        </CardContent>
      </Card>

      {/* Create/Edit Dialog */}
      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>{editingWebhook ? 'Edit Webhook' : 'Create Webhook'}</DialogTitle>
            <DialogDescription>
              {editingWebhook
                ? 'Update your webhook configuration.'
                : 'Configure a new webhook endpoint.'}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <div className="space-y-2">
              <Label htmlFor="name">Name</Label>
              <Input
                id="name"
                placeholder="e.g., Production Notifications"
                value={formData.name}
                onChange={(e) => setFormData(prev => ({ ...prev, name: e.target.value }))}
                disabled={isSaving}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="url">URL</Label>
              <Input
                id="url"
                placeholder="https://your-server.com/webhook"
                value={formData.url}
                onChange={(e) => setFormData(prev => ({ ...prev, url: e.target.value }))}
                disabled={isSaving}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="secret">
                Secret {editingWebhook && '(leave empty to keep current)'}
              </Label>
              <Input
                id="secret"
                type="password"
                placeholder={editingWebhook ? '********' : 'Optional signing secret'}
                value={formData.secret}
                onChange={(e) => setFormData(prev => ({ ...prev, secret: e.target.value }))}
                disabled={isSaving}
              />
              <p className="text-xs text-zinc-500">
                Used to sign webhook payloads with HMAC-SHA256 for verification.
              </p>
            </div>
            <div className="space-y-2">
              <Label>Events</Label>
              <Select
                value={formData.events.includes('*') ? '*' : 'custom'}
                onValueChange={(value) => {
                  if (value === '*') {
                    setFormData(prev => ({ ...prev, events: ['*'] }));
                  }
                }}
              >
                <SelectTrigger>
                  <SelectValue placeholder="Select events" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="*">All Events</SelectItem>
                  <SelectItem value="custom">Custom Selection</SelectItem>
                </SelectContent>
              </Select>
              {!formData.events.includes('*') && (
                <div className="flex flex-wrap gap-2 mt-2">
                  {WEBHOOK_EVENTS.filter(e => e.value !== '*').map((event) => (
                    <Badge
                      key={event.value}
                      variant={formData.events.includes(event.value) ? 'default' : 'outline'}
                      className="cursor-pointer"
                      onClick={() => {
                        setFormData(prev => ({
                          ...prev,
                          events: prev.events.includes(event.value)
                            ? prev.events.filter(e => e !== event.value)
                            : [...prev.events, event.value],
                        }));
                      }}
                    >
                      {event.label}
                    </Badge>
                  ))}
                </div>
              )}
            </div>
            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <Label>Custom Headers</Label>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={addHeader}
                  disabled={formData.headers.length >= 10}
                >
                  Add Header
                </Button>
              </div>
              {formData.headers.map((header, index) => (
                <div key={index} className="flex gap-2">
                  <Input
                    placeholder="Header name"
                    value={header.name}
                    onChange={(e) => updateHeader(index, 'name', e.target.value)}
                    disabled={isSaving}
                  />
                  <Input
                    placeholder="Header value"
                    value={header.value}
                    onChange={(e) => updateHeader(index, 'value', e.target.value)}
                    disabled={isSaving}
                  />
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    onClick={() => removeHeader(index)}
                  >
                    X
                  </Button>
                </div>
              ))}
            </div>
            <div className="flex items-center space-x-2">
              <Switch
                id="is_active"
                checked={formData.is_active}
                onCheckedChange={(checked) => setFormData(prev => ({ ...prev, is_active: checked }))}
                disabled={isSaving}
              />
              <Label htmlFor="is_active">Active</Label>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDialogOpen(false)}>
              Cancel
            </Button>
            <Button onClick={handleSave} disabled={isSaving}>
              {isSaving ? 'Saving...' : 'Save'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Deliveries Dialog */}
      <Dialog open={deliveriesDialogOpen} onOpenChange={setDeliveriesDialogOpen}>
        <DialogContent className="max-w-3xl max-h-[80vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>Webhook Deliveries</DialogTitle>
            <DialogDescription>
              Recent delivery attempts for {selectedWebhook?.name}
            </DialogDescription>
          </DialogHeader>
          <div className="py-4">
            {loadingDeliveries ? (
              <div className="flex items-center justify-center h-32">
                <div className="animate-spin rounded-full h-6 w-6 border-b-2 border-zinc-900 dark:border-white" />
              </div>
            ) : deliveries.length === 0 ? (
              <p className="text-center text-zinc-500 dark:text-zinc-400 py-8">
                No deliveries yet
              </p>
            ) : (
              <div className="space-y-3">
                {deliveries.map((delivery) => (
                  <div
                    key={delivery.id}
                    className="rounded-lg border border-zinc-200 dark:border-zinc-800 p-3"
                  >
                    <div className="flex items-center justify-between mb-2">
                      <div className="flex items-center gap-2">
                        <Badge className={getStatusColor(delivery.status)}>
                          {delivery.status}
                        </Badge>
                        <Badge variant="outline" className="text-xs">
                          {delivery.event_type}
                        </Badge>
                        {delivery.status_code && (
                          <span className="text-sm text-zinc-500">
                            HTTP {delivery.status_code}
                          </span>
                        )}
                      </div>
                      <span className="text-xs text-zinc-400">
                        {formatDate(delivery.created_at)}
                      </span>
                    </div>
                    <div className="text-sm space-y-1">
                      <p className="text-zinc-500 dark:text-zinc-400 font-mono text-xs">
                        Job: {delivery.job_id}
                      </p>
                      <p className="text-zinc-500 dark:text-zinc-400">
                        Response time: {formatDuration(delivery.response_time_ms)}
                      </p>
                      {delivery.attempt_number > 1 && (
                        <p className="text-zinc-500 dark:text-zinc-400">
                          Attempt {delivery.attempt_number} of {delivery.max_attempts}
                        </p>
                      )}
                      {delivery.error_message && (
                        <p className="text-red-500 text-xs">
                          Error: {delivery.error_message}
                        </p>
                      )}
                      {delivery.next_retry_at && (
                        <p className="text-yellow-600 dark:text-yellow-400 text-xs">
                          Next retry: {formatDate(delivery.next_retry_at)}
                        </p>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
          <DialogFooter>
            <Button onClick={() => setDeliveriesDialogOpen(false)}>
              Close
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
