'use client';

import { useEffect, useRef, useState } from 'react';
import { useTheme } from 'next-themes';
import { cn } from '@/lib/utils';

type OutputFormat = 'json' | 'jsonl' | 'yaml';

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

type SchemaFormat = 'json' | 'yaml';

function SchemaDefinition({ compact = false }: { compact?: boolean }) {
  const [format, setFormat] = useState<SchemaFormat>('yaml');

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
          {format === 'yaml' ? <SchemaYAML /> : <SchemaJSON />}
        </pre>
      </div>
    </div>
  );
}

function SchemaYAML() {
  return (
    <code>
      <span className="text-indigo-600 dark:text-indigo-400">name</span>
      <span className="text-zinc-400">: </span>
      <span className="text-amber-600 dark:text-amber-400">Product</span>{'\n'}
      <span className="text-indigo-600 dark:text-indigo-400">description</span>
      <span className="text-zinc-400">: </span>
      <span className="text-zinc-500 dark:text-zinc-400">|</span>{'\n'}
      <span className="text-zinc-500 dark:text-zinc-400 italic">{'  '}Extract product listing details</span>{'\n'}
      <span className="text-zinc-500 dark:text-zinc-400 italic">{'  '}including pricing and shipping.</span>{'\n'}
      {'\n'}
      <span className="text-indigo-600 dark:text-indigo-400">fields</span>
      <span className="text-zinc-400">:</span>{'\n'}
      <span className="text-zinc-400">  - </span>
      <span className="text-indigo-600 dark:text-indigo-400">name</span>
      <span className="text-zinc-400">: </span>
      <span className="text-amber-600 dark:text-amber-400">title</span>{'\n'}
      <span className="text-zinc-400">    </span>
      <span className="text-indigo-600 dark:text-indigo-400">type</span>
      <span className="text-zinc-400">: </span>
      <span className="text-blue-600 dark:text-blue-400">string</span>{'\n'}
      <span className="text-zinc-400">    </span>
      <span className="text-indigo-600 dark:text-indigo-400">required</span>
      <span className="text-zinc-400">: </span>
      <span className="text-purple-600 dark:text-purple-400">true</span>{'\n'}
      {'\n'}
      <span className="text-zinc-400">  - </span>
      <span className="text-indigo-600 dark:text-indigo-400">name</span>
      <span className="text-zinc-400">: </span>
      <span className="text-amber-600 dark:text-amber-400">price</span>{'\n'}
      <span className="text-zinc-400">    </span>
      <span className="text-indigo-600 dark:text-indigo-400">type</span>
      <span className="text-zinc-400">: </span>
      <span className="text-blue-600 dark:text-blue-400">object</span>{'\n'}
      <span className="text-zinc-400">    </span>
      <span className="text-indigo-600 dark:text-indigo-400">required</span>
      <span className="text-zinc-400">: </span>
      <span className="text-purple-600 dark:text-purple-400">true</span>{'\n'}
      <span className="text-zinc-400">    </span>
      <span className="text-indigo-600 dark:text-indigo-400">properties</span>
      <span className="text-zinc-400">:</span>{'\n'}
      <span className="text-zinc-400">      </span>
      <span className="text-indigo-600 dark:text-indigo-400">amount</span>
      <span className="text-zinc-400">: </span>
      <span className="text-blue-600 dark:text-blue-400">number</span>{'\n'}
      <span className="text-zinc-400">      </span>
      <span className="text-indigo-600 dark:text-indigo-400">currency</span>
      <span className="text-zinc-400">: </span>
      <span className="text-blue-600 dark:text-blue-400">string</span>{'\n'}
      {'\n'}
      <span className="text-zinc-400">  - </span>
      <span className="text-indigo-600 dark:text-indigo-400">name</span>
      <span className="text-zinc-400">: </span>
      <span className="text-amber-600 dark:text-amber-400">volume_pricing</span>{'\n'}
      <span className="text-zinc-400">    </span>
      <span className="text-indigo-600 dark:text-indigo-400">type</span>
      <span className="text-zinc-400">: </span>
      <span className="text-blue-600 dark:text-blue-400">array</span>{'\n'}
      <span className="text-zinc-400">    </span>
      <span className="text-indigo-600 dark:text-indigo-400">items</span>
      <span className="text-zinc-400">:</span>{'\n'}
      <span className="text-zinc-400">      </span>
      <span className="text-indigo-600 dark:text-indigo-400">type</span>
      <span className="text-zinc-400">: </span>
      <span className="text-blue-600 dark:text-blue-400">object</span>{'\n'}
      <span className="text-zinc-400">      </span>
      <span className="text-indigo-600 dark:text-indigo-400">properties</span>
      <span className="text-zinc-400">:</span>{'\n'}
      <span className="text-zinc-400">        </span>
      <span className="text-indigo-600 dark:text-indigo-400">min_qty</span>
      <span className="text-zinc-400">: </span>
      <span className="text-blue-600 dark:text-blue-400">integer</span>{'\n'}
      <span className="text-zinc-400">        </span>
      <span className="text-indigo-600 dark:text-indigo-400">amount</span>
      <span className="text-zinc-400">: </span>
      <span className="text-blue-600 dark:text-blue-400">number</span>{'\n'}
      <span className="text-zinc-400">        </span>
      <span className="text-indigo-600 dark:text-indigo-400">currency</span>
      <span className="text-zinc-400">: </span>
      <span className="text-blue-600 dark:text-blue-400">string</span>{'\n'}
      {'\n'}
      <span className="text-zinc-400">  - </span>
      <span className="text-indigo-600 dark:text-indigo-400">name</span>
      <span className="text-zinc-400">: </span>
      <span className="text-amber-600 dark:text-amber-400">shipping</span>{'\n'}
      <span className="text-zinc-400">    </span>
      <span className="text-indigo-600 dark:text-indigo-400">type</span>
      <span className="text-zinc-400">: </span>
      <span className="text-blue-600 dark:text-blue-400">object</span>{'\n'}
      <span className="text-zinc-400">    </span>
      <span className="text-indigo-600 dark:text-indigo-400">properties</span>
      <span className="text-zinc-400">:</span>{'\n'}
      <span className="text-zinc-400">      </span>
      <span className="text-indigo-600 dark:text-indigo-400">cost</span>
      <span className="text-zinc-400">: </span>
      <span className="text-blue-600 dark:text-blue-400">number</span>{'\n'}
      <span className="text-zinc-400">      </span>
      <span className="text-indigo-600 dark:text-indigo-400">currency</span>
      <span className="text-zinc-400">: </span>
      <span className="text-blue-600 dark:text-blue-400">string</span>{'\n'}
      <span className="text-zinc-400">      </span>
      <span className="text-indigo-600 dark:text-indigo-400">estimated_days</span>
      <span className="text-zinc-400">: </span>
      <span className="text-blue-600 dark:text-blue-400">string</span>
    </code>
  );
}

function SchemaJSON() {
  return (
    <code>
      <span className="text-zinc-500">{'{'}</span>{'\n'}
      {'  '}<span className="text-indigo-600 dark:text-indigo-400">&quot;name&quot;</span>
      <span className="text-zinc-400">: </span>
      <span className="text-amber-600 dark:text-amber-400">&quot;Product&quot;</span>
      <span className="text-zinc-500">,</span>{'\n'}
      {'  '}<span className="text-indigo-600 dark:text-indigo-400">&quot;description&quot;</span>
      <span className="text-zinc-400">: </span>
      <span className="text-amber-600 dark:text-amber-400">&quot;Extract product listing details&quot;</span>
      <span className="text-zinc-500">,</span>{'\n'}
      {'  '}<span className="text-indigo-600 dark:text-indigo-400">&quot;fields&quot;</span>
      <span className="text-zinc-400">: </span>
      <span className="text-zinc-500">[</span>{'\n'}
      {'    '}<span className="text-zinc-500">{'{'}</span>{'\n'}
      {'      '}<span className="text-indigo-600 dark:text-indigo-400">&quot;name&quot;</span>
      <span className="text-zinc-400">: </span>
      <span className="text-amber-600 dark:text-amber-400">&quot;title&quot;</span>
      <span className="text-zinc-500">,</span>{'\n'}
      {'      '}<span className="text-indigo-600 dark:text-indigo-400">&quot;type&quot;</span>
      <span className="text-zinc-400">: </span>
      <span className="text-blue-600 dark:text-blue-400">&quot;string&quot;</span>
      <span className="text-zinc-500">,</span>{'\n'}
      {'      '}<span className="text-indigo-600 dark:text-indigo-400">&quot;required&quot;</span>
      <span className="text-zinc-400">: </span>
      <span className="text-purple-600 dark:text-purple-400">true</span>{'\n'}
      {'    '}<span className="text-zinc-500">{'}'}</span>
      <span className="text-zinc-500">,</span>{'\n'}
      {'    '}<span className="text-zinc-500">{'{'}</span>{'\n'}
      {'      '}<span className="text-indigo-600 dark:text-indigo-400">&quot;name&quot;</span>
      <span className="text-zinc-400">: </span>
      <span className="text-amber-600 dark:text-amber-400">&quot;price&quot;</span>
      <span className="text-zinc-500">,</span>{'\n'}
      {'      '}<span className="text-indigo-600 dark:text-indigo-400">&quot;type&quot;</span>
      <span className="text-zinc-400">: </span>
      <span className="text-blue-600 dark:text-blue-400">&quot;object&quot;</span>
      <span className="text-zinc-500">,</span>{'\n'}
      {'      '}<span className="text-indigo-600 dark:text-indigo-400">&quot;required&quot;</span>
      <span className="text-zinc-400">: </span>
      <span className="text-purple-600 dark:text-purple-400">true</span>
      <span className="text-zinc-500">,</span>{'\n'}
      {'      '}<span className="text-indigo-600 dark:text-indigo-400">&quot;properties&quot;</span>
      <span className="text-zinc-400">: </span>
      <span className="text-zinc-500">{'{'}</span>{'\n'}
      {'        '}<span className="text-indigo-600 dark:text-indigo-400">&quot;amount&quot;</span>
      <span className="text-zinc-400">: </span>
      <span className="text-zinc-500">{'{'}</span>
      <span className="text-indigo-600 dark:text-indigo-400">&quot;type&quot;</span>
      <span className="text-zinc-400">:</span>
      <span className="text-blue-600 dark:text-blue-400">&quot;number&quot;</span>
      <span className="text-zinc-500">{'}'}</span>
      <span className="text-zinc-500">,</span>{'\n'}
      {'        '}<span className="text-indigo-600 dark:text-indigo-400">&quot;currency&quot;</span>
      <span className="text-zinc-400">: </span>
      <span className="text-zinc-500">{'{'}</span>
      <span className="text-indigo-600 dark:text-indigo-400">&quot;type&quot;</span>
      <span className="text-zinc-400">:</span>
      <span className="text-blue-600 dark:text-blue-400">&quot;string&quot;</span>
      <span className="text-zinc-500">{'}'}</span>{'\n'}
      {'      '}<span className="text-zinc-500">{'}'}</span>{'\n'}
      {'    '}<span className="text-zinc-500">{'}'}</span>
      <span className="text-zinc-500">,</span>{'\n'}
      {'    '}<span className="text-zinc-500">{'{'}</span>{'\n'}
      {'      '}<span className="text-indigo-600 dark:text-indigo-400">&quot;name&quot;</span>
      <span className="text-zinc-400">: </span>
      <span className="text-amber-600 dark:text-amber-400">&quot;volume_pricing&quot;</span>
      <span className="text-zinc-500">,</span>{'\n'}
      {'      '}<span className="text-indigo-600 dark:text-indigo-400">&quot;type&quot;</span>
      <span className="text-zinc-400">: </span>
      <span className="text-blue-600 dark:text-blue-400">&quot;array&quot;</span>
      <span className="text-zinc-500">,</span>{'\n'}
      {'      '}<span className="text-indigo-600 dark:text-indigo-400">&quot;items&quot;</span>
      <span className="text-zinc-400">: </span>
      <span className="text-zinc-500">{'{'}</span>
      <span className="text-zinc-500">...</span>
      <span className="text-zinc-500">{'}'}</span>{'\n'}
      {'    '}<span className="text-zinc-500">{'}'}</span>
      <span className="text-zinc-500">,</span>{'\n'}
      {'    '}<span className="text-zinc-500">{'{'}</span>{'\n'}
      {'      '}<span className="text-indigo-600 dark:text-indigo-400">&quot;name&quot;</span>
      <span className="text-zinc-400">: </span>
      <span className="text-amber-600 dark:text-amber-400">&quot;shipping&quot;</span>
      <span className="text-zinc-500">,</span>{'\n'}
      {'      '}<span className="text-indigo-600 dark:text-indigo-400">&quot;type&quot;</span>
      <span className="text-zinc-400">: </span>
      <span className="text-blue-600 dark:text-blue-400">&quot;object&quot;</span>
      <span className="text-zinc-500">,</span>{'\n'}
      {'      '}<span className="text-indigo-600 dark:text-indigo-400">&quot;properties&quot;</span>
      <span className="text-zinc-400">: </span>
      <span className="text-zinc-500">{'{'}</span>
      <span className="text-zinc-500">...</span>
      <span className="text-zinc-500">{'}'}</span>{'\n'}
      {'    '}<span className="text-zinc-500">{'}'}</span>{'\n'}
      {'  '}<span className="text-zinc-500">]</span>{'\n'}
      <span className="text-zinc-500">{'}'}</span>
    </code>
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
  const [format, setFormat] = useState<OutputFormat>('json');

  const fileExtensions: Record<OutputFormat, string> = {
    json: 'response.json',
    jsonl: 'response.jsonl',
    yaml: 'response.yaml',
  };

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
          {format === 'json' && (
            <code>
              <span className="text-zinc-500">{'{'}</span>{'\n'}
              {'  '}<span className="text-emerald-400">&quot;title&quot;</span>
              <span className="text-zinc-500">: </span>
              <span className="text-amber-300">&quot;Premium Widget Pro&quot;</span>
              <span className="text-zinc-500">,</span>{'\n'}
              {'  '}<span className="text-emerald-400">&quot;price&quot;</span>
              <span className="text-zinc-500">: {'{'}</span>{'\n'}
              {'    '}<span className="text-emerald-400">&quot;amount&quot;</span>
              <span className="text-zinc-500">: </span>
              <span className="text-blue-400">49.99</span>
              <span className="text-zinc-500">,</span>{'\n'}
              {'    '}<span className="text-emerald-400">&quot;currency&quot;</span>
              <span className="text-zinc-500">: </span>
              <span className="text-amber-300">&quot;USD&quot;</span>{'\n'}
              {'  '}<span className="text-zinc-500">{'}'}</span>
              <span className="text-zinc-500">,</span>{'\n'}
              {'  '}<span className="text-emerald-400">&quot;volume_pricing&quot;</span>
              <span className="text-zinc-500">: [</span>{'\n'}
              {'    '}<span className="text-zinc-500">{'{'}</span>
              <span className="text-emerald-400">&quot;min_qty&quot;</span>
              <span className="text-zinc-500">:</span>
              <span className="text-blue-400">10</span>
              <span className="text-zinc-500">,</span>
              <span className="text-emerald-400">&quot;amount&quot;</span>
              <span className="text-zinc-500">:</span>
              <span className="text-blue-400">44.99</span>
              <span className="text-zinc-500">,</span>
              <span className="text-emerald-400">&quot;currency&quot;</span>
              <span className="text-zinc-500">:</span>
              <span className="text-amber-300">&quot;USD&quot;</span>
              <span className="text-zinc-500">{'}'}</span>
              <span className="text-zinc-500">,</span>{'\n'}
              {'    '}<span className="text-zinc-500">{'{'}</span>
              <span className="text-emerald-400">&quot;min_qty&quot;</span>
              <span className="text-zinc-500">:</span>
              <span className="text-blue-400">50</span>
              <span className="text-zinc-500">,</span>
              <span className="text-emerald-400">&quot;amount&quot;</span>
              <span className="text-zinc-500">:</span>
              <span className="text-blue-400">39.99</span>
              <span className="text-zinc-500">,</span>
              <span className="text-emerald-400">&quot;currency&quot;</span>
              <span className="text-zinc-500">:</span>
              <span className="text-amber-300">&quot;USD&quot;</span>
              <span className="text-zinc-500">{'}'}</span>{'\n'}
              {'  '}<span className="text-zinc-500">]</span>
              <span className="text-zinc-500">,</span>{'\n'}
              {'  '}<span className="text-emerald-400">&quot;shipping&quot;</span>
              <span className="text-zinc-500">: {'{'}</span>{'\n'}
              {'    '}<span className="text-emerald-400">&quot;cost&quot;</span>
              <span className="text-zinc-500">: </span>
              <span className="text-blue-400">5.99</span>
              <span className="text-zinc-500">,</span>{'\n'}
              {'    '}<span className="text-emerald-400">&quot;currency&quot;</span>
              <span className="text-zinc-500">: </span>
              <span className="text-amber-300">&quot;USD&quot;</span>
              <span className="text-zinc-500">,</span>{'\n'}
              {'    '}<span className="text-emerald-400">&quot;estimated_days&quot;</span>
              <span className="text-zinc-500">: </span>
              <span className="text-amber-300">&quot;3-5 business days&quot;</span>{'\n'}
              {'  '}<span className="text-zinc-500">{'}'}</span>{'\n'}
              <span className="text-zinc-500">{'}'}</span>
            </code>
          )}
          {format === 'jsonl' && (
            <code>
              <span className="text-zinc-500">{'{'}</span>
              <span className="text-emerald-400">&quot;title&quot;</span>
              <span className="text-zinc-500">:</span>
              <span className="text-amber-300">&quot;Premium Widget Pro&quot;</span>
              <span className="text-zinc-500">,</span>
              <span className="text-emerald-400">&quot;price&quot;</span>
              <span className="text-zinc-500">:{'{'}</span>
              <span className="text-emerald-400">&quot;amount&quot;</span>
              <span className="text-zinc-500">:</span>
              <span className="text-blue-400">49.99</span>
              <span className="text-zinc-500">,</span>
              <span className="text-emerald-400">&quot;currency&quot;</span>
              <span className="text-zinc-500">:</span>
              <span className="text-amber-300">&quot;USD&quot;</span>
              <span className="text-zinc-500">{'},...}'}</span>{'\n'}
              <span className="text-zinc-500">{'{'}</span>
              <span className="text-emerald-400">&quot;title&quot;</span>
              <span className="text-zinc-500">:</span>
              <span className="text-amber-300">&quot;Basic Widget&quot;</span>
              <span className="text-zinc-500">,</span>
              <span className="text-emerald-400">&quot;price&quot;</span>
              <span className="text-zinc-500">:{'{'}</span>
              <span className="text-emerald-400">&quot;amount&quot;</span>
              <span className="text-zinc-500">:</span>
              <span className="text-blue-400">19.99</span>
              <span className="text-zinc-500">,</span>
              <span className="text-emerald-400">&quot;currency&quot;</span>
              <span className="text-zinc-500">:</span>
              <span className="text-amber-300">&quot;USD&quot;</span>
              <span className="text-zinc-500">{'},...}'}</span>{'\n'}
              <span className="text-zinc-500">{'{'}</span>
              <span className="text-emerald-400">&quot;title&quot;</span>
              <span className="text-zinc-500">:</span>
              <span className="text-amber-300">&quot;Enterprise Widget&quot;</span>
              <span className="text-zinc-500">,</span>
              <span className="text-emerald-400">&quot;price&quot;</span>
              <span className="text-zinc-500">:{'{'}</span>
              <span className="text-emerald-400">&quot;amount&quot;</span>
              <span className="text-zinc-500">:</span>
              <span className="text-blue-400">199.99</span>
              <span className="text-zinc-500">,</span>
              <span className="text-emerald-400">&quot;currency&quot;</span>
              <span className="text-zinc-500">:</span>
              <span className="text-amber-300">&quot;USD&quot;</span>
              <span className="text-zinc-500">{'},...}'}</span>
            </code>
          )}
          {format === 'yaml' && (
            <code>
              <span className="text-emerald-400">title</span>
              <span className="text-zinc-500">: </span>
              <span className="text-amber-300">Premium Widget Pro</span>{'\n'}
              <span className="text-emerald-400">price</span>
              <span className="text-zinc-500">:</span>{'\n'}
              <span className="text-zinc-500">  </span>
              <span className="text-emerald-400">amount</span>
              <span className="text-zinc-500">: </span>
              <span className="text-blue-400">49.99</span>{'\n'}
              <span className="text-zinc-500">  </span>
              <span className="text-emerald-400">currency</span>
              <span className="text-zinc-500">: </span>
              <span className="text-amber-300">USD</span>{'\n'}
              <span className="text-emerald-400">volume_pricing</span>
              <span className="text-zinc-500">:</span>{'\n'}
              <span className="text-zinc-500">  - </span>
              <span className="text-emerald-400">min_qty</span>
              <span className="text-zinc-500">: </span>
              <span className="text-blue-400">10</span>{'\n'}
              <span className="text-zinc-500">    </span>
              <span className="text-emerald-400">amount</span>
              <span className="text-zinc-500">: </span>
              <span className="text-blue-400">44.99</span>{'\n'}
              <span className="text-zinc-500">    </span>
              <span className="text-emerald-400">currency</span>
              <span className="text-zinc-500">: </span>
              <span className="text-amber-300">USD</span>{'\n'}
              <span className="text-zinc-500">  - </span>
              <span className="text-emerald-400">min_qty</span>
              <span className="text-zinc-500">: </span>
              <span className="text-blue-400">50</span>{'\n'}
              <span className="text-zinc-500">    </span>
              <span className="text-emerald-400">amount</span>
              <span className="text-zinc-500">: </span>
              <span className="text-blue-400">39.99</span>{'\n'}
              <span className="text-zinc-500">    </span>
              <span className="text-emerald-400">currency</span>
              <span className="text-zinc-500">: </span>
              <span className="text-amber-300">USD</span>{'\n'}
              <span className="text-emerald-400">shipping</span>
              <span className="text-zinc-500">:</span>{'\n'}
              <span className="text-zinc-500">  </span>
              <span className="text-emerald-400">cost</span>
              <span className="text-zinc-500">: </span>
              <span className="text-blue-400">5.99</span>{'\n'}
              <span className="text-zinc-500">  </span>
              <span className="text-emerald-400">currency</span>
              <span className="text-zinc-500">: </span>
              <span className="text-amber-300">USD</span>{'\n'}
              <span className="text-zinc-500">  </span>
              <span className="text-emerald-400">estimated_days</span>
              <span className="text-zinc-500">: </span>
              <span className="text-amber-300">3-5 business days</span>
            </code>
          )}
        </pre>
      </div>

      {/* Glow effect */}
      <div className="absolute -bottom-8 -right-8 h-24 w-24 rounded-full bg-emerald-500/10 blur-2xl" />
    </div>
  );
}
