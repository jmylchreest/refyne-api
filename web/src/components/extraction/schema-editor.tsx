'use client';

import { useMemo } from 'react';
import yaml from 'js-yaml';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Textarea } from '@/components/ui/textarea';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Loader2, Play, Save, BookOpen, Copy, Check } from 'lucide-react';
import { cn } from '@/lib/utils';
import type { Schema } from '@/lib/api';
import type { ExtractionMode } from '@/components/crawl-mode-section';

export type InputFormat = 'JSON' | 'YAML' | 'PROMPT';

interface SchemaEditorProps {
  schema: string;
  onSchemaChange: (schema: string) => void;
  schemas: Schema[];
  selectedSchemaId: string;
  onSchemaSelect: (schemaId: string) => void;
  onSave: () => void;
  onCopy: () => void;
  isCopied: boolean;
  onExtract: (e: React.FormEvent) => void;
  isLoading: boolean;
  disabled: boolean;
  extractionMode: ExtractionMode;
  canCrawl: boolean;
}

export function SchemaEditor({
  schema,
  onSchemaChange,
  schemas,
  selectedSchemaId,
  onSchemaSelect,
  onSave,
  onCopy,
  isCopied,
  onExtract,
  isLoading,
  disabled,
  extractionMode,
  canCrawl,
}: SchemaEditorProps) {
  // Detect input format for visual indicator
  const detectedInputFormat = useMemo((): InputFormat => {
    const trimmed = schema.trim();
    if (!trimmed) return 'YAML'; // Default when empty

    // Try JSON first
    try {
      const parsed = JSON.parse(trimmed);
      if (typeof parsed === 'object' && parsed !== null) {
        return 'JSON';
      }
    } catch {
      // Not JSON
    }

    // Try YAML
    try {
      const parsed = yaml.load(trimmed);
      if (typeof parsed === 'object' && parsed !== null) {
        return 'YAML';
      }
    } catch {
      // Not valid YAML object either
    }

    // If neither parsed as an object, it's a freeform prompt
    return 'PROMPT';
  }, [schema]);

  const getButtonLabel = () => {
    if (extractionMode === 'crawl' && canCrawl) {
      return 'Crawl & Extract';
    }
    if (extractionMode === 'sitemap' && canCrawl) {
      return 'Sitemap Extract';
    }
    return 'Extract';
  };

  const getButtonClass = () => {
    if (extractionMode === 'crawl' && canCrawl) {
      return 'bg-amber-600 hover:bg-amber-700';
    }
    if (extractionMode === 'sitemap' && canCrawl) {
      return 'bg-emerald-600 hover:bg-emerald-700';
    }
    return '';
  };

  return (
    <div className="flex-1 rounded-lg border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 shadow-sm flex flex-col min-h-0">
      {/* Schema Header */}
      <div className="flex items-center justify-between border-b border-zinc-200 dark:border-zinc-800 px-4 py-2">
        <div className="flex items-center gap-3">
          <span className="text-sm font-medium">Schema</span>
          <Badge
            variant={detectedInputFormat === 'PROMPT' ? 'default' : 'secondary'}
            className={`text-[10px] px-1.5 py-0 ${
              detectedInputFormat === 'PROMPT'
                ? 'bg-violet-500 hover:bg-violet-600 text-white'
                : 'bg-zinc-100 dark:bg-zinc-800 text-zinc-600 dark:text-zinc-400'
            }`}
          >
            {detectedInputFormat}
          </Badge>
          <Select value={selectedSchemaId} onValueChange={onSchemaSelect}>
            <SelectTrigger className="h-8 w-[160px] text-xs">
              <BookOpen className="h-3 w-3 mr-1.5 shrink-0" />
              <span className="truncate">
                <SelectValue placeholder="Load schema..." />
              </span>
            </SelectTrigger>
            <SelectContent>
              {schemas.map((s) => (
                <SelectItem key={s.id} value={s.id}>
                  <div className="flex items-center gap-2">
                    {s.name}
                    {s.is_platform && (
                      <Badge variant="secondary" className="text-xs">
                        Platform
                      </Badge>
                    )}
                  </div>
                </SelectItem>
              ))}
              {schemas.length === 0 && (
                <SelectItem value="none" disabled>
                  No schemas available
                </SelectItem>
              )}
            </SelectContent>
          </Select>
          <Button
            variant="ghost"
            size="sm"
            onClick={onSave}
            className="h-8 text-xs"
          >
            <Save className="h-3 w-3 mr-1" />
            Save
          </Button>
          <Button
            variant="ghost"
            size="icon"
            onClick={onCopy}
            className="h-8 w-8"
            title="Copy schema"
          >
            {isCopied ? (
              <Check className="h-3 w-3 text-green-500" />
            ) : (
              <Copy className="h-3 w-3" />
            )}
          </Button>
        </div>
        <Button
          size="sm"
          onClick={onExtract}
          disabled={isLoading || disabled}
          className={cn('h-8', getButtonClass())}
        >
          {isLoading ? (
            <Loader2 className="h-3 w-3 animate-spin mr-1" />
          ) : (
            <Play className="h-3 w-3 mr-1" />
          )}
          {getButtonLabel()}
        </Button>
      </div>

      {/* Schema Editor */}
      <form onSubmit={onExtract} className="flex-1 min-h-0 flex flex-col">
        <Textarea
          placeholder="Enter a schema (YAML/JSON) or natural language prompt..."
          value={schema}
          onChange={(e) => onSchemaChange(e.target.value)}
          className="flex-1 font-mono text-sm border-0 rounded-none resize-none focus-visible:ring-0 focus-visible:ring-offset-0"
          disabled={isLoading}
        />
      </form>
    </div>
  );
}

// Convert YAML schema to JSON, or pass through freeform prompts as-is
export function yamlToJson(yamlStr: string): object | string {
  const trimmed = yamlStr.trim();

  // Try to parse as YAML/JSON
  const parsed = yaml.load(trimmed);

  // If it's an object with schema-like structure, return it as parsed object
  if (typeof parsed === 'object' && parsed !== null) {
    return parsed as object;
  }

  // Otherwise, it's a freeform prompt - return the original string
  return trimmed;
}
