'use client';

import { useState, useEffect, useCallback } from 'react';
import {
  listSchemas,
  listSavedSites,
  createSavedSite,
  updateSavedSite,
  getSavedSite,
  createSchema,
  updateSchema,
  getSchema,
  Schema,
  SavedSite,
  AnalyzeResult,
} from '@/lib/api';
import { normalizeUrl, getHostname } from '@/lib/utils';
import { toast } from 'sonner';

export interface CatalogCrawlOptions {
  followSelector: string;
  followPattern: string;
  maxPages: number;
  maxDepth: number;
  useSitemap: boolean;
}

interface UseCatalogOptions {
  onSchemaLoaded?: (schema: Schema) => void;
  onSiteLoaded?: (site: SavedSite) => void;
}

export function useCatalog(options: UseCatalogOptions = {}) {
  const [schemas, setSchemas] = useState<Schema[]>([]);
  const [savedSites, setSavedSites] = useState<SavedSite[]>([]);
  const [selectedSchemaId, setSelectedSchemaId] = useState<string>('');
  const [selectedSiteId, setSelectedSiteId] = useState<string>('');

  const loadSchemas = useCallback(async () => {
    try {
      const response = await listSchemas();
      setSchemas(response.schemas || []);
    } catch {
      // Schema loading is optional
    }
  }, []);

  const loadSavedSites = useCallback(async () => {
    try {
      const response = await listSavedSites();
      setSavedSites(response.sites || []);
    } catch {
      // Site loading is optional
    }
  }, []);

  // Load initial data
  useEffect(() => {
    loadSchemas();
    loadSavedSites();
  }, [loadSchemas, loadSavedSites]);

  const selectSchema = useCallback(async (schemaId: string) => {
    setSelectedSchemaId(schemaId);
    const selected = schemas.find(s => s.id === schemaId);
    if (selected) {
      options.onSchemaLoaded?.(selected);
    }
    return selected;
  }, [schemas, options]);

  const selectSite = useCallback(async (siteId: string) => {
    setSelectedSiteId(siteId);
    const selected = savedSites.find(s => s.id === siteId);
    if (selected) {
      options.onSiteLoaded?.(selected);
    }
    return selected;
  }, [savedSites, options]);

  const loadSiteById = useCallback(async (siteId: string) => {
    try {
      const site = await getSavedSite(siteId);
      setSelectedSiteId(site.id);
      options.onSiteLoaded?.(site);
      return site;
    } catch {
      toast.error('Failed to load site');
      return null;
    }
  }, [options]);

  const loadSchemaById = useCallback(async (schemaId: string) => {
    try {
      const schema = await getSchema(schemaId);
      setSelectedSchemaId(schema.id);
      options.onSchemaLoaded?.(schema);
      return schema;
    } catch {
      toast.error('Failed to load schema');
      return null;
    }
  }, [options]);

  const saveSchema = useCallback(async (
    name: string,
    description: string,
    schemaYaml: string,
    visibility: 'public' | 'private' = 'private'
  ): Promise<{ success: boolean; existingSchema?: Schema }> => {
    // Check for existing schema with same name
    const existingSchema = schemas.find(s => s.name === name && !s.is_platform);
    if (existingSchema) {
      return { success: false, existingSchema };
    }

    try {
      await createSchema({
        name,
        description,
        schema_yaml: schemaYaml,
        visibility,
      });
      toast.success('Schema saved');
      loadSchemas();
      return { success: true };
    } catch (err) {
      const error = err as { error?: string };
      toast.error(error.error || 'Failed to save schema');
      return { success: false };
    }
  }, [schemas, loadSchemas]);

  const overwriteSchema = useCallback(async (
    schemaId: string,
    name: string,
    description: string,
    schemaYaml: string,
    visibility: 'public' | 'private' = 'private'
  ) => {
    try {
      await updateSchema(schemaId, {
        name,
        description,
        schema_yaml: schemaYaml,
        visibility,
      });
      toast.success('Schema updated');
      loadSchemas();
      return true;
    } catch (err) {
      const error = err as { error?: string };
      toast.error(error.error || 'Failed to update schema');
      return false;
    }
  }, [loadSchemas]);

  const saveSite = useCallback(async (
    url: string,
    analysisResult: AnalyzeResult,
    crawlOptions: CatalogCrawlOptions
  ): Promise<{ success: boolean; existingSite?: SavedSite }> => {
    const normalizedUrl = normalizeUrl(url);
    const hostname = getHostname(normalizedUrl);

    // Check for existing site
    const existingSite = savedSites.find(s => s.url === normalizedUrl);
    if (existingSite) {
      return { success: false, existingSite };
    }

    const cleanAnalysisResult = {
      site_summary: analysisResult.site_summary,
      page_type: analysisResult.page_type,
      detected_elements: analysisResult.detected_elements,
      suggested_schema: analysisResult.suggested_schema,
      follow_patterns: analysisResult.follow_patterns,
      sample_links: analysisResult.sample_links,
      recommended_fetch_mode: analysisResult.recommended_fetch_mode,
    };

    try {
      await createSavedSite({
        url: normalizedUrl,
        name: hostname,
        analysis_result: cleanAnalysisResult,
        fetch_mode: analysisResult.recommended_fetch_mode,
        crawl_options: {
          follow_selector: crawlOptions.followSelector || undefined,
          follow_pattern: crawlOptions.followPattern || undefined,
          max_pages: crawlOptions.maxPages,
          max_depth: crawlOptions.maxDepth,
        },
      });
      toast.success('Site saved');
      loadSavedSites();
      return { success: true };
    } catch (err) {
      const error = err as { error?: string };
      toast.error(error.error || 'Failed to save site');
      return { success: false };
    }
  }, [savedSites, loadSavedSites]);

  const overwriteSite = useCallback(async (
    siteId: string,
    url: string,
    analysisResult: AnalyzeResult,
    crawlOptions: CatalogCrawlOptions
  ) => {
    const normalizedUrl = normalizeUrl(url);
    const hostname = getHostname(normalizedUrl);

    const cleanAnalysisResult = {
      site_summary: analysisResult.site_summary,
      page_type: analysisResult.page_type,
      detected_elements: analysisResult.detected_elements,
      suggested_schema: analysisResult.suggested_schema,
      follow_patterns: analysisResult.follow_patterns,
      sample_links: analysisResult.sample_links,
      recommended_fetch_mode: analysisResult.recommended_fetch_mode,
    };

    try {
      await updateSavedSite(siteId, {
        name: hostname,
        analysis_result: cleanAnalysisResult,
        fetch_mode: analysisResult.recommended_fetch_mode,
        crawl_options: {
          follow_selector: crawlOptions.followSelector || undefined,
          follow_pattern: crawlOptions.followPattern || undefined,
          max_pages: crawlOptions.maxPages,
          max_depth: crawlOptions.maxDepth,
        },
      });
      toast.success('Site updated');
      loadSavedSites();
      return true;
    } catch (err) {
      const error = err as { error?: string };
      toast.error(error.error || 'Failed to update site');
      return false;
    }
  }, [loadSavedSites]);

  return {
    // State
    schemas,
    savedSites,
    selectedSchemaId,
    selectedSiteId,

    // Actions
    loadSchemas,
    loadSavedSites,
    selectSchema,
    selectSite,
    loadSiteById,
    loadSchemaById,
    saveSchema,
    overwriteSchema,
    saveSite,
    overwriteSite,
    setSelectedSchemaId,
    setSelectedSiteId,
  };
}
