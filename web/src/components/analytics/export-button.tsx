'use client';

import { useState } from 'react';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Download, Loader2 } from 'lucide-react';
import type { AnalyticsJob, AnalyticsUserSummary, TrendDataPoint } from '@/lib/api';

type ExportData =
  | { type: 'jobs'; data: AnalyticsJob[] }
  | { type: 'users'; data: AnalyticsUserSummary[] }
  | { type: 'trends'; data: TrendDataPoint[] };

interface ExportButtonProps {
  getData: () => Promise<ExportData>;
  filename?: string;
}

export function ExportButton({ getData, filename = 'analytics' }: ExportButtonProps) {
  const [isExporting, setIsExporting] = useState(false);

  const handleExport = async (format: 'csv' | 'json') => {
    setIsExporting(true);
    try {
      const exportData = await getData();

      if (format === 'csv') {
        exportToCsv(exportData, filename);
      } else {
        exportToJson(exportData, filename);
      }
    } catch (error) {
      console.error('Export failed:', error);
    } finally {
      setIsExporting(false);
    }
  };

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="outline" disabled={isExporting}>
          {isExporting ? (
            <Loader2 className="h-4 w-4 mr-2 animate-spin" />
          ) : (
            <Download className="h-4 w-4 mr-2" />
          )}
          Export
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem onClick={() => handleExport('csv')}>
          Export as CSV
        </DropdownMenuItem>
        <DropdownMenuItem onClick={() => handleExport('json')}>
          Export as JSON
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function exportToCsv(exportData: ExportData, filename: string) {
  let csvContent = '';
  const data = exportData.data;

  if (data.length === 0) {
    csvContent = 'No data to export';
  } else {
    // Get headers from first object
    const headers = Object.keys(data[0]);
    csvContent = headers.join(',') + '\n';

    // Add rows
    data.forEach((row) => {
      const values = headers.map((header) => {
        const value = (row as unknown as Record<string, unknown>)[header];
        // Handle values that might contain commas
        if (typeof value === 'string' && (value.includes(',') || value.includes('"'))) {
          return `"${value.replace(/"/g, '""')}"`;
        }
        return value ?? '';
      });
      csvContent += values.join(',') + '\n';
    });
  }

  downloadFile(csvContent, `${filename}-${exportData.type}.csv`, 'text/csv');
}

function exportToJson(exportData: ExportData, filename: string) {
  const jsonContent = JSON.stringify(exportData.data, null, 2);
  downloadFile(jsonContent, `${filename}-${exportData.type}.json`, 'application/json');
}

function downloadFile(content: string, filename: string, mimeType: string) {
  const blob = new Blob([content], { type: mimeType });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
  URL.revokeObjectURL(url);
}
