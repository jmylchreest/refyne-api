'use client';

import { useMemo, useCallback, useState, useEffect } from 'react';
import yaml from 'js-yaml';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { CodeEditor } from '@/components/ui/code-editor';
import { FormatToggle, type FormatType } from '@/components/ui/format-toggle';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip';
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
  // Track preferred format for display
  const [displayFormat, setDisplayFormat] = useState<FormatType>('json');

  // Detect if content is a structured schema or freeform prompt
  const contentType = useMemo((): 'schema' | 'prompt' => {
    const trimmed = schema.trim();
    if (!trimmed) return 'schema';

    // Try JSON
    try {
      const parsed = JSON.parse(trimmed);
      if (typeof parsed === 'object' && parsed !== null) {
        return 'schema';
      }
    } catch {
      // Not JSON
    }

    // Try YAML
    try {
      const parsed = yaml.load(trimmed);
      if (typeof parsed === 'object' && parsed !== null) {
        return 'schema';
      }
    } catch {
      // Not YAML
    }

    return 'prompt';
  }, [schema]);

  // Detect current format of schema content
  const currentFormat = useMemo((): FormatType => {
    const trimmed = schema.trim();
    if (!trimmed) return displayFormat;

    try {
      JSON.parse(trimmed);
      return 'json';
    } catch {
      return 'yaml';
    }
  }, [schema, displayFormat]);

  // Convert schema between formats
  const convertSchema = useCallback(
    (targetFormat: FormatType) => {
      const trimmed = schema.trim();
      if (!trimmed || contentType === 'prompt') {
        setDisplayFormat(targetFormat);
        return;
      }

      try {
        // Parse current content
        let parsed: unknown;
        try {
          parsed = JSON.parse(trimmed);
        } catch {
          parsed = yaml.load(trimmed);
        }

        if (typeof parsed !== 'object' || parsed === null) {
          setDisplayFormat(targetFormat);
          return;
        }

        // Convert to target format
        if (targetFormat === 'json') {
          onSchemaChange(JSON.stringify(parsed, null, 2));
        } else {
          onSchemaChange(yaml.dump(parsed, { indent: 2, lineWidth: -1 }));
        }
        setDisplayFormat(targetFormat);
      } catch {
        // If conversion fails, just change the display preference
        setDisplayFormat(targetFormat);
      }
    },
    [schema, contentType, onSchemaChange]
  );

  // Update display format when schema changes from external source (e.g., analyze result)
  useEffect(() => {
    const trimmed = schema.trim();
    if (!trimmed) return;

    try {
      JSON.parse(trimmed);
      setDisplayFormat('json');
    } catch {
      // Keep current format preference for YAML
    }
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

  const getButtonVariant = () => {
    if (extractionMode === 'crawl' && canCrawl) {
      return 'bg-amber-600 hover:bg-amber-700 text-white';
    }
    if (extractionMode === 'sitemap' && canCrawl) {
      return 'bg-emerald-600 hover:bg-emerald-700 text-white';
    }
    return '';
  };

  const editorLanguage = contentType === 'prompt' ? 'text' : currentFormat;

  return (
    <div className="flex-1 rounded-lg border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 shadow-sm flex flex-col min-h-0 overflow-hidden">
      {/* Header */}
      <div className="flex items-center justify-between border-b border-zinc-200 dark:border-zinc-800 px-3 py-2 gap-2 shrink-0">
        {/* Left side: Title, Format Toggle, Schema Select */}
        <div className="flex items-center gap-2 min-w-0">
          <span className="text-sm font-medium shrink-0">Schema</span>

          {contentType === 'schema' ? (
            <FormatToggle
              value={displayFormat}
              onChange={convertSchema}
              disabled={isLoading}
            />
          ) : (
            <Badge
              variant="default"
              className="bg-violet-500 hover:bg-violet-600 text-white text-[10px] px-1.5 py-0"
            >
              PROMPT
            </Badge>
          )}

          <div className="w-px h-4 bg-zinc-200 dark:bg-zinc-700 shrink-0" />

          <Select value={selectedSchemaId} onValueChange={onSchemaSelect}>
            <SelectTrigger className="h-7 w-[140px] text-xs">
              <BookOpen className="h-3 w-3 mr-1.5 shrink-0" />
              <span className="truncate">
                <SelectValue placeholder="Load..." />
              </span>
            </SelectTrigger>
            <SelectContent>
              {schemas.map((s) => (
                <SelectItem key={s.id} value={s.id}>
                  <div className="flex items-center gap-2">
                    {s.name}
                    {s.is_platform && (
                      <Badge variant="secondary" className="text-[10px] px-1">
                        Platform
                      </Badge>
                    )}
                  </div>
                </SelectItem>
              ))}
              {schemas.length === 0 && (
                <SelectItem value="none" disabled>
                  No schemas
                </SelectItem>
              )}
            </SelectContent>
          </Select>
        </div>

        {/* Right side: Actions */}
        <div className="flex items-center gap-1 shrink-0">
          <TooltipProvider delayDuration={300}>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  onClick={onSave}
                  className="h-7 w-7"
                  disabled={isLoading}
                >
                  <Save className="h-3.5 w-3.5" />
                </Button>
              </TooltipTrigger>
              <TooltipContent side="bottom">Save schema</TooltipContent>
            </Tooltip>
          </TooltipProvider>

          <TooltipProvider delayDuration={300}>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  onClick={onCopy}
                  className="h-7 w-7"
                  disabled={isLoading}
                >
                  {isCopied ? (
                    <Check className="h-3.5 w-3.5 text-green-500" />
                  ) : (
                    <Copy className="h-3.5 w-3.5" />
                  )}
                </Button>
              </TooltipTrigger>
              <TooltipContent side="bottom">
                {isCopied ? 'Copied!' : 'Copy schema'}
              </TooltipContent>
            </Tooltip>
          </TooltipProvider>

          <div className="w-px h-4 bg-zinc-200 dark:bg-zinc-700" />

          <Button
            size="sm"
            onClick={onExtract}
            disabled={isLoading || disabled}
            className={cn('h-7 text-xs px-3', getButtonVariant())}
          >
            {isLoading ? (
              <Loader2 className="h-3 w-3 animate-spin mr-1.5" />
            ) : (
              <Play className="h-3 w-3 mr-1.5" />
            )}
            {getButtonLabel()}
          </Button>
        </div>
      </div>

      {/* Editor */}
      <div className="flex-1 min-h-0">
        <CodeEditor
          value={schema}
          onChange={onSchemaChange}
          language={editorLanguage}
          placeholder="Enter a schema (YAML/JSON) or natural language prompt..."
          disabled={isLoading}
          className="h-full"
        />
      </div>
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
