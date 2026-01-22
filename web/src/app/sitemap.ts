import { MetadataRoute } from 'next';

const BASE_URL = 'https://refyne.uk';

export default async function sitemap(): Promise<MetadataRoute.Sitemap> {
  // Static pages for main site
  // Documentation is now at docs.refyne.uk (separate sitemap)
  return [
    {
      url: BASE_URL,
      lastModified: new Date(),
      changeFrequency: 'weekly',
      priority: 1,
    },
    {
      url: `${BASE_URL}/privacy`,
      lastModified: new Date('2025-01-01'),
      changeFrequency: 'monthly',
      priority: 0.3,
    },
    {
      url: `${BASE_URL}/terms`,
      lastModified: new Date('2025-01-01'),
      changeFrequency: 'monthly',
      priority: 0.3,
    },
  ];
}
