'use client';

import { useEffect, useRef, useState, useMemo } from 'react';
import { useTheme } from 'next-themes';
import yaml from 'js-yaml';
import { cn } from '@/lib/utils';

type OutputFormat = 'json' | 'jsonl' | 'yaml';
type SchemaFormat = 'json' | 'yaml';

// Single source of truth for schema definition
const SCHEMA_DATA = {
  name: 'Product',
  description: 'Extract product listing details including pricing and shipping.',
  fields: [
    { name: 'title', type: 'string', required: true },
    {
      name: 'price',
      type: 'object',
      required: true,
      properties: {
        amount: { type: 'number' },
        currency: { type: 'string' },
      },
    },
    {
      name: 'volume_pricing',
      type: 'array',
      items: {
        type: 'object',
        properties: {
          min_qty: { type: 'integer' },
          amount: { type: 'number' },
          currency: { type: 'string' },
        },
      },
    },
    {
      name: 'shipping',
      type: 'object',
      properties: {
        cost: { type: 'number' },
        currency: { type: 'string' },
        estimated_days: { type: 'string' },
      },
    },
  ],
};

// Hand-crafted YAML with comments (js-yaml doesn't support comment generation)
const SCHEMA_YAML_WITH_COMMENTS = `name: Product
description: Extract product listing details including pricing and shipping.

fields:
  - name: title  # The product name or headline
    type: string
    required: true

  - name: price  # Current listed price
    type: object
    required: true
    properties:
      amount:  # Numeric price value
        type: number
      currency:  # ISO currency code
        type: string

  - name: volume_pricing  # Bulk discount tiers if available
    type: array
    items:
      type: object
      properties:
        min_qty:  # Minimum quantity for this tier
          type: integer
        amount:  # Discounted price
          type: number
        currency:
          type: string

  - name: shipping  # Delivery details
    type: object
    properties:
      cost:  # Shipping fee
        type: number
      currency:
        type: string
      estimated_days:  # e.g. "3-5 business days"
        type: string`;

// Single source of truth for output example
const OUTPUT_DATA = {
  title: 'Premium Widget Pro',
  price: { amount: 49.99, currency: 'USD' },
  volume_pricing: [
    { min_qty: 10, amount: 44.99, currency: 'USD' },
    { min_qty: 50, amount: 39.99, currency: 'USD' },
  ],
  shipping: { cost: 5.99, currency: 'USD', estimated_days: '3-5 business days' },
};

// Additional items for JSONL format
const JSONL_ITEMS = [
  OUTPUT_DATA,
  { title: 'Basic Widget', price: { amount: 19.99, currency: 'USD' } },
  { title: 'Enterprise Widget', price: { amount: 199.99, currency: 'USD' } },
];

// Color theme types for syntax highlighting
type ColorTheme = 'schema' | 'output';

// Syntax highlighting component that renders colored code
function SyntaxHighlight({
  content,
  format,
  theme,
}: {
  content: string;
  format: 'json' | 'yaml' | 'jsonl';
  theme: ColorTheme;
}) {
  // Tokenize and colorize the content
  const highlighted = useMemo(() => {
    const colors = theme === 'schema'
      ? {
          key: 'text-indigo-600 dark:text-indigo-400',
          string: 'text-amber-600 dark:text-amber-400',
          number: 'text-blue-600 dark:text-blue-400',
          boolean: 'text-purple-600 dark:text-purple-400',
          punctuation: 'text-zinc-400',
          bracket: 'text-zinc-500',
          comment: 'text-zinc-400 dark:text-zinc-500 italic',
        }
      : {
          key: 'text-emerald-400',
          string: 'text-amber-300',
          number: 'text-blue-400',
          boolean: 'text-purple-400',
          punctuation: 'text-zinc-500',
          bracket: 'text-zinc-500',
          comment: 'text-zinc-500 italic',
        };
    if (format === 'yaml') {
      return content.split('\n').map((line, i) => {
        // Match YAML patterns: key: value
        const parts: React.ReactNode[] = [];
        let remaining = line;
        let keyId = 0;

        // Handle comment lines (full line comments)
        const commentMatch = remaining.match(/^(\s*)(#.*)$/);
        if (commentMatch) {
          parts.push(<span key={`indent-${i}`}>{commentMatch[1]}</span>);
          parts.push(<span key={`comment-${i}`} className={colors.comment}>{commentMatch[2]}</span>);
          return <div key={i}>{parts}</div>;
        }

        // Handle list items prefix
        const listMatch = remaining.match(/^(\s*)(- )(.*)/);
        if (listMatch) {
          parts.push(<span key={`indent-${i}`}>{listMatch[1]}</span>);
          parts.push(<span key={`dash-${i}`} className={colors.punctuation}>- </span>);
          remaining = listMatch[3];
        }

        // Handle key: value pairs
        const kvMatch = remaining.match(/^(\s*)([a-zA-Z_][a-zA-Z0-9_]*)(:)(\s*)(.*)/);
        if (kvMatch) {
          const [, indent, key, colon, space, value] = kvMatch;
          parts.push(<span key={`ind-${i}`}>{indent}</span>);
          parts.push(<span key={`key-${i}-${keyId++}`} className={colors.key}>{key}</span>);
          parts.push(<span key={`col-${i}`} className={colors.punctuation}>{colon}</span>);
          parts.push(<span key={`sp-${i}`}>{space}</span>);

          // Color the value (check for inline comment)
          if (value) {
            const inlineCommentMatch = value.match(/^(.*?)\s*(#.*)$/);
            const actualValue = inlineCommentMatch ? inlineCommentMatch[1].trim() : value;
            const inlineComment = inlineCommentMatch ? inlineCommentMatch[2] : null;

            if (actualValue) {
              if (actualValue === 'true' || actualValue === 'false') {
                parts.push(<span key={`val-${i}`} className={colors.boolean}>{actualValue}</span>);
              } else if (/^-?\d+(\.\d+)?$/.test(actualValue)) {
                parts.push(<span key={`val-${i}`} className={colors.number}>{actualValue}</span>);
              } else if (actualValue === '|' || actualValue === '>') {
                parts.push(<span key={`val-${i}`} className={colors.punctuation}>{actualValue}</span>);
              } else {
                parts.push(<span key={`val-${i}`} className={colors.string}>{actualValue}</span>);
              }
            }
            if (inlineComment) {
              parts.push(<span key={`icom-${i}`}> </span>);
              parts.push(<span key={`com-${i}`} className={colors.comment}>{inlineComment}</span>);
            }
          }
        } else if (parts.length === 0) {
          // Plain indented content (like multiline strings)
          parts.push(<span key={`plain-${i}`} className={`${colors.string} italic`}>{remaining}</span>);
        } else if (remaining) {
          // Rest after list item dash
          const restKv = remaining.match(/^([a-zA-Z_][a-zA-Z0-9_]*)(:)(\s*)(.*)/);
          if (restKv) {
            const [, key, colon, space, value] = restKv;
            parts.push(<span key={`rkey-${i}`} className={colors.key}>{key}</span>);
            parts.push(<span key={`rcol-${i}`} className={colors.punctuation}>{colon}</span>);
            parts.push(<span key={`rsp-${i}`}>{space}</span>);
            if (value) {
              const inlineCommentMatch = value.match(/^(.*?)\s*(#.*)$/);
              const actualValue = inlineCommentMatch ? inlineCommentMatch[1].trim() : value;
              const inlineComment = inlineCommentMatch ? inlineCommentMatch[2] : null;

              if (actualValue) {
                if (/^-?\d+(\.\d+)?$/.test(actualValue)) {
                  parts.push(<span key={`rval-${i}`} className={colors.number}>{actualValue}</span>);
                } else {
                  parts.push(<span key={`rval-${i}`} className={colors.string}>{actualValue}</span>);
                }
              }
              if (inlineComment) {
                parts.push(<span key={`ricom-${i}`}> </span>);
                parts.push(<span key={`rcom-${i}`} className={colors.comment}>{inlineComment}</span>);
              }
            }
          } else {
            parts.push(<span key={`rest-${i}`}>{remaining}</span>);
          }
        }

        return <div key={i}>{parts.length > 0 ? parts : line || '\u00A0'}</div>;
      });
    }

    // JSON/JSONL tokenizer
    const tokens: React.ReactNode[] = [];
    let idx = 0;
    const regex = /("(?:[^"\\]|\\.)*")|(\d+\.?\d*)|(\btrue\b|\bfalse\b|\bnull\b)|([{}\[\]:,])|(\s+)/g;
    let match;

    while ((match = regex.exec(content)) !== null) {
      const [, str, num, bool, punct, space] = match;
      if (str) {
        // Check if this is a key (followed by colon)
        const afterMatch = content.slice(regex.lastIndex);
        const isKey = /^\s*:/.test(afterMatch);
        tokens.push(
          <span key={idx++} className={isKey ? colors.key : colors.string}>
            {str}
          </span>
        );
      } else if (num) {
        tokens.push(<span key={idx++} className={colors.number}>{num}</span>);
      } else if (bool) {
        tokens.push(<span key={idx++} className={colors.boolean}>{bool}</span>);
      } else if (punct) {
        tokens.push(<span key={idx++} className={punct === ':' || punct === ',' ? colors.punctuation : colors.bracket}>{punct}</span>);
      } else if (space) {
        tokens.push(<span key={idx++}>{space}</span>);
      }
    }

    return tokens;
  }, [content, format, theme]);

  return <code>{highlighted}</code>;
}

interface DataParticle {
  x: number;
  y: number;
  progress: number;
  speed: number;
  size: number;
  opacity: number;
}

export function HeroVisualization() {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const particlesRef = useRef<DataParticle[]>([]);
  const animationRef = useRef<number | null>(null);
  const { resolvedTheme } = useTheme();
  const [mounted, setMounted] = useState(false);

  useEffect(() => {
    setMounted(true);
  }, []);

  // Canvas animation for data flow particles
  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas || !mounted) return;

    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    const resizeCanvas = () => {
      const dpr = window.devicePixelRatio || 1;
      const rect = canvas.getBoundingClientRect();
      canvas.width = rect.width * dpr;
      canvas.height = rect.height * dpr;
      ctx.scale(dpr, dpr);
    };

    const spawnParticle = () => {
      const rect = canvas.getBoundingClientRect();
      particlesRef.current.push({
        x: 0,
        y: Math.random() * rect.height,
        progress: 0,
        speed: 0.003 + Math.random() * 0.004,
        size: 2 + Math.random() * 2,
        opacity: 0.4 + Math.random() * 0.4,
      });
    };

    const animate = () => {
      if (!ctx || !canvas) return;
      const rect = canvas.getBoundingClientRect();

      ctx.clearRect(0, 0, rect.width, rect.height);

      const isDark = resolvedTheme === 'dark';
      const particleColor = isDark ? '99, 102, 241' : '79, 70, 229'; // Indigo

      // Spawn new particles
      if (Math.random() < 0.1 && particlesRef.current.length < 30) {
        spawnParticle();
      }

      // Update and draw particles
      particlesRef.current = particlesRef.current.filter((p) => {
        p.progress += p.speed;

        // Gentle wave path from left to right - less funnel-like
        const t = p.progress;
        const startY = p.y;

        // Very subtle convergence - particles stay mostly at their original height
        // with just a gentle wave motion
        const waveAmount = 15; // Small wave amplitude
        const convergeFactor = 0.1; // Very slight convergence (10% toward center)
        const midY = rect.height / 2;

        // Gentle sine wave plus very subtle convergence
        const wave = Math.sin(t * Math.PI * 2) * waveAmount;
        const converge = (midY - startY) * convergeFactor * t;

        const x = t * rect.width;
        const y = startY + wave + converge;

        p.x = x;
        p.y = y;

        // Draw particle with trail
        const gradient = ctx.createRadialGradient(p.x, p.y, 0, p.x, p.y, p.size * 3);
        gradient.addColorStop(0, `rgba(${particleColor}, ${p.opacity})`);
        gradient.addColorStop(1, `rgba(${particleColor}, 0)`);

        ctx.beginPath();
        ctx.arc(p.x, p.y, p.size * 3, 0, Math.PI * 2);
        ctx.fillStyle = gradient;
        ctx.fill();

        // Core
        ctx.beginPath();
        ctx.arc(p.x, p.y, p.size, 0, Math.PI * 2);
        ctx.fillStyle = `rgba(${particleColor}, ${p.opacity})`;
        ctx.fill();

        return p.progress < 1;
      });

      animationRef.current = requestAnimationFrame(animate);
    };

    resizeCanvas();
    window.addEventListener('resize', resizeCanvas);
    animate();

    return () => {
      window.removeEventListener('resize', resizeCanvas);
      if (animationRef.current) {
        cancelAnimationFrame(animationRef.current);
      }
    };
  }, [resolvedTheme, mounted]);

  if (!mounted) {
    return <div className="h-80 lg:h-96" />;
  }

  return (
    <div className="relative w-full py-8">
      {/* Particle canvas - full width, behind content */}
      <canvas
        ref={canvasRef}
        className="absolute -inset-y-12 -inset-x-4 sm:-inset-x-8 lg:-inset-x-16 pointer-events-none z-0"
        style={{ width: 'calc(100% + 2rem)', height: 'calc(100% + 96px)' }}
      />

      <div className="relative z-10 grid grid-cols-1 lg:grid-cols-3 gap-6 lg:gap-8 items-start">
        {/* Left: Unstructured Website Skeleton */}
        <div className="relative">
          <div className="text-xs uppercase tracking-wider text-zinc-500 dark:text-zinc-400 mb-2 font-medium">
            Unstructured HTML
          </div>
          <WebsiteSkeleton />
        </div>

        {/* Center: Schema Definition */}
        <div className="hidden lg:block relative">
          <div className="text-xs uppercase tracking-wider text-transparent mb-2 font-medium select-none" aria-hidden="true">
            &nbsp;
          </div>
          <SchemaDefinition />
        </div>

        {/* Mobile: Arrow and Schema */}
        <div className="flex lg:hidden flex-col items-center gap-3 py-4">
          <SchemaArrow direction="down" />
          <div className="w-full">
            <SchemaDefinition compact />
          </div>
          <SchemaArrow direction="down" />
        </div>

        {/* Right: Structured Output */}
        <div className="relative">
          <div className="text-xs uppercase tracking-wider text-zinc-500 dark:text-zinc-400 mb-2 font-medium">
            Structured Output
          </div>
          <StructuredOutput />
        </div>
      </div>
    </div>
  );
}

function WebsiteSkeleton() {
  return (
    <div className="relative rounded-lg border border-zinc-200 dark:border-zinc-700/50 bg-white/70 dark:bg-zinc-900/70 backdrop-blur-sm p-3 sm:p-5 space-y-3 sm:space-y-4 overflow-hidden h-[280px] sm:h-[360px] lg:h-[480px]">
      {/* Browser chrome */}
      <div className="flex items-center gap-1.5 pb-3 border-b border-zinc-200 dark:border-zinc-700/50">
        <div className="h-2.5 w-2.5 rounded-full bg-red-400/60" />
        <div className="h-2.5 w-2.5 rounded-full bg-yellow-400/60" />
        <div className="h-2.5 w-2.5 rounded-full bg-green-400/60" />
        <div className="ml-2 h-4 flex-1 rounded bg-zinc-100 dark:bg-zinc-800" />
      </div>

      {/* Header skeleton */}
      <div className="flex items-center justify-between">
        <div className="h-5 w-20 rounded bg-zinc-200/80 dark:bg-zinc-700/60" />
        <div className="flex gap-2">
          <div className="h-3 w-12 rounded bg-zinc-100 dark:bg-zinc-800/60" />
          <div className="h-3 w-12 rounded bg-zinc-100 dark:bg-zinc-800/60" />
          <div className="h-3 w-12 rounded bg-zinc-100 dark:bg-zinc-800/60" />
        </div>
      </div>

      {/* Content chaos - representing unstructured data */}
      <div className="space-y-2 pt-2">
        {/* Product image placeholder */}
        <div className="flex gap-3">
          <div className="h-16 w-16 rounded bg-zinc-200/60 dark:bg-zinc-700/40 flex-shrink-0" />
          <div className="flex-1 space-y-1.5">
            <div className="h-4 w-3/4 rounded bg-zinc-200/80 dark:bg-zinc-700/60" />
            <div className="h-3 w-1/2 rounded bg-zinc-100 dark:bg-zinc-800/40" />
            <div className="h-3 w-2/3 rounded bg-zinc-100 dark:bg-zinc-800/40" />
          </div>
        </div>

        {/* Random scattered elements */}
        <div className="flex gap-2 flex-wrap">
          <div className="h-6 w-16 rounded-full bg-zinc-100 dark:bg-zinc-800/60" />
          <div className="h-6 w-12 rounded-full bg-zinc-200/60 dark:bg-zinc-700/40" />
          <div className="h-6 w-20 rounded-full bg-zinc-100 dark:bg-zinc-800/60" />
        </div>

        {/* More chaotic content */}
        <div className="grid grid-cols-3 gap-2">
          <div className="h-3 rounded bg-zinc-100 dark:bg-zinc-800/40" />
          <div className="h-3 col-span-2 rounded bg-zinc-200/60 dark:bg-zinc-700/40" />
          <div className="h-3 col-span-2 rounded bg-zinc-100 dark:bg-zinc-800/40" />
          <div className="h-3 rounded bg-zinc-200/60 dark:bg-zinc-700/40" />
        </div>

        {/* Price-like element buried in noise */}
        <div className="flex items-center gap-2">
          <div className="h-3 w-8 rounded bg-zinc-100 dark:bg-zinc-800/40" />
          <div className="h-5 w-14 rounded bg-zinc-300/80 dark:bg-zinc-600/60" />
          <div className="h-3 w-16 rounded bg-zinc-100 dark:bg-zinc-800/40" />
        </div>

        {/* More scattered content to fill height */}
        <div className="flex gap-3 pt-2">
          <div className="h-12 w-12 rounded bg-zinc-200/60 dark:bg-zinc-700/40 flex-shrink-0" />
          <div className="flex-1 space-y-1.5">
            <div className="h-3 w-2/3 rounded bg-zinc-200/80 dark:bg-zinc-700/60" />
            <div className="h-3 w-1/2 rounded bg-zinc-100 dark:bg-zinc-800/40" />
          </div>
        </div>

        {/* Additional noise elements */}
        <div className="grid grid-cols-4 gap-2">
          <div className="h-3 rounded bg-zinc-100 dark:bg-zinc-800/40" />
          <div className="h-3 rounded bg-zinc-200/60 dark:bg-zinc-700/40" />
          <div className="h-3 col-span-2 rounded bg-zinc-100 dark:bg-zinc-800/40" />
        </div>

        <div className="flex gap-2 flex-wrap">
          <div className="h-5 w-14 rounded-full bg-zinc-200/60 dark:bg-zinc-700/40" />
          <div className="h-5 w-10 rounded-full bg-zinc-100 dark:bg-zinc-800/60" />
          <div className="h-5 w-16 rounded-full bg-zinc-200/60 dark:bg-zinc-700/40" />
        </div>
      </div>

      {/* Noise overlay */}
      <div className="absolute inset-0 opacity-[0.015] dark:opacity-[0.03]" style={{
        backgroundImage: `url("data:image/svg+xml,%3Csvg viewBox='0 0 256 256' xmlns='http://www.w3.org/2000/svg'%3E%3Cfilter id='noise'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.8' numOctaves='4' stitchTiles='stitch'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23noise)'/%3E%3C/svg%3E")`,
      }} />
    </div>
  );
}

function SchemaDefinition({ compact = false }: { compact?: boolean }) {
  const [format, setFormat] = useState<SchemaFormat>('yaml');

  const content = useMemo(() => {
    if (format === 'yaml') {
      return SCHEMA_YAML_WITH_COMMENTS;
    }
    return JSON.stringify(SCHEMA_DATA, null, 2);
  }, [format]);

  return (
    <div className={cn(
      'rounded-lg border-2 border-indigo-500/30 bg-indigo-50/70 dark:bg-indigo-950/40 backdrop-blur-sm overflow-hidden',
      compact ? 'p-3 h-[200px]' : 'p-3 sm:p-5 h-[280px] sm:h-[360px] lg:h-[480px]',
    )}>
      <div className="flex items-center justify-between mb-2">
        <span className="text-[10px] uppercase tracking-wider text-indigo-600 dark:text-indigo-400 font-medium">
          Schema
        </span>
        <div className="flex gap-1">
          {(['json', 'yaml'] as SchemaFormat[]).map((f) => (
            <button
              key={f}
              onClick={() => setFormat(f)}
              className={cn(
                'text-[10px] px-1.5 py-0.5 rounded uppercase font-medium transition-colors',
                format === f
                  ? 'bg-indigo-500/20 text-indigo-600 dark:text-indigo-400'
                  : 'bg-indigo-100/50 dark:bg-indigo-900/20 text-indigo-400 dark:text-indigo-500 hover:text-indigo-500 dark:hover:text-indigo-400'
              )}
            >
              {f}
            </button>
          ))}
        </div>
      </div>
      <div className="overflow-y-auto h-[calc(100%-2.5rem)] schema-scrollbar">
        <pre className="font-mono text-[11px] leading-relaxed">
          <SyntaxHighlight content={content} format={format} theme="schema" />
        </pre>
      </div>
    </div>
  );
}


function SchemaArrow({ direction = 'right' }: { direction?: 'right' | 'down' }) {
  if (direction === 'down') {
    return (
      <div className="relative">
        <svg
          className="h-6 w-6 text-indigo-500/60"
          fill="none"
          viewBox="0 0 24 24"
          strokeWidth={2}
          stroke="currentColor"
        >
          <path strokeLinecap="round" strokeLinejoin="round" d="M12 4.5v15m0 0l6.75-6.75M12 19.5l-6.75-6.75" />
        </svg>
        {/* Animated pulse */}
        <div className="absolute inset-0 flex items-center justify-center">
          <div className="h-2 w-2 rounded-full bg-indigo-500/40 animate-ping" />
        </div>
      </div>
    );
  }

  return (
    <div className="relative">
      <svg
        className="h-6 w-6 text-indigo-500/60"
        fill="none"
        viewBox="0 0 24 24"
        strokeWidth={2}
        stroke="currentColor"
      >
        <path strokeLinecap="round" strokeLinejoin="round" d="M13.5 4.5L21 12m0 0l-7.5 7.5M21 12H3" />
      </svg>
      {/* Animated pulse */}
      <div className="absolute inset-0 flex items-center justify-center">
        <div className="h-2 w-2 rounded-full bg-indigo-500/40 animate-ping" />
      </div>
    </div>
  );
}

function StructuredOutput() {
  const [format, setFormat] = useState<OutputFormat>('yaml');

  const fileExtensions: Record<OutputFormat, string> = {
    json: 'response.json',
    jsonl: 'response.jsonl',
    yaml: 'response.yaml',
  };

  const content = useMemo(() => {
    if (format === 'yaml') {
      return yaml.dump(OUTPUT_DATA, { lineWidth: -1, quotingType: '"', forceQuotes: false });
    }
    if (format === 'jsonl') {
      return JSONL_ITEMS.map((item) => JSON.stringify(item)).join('\n');
    }
    return JSON.stringify(OUTPUT_DATA, null, 2);
  }, [format]);

  return (
    <div className="relative rounded-lg border border-emerald-500/30 bg-zinc-950/70 backdrop-blur-sm p-3 sm:p-5 overflow-hidden h-[280px] sm:h-[360px] lg:h-[480px]">
      {/* Terminal header with format tabs */}
      <div className="flex items-center gap-1.5 pb-3 border-b border-zinc-800/60">
        <div className="h-2.5 w-2.5 rounded-full bg-red-500/60" />
        <div className="h-2.5 w-2.5 rounded-full bg-yellow-500/60" />
        <div className="h-2.5 w-2.5 rounded-full bg-green-500/60" />
        <span className="ml-2 text-xs text-zinc-500 font-[family-name:var(--font-code)]">
          {fileExtensions[format]}
        </span>
        <div className="ml-auto flex gap-1">
          {(['json', 'jsonl', 'yaml'] as OutputFormat[]).map((f) => (
            <button
              key={f}
              onClick={() => setFormat(f)}
              className={cn(
                'text-[10px] px-1.5 py-0.5 rounded uppercase font-medium transition-colors',
                format === f
                  ? 'bg-emerald-500/20 text-emerald-400'
                  : 'bg-zinc-800/50 text-zinc-500 hover:text-zinc-400 hover:bg-zinc-800'
              )}
            >
              {f}
            </button>
          ))}
        </div>
      </div>

      {/* Content based on format */}
      <div className="mt-3 overflow-y-auto h-[calc(100%-3rem)] output-scrollbar">
        <pre className="font-[family-name:var(--font-code)] text-xs leading-relaxed">
          <SyntaxHighlight content={content} format={format} theme="output" />
        </pre>
      </div>

      {/* Glow effect */}
      <div className="absolute -bottom-8 -right-8 h-24 w-24 rounded-full bg-emerald-500/10 blur-2xl" />
    </div>
  );
}
