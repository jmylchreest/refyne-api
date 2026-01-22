import Link from 'next/link';
import type { Metadata } from 'next';
import { Button } from '@/components/ui/button';
import { AnimatedGrid } from '@/components/animated-grid';
import { HeroVisualization } from '@/components/hero-visualization';
import { RefyneText } from '@/components/refyne-logo';
import { PricingSection } from '@/components/pricing-section';
import { SiteHeader } from '@/components/site-header';

export const metadata: Metadata = {
  title: 'Refyne - AI-Powered Web Scraping API | Extract Structured Data',
  description: 'Transform any website into structured JSON with AI-powered extraction. Refyne uses LLMs to extract, parse, and structure data from any webpage. No CSS selectors needed. Alternative to Scrapy, Puppeteer, Selenium, Apify, and Bright Data.',
  keywords: 'web scraping API, AI web scraper, website to JSON, LLM data extraction, Scrapy alternative, Puppeteer alternative, Selenium alternative, Apify alternative, Bright Data alternative, Firecrawl alternative, no-code web scraper',
  alternates: {
    canonical: 'https://refyne.uk',
  },
};

export default function Home() {
  return (
    <div className="min-h-screen bg-zinc-50 dark:bg-zinc-950 relative overflow-hidden">
      {/* Animated Grid Background */}
      <div className="fixed inset-0 z-0">
        <AnimatedGrid />
      </div>

      {/* Content */}
      <div className="relative z-10">
        <SiteHeader />

        {/* Hero Section */}
        <section className="container mx-auto px-4 pt-20 pb-24">
          <div className="text-center mb-16">
            <h1 className="mx-auto max-w-4xl text-4xl font-bold tracking-tight sm:text-5xl lg:text-6xl leading-[1.1]">
              The{' '}
              <span className="relative inline-block">
                <span className="bg-gradient-to-r from-indigo-600 via-purple-600 to-indigo-600 bg-clip-text text-transparent">
                  structured
                </span>
                <svg
                  className="absolute -bottom-2 left-0 w-full h-3 text-indigo-500/30"
                  viewBox="0 0 200 12"
                  fill="none"
                  xmlns="http://www.w3.org/2000/svg"
                >
                  <path
                    d="M1 9C20 3 60 1 100 5C140 9 180 7 199 3"
                    stroke="currentColor"
                    strokeWidth="2"
                    strokeLinecap="round"
                  />
                </svg>
              </span>
              {' '}web awaits
            </h1>
            <p className="mx-auto mt-8 max-w-2xl text-base sm:text-lg text-zinc-600 dark:text-zinc-400 leading-relaxed">
              Transform any website into structured, typed data with AI-powered extraction.{' '}
              <RefyneText /> understands the chaos so you don&apos;t have to.
            </p>
            <div className="mt-10 flex items-center justify-center gap-4">
              <Link href="/sign-up">
                <Button size="lg" className="h-12 px-8 font-medium">
                  Start Free
                  <svg className="ml-2 h-4 w-4" fill="none" viewBox="0 0 24 24" strokeWidth={2} stroke="currentColor">
                    <path strokeLinecap="round" strokeLinejoin="round" d="M13.5 4.5L21 12m0 0l-7.5 7.5M21 12H3" />
                  </svg>
                </Button>
              </Link>
              <Link href="https://github.com/jmylchreest/refyne" target="_blank" rel="noopener noreferrer">
                <Button size="lg" variant="outline" className="h-12 px-8 font-medium">
                  <svg className="mr-2 h-5 w-5" fill="currentColor" viewBox="0 0 24 24">
                    <path fillRule="evenodd" clipRule="evenodd" d="M12 2C6.477 2 2 6.484 2 12.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.531 1.032 1.531 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0112 6.844c.85.004 1.705.115 2.504.337 1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.202 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.943.359.309.678.92.678 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.019 10.019 0 0022 12.017C22 6.484 17.522 2 12 2z" />
                  </svg>
                  View on GitHub
                </Button>
              </Link>
            </div>
          </div>

          {/* Hero Visualization */}
          <HeroVisualization />
        </section>

        {/* Capabilities Section */}
        <section id="capabilities" className="border-t border-zinc-200/50 dark:border-zinc-800/50">
          <div className="container mx-auto px-4 py-16 sm:py-24">
            {/* Capability 1: Intent-Driven */}
            <div className="grid md:grid-cols-2 gap-8 md:gap-16 items-stretch max-w-5xl mx-auto mb-16 sm:mb-24">
              <div className="flex flex-col justify-center">
                <h3 className="text-xl sm:text-2xl font-bold tracking-tight mb-4">Intent-Driven Extraction</h3>
                <p className="text-sm sm:text-base text-zinc-600 dark:text-zinc-400 mb-6">
                  Describe <em>what</em> you want, not <em>how</em> to find it. Define your schema in plain language and let AI understand the page structure.
                </p>
                <p className="text-sm font-medium text-indigo-600 dark:text-indigo-400">
                  No CSS selectors. No XPath. Just describe your data.
                </p>
              </div>
              <div className="bg-zinc-900 dark:bg-zinc-950 rounded-lg border border-zinc-700/50 p-4 sm:p-6 font-mono text-xs sm:text-sm overflow-x-auto">
                <div className="text-zinc-500 mb-2"># Your schema (JSON or YAML)</div>
                <div className="text-purple-400">products:</div>
                <div className="text-zinc-300 pl-2 sm:pl-4">
                  <span>- </span>
                  <span className="text-purple-400">name</span>: <span className="text-amber-400">string</span>
                  <span className="text-zinc-500 pl-4"># Product display name</span>
                </div>
                <div className="text-zinc-300 pl-4 sm:pl-8">
                  <span className="text-purple-400">price</span>: <span className="text-amber-400">number</span>
                  <span className="text-zinc-500 pl-3"># Price in local currency</span>
                </div>
                <div className="text-zinc-300 pl-4 sm:pl-8">
                  <span className="text-purple-400">inStock</span>: <span className="text-amber-400">boolean</span>
                  <span className="text-zinc-500 pl-1"># Availability status</span>
                </div>
              </div>
            </div>

            {/* Capability 2: Developer-Native & Open Source */}
            <div className="grid md:grid-cols-2 gap-8 md:gap-16 items-stretch max-w-5xl mx-auto mb-16 sm:mb-24">
              <div className="order-2 md:order-1 bg-zinc-900 dark:bg-zinc-950 rounded-lg border border-zinc-700/50 p-4 sm:p-6 font-mono text-xs sm:text-sm overflow-x-auto">
                <div className="text-zinc-500 mb-2"># Extract with your own API keys</div>
                <div>
                  <span className="text-purple-400">curl</span>
                  <span className="text-zinc-300"> -X POST </span>
                  <span className="text-amber-400">&quot;/api/v1/extract&quot;</span>
                  <span className="text-zinc-300"> \</span><br />
                  <span className="pl-2 sm:pl-4 text-zinc-300">-H </span>
                  <span className="text-amber-400">&quot;Authorization: Bearer $TOKEN&quot;</span>
                  <span className="text-zinc-300"> \</span><br />
                  <span className="pl-2 sm:pl-4 text-zinc-300">-d </span>
                  <span className="text-emerald-400">{"'{"}</span><br />
                  <span className="pl-4 sm:pl-8 text-blue-400">&quot;url&quot;</span>
                  <span className="text-zinc-300">: </span>
                  <span className="text-amber-400">&quot;https://shop.example.com&quot;</span>
                  <span className="text-zinc-300">,</span><br />
                  <span className="pl-4 sm:pl-8 text-blue-400">&quot;schema&quot;</span>
                  <span className="text-zinc-300">: </span>
                  <span className="text-zinc-500">{'{ ... }'}</span>
                  <span className="text-zinc-300">,</span><br />
                  <span className="pl-4 sm:pl-8 text-blue-400">&quot;llm_config&quot;</span>
                  <span className="text-zinc-300">: {'{'}</span><br />
                  <span className="pl-6 sm:pl-12 text-blue-400">&quot;provider&quot;</span>
                  <span className="text-zinc-300">: </span>
                  <span className="text-amber-400">&quot;openrouter&quot;</span><br />
                  <span className="pl-4 sm:pl-8 text-zinc-300">{'}'}</span><br />
                  <span className="pl-2 sm:pl-4 text-emerald-400">{"}'"}</span>
                </div>
              </div>
              <div className="order-1 md:order-2 flex flex-col justify-center">
                <div className="flex items-center gap-3 mb-4">
                  <h3 className="text-xl sm:text-2xl font-bold tracking-tight">Developer-Native & Open Source</h3>
                </div>
                <p className="text-sm sm:text-base text-zinc-600 dark:text-zinc-400 mb-6">
                  The core extraction engine is open source. Full control over cleansing and extraction chains. Customize, extend, or self-host.
                </p>
                <p className="text-sm font-medium text-indigo-600 dark:text-indigo-400">
                  Open source core. Full control. Self-hosted API coming soon.
                </p>
              </div>
            </div>

            {/* Capability 3: BYOK */}
            <div className="grid md:grid-cols-2 gap-8 md:gap-16 items-stretch max-w-5xl mx-auto mb-16 sm:mb-24">
              <div className="flex flex-col justify-center">
                <h3 className="text-xl sm:text-2xl font-bold tracking-tight mb-4">Bring Your Own Keys</h3>
                <p className="text-sm sm:text-base text-zinc-600 dark:text-zinc-400 mb-6">
                  Use your own LLM API keys - Claude, GPT-4, or OpenRouter. Pay only for what you use with no markup surprises.
                </p>
                <p className="text-sm font-medium text-indigo-600 dark:text-indigo-400">
                  Your keys. Your models. Your costs.
                </p>
              </div>
              <div className="bg-zinc-900 dark:bg-zinc-950 rounded-lg border border-zinc-700/50 p-6 sm:p-8 flex items-center justify-center">
                <div className="flex items-center justify-center gap-6 sm:gap-8">
                  <div className="flex flex-col items-center gap-2">
                    <div className="h-12 w-12 sm:h-14 sm:w-14 rounded-xl bg-gradient-to-br from-orange-400 to-orange-600 flex items-center justify-center text-white font-bold text-lg sm:text-xl shadow-lg">
                      A
                    </div>
                    <span className="text-[10px] sm:text-xs text-zinc-500">Anthropic</span>
                  </div>
                  <div className="flex flex-col items-center gap-2">
                    <div className="h-12 w-12 sm:h-14 sm:w-14 rounded-xl bg-gradient-to-br from-emerald-400 to-teal-600 flex items-center justify-center text-white shadow-lg">
                      <svg className="h-6 w-6 sm:h-7 sm:w-7" viewBox="0 0 24 24" fill="currentColor">
                        <path d="M22.2819 9.8211a5.9847 5.9847 0 0 0-.5157-4.9108 6.0462 6.0462 0 0 0-6.5098-2.9A6.0651 6.0651 0 0 0 4.9807 4.1818a5.9847 5.9847 0 0 0-3.9977 2.9 6.0462 6.0462 0 0 0 .7427 7.0966 5.98 5.98 0 0 0 .511 4.9107 6.051 6.051 0 0 0 6.5146 2.9001A5.9847 5.9847 0 0 0 13.2599 24a6.0557 6.0557 0 0 0 5.7718-4.2058 5.9894 5.9894 0 0 0 3.9977-2.9001 6.0557 6.0557 0 0 0-.7475-7.0729zm-9.022 12.6081a4.4755 4.4755 0 0 1-2.8764-1.0408l.1419-.0804 4.7783-2.7582a.7948.7948 0 0 0 .3927-.6813v-6.7369l2.02 1.1686a.071.071 0 0 1 .038.052v5.5826a4.504 4.504 0 0 1-4.4945 4.4944zm-9.6607-4.1254a4.4708 4.4708 0 0 1-.5346-3.0137l.142.0852 4.783 2.7582a.7712.7712 0 0 0 .7806 0l5.8428-3.3685v2.3324a.0804.0804 0 0 1-.0332.0615L9.74 19.9502a4.4992 4.4992 0 0 1-6.1408-1.6464zM2.3408 7.8956a4.485 4.485 0 0 1 2.3655-1.9728V11.6a.7664.7664 0 0 0 .3879.6765l5.8144 3.3543-2.0201 1.1685a.0757.0757 0 0 1-.071 0l-4.8303-2.7865A4.504 4.504 0 0 1 2.3408 7.8956zm16.5963 3.8558L13.1038 8.364 15.1192 7.2a.0757.0757 0 0 1 .071 0l4.8303 2.7913a4.4944 4.4944 0 0 1-.6765 8.1042v-5.6772a.79.79 0 0 0-.407-.667zm2.0107-3.0231l-.142-.0852-4.7735-2.7818a.7759.7759 0 0 0-.7854 0L9.409 9.2297V6.8974a.0662.0662 0 0 1 .0284-.0615l4.8303-2.7866a4.4992 4.4992 0 0 1 6.6802 4.66zM8.3065 12.863l-2.02-1.1638a.0804.0804 0 0 1-.038-.0567V6.0742a4.4992 4.4992 0 0 1 7.3757-3.4537l-.142.0805L8.704 5.459a.7948.7948 0 0 0-.3927.6813zm1.0976-2.3654l2.602-1.4998 2.6069 1.4998v2.9994l-2.5974 1.4997-2.6067-1.4997Z" />
                      </svg>
                    </div>
                    <span className="text-[10px] sm:text-xs text-zinc-500">OpenAI</span>
                  </div>
                  <div className="flex flex-col items-center gap-2">
                    <div className="h-12 w-12 sm:h-14 sm:w-14 rounded-xl bg-gradient-to-br from-blue-500 to-indigo-600 flex items-center justify-center text-white shadow-lg">
                      <svg className="h-6 w-6 sm:h-7 sm:w-7" viewBox="0 0 24 24" fill="currentColor">
                        <path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" fill="none" />
                      </svg>
                    </div>
                    <span className="text-[10px] sm:text-xs text-zinc-500">OpenRouter</span>
                  </div>
                </div>
              </div>
            </div>

            {/* Capability 4: Full Web Rendering */}
            <div className="grid md:grid-cols-2 gap-8 md:gap-16 items-stretch max-w-5xl mx-auto">
              <div className="order-2 md:order-1">
                <div className="bg-zinc-900 dark:bg-zinc-950 rounded-lg border border-zinc-700/50 p-6 sm:p-8 h-full flex items-center justify-center">
                  <div className="flex items-center justify-center gap-6 sm:gap-8">
                    {/* JavaScript */}
                    <div className="flex flex-col items-center gap-2">
                      <div className="h-12 w-12 sm:h-14 sm:w-14 rounded-xl bg-gradient-to-br from-yellow-400 to-yellow-500 flex items-center justify-center text-zinc-900 font-bold text-lg sm:text-xl shadow-lg">
                        JS
                      </div>
                      <span className="text-[10px] sm:text-xs text-zinc-500">JavaScript</span>
                    </div>
                    {/* React */}
                    <div className="flex flex-col items-center gap-2">
                      <div className="h-12 w-12 sm:h-14 sm:w-14 rounded-xl bg-gradient-to-br from-cyan-400 to-cyan-600 flex items-center justify-center text-white shadow-lg">
                        <svg className="h-7 w-7 sm:h-8 sm:w-8" viewBox="0 0 24 24" fill="currentColor">
                          <path d="M12 10.11c1.03 0 1.87.84 1.87 1.89 0 1-.84 1.85-1.87 1.85-1.03 0-1.87-.85-1.87-1.85 0-1.05.84-1.89 1.87-1.89M7.37 20c.63.38 2.01-.2 3.6-1.7-.52-.59-1.03-1.23-1.51-1.9-.82-.08-1.63-.2-2.4-.36-.51 2.14-.32 3.61.31 3.96m.71-5.74l-.29-.51c-.11.29-.22.58-.29.86.27.06.57.11.88.16l-.3-.51m6.54-.76l.81-1.5-.81-1.5c-.3-.53-.62-1-.91-1.47C13.17 9 12.6 9 12 9c-.6 0-1.17 0-1.71.03-.29.47-.61.94-.91 1.47L8.57 12l.81 1.5c.3.53.62 1 .91 1.47.54.03 1.11.03 1.71.03.6 0 1.17 0 1.71-.03.29-.47.61-.94.91-1.47M12 6.78c-.19.22-.39.45-.59.72h1.18c-.2-.27-.4-.5-.59-.72m0 10.44c.19-.22.39-.45.59-.72h-1.18c.2.27.4.5.59.72M16.62 4c-.62-.38-2 .2-3.59 1.7.52.59 1.03 1.23 1.51 1.9.82.08 1.63.2 2.4.36.51-2.14.32-3.61-.32-3.96m-.7 5.74l.29.51c.11-.29.22-.58.29-.86-.27-.06-.57-.11-.88-.16l.3.51m1.45-7.05c1.47.84 1.63 3.05 1.01 5.63 2.54.75 4.37 1.99 4.37 3.68 0 1.69-1.83 2.93-4.37 3.68.62 2.58.46 4.79-1.01 5.63-1.46.84-3.45-.12-5.37-1.95-1.92 1.83-3.91 2.79-5.38 1.95-1.46-.84-1.62-3.05-1-5.63-2.54-.75-4.37-1.99-4.37-3.68 0-1.69 1.83-2.93 4.37-3.68-.62-2.58-.46-4.79 1-5.63 1.47-.84 3.46.12 5.38 1.95 1.92-1.83 3.91-2.79 5.37-1.95M17.08 12c.34.75.64 1.5.89 2.26 2.1-.63 3.28-1.53 3.28-2.26 0-.73-1.18-1.63-3.28-2.26-.25.76-.55 1.51-.89 2.26M6.92 12c-.34-.75-.64-1.5-.89-2.26-2.1.63-3.28 1.53-3.28 2.26 0 .73 1.18 1.63 3.28 2.26.25-.76.55-1.51.89-2.26m9 2.26l-.3.51c.31-.05.61-.1.88-.16-.07-.28-.18-.57-.29-.86l-.29.51m-9.82 1.67c.77.16 1.58.28 2.4.36.48-.67.99-1.31 1.51-1.9-1.59-1.5-2.97-2.08-3.6-1.7-.63.35-.82 1.82-.31 3.96m11.28-8.64c-.26-.06-.56-.11-.88-.16l.3-.51.29.51c.11-.29.22-.58.29-.86m-1.77-2.22c-.48.67-.99 1.31-1.51 1.9 1.59 1.5 2.97 2.08 3.59 1.7.64-.35.83-1.82.32-3.96-.77-.16-1.58-.28-2.4-.36z" />
                        </svg>
                      </div>
                      <span className="text-[10px] sm:text-xs text-zinc-500">React</span>
                    </div>
                    {/* Vue */}
                    <div className="flex flex-col items-center gap-2">
                      <div className="h-12 w-12 sm:h-14 sm:w-14 rounded-xl bg-gradient-to-br from-emerald-400 to-emerald-600 flex items-center justify-center text-white shadow-lg">
                        <svg className="h-6 w-6 sm:h-7 sm:w-7" viewBox="0 0 24 24" fill="currentColor">
                          <path d="M2 3h3.5L12 15l6.5-12H22L12 21 2 3m4.5 0h3L12 7.58 14.5 3h3L12 13.08 6.5 3z" />
                        </svg>
                      </div>
                      <span className="text-[10px] sm:text-xs text-zinc-500">Vue</span>
                    </div>
                  </div>
                </div>
              </div>
              <div className="order-1 md:order-2 flex flex-col justify-center">
                <h3 className="text-xl sm:text-2xl font-bold tracking-tight mb-4">Full Web Rendering</h3>
                <p className="text-sm sm:text-base text-zinc-600 dark:text-zinc-400 mb-6">
                  Headless Chrome for SPAs. JavaScript execution, dynamic content waiting. We handle the complexity so you don&apos;t have to.
                </p>
                <p className="text-sm font-medium text-indigo-600 dark:text-indigo-400">
                  React, Vue, Angular - if a browser can render it, we can extract it.
                </p>
              </div>
            </div>
          </div>
        </section>

        {/* Pricing Section */}
        <section id="pricing" className="border-t border-zinc-200/50 dark:border-zinc-800/50 bg-white/50 dark:bg-zinc-900/30 backdrop-blur-sm">
          <div className="container mx-auto px-4 py-24">
            <div className="text-center mb-16">
              <h2 className="text-2xl sm:text-3xl font-bold tracking-tight">
                Simple pricing
              </h2>
              <p className="mt-4 text-zinc-600 dark:text-zinc-400">
                Start free. Scale when ready.
              </p>
            </div>

            <PricingSection />
          </div>
        </section>

        {/* Footer */}
        <footer className="border-t border-zinc-200/50 dark:border-zinc-800/50 bg-white/50 dark:bg-zinc-900/30 backdrop-blur-sm">
          <div className="container mx-auto px-4 py-12">
            <div className="flex flex-col items-center justify-between gap-6 md:flex-row">
              <p className="text-sm text-zinc-500">
                Copyright 2026 <RefyneText />. All rights reserved.
              </p>
              <div className="flex gap-6">
                <Link href="/privacy" className="text-sm text-zinc-500 hover:text-zinc-900 dark:hover:text-white transition-colors">
                  Privacy
                </Link>
                <Link href="/terms" className="text-sm text-zinc-500 hover:text-zinc-900 dark:hover:text-white transition-colors">
                  Terms
                </Link>
              </div>
            </div>
          </div>
        </footer>
      </div>
    </div>
  );
}
