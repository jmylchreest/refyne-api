import type { Metadata } from 'next';
import { Fira_Code, Victor_Mono } from 'next/font/google';
import './globals.css';
import { ThemeProvider } from '@/components/theme-provider';
import { ClerkProvider } from '@/components/clerk-provider';

// Main site font - Fira Code
const firaCode = Fira_Code({
  subsets: ['latin'],
  variable: '--font-mono',
  display: 'swap',
});

// Cursive mono for Refyne branding - Victor Mono italic
const victorMono = Victor_Mono({
  subsets: ['latin'],
  style: ['normal', 'italic'],
  variable: '--font-cursive',
  display: 'swap',
});

// SEO Configuration
const siteConfig = {
  name: 'Refyne Docs',
  url: 'https://docs.refyne.uk',
  title: 'Documentation | Refyne - AI-Powered Web Scraping API',
  description: 'Complete documentation for Refyne API. Learn how to extract structured data from any website using AI-powered extraction with our comprehensive guides and API reference.',
  keywords: [
    'Refyne API documentation',
    'web scraping API docs',
    'AI data extraction guide',
    'structured data extraction API',
    'web scraping tutorial',
    'API reference',
    'developer documentation',
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
  authors: [{ name: 'Refyne' }],
  creator: 'Refyne',
  publisher: 'Refyne',
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
  },
  twitter: {
    card: 'summary_large_image',
    title: siteConfig.title,
    description: siteConfig.description,
    creator: '@refaborin',
  },
  alternates: {
    canonical: siteConfig.url,
  },
  category: 'technology',
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en" suppressHydrationWarning>
      <body className={`${firaCode.variable} ${victorMono.variable} font-mono antialiased`}>
        <ThemeProvider
          attribute="class"
          defaultTheme="system"
          enableSystem
          disableTransitionOnChange
        >
          <ClerkProvider>
            {children}
          </ClerkProvider>
        </ThemeProvider>
      </body>
    </html>
  );
}
