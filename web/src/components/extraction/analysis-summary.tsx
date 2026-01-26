'use client';

import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { Sparkles, Save, ChevronDown, ChevronUp, Globe } from 'lucide-react';
import type { AnalyzeResult } from '@/lib/api';

export type FetchMode = 'auto' | 'static' | 'dynamic';

interface AnalysisSummaryProps {
  analysisResult: AnalyzeResult;
  fetchMode: FetchMode;
  onFetchModeChange: (mode: FetchMode) => void;
  canDynamic: boolean;
  expanded: boolean;
  onExpandedChange: (expanded: boolean) => void;
  onSave: () => void;
}

export function AnalysisSummary({
  analysisResult,
  fetchMode,
  onFetchModeChange,
  canDynamic,
  expanded,
  onExpandedChange,
  onSave,
}: AnalysisSummaryProps) {
  // Check if browser rendering was used for this analysis
  const usedBrowserRendering = analysisResult.fetch_mode_used === 'dynamic';

  return (
    <div className="border-t border-zinc-200 dark:border-zinc-800 px-4 py-3 bg-zinc-50 dark:bg-zinc-900/50">
      <div className="flex items-start justify-between gap-4">
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 mb-1">
            <Sparkles className="h-4 w-4 text-amber-500 shrink-0" />
            <span className="text-sm font-medium truncate">
              {analysisResult.page_type} page
            </span>
            {usedBrowserRendering && (
              <Tooltip>
                <TooltipTrigger asChild>
                  <Badge variant="outline" className="text-[10px] px-1.5 py-0 h-5 gap-1 border-blue-300 text-blue-600 dark:border-blue-600 dark:text-blue-400">
                    <Globe className="h-3 w-3" />
                    Browser
                  </Badge>
                </TooltipTrigger>
                <TooltipContent side="bottom" className="text-xs">
                  <p>This page required browser rendering to fetch content.</p>
                  <p className="text-zinc-400">Bot protection was detected and bypassed.</p>
                </TooltipContent>
              </Tooltip>
            )}
            <span className="text-xs text-zinc-500 shrink-0">
              {analysisResult.detected_elements.length} fields detected
            </span>
          </div>
          <p className="text-sm text-zinc-600 dark:text-zinc-400 line-clamp-1">
            {analysisResult.site_summary}
          </p>
          {expanded && (
            <div className="mt-3 flex flex-wrap gap-1.5">
              {analysisResult.detected_elements.map((elem, i) => (
                <Badge
                  key={i}
                  variant="secondary"
                  className="text-xs font-normal"
                >
                  {elem.name}
                  <span className="text-zinc-400 ml-1">({elem.type})</span>
                </Badge>
              ))}
            </div>
          )}
        </div>
        <div className="flex items-center gap-2 shrink-0">
          <Button
            variant="ghost"
            size="sm"
            onClick={() => onExpandedChange(!expanded)}
            className="h-8 w-8 p-0"
          >
            {expanded ? (
              <ChevronUp className="h-4 w-4" />
            ) : (
              <ChevronDown className="h-4 w-4" />
            )}
          </Button>
          <Tooltip>
            <TooltipTrigger asChild>
              <div>
                <Select
                  value={fetchMode}
                  onValueChange={(v) => onFetchModeChange(v as FetchMode)}
                >
                  <SelectTrigger className="h-8 w-[90px] text-xs">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="auto">Auto</SelectItem>
                    <SelectItem value="static">Static</SelectItem>
                    <SelectItem value="dynamic" disabled={!canDynamic}>
                      Dynamic {!canDynamic && '(Pro)'}
                    </SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </TooltipTrigger>
            <TooltipContent side="bottom" className="max-w-xs text-xs">
              <p className="font-medium mb-1">Fetch Mode</p>
              <p>
                <strong>Auto:</strong> Try static first, retry with browser if
                protected
              </p>
              <p>
                <strong>Static:</strong> Fast HTTP fetch (most sites)
              </p>
              <p>
                <strong>Dynamic:</strong> Browser rendering (anti-bot
                protection)
              </p>
              {analysisResult.recommended_fetch_mode && analysisResult.recommended_fetch_mode !== fetchMode && (
                <p className="mt-1 text-amber-400">
                  Recommended: {analysisResult.recommended_fetch_mode}
                </p>
              )}
            </TooltipContent>
          </Tooltip>
          <Button variant="outline" size="sm" onClick={onSave}>
            <Save className="h-4 w-4 mr-1" />
            Save
          </Button>
        </div>
      </div>
    </div>
  );
}
