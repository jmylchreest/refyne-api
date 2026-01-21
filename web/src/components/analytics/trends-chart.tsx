'use client';

import { useMemo } from 'react';
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  Legend,
} from 'recharts';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import type { TrendDataPoint } from '@/lib/api';

interface TrendsChartProps {
  data: TrendDataPoint[];
  title?: string;
}

export function TrendsChart({ data, title = 'Jobs & Costs Over Time' }: TrendsChartProps) {
  const chartData = useMemo(() => {
    return data.map((point) => ({
      ...point,
      date: formatDate(point.date),
      cost_usd: Number(point.cost_usd.toFixed(4)),
    }));
  }, [data]);

  if (data.length === 0) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-lg">{title}</CardTitle>
        </CardHeader>
        <CardContent className="h-64 flex items-center justify-center">
          <p className="text-zinc-500 dark:text-zinc-400">No data available</p>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-lg">{title}</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="h-64">
          <ResponsiveContainer width="100%" height="100%">
            <LineChart
              data={chartData}
              margin={{ top: 5, right: 30, left: 0, bottom: 5 }}
            >
              <CartesianGrid
                strokeDasharray="3 3"
                className="stroke-zinc-200 dark:stroke-zinc-700"
              />
              <XAxis
                dataKey="date"
                tick={{ fontSize: 12 }}
                className="text-zinc-600 dark:text-zinc-400"
              />
              <YAxis
                yAxisId="left"
                tick={{ fontSize: 12 }}
                className="text-zinc-600 dark:text-zinc-400"
              />
              <YAxis
                yAxisId="right"
                orientation="right"
                tick={{ fontSize: 12 }}
                tickFormatter={(v) => `$${v}`}
                className="text-zinc-600 dark:text-zinc-400"
              />
              <Tooltip
                contentStyle={{
                  backgroundColor: 'var(--tooltip-bg, #fff)',
                  borderColor: 'var(--tooltip-border, #e4e4e7)',
                  borderRadius: '6px',
                }}
                labelStyle={{
                  fontWeight: 600,
                }}
                formatter={(value, name) => {
                  if (name === 'cost_usd') return [`$${Number(value).toFixed(4)}`, 'Cost'];
                  if (name === 'job_count') return [value, 'Jobs'];
                  if (name === 'error_count') return [value, 'Errors'];
                  return [value, String(name)];
                }}
              />
              <Legend />
              <Line
                yAxisId="left"
                type="monotone"
                dataKey="job_count"
                stroke="#3b82f6"
                strokeWidth={2}
                dot={false}
                name="Jobs"
              />
              <Line
                yAxisId="right"
                type="monotone"
                dataKey="cost_usd"
                stroke="#10b981"
                strokeWidth={2}
                dot={false}
                name="Cost ($)"
              />
              <Line
                yAxisId="left"
                type="monotone"
                dataKey="error_count"
                stroke="#ef4444"
                strokeWidth={2}
                dot={false}
                name="Errors"
              />
            </LineChart>
          </ResponsiveContainer>
        </div>
      </CardContent>
    </Card>
  );
}

function formatDate(dateStr: string): string {
  // Handle different date formats
  if (dateStr.includes('W')) {
    // Week format: 2024-W01
    return dateStr;
  }
  if (dateStr.length === 7) {
    // Month format: 2024-01
    const [year, month] = dateStr.split('-');
    return `${month}/${year.slice(2)}`;
  }
  // Day format: 2024-01-15
  const parts = dateStr.split('-');
  if (parts.length === 3) {
    return `${parts[1]}/${parts[2]}`;
  }
  return dateStr;
}
