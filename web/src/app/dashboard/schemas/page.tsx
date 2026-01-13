'use client';

import { useState, useEffect } from 'react';
import { useRouter } from 'next/navigation';
import {
  listSchemas,
  updateSchema,
  deleteSchema,
  Schema,
} from '@/lib/api';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Badge } from '@/components/ui/badge';
import { toast } from 'sonner';
import {
  Loader2,
  BookOpen,
  Trash2,
  Pencil,
  ArrowRight,
  Building2,
  User,
  Search,
  Shield,
} from 'lucide-react';

export default function SchemasPage() {
  const router = useRouter();

  // Schemas state
  const [schemas, setSchemas] = useState<Schema[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [searchQuery, setSearchQuery] = useState('');
  const [filterType, setFilterType] = useState<'all' | 'platform' | 'personal'>('all');

  // Edit schema dialog state
  const [showEditSchemaDialog, setShowEditSchemaDialog] = useState(false);
  const [editingSchema, setEditingSchema] = useState<Schema | null>(null);
  const [editSchemaName, setEditSchemaName] = useState('');
  const [editSchemaDescription, setEditSchemaDescription] = useState('');
  const [editSchemaYaml, setEditSchemaYaml] = useState('');

  // Delete confirmation state
  const [showDeleteSchemaDialog, setShowDeleteSchemaDialog] = useState(false);
  const [schemaToDelete, setSchemaToDelete] = useState<Schema | null>(null);

  // Load schemas on mount
  useEffect(() => {
    loadSchemas();
  }, []);

  const loadSchemas = async () => {
    try {
      setIsLoading(true);
      const response = await listSchemas();
      setSchemas(response.schemas || []);
    } catch {
      toast.error('Failed to load schemas');
    } finally {
      setIsLoading(false);
    }
  };

  const handleUseSchema = (schema: Schema) => {
    // Navigate to dashboard with schema data in URL params
    const params = new URLSearchParams();
    params.set('schemaId', schema.id);
    router.push(`/dashboard?${params.toString()}`);
  };

  const handleEditSchema = (schema: Schema) => {
    setEditingSchema(schema);
    setEditSchemaName(schema.name);
    setEditSchemaDescription(schema.description || '');
    setEditSchemaYaml(schema.schema_yaml);
    setShowEditSchemaDialog(true);
  };

  const handleSaveEditSchema = async () => {
    if (!editingSchema) return;

    try {
      await updateSchema(editingSchema.id, {
        name: editSchemaName,
        description: editSchemaDescription,
        schema_yaml: editSchemaYaml,
        visibility: editingSchema.visibility as 'public' | 'private',
      });
      toast.success('Schema updated');
      loadSchemas();
      setShowEditSchemaDialog(false);
      setEditingSchema(null);
    } catch (err) {
      const error = err as { error?: string };
      toast.error(error.error || 'Failed to update schema');
    }
  };

  const handleDeleteSchema = async () => {
    if (!schemaToDelete) return;

    try {
      await deleteSchema(schemaToDelete.id);
      toast.success('Schema deleted');
      loadSchemas();
    } catch (err) {
      const error = err as { error?: string };
      toast.error(error.error || 'Failed to delete schema');
    } finally {
      setShowDeleteSchemaDialog(false);
      setSchemaToDelete(null);
    }
  };

  // Check if user can modify a schema (not platform schemas)
  const canModifySchema = (schema: Schema) => !schema.is_platform;

  // Filter schemas based on search query and filter type
  const filteredSchemas = schemas.filter((schema) => {
    const query = searchQuery.toLowerCase();
    const matchesSearch =
      schema.name.toLowerCase().includes(query) ||
      schema.description?.toLowerCase().includes(query) ||
      schema.category?.toLowerCase().includes(query);

    const matchesFilter =
      filterType === 'all' ||
      (filterType === 'platform' && schema.is_platform) ||
      (filterType === 'personal' && !schema.is_platform);

    return matchesSearch && matchesFilter;
  });

  // Count schemas by type
  const platformCount = schemas.filter((s) => s.is_platform).length;
  const personalCount = schemas.filter((s) => !s.is_platform).length;

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-64">
        <Loader2 className="h-8 w-8 animate-spin text-zinc-400" />
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full">
      <div className="mb-6">
        <h1 className="text-3xl font-bold tracking-tight">Schemas</h1>
        <p className="mt-2 text-zinc-600 dark:text-zinc-400">
          Browse and manage extraction schemas for structured data extraction.
        </p>
      </div>

      {/* Search and Filters */}
      <div className="mb-4 flex flex-col sm:flex-row items-start sm:items-center gap-4">
        <div className="relative flex-1 max-w-md">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-zinc-400" />
          <Input
            placeholder="Search schemas..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="pl-10"
          />
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant={filterType === 'all' ? 'secondary' : 'ghost'}
            size="sm"
            onClick={() => setFilterType('all')}
          >
            All ({schemas.length})
          </Button>
          <Button
            variant={filterType === 'platform' ? 'secondary' : 'ghost'}
            size="sm"
            onClick={() => setFilterType('platform')}
          >
            <Shield className="h-3 w-3 mr-1" />
            Platform ({platformCount})
          </Button>
          <Button
            variant={filterType === 'personal' ? 'secondary' : 'ghost'}
            size="sm"
            onClick={() => setFilterType('personal')}
          >
            <User className="h-3 w-3 mr-1" />
            Personal ({personalCount})
          </Button>
        </div>
      </div>

      {/* Schemas List */}
      <div className="rounded-lg border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 shadow-sm">
        {filteredSchemas.length === 0 ? (
          <div className="px-4 py-12 text-center text-zinc-400">
            <BookOpen className="h-12 w-12 mx-auto mb-3 opacity-50" />
            {searchQuery || filterType !== 'all' ? (
              <>
                <p className="text-lg font-medium">No schemas found</p>
                <p className="text-sm mt-1">
                  Try adjusting your search or filter
                </p>
              </>
            ) : (
              <>
                <p className="text-lg font-medium">No schemas available</p>
                <p className="text-sm mt-1">
                  Save a schema from the Extract page to see it here
                </p>
                <Button
                  variant="outline"
                  className="mt-4"
                  onClick={() => router.push('/dashboard')}
                >
                  Go to Extract
                </Button>
              </>
            )}
          </div>
        ) : (
          <div className="divide-y divide-zinc-200 dark:divide-zinc-800">
            {filteredSchemas.map((schema) => (
              <div
                key={schema.id}
                className="px-4 py-4 flex items-center justify-between gap-4 hover:bg-zinc-50 dark:hover:bg-zinc-800/50 transition-colors"
              >
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2 mb-1">
                    <BookOpen className="h-4 w-4 text-zinc-400 shrink-0" />
                    <span className="font-medium text-sm truncate">
                      {schema.name}
                    </span>
                    {schema.is_platform ? (
                      <Badge variant="secondary" className="text-xs shrink-0">
                        <Shield className="h-3 w-3 mr-1" />
                        Platform
                      </Badge>
                    ) : schema.organization_id ? (
                      <Badge variant="secondary" className="text-xs shrink-0">
                        <Building2 className="h-3 w-3 mr-1" />
                        Organization
                      </Badge>
                    ) : (
                      <Badge variant="outline" className="text-xs shrink-0">
                        <User className="h-3 w-3 mr-1" />
                        Personal
                      </Badge>
                    )}
                    {schema.category && (
                      <Badge variant="outline" className="text-xs shrink-0">
                        {schema.category}
                      </Badge>
                    )}
                  </div>
                  {schema.description && (
                    <p className="text-xs text-zinc-500 truncate">
                      {schema.description}
                    </p>
                  )}
                  {schema.tags && schema.tags.length > 0 && (
                    <div className="flex items-center gap-1 mt-1">
                      {schema.tags.slice(0, 3).map((tag, i) => (
                        <Badge
                          key={i}
                          variant="outline"
                          className="text-xs font-normal"
                        >
                          {tag}
                        </Badge>
                      ))}
                      {schema.tags.length > 3 && (
                        <span className="text-xs text-zinc-400">
                          +{schema.tags.length - 3} more
                        </span>
                      )}
                    </div>
                  )}
                </div>
                <div className="flex items-center gap-1 shrink-0">
                  <Button
                    variant="default"
                    size="sm"
                    onClick={() => handleUseSchema(schema)}
                    className="h-8"
                  >
                    <ArrowRight className="h-4 w-4 mr-1" />
                    Use
                  </Button>
                  {canModifySchema(schema) && (
                    <>
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => handleEditSchema(schema)}
                        className="h-8 px-2"
                        title="Edit schema"
                      >
                        <Pencil className="h-4 w-4" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => {
                          setSchemaToDelete(schema);
                          setShowDeleteSchemaDialog(true);
                        }}
                        className="h-8 px-2 text-red-500 hover:text-red-600 hover:bg-red-50 dark:hover:bg-red-950/50"
                        title="Delete schema"
                      >
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    </>
                  )}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Edit Schema Dialog */}
      <Dialog open={showEditSchemaDialog} onOpenChange={setShowEditSchemaDialog}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>Edit Schema</DialogTitle>
            <DialogDescription>
              Update this schema&apos;s details and content.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="edit-schema-name">Name</Label>
                <Input
                  id="edit-schema-name"
                  value={editSchemaName}
                  onChange={(e) => setEditSchemaName(e.target.value)}
                  placeholder="Schema name"
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="edit-schema-description">Description</Label>
                <Input
                  id="edit-schema-description"
                  value={editSchemaDescription}
                  onChange={(e) => setEditSchemaDescription(e.target.value)}
                  placeholder="Optional description"
                />
              </div>
            </div>
            <div className="space-y-2">
              <Label htmlFor="edit-schema-yaml">Schema (YAML)</Label>
              <Textarea
                id="edit-schema-yaml"
                value={editSchemaYaml}
                onChange={(e) => setEditSchemaYaml(e.target.value)}
                className="font-mono text-sm min-h-[200px]"
                placeholder="Enter schema YAML..."
              />
            </div>
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setShowEditSchemaDialog(false)}
            >
              Cancel
            </Button>
            <Button onClick={handleSaveEditSchema}>Save Changes</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete Schema Confirmation Dialog */}
      <Dialog
        open={showDeleteSchemaDialog}
        onOpenChange={setShowDeleteSchemaDialog}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete Schema</DialogTitle>
            <DialogDescription>
              Are you sure you want to delete this schema? This action cannot be
              undone.
            </DialogDescription>
          </DialogHeader>
          <div className="py-4">
            <div className="rounded-md bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-900 p-3">
              <p className="text-sm font-medium text-red-700 dark:text-red-300">
                {schemaToDelete?.name}
              </p>
              {schemaToDelete?.description && (
                <p className="text-xs text-red-600 dark:text-red-400 mt-1">
                  {schemaToDelete.description}
                </p>
              )}
            </div>
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setShowDeleteSchemaDialog(false)}
            >
              Cancel
            </Button>
            <Button variant="destructive" onClick={handleDeleteSchema}>
              Delete Schema
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
