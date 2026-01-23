import Parser from 'rss-parser';

export interface NewsItem {
  id: string;
  title: string;
  source: string;
  url: string;
  published_at: Date;
  summary: string;
  category: string;
}

interface HNItem {
  id: number;
  title: string;
  url?: string;
  time: number;
  score: number;
}

const parser = new Parser();

let cachedNews: NewsItem[] | null = null;

async function fetchHackerNews(): Promise<NewsItem[]> {
  try {
    const response = await fetch('https://hacker-news.firebaseio.com/v0/topstories.json');
    const ids: number[] = await response.json();

    const topIds = ids.slice(0, 20);

    const items = await Promise.all(
      topIds.map(async (id) => {
        const itemResponse = await fetch(`https://hacker-news.firebaseio.com/v0/item/${id}.json`);
        return itemResponse.json() as Promise<HNItem>;
      })
    );

    return items
      .filter((item) => item && item.url)
      .map((item): NewsItem => ({
        id: `hn-${item.id}`,
        title: item.title,
        source: 'Hacker News',
        url: item.url!,
        published_at: new Date(item.time * 1000),
        summary: `Score: ${item.score} points`,
        category: 'Technology',
      }));
  } catch (error) {
    console.error('Failed to fetch Hacker News:', error);
    return [];
  }
}

async function fetchRSSFeed(feedUrl: string, source: string, category: string): Promise<NewsItem[]> {
  try {
    const feed = await parser.parseURL(feedUrl);

    return feed.items.slice(0, 15).map((item, index): NewsItem => ({
      id: `${source.toLowerCase().replace(/\s+/g, '-')}-${index}`,
      title: item.title || 'Untitled',
      source,
      url: item.link || '#',
      published_at: item.pubDate ? new Date(item.pubDate) : new Date(),
      summary: item.contentSnippet?.slice(0, 200) || item.content?.slice(0, 200) || '',
      category,
    }));
  } catch (error) {
    console.error(`Failed to fetch RSS from ${source}:`, error);
    return [];
  }
}

export async function getNews(): Promise<NewsItem[]> {
  if (cachedNews) return cachedNews;

  const [hnNews, techCrunch, bbc] = await Promise.all([
    fetchHackerNews(),
    fetchRSSFeed('https://techcrunch.com/feed/', 'TechCrunch', 'Startups'),
    fetchRSSFeed('https://feeds.bbci.co.uk/news/technology/rss.xml', 'BBC Tech', 'Technology'),
  ]);

  cachedNews = [...hnNews, ...techCrunch, ...bbc]
    .sort((a, b) => b.published_at.getTime() - a.published_at.getTime());

  return cachedNews;
}

export async function getNewsById(id: string): Promise<NewsItem | undefined> {
  const news = await getNews();
  return news.find((item) => item.id === id);
}

export async function getNewsByCategory(category: string): Promise<NewsItem[]> {
  const news = await getNews();
  return news.filter((item) => item.category === category);
}

export async function getNewsBySource(source: string): Promise<NewsItem[]> {
  const news = await getNews();
  return news.filter((item) => item.source === source);
}

export async function getNewsCategories(): Promise<string[]> {
  const news = await getNews();
  return [...new Set(news.map((item) => item.category))].sort();
}

export async function getNewsSources(): Promise<string[]> {
  const news = await getNews();
  return [...new Set(news.map((item) => item.source))].sort();
}

export function formatRelativeTime(date: Date): string {
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffMins = Math.floor(diffMs / 60000);
  const diffHours = Math.floor(diffMs / 3600000);
  const diffDays = Math.floor(diffMs / 86400000);

  if (diffMins < 60) {
    return `${diffMins} minute${diffMins !== 1 ? 's' : ''} ago`;
  } else if (diffHours < 24) {
    return `${diffHours} hour${diffHours !== 1 ? 's' : ''} ago`;
  } else if (diffDays < 7) {
    return `${diffDays} day${diffDays !== 1 ? 's' : ''} ago`;
  } else {
    return date.toLocaleDateString('en-US', {
      month: 'short',
      day: 'numeric',
      year: diffDays > 365 ? 'numeric' : undefined,
    });
  }
}

export function getSourceColor(source: string): string {
  switch (source) {
    case 'Hacker News':
      return 'badge-yellow';
    case 'TechCrunch':
      return 'badge-green';
    case 'BBC Tech':
      return 'badge-red';
    default:
      return 'badge-gray';
  }
}
