'use client';

import { useMemo } from 'react';
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from 'recharts';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import type { ErrorCategorySummary } from '@/lib/api';

interface ErrorsChartProps {
  data: ErrorCategorySummary[];
  title?: string;
}

export function ErrorsChart({ data, title = 'Errors by Category' }: ErrorsChartProps) {
  const chartData = useMemo(() => {
    return data.map((item) => ({
      category: formatCategory(item.category),
      count: item.count,
      percentage: item.percentage,
    }));
  }, [data]);

  if (data.length === 0) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-lg">{title}</CardTitle>
        </CardHeader>
        <CardContent className="h-64 flex items-center justify-center">
          <p className="text-zinc-500 dark:text-zinc-400">No errors in this period</p>
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
            <BarChart
              data={chartData}
              layout="vertical"
              margin={{ top: 5, right: 30, left: 80, bottom: 5 }}
            >
              <CartesianGrid
                strokeDasharray="3 3"
                horizontal={true}
                vertical={false}
                className="stroke-zinc-200 dark:stroke-zinc-700"
              />
              <XAxis
                type="number"
                tick={{ fontSize: 12 }}
                className="text-zinc-600 dark:text-zinc-400"
              />
              <YAxis
                type="category"
                dataKey="category"
                tick={{ fontSize: 12 }}
                width={80}
                className="text-zinc-600 dark:text-zinc-400"
              />
              <Tooltip
                contentStyle={{
                  backgroundColor: 'var(--tooltip-bg, #fff)',
                  borderColor: 'var(--tooltip-border, #e4e4e7)',
                  borderRadius: '6px',
                }}
                formatter={(value, name, props) => {
                  if (name === 'count' && props?.payload) {
                    const payload = props.payload as { percentage: number };
                    return [`${value} (${payload.percentage.toFixed(1)}%)`, 'Errors'];
                  }
                  return [value, String(name)];
                }}
              />
              <Bar
                dataKey="count"
                fill="#ef4444"
                radius={[0, 4, 4, 0]}
              />
            </BarChart>
          </ResponsiveContainer>
        </div>
      </CardContent>
    </Card>
  );
}

function formatCategory(category: string): string {
  // Format error category names for display
  return category
    .replace(/_/g, ' ')
    .split(' ')
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
    .join(' ');
}
