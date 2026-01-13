'use client';

import { useEffect, useState } from 'react';
import { useUser } from '@clerk/nextjs';
import {
  listAllSchemas,
  createPlatformSchema,
  updateSchema,
  deleteSchema,
  Schema,
  CreatePlatformSchemaInput,
} from '@/lib/api';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
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
import { Textarea } from '@/components/ui/textarea';
import { toast } from 'sonner';

const CATEGORIES = [
  { value: 'ecommerce', label: 'E-Commerce' },
  { value: 'recipes', label: 'Recipes' },
  { value: 'realestate', label: 'Real Estate' },
  { value: 'jobs', label: 'Job Listings' },
  { value: 'events', label: 'Events' },
  { value: 'news', label: 'News/Articles' },
  { value: 'social', label: 'Social Media' },
  { value: 'other', label: 'Other' },
];

const VISIBILITY_OPTIONS = [
  { value: 'platform', label: 'Platform', description: 'Visible to all users, managed by admins' },
  { value: 'public', label: 'Public', description: 'Visible to all users, owned by creator' },
  { value: 'private', label: 'Private', description: 'Visible only to the owner' },
];

type FilterType = 'all' | 'platform' | 'public' | 'private';

export default function AdminSchemasPage() {
  const { user, isLoaded } = useUser();
  const [schemas, setSchemas] = useState<Schema[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [filter, setFilter] = useState<FilterType>('all');
  const [searchQuery, setSearchQuery] = useState('');

  // Dialog states
  const [showCreateDialog, setShowCreateDialog] = useState(false);
  const [showEditDialog, setShowEditDialog] = useState(false);
  const [showDeleteDialog, setShowDeleteDialog] = useState(false);
  const [selectedSchema, setSelectedSchema] = useState<Schema | null>(null);
  const [isSaving, setIsSaving] = useState(false);

  // Form data
  const [formData, setFormData] = useState<Partial<CreatePlatformSchemaInput>>({
    name: '',
    description: '',
    category: '',
    schema_yaml: '',
    tags: [],
  });
  const [tagsInput, setTagsInput] = useState('');

  const isSuperadmin = user?.publicMetadata?.global_superadmin === true;

  const loadSchemas = async () => {
    try {
      const result = await listAllSchemas();
      setSchemas(result.schemas || []);
    } catch (err) {
      const error = err as { error?: string };
      toast.error(error.error || 'Failed to load schemas');
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    if (isLoaded && isSuperadmin) {
      loadSchemas();
    } else if (isLoaded) {
      setIsLoading(false);
    }
  }, [isLoaded, isSuperadmin]);

  const filteredSchemas = schemas.filter(schema => {
    if (filter !== 'all' && schema.visibility !== filter) {
      return false;
    }
    if (searchQuery) {
      const query = searchQuery.toLowerCase();
      return (
        schema.name.toLowerCase().includes(query) ||
        schema.description?.toLowerCase().includes(query) ||
        schema.category?.toLowerCase().includes(query) ||
        schema.tags?.some(tag => tag.toLowerCase().includes(query))
      );
    }
    return true;
  });

  const resetForm = () => {
    setFormData({
      name: '',
      description: '',
      category: '',
      schema_yaml: '',
      tags: [],
    });
    setTagsInput('');
  };

  const openCreateDialog = () => {
    resetForm();
    setShowCreateDialog(true);
  };

  const openEditDialog = (schema: Schema) => {
    setSelectedSchema(schema);
    setFormData({
      name: schema.name,
      description: schema.description || '',
      category: schema.category || '',
      schema_yaml: schema.schema_yaml,
      tags: schema.tags || [],
    });
    setTagsInput((schema.tags || []).join(', '));
    setShowEditDialog(true);
  };

  const openDeleteDialog = (schema: Schema) => {
    setSelectedSchema(schema);
    setShowDeleteDialog(true);
  };

  const handleCreate = async () => {
    if (!formData.name || !formData.schema_yaml || !formData.category) {
      toast.error('Name, category, and schema YAML are required');
      return;
    }

    setIsSaving(true);
    try {
      const tags = tagsInput
        .split(',')
        .map(t => t.trim())
        .filter(t => t.length > 0);

      await createPlatformSchema({
        name: formData.name,
        description: formData.description,
        category: formData.category,
        schema_yaml: formData.schema_yaml,
        tags,
      });

      toast.success('Platform schema created');
      setShowCreateDialog(false);
      resetForm();
      loadSchemas();
    } catch (err) {
      const error = err as { error?: string };
      toast.error(error.error || 'Failed to create schema');
    } finally {
      setIsSaving(false);
    }
  };

  const handleUpdate = async () => {
    if (!selectedSchema) return;

    setIsSaving(true);
    try {
      const tags = tagsInput
        .split(',')
        .map(t => t.trim())
        .filter(t => t.length > 0);

      await updateSchema(selectedSchema.id, {
        name: formData.name,
        description: formData.description,
        category: formData.category,
        schema_yaml: formData.schema_yaml,
        tags,
      });

      toast.success('Schema updated');
      setShowEditDialog(false);
      setSelectedSchema(null);
      resetForm();
      loadSchemas();
    } catch (err) {
      const error = err as { error?: string };
      toast.error(error.error || 'Failed to update schema');
    } finally {
      setIsSaving(false);
    }
  };

  const handleDelete = async () => {
    if (!selectedSchema) return;

    setIsSaving(true);
    try {
      await deleteSchema(selectedSchema.id);
      toast.success('Schema deleted');
      setShowDeleteDialog(false);
      setSelectedSchema(null);
      loadSchemas();
    } catch (err) {
      const error = err as { error?: string };
      toast.error(error.error || 'Failed to delete schema');
    } finally {
      setIsSaving(false);
    }
  };

  const getVisibilityBadge = (visibility: string) => {
    switch (visibility) {
      case 'platform':
        return <Badge className="bg-blue-600">Platform</Badge>;
      case 'public':
        return <Badge className="bg-green-600">Public</Badge>;
      case 'private':
        return <Badge variant="secondary">Private</Badge>;
      default:
        return <Badge variant="outline">{visibility}</Badge>;
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
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle>Schema Catalog</CardTitle>
              <CardDescription>
                Manage platform schemas and view all user schemas. Platform schemas are available to all users.
              </CardDescription>
            </div>
            <Button onClick={openCreateDialog}>
              <PlusIcon className="h-4 w-4 mr-2" />
              New Platform Schema
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          {/* Filters */}
          <div className="flex gap-4 mb-6">
            <div className="flex gap-2">
              {(['all', 'platform', 'public', 'private'] as const).map((f) => (
                <Button
                  key={f}
                  variant={filter === f ? 'default' : 'outline'}
                  size="sm"
                  onClick={() => setFilter(f)}
                >
                  {f.charAt(0).toUpperCase() + f.slice(1)}
                  {f === 'all' && (
                    <Badge variant="secondary" className="ml-2 text-xs">
                      {schemas.length}
                    </Badge>
                  )}
                  {f !== 'all' && (
                    <Badge variant="secondary" className="ml-2 text-xs">
                      {schemas.filter(s => s.visibility === f).length}
                    </Badge>
                  )}
                </Button>
              ))}
            </div>
            <div className="flex-1">
              <Input
                placeholder="Search schemas..."
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className="max-w-sm"
              />
            </div>
          </div>

          {/* Schema list */}
          {filteredSchemas.length > 0 ? (
            <div className="border rounded-lg overflow-hidden">
              <table className="w-full">
                <thead className="bg-zinc-50 dark:bg-zinc-800">
                  <tr>
                    <th className="px-4 py-3 text-left text-sm font-medium text-zinc-500">Name</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-zinc-500">Category</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-zinc-500">Visibility</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-zinc-500">Usage</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-zinc-500">Created</th>
                    <th className="px-4 py-3 text-right text-sm font-medium text-zinc-500">Actions</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-zinc-200 dark:divide-zinc-700">
                  {filteredSchemas.map((schema) => (
                    <tr key={schema.id} className="hover:bg-zinc-50 dark:hover:bg-zinc-900">
                      <td className="px-4 py-3">
                        <div>
                          <div className="font-medium">{schema.name}</div>
                          {schema.description && (
                            <div className="text-sm text-zinc-500 truncate max-w-xs">
                              {schema.description}
                            </div>
                          )}
                          {schema.tags && schema.tags.length > 0 && (
                            <div className="flex gap-1 mt-1">
                              {schema.tags.slice(0, 3).map((tag) => (
                                <Badge key={tag} variant="outline" className="text-xs">
                                  {tag}
                                </Badge>
                              ))}
                              {schema.tags.length > 3 && (
                                <Badge variant="outline" className="text-xs">
                                  +{schema.tags.length - 3}
                                </Badge>
                              )}
                            </div>
                          )}
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <span className="text-sm">
                          {CATEGORIES.find(c => c.value === schema.category)?.label || schema.category || '-'}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        {getVisibilityBadge(schema.visibility)}
                        {schema.is_platform && (
                          <Badge variant="outline" className="ml-1 text-xs">Admin</Badge>
                        )}
                      </td>
                      <td className="px-4 py-3">
                        <span className="text-sm text-zinc-500">{schema.usage_count}</span>
                      </td>
                      <td className="px-4 py-3">
                        <span className="text-sm text-zinc-500">
                          {new Date(schema.created_at).toLocaleDateString()}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-right">
                        <div className="flex justify-end gap-2">
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => openEditDialog(schema)}
                          >
                            <EditIcon className="h-4 w-4" />
                          </Button>
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => openDeleteDialog(schema)}
                            className="text-red-500 hover:text-red-700"
                          >
                            <TrashIcon className="h-4 w-4" />
                          </Button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : (
            <div className="text-center py-12 text-zinc-500">
              {searchQuery || filter !== 'all'
                ? 'No schemas match your filters'
                : 'No schemas found. Create a platform schema to get started.'}
            </div>
          )}
        </CardContent>
      </Card>

      {/* Create Dialog */}
      <Dialog open={showCreateDialog} onOpenChange={setShowCreateDialog}>
        <DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>Create Platform Schema</DialogTitle>
            <DialogDescription>
              Create a new schema that will be available to all users on the platform.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div>
              <Label htmlFor="name">Name</Label>
              <Input
                id="name"
                value={formData.name || ''}
                onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                placeholder="e.g., Product Listing"
              />
            </div>
            <div>
              <Label htmlFor="description">Description</Label>
              <Input
                id="description"
                value={formData.description || ''}
                onChange={(e) => setFormData({ ...formData, description: e.target.value })}
                placeholder="Brief description of what this schema extracts"
              />
            </div>
            <div>
              <Label htmlFor="category">Category</Label>
              <Select
                value={formData.category || ''}
                onValueChange={(value) => setFormData({ ...formData, category: value })}
              >
                <SelectTrigger>
                  <SelectValue placeholder="Select a category" />
                </SelectTrigger>
                <SelectContent>
                  {CATEGORIES.map((cat) => (
                    <SelectItem key={cat.value} value={cat.value}>
                      {cat.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div>
              <Label htmlFor="tags">Tags (comma-separated)</Label>
              <Input
                id="tags"
                value={tagsInput}
                onChange={(e) => setTagsInput(e.target.value)}
                placeholder="e.g., products, listing, prices"
              />
            </div>
            <div>
              <Label htmlFor="schema_yaml">Schema YAML</Label>
              <Textarea
                id="schema_yaml"
                value={formData.schema_yaml || ''}
                onChange={(e) => setFormData({ ...formData, schema_yaml: e.target.value })}
                placeholder={`name: Product
description: Extract product information
fields:
  - name: title
    type: string
    description: Product title
  - name: price
    type: number
    description: Product price`}
                className="font-mono text-sm min-h-[200px]"
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowCreateDialog(false)}>
              Cancel
            </Button>
            <Button onClick={handleCreate} disabled={isSaving}>
              {isSaving ? 'Creating...' : 'Create Schema'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Edit Dialog */}
      <Dialog open={showEditDialog} onOpenChange={setShowEditDialog}>
        <DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>Edit Schema</DialogTitle>
            <DialogDescription>
              Update the schema details. Changes will be reflected immediately.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div>
              <Label htmlFor="edit-name">Name</Label>
              <Input
                id="edit-name"
                value={formData.name || ''}
                onChange={(e) => setFormData({ ...formData, name: e.target.value })}
              />
            </div>
            <div>
              <Label htmlFor="edit-description">Description</Label>
              <Input
                id="edit-description"
                value={formData.description || ''}
                onChange={(e) => setFormData({ ...formData, description: e.target.value })}
              />
            </div>
            <div>
              <Label htmlFor="edit-category">Category</Label>
              <Select
                value={formData.category || ''}
                onValueChange={(value) => setFormData({ ...formData, category: value })}
              >
                <SelectTrigger>
                  <SelectValue placeholder="Select a category" />
                </SelectTrigger>
                <SelectContent>
                  {CATEGORIES.map((cat) => (
                    <SelectItem key={cat.value} value={cat.value}>
                      {cat.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div>
              <Label htmlFor="edit-tags">Tags (comma-separated)</Label>
              <Input
                id="edit-tags"
                value={tagsInput}
                onChange={(e) => setTagsInput(e.target.value)}
              />
            </div>
            <div>
              <Label htmlFor="edit-schema_yaml">Schema YAML</Label>
              <Textarea
                id="edit-schema_yaml"
                value={formData.schema_yaml || ''}
                onChange={(e) => setFormData({ ...formData, schema_yaml: e.target.value })}
                className="font-mono text-sm min-h-[200px]"
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowEditDialog(false)}>
              Cancel
            </Button>
            <Button onClick={handleUpdate} disabled={isSaving}>
              {isSaving ? 'Saving...' : 'Save Changes'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete Confirmation Dialog */}
      <Dialog open={showDeleteDialog} onOpenChange={setShowDeleteDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete Schema</DialogTitle>
            <DialogDescription>
              Are you sure you want to delete &quot;{selectedSchema?.name}&quot;? This action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowDeleteDialog(false)}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={handleDelete} disabled={isSaving}>
              {isSaving ? 'Deleting...' : 'Delete Schema'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

function PlusIcon({ className }: { className?: string }) {
  return (
    <svg className={className} fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" d="M12 4.5v15m7.5-7.5h-15" />
    </svg>
  );
}

function EditIcon({ className }: { className?: string }) {
  return (
    <svg className={className} fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" d="m16.862 4.487 1.687-1.688a1.875 1.875 0 1 1 2.652 2.652L10.582 16.07a4.5 4.5 0 0 1-1.897 1.13L6 18l.8-2.685a4.5 4.5 0 0 1 1.13-1.897l8.932-8.931Zm0 0L19.5 7.125M18 14v4.75A2.25 2.25 0 0 1 15.75 21H5.25A2.25 2.25 0 0 1 3 18.75V8.25A2.25 2.25 0 0 1 5.25 6H10" />
    </svg>
  );
}

function TrashIcon({ className }: { className?: string }) {
  return (
    <svg className={className} fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" d="m14.74 9-.346 9m-4.788 0L9.26 9m9.968-3.21c.342.052.682.107 1.022.166m-1.022-.165L18.16 19.673a2.25 2.25 0 0 1-2.244 2.077H8.084a2.25 2.25 0 0 1-2.244-2.077L4.772 5.79m14.456 0a48.108 48.108 0 0 0-3.478-.397m-12 .562c.34-.059.68-.114 1.022-.165m0 0a48.11 48.11 0 0 1 3.478-.397m7.5 0v-.916c0-1.18-.91-2.164-2.09-2.201a51.964 51.964 0 0 0-3.32 0c-1.18.037-2.09 1.022-2.09 2.201v.916m7.5 0a48.667 48.667 0 0 0-7.5 0" />
    </svg>
  );
}
