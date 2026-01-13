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

export const metadata: Metadata = {
  title: 'Refyne - AI-Powered API for the Structured Web',
  description: 'The web is chaos. Your data should not be. Refyne transforms unstructured websites into clean, typed JSON using LLM-powered extraction.',
  keywords: 'web scraping, data extraction, LLM, API, structured data, web data, AI scraper, JSON API',
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
