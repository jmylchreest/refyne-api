'use client';

import { useState, useEffect } from 'react';
import { useRouter } from 'next/navigation';
import {
  listSavedSites,
  updateSavedSite,
  deleteSavedSite,
  SavedSite,
} from '@/lib/api';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
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
  Globe,
  Trash2,
  Pencil,
  ArrowRight,
  Building2,
  User,
  Search,
} from 'lucide-react';

export default function SitesPage() {
  const router = useRouter();

  // Sites state
  const [savedSites, setSavedSites] = useState<SavedSite[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [searchQuery, setSearchQuery] = useState('');

  // Edit site dialog state
  const [showEditSiteDialog, setShowEditSiteDialog] = useState(false);
  const [editingSite, setEditingSite] = useState<SavedSite | null>(null);
  const [editSiteName, setEditSiteName] = useState('');

  // Delete confirmation state
  const [showDeleteSiteDialog, setShowDeleteSiteDialog] = useState(false);
  const [siteToDelete, setSiteToDelete] = useState<SavedSite | null>(null);

  // Load saved sites on mount
  useEffect(() => {
    loadSavedSites();
  }, []);

  const loadSavedSites = async () => {
    try {
      setIsLoading(true);
      const response = await listSavedSites();
      setSavedSites(response.sites || []);
    } catch {
      toast.error('Failed to load saved sites');
    } finally {
      setIsLoading(false);
    }
  };

  const handleUseSite = (site: SavedSite) => {
    // Navigate to dashboard with site data in URL params
    const params = new URLSearchParams();
    params.set('siteId', site.id);
    router.push(`/dashboard?${params.toString()}`);
  };

  const handleEditSite = (site: SavedSite) => {
    setEditingSite(site);
    setEditSiteName(site.name || '');
    setShowEditSiteDialog(true);
  };

  const handleSaveEditSite = async () => {
    if (!editingSite) return;

    try {
      await updateSavedSite(editingSite.id, {
        name: editSiteName,
        fetch_mode: editingSite.fetch_mode,
      });
      toast.success('Site updated');
      loadSavedSites();
      setShowEditSiteDialog(false);
      setEditingSite(null);
    } catch (err) {
      const error = err as { error?: string };
      toast.error(error.error || 'Failed to update site');
    }
  };

  const handleDeleteSite = async () => {
    if (!siteToDelete) return;

    try {
      await deleteSavedSite(siteToDelete.id);
      toast.success('Site deleted');
      loadSavedSites();
    } catch (err) {
      const error = err as { error?: string };
      toast.error(error.error || 'Failed to delete site');
    } finally {
      setShowDeleteSiteDialog(false);
      setSiteToDelete(null);
    }
  };

  // Filter sites based on search query
  const filteredSites = savedSites.filter((site) => {
    const query = searchQuery.toLowerCase();
    return (
      site.name?.toLowerCase().includes(query) ||
      site.url.toLowerCase().includes(query) ||
      site.domain.toLowerCase().includes(query)
    );
  });

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
        <h1 className="text-3xl font-bold tracking-tight">Sites</h1>
        <p className="mt-2 text-zinc-600 dark:text-zinc-400">
          Manage your saved websites and their analysis configurations.
        </p>
      </div>

      {/* Search and Actions */}
      <div className="mb-4 flex items-center gap-4">
        <div className="relative flex-1 max-w-md">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-zinc-400" />
          <Input
            placeholder="Search sites..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="pl-10"
          />
        </div>
        <Badge variant="secondary" className="text-sm">
          {filteredSites.length} {filteredSites.length === 1 ? 'site' : 'sites'}
        </Badge>
      </div>

      {/* Sites List */}
      <div className="rounded-lg border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 shadow-sm">
        {filteredSites.length === 0 ? (
          <div className="px-4 py-12 text-center text-zinc-400">
            <Globe className="h-12 w-12 mx-auto mb-3 opacity-50" />
            {searchQuery ? (
              <>
                <p className="text-lg font-medium">No sites found</p>
                <p className="text-sm mt-1">Try adjusting your search query</p>
              </>
            ) : (
              <>
                <p className="text-lg font-medium">No saved sites yet</p>
                <p className="text-sm mt-1">
                  Analyze a URL from the Extract page to save it here
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
            {filteredSites.map((site) => (
              <div
                key={site.id}
                className="px-4 py-4 flex items-center justify-between gap-4 hover:bg-zinc-50 dark:hover:bg-zinc-800/50 transition-colors"
              >
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2 mb-1">
                    <Globe className="h-4 w-4 text-zinc-400 shrink-0" />
                    <span className="font-medium text-sm truncate">
                      {site.name || site.domain}
                    </span>
                    {site.organization_id ? (
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
                  </div>
                  <p className="text-xs text-zinc-500 font-mono truncate">
                    {site.url}
                  </p>
                  {site.analysis_result && (
                    <div className="flex items-center gap-2 mt-1">
                      {site.analysis_result.page_type && (
                        <Badge variant="outline" className="text-xs">
                          {site.analysis_result.page_type}
                        </Badge>
                      )}
                      <span className="text-xs text-zinc-400">
                        {site.fetch_mode} fetch
                      </span>
                      {site.analysis_result.detected_elements && (
                        <span className="text-xs text-zinc-400">
                          {site.analysis_result.detected_elements.length} fields
                        </span>
                      )}
                    </div>
                  )}
                </div>
                <div className="flex items-center gap-1 shrink-0">
                  <Button
                    variant="default"
                    size="sm"
                    onClick={() => handleUseSite(site)}
                    className="h-8"
                  >
                    <ArrowRight className="h-4 w-4 mr-1" />
                    Extract
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => handleEditSite(site)}
                    className="h-8 px-2"
                    title="Edit site"
                  >
                    <Pencil className="h-4 w-4" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => {
                      setSiteToDelete(site);
                      setShowDeleteSiteDialog(true);
                    }}
                    className="h-8 px-2 text-red-500 hover:text-red-600 hover:bg-red-50 dark:hover:bg-red-950/50"
                    title="Delete site"
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Edit Site Dialog */}
      <Dialog open={showEditSiteDialog} onOpenChange={setShowEditSiteDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Edit Site</DialogTitle>
            <DialogDescription>
              Update the name for this saved site.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <div className="space-y-2">
              <Label htmlFor="edit-site-name">Name</Label>
              <Input
                id="edit-site-name"
                value={editSiteName}
                onChange={(e) => setEditSiteName(e.target.value)}
                placeholder="Site name"
              />
            </div>
            <div className="space-y-2">
              <Label className="text-zinc-500">URL</Label>
              <p className="text-sm font-mono text-zinc-400 truncate">
                {editingSite?.url}
              </p>
            </div>
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setShowEditSiteDialog(false)}
            >
              Cancel
            </Button>
            <Button onClick={handleSaveEditSite}>Save Changes</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete Site Confirmation Dialog */}
      <Dialog open={showDeleteSiteDialog} onOpenChange={setShowDeleteSiteDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete Site</DialogTitle>
            <DialogDescription>
              Are you sure you want to delete this saved site? This action
              cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <div className="py-4">
            <div className="rounded-md bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-900 p-3">
              <p className="text-sm font-medium text-red-700 dark:text-red-300">
                {siteToDelete?.name || siteToDelete?.domain}
              </p>
              <p className="text-xs text-red-600 dark:text-red-400 font-mono mt-1 truncate">
                {siteToDelete?.url}
              </p>
            </div>
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setShowDeleteSiteDialog(false)}
            >
              Cancel
            </Button>
            <Button variant="destructive" onClick={handleDeleteSite}>
              Delete Site
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
