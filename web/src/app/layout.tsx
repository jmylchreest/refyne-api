import type { Metadata } from 'next';
import { Fira_Code, Victor_Mono } from 'next/font/google';
import './globals.css';
import { Toaster } from '@/components/ui/sonner';
import { ApiProvider } from '@/lib/api-provider';
import { ThemeProvider } from '@/components/theme-provider';
import { ClerkThemeProvider } from '@/components/clerk-theme-provider';

// Main site font - Fira Code
const firaCode = Fira_Code({
  subsets: ['latin'],
  variable: '--font-mono',
  display: 'swap',
});

// Cursive mono for Refyne branding - Victor Mono italic
// Note: Cascadia Code isn't on Google Fonts (it's a Microsoft font)
const victorMono = Victor_Mono({
  subsets: ['latin'],
  style: ['normal', 'italic'],
  variable: '--font-cursive',
  display: 'swap',
});

// SEO Configuration
const siteConfig = {
  name: 'Refyne',
  url: 'https://refyne.uk',
  title: 'Refyne - AI-Powered Web Scraping API | Extract Structured Data',
  description: 'Transform any website into structured JSON with AI-powered extraction. Refyne is a web scraping API that uses LLMs to extract, parse, and structure data from any webpage. No CSS selectors needed.',
  // Extended keywords targeting competitor alternatives and use cases
  keywords: [
    // Primary keywords
    'web scraping API',
    'screen scraping',
    'automated website scraping',
    'AI web scraper',
    'web data extraction API',
    'website to JSON',
    'LLM data extraction',
    'structured data API',
    // Technical keywords
    'headless browser API',
    'web crawler API',
    'HTML to JSON converter',
    'website parser API',
    'data scraping service',
    'web content extraction',
    'automated data collection',
    // Use case keywords
    'no-code web scraper',
    'web scraper for developers',
    'e-commerce data extraction',
    'lead generation scraping',
    'price monitoring API',
    'product data extraction',
    'news aggregation API',
    // Competitor alternatives
    'Scrapy alternative',
    'Beautiful Soup alternative',
    'Puppeteer alternative',
    'Playwright alternative',
    'Selenium alternative',
    'Apify alternative',
    'Bright Data alternative',
    'ScrapingBee alternative',
    'Octoparse alternative',
    'ParseHub alternative',
    'Import.io alternative',
    'Diffbot alternative',
    'Mozenda alternative',
    'Crawlera alternative',
    'Zyte alternative',
    'ScraperAPI alternative',
    'Firecrawl alternative',
    // AI/LLM specific
    'GPT web scraper',
    'Claude web scraper',
    'AI-powered data extraction',
    'natural language web scraping',
    'intelligent web scraper',
    'machine learning scraper',
  ].join(', '),
};

export const metadata: Metadata = {
  metadataBase: new URL(siteConfig.url),
  title: {
    default: siteConfig.title,
    template: `%s | ${siteConfig.name}`,
  },
  description: siteConfig.description,
  keywords: siteConfig.keywords,
  authors: [{ name: siteConfig.name }],
  creator: siteConfig.name,
  publisher: siteConfig.name,
  robots: {
    index: true,
    follow: true,
    googleBot: {
      index: true,
      follow: true,
      'max-video-preview': -1,
      'max-image-preview': 'large',
      'max-snippet': -1,
    },
  },
  openGraph: {
    type: 'website',
    locale: 'en_GB',
    url: siteConfig.url,
    siteName: siteConfig.name,
    title: siteConfig.title,
    description: siteConfig.description,
    images: [
      {
        url: '/og-image.png',
        width: 1200,
        height: 630,
        alt: 'Refyne - AI-Powered Web Scraping API',
      },
    ],
  },
  twitter: {
    card: 'summary_large_image',
    title: siteConfig.title,
    description: siteConfig.description,
    images: ['/twitter-image.png'],
    creator: '@refaborin',
  },
  alternates: {
    canonical: siteConfig.url,
  },
  category: 'technology',
  icons: {
    icon: '/favicon.svg',
    shortcut: '/favicon.svg',
    apple: '/favicon.svg',
  },
};

// JSON-LD Structured Data
const jsonLd = {
  '@context': 'https://schema.org',
  '@graph': [
    {
      '@type': 'Organization',
      '@id': `${siteConfig.url}/#organization`,
      name: siteConfig.name,
      url: siteConfig.url,
      logo: {
        '@type': 'ImageObject',
        url: `${siteConfig.url}/og-image.png`,
      },
      sameAs: [
        'https://github.com/jmylchreest/refyne-api',
      ],
    },
    {
      '@type': 'WebSite',
      '@id': `${siteConfig.url}/#website`,
      url: siteConfig.url,
      name: siteConfig.name,
      description: siteConfig.description,
      publisher: {
        '@id': `${siteConfig.url}/#organization`,
      },
    },
    {
      '@type': 'SoftwareApplication',
      '@id': `${siteConfig.url}/#software`,
      name: 'Refyne API',
      applicationCategory: 'DeveloperApplication',
      operatingSystem: 'Web',
      description: 'AI-powered web scraping API that extracts structured data from any website using LLMs.',
      offers: {
        '@type': 'Offer',
        price: '0',
        priceCurrency: 'USD',
        description: 'Free tier available with BYOK (Bring Your Own Key) option',
      },
      featureList: [
        'AI-powered data extraction',
        'No CSS selectors required',
        'JSON schema output',
        'Multi-page crawling',
        'BYOK support for LLM providers',
        'RESTful API',
        'TypeScript, Python, Go SDKs',
      ],
    },
  ],
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en" suppressHydrationWarning>
      <head>
        <script
          type="application/ld+json"
          dangerouslySetInnerHTML={{ __html: JSON.stringify(jsonLd) }}
        />
      </head>
      <body className={`${firaCode.variable} ${victorMono.variable} font-mono antialiased`}>
        <ThemeProvider
          attribute="class"
          defaultTheme="system"
          enableSystem
          disableTransitionOnChange
        >
          <ClerkThemeProvider>
            <ApiProvider>
              {children}
            </ApiProvider>
            <Toaster />
          </ClerkThemeProvider>
        </ThemeProvider>
      </body>
    </html>
  );
}
