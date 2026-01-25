'use client';

import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Loader2, Sparkles, Globe } from 'lucide-react';
import type { SavedSite } from '@/lib/api';

interface UrlInputSectionProps {
  url: string;
  onUrlChange: (url: string) => void;
  savedSites: SavedSite[];
  selectedSiteId: string;
  onSiteSelect: (siteId: string) => void;
  onAnalyze: () => void;
  isAnalyzing: boolean;
  isLoading: boolean;
}

export function UrlInputSection({
  url,
  onUrlChange,
  savedSites,
  selectedSiteId,
  onSiteSelect,
  onAnalyze,
  isAnalyzing,
  isLoading,
}: UrlInputSectionProps) {
  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !isLoading && !isAnalyzing && url) {
      e.preventDefault();
      onAnalyze();
    }
  };

  return (
    <div className="flex flex-col sm:flex-row gap-3">
      <div className="flex-1 flex gap-2">
        <Input
          type="url"
          placeholder="https://example.com/product"
          value={url}
          onChange={(e) => onUrlChange(e.target.value)}
          onKeyDown={handleKeyDown}
          disabled={isLoading || isAnalyzing}
          className="flex-1 font-mono text-sm"
        />
        <Select value={selectedSiteId} onValueChange={onSiteSelect}>
          <SelectTrigger className="w-[140px]">
            <Globe className="h-4 w-4 mr-2 shrink-0" />
            <span className="truncate">
              <SelectValue placeholder="Sites" />
            </span>
          </SelectTrigger>
          <SelectContent>
            {savedSites.map((s) => (
              <SelectItem key={s.id} value={s.id}>
                {s.name || s.domain}
              </SelectItem>
            ))}
            {savedSites.length === 0 && (
              <SelectItem value="none" disabled>
                No saved sites
              </SelectItem>
            )}
          </SelectContent>
        </Select>
        <Button
          variant="secondary"
          onClick={onAnalyze}
          disabled={isLoading || isAnalyzing || !url}
          className="shrink-0"
        >
          {isAnalyzing ? (
            <Loader2 className="h-4 w-4 animate-spin" />
          ) : (
            <Sparkles className="h-4 w-4" />
          )}
          <span className="ml-2 hidden sm:inline">Analyze</span>
        </Button>
      </div>
    </div>
  );
}
