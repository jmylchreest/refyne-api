import { MetadataRoute } from 'next';
import { source } from '@/lib/source';

// Required for static export
export const dynamic = 'force-static';

const BASE_URL = 'https://docs.refyne.uk';

export default async function sitemap(): Promise<MetadataRoute.Sitemap> {
  // Documentation pages from Fumadocs
  return source.getPages().map((page) => ({
    url: `${BASE_URL}${page.url}`,
    lastModified: new Date(),
    changeFrequency: 'weekly' as const,
    priority: page.url === '/' ? 1 : 0.8,
  }));
}
