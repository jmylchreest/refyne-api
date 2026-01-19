export interface BlogPost {
  id: number;
  slug: string;
  title: string;
  author: string;
  author_avatar: string;
  published_at: Date;
  excerpt: string;
  content: string;
  tags: string[];
  category: string;
  image_url: string;
  reading_time: number;
  url: string;
}

interface DevToArticle {
  id: number;
  title: string;
  description: string;
  slug: string;
  body_markdown?: string;
  body_html?: string;
  user: {
    name: string;
    profile_image: string;
  };
  published_at: string;
  tag_list: string[];
  cover_image: string | null;
  reading_time_minutes: number;
  url: string;
}

const CATEGORY_MAP: Record<string, string> = {
  javascript: 'Development',
  typescript: 'Development',
  react: 'Development',
  webdev: 'Development',
  programming: 'Development',
  python: 'Development',
  go: 'Development',
  rust: 'Development',
  devops: 'DevOps',
  cloud: 'DevOps',
  aws: 'DevOps',
  docker: 'DevOps',
  kubernetes: 'DevOps',
  tutorial: 'Tutorials',
  beginners: 'Tutorials',
  career: 'Career',
  productivity: 'Career',
  security: 'Security',
  ai: 'AI & ML',
  machinelearning: 'AI & ML',
};

function categorizePost(tags: string[]): string {
  for (const tag of tags) {
    const category = CATEGORY_MAP[tag.toLowerCase()];
    if (category) return category;
  }
  return 'General';
}

let cachedPosts: BlogPost[] | null = null;

export async function getBlogPosts(): Promise<BlogPost[]> {
  if (cachedPosts) return cachedPosts;

  try {
    const response = await fetch('https://dev.to/api/articles?per_page=50&top=7', {
      headers: {
        'Accept': 'application/json',
      },
    });

    if (!response.ok) {
      throw new Error(`Dev.to API error: ${response.status}`);
    }

    const articles: DevToArticle[] = await response.json();

    cachedPosts = articles.map((article): BlogPost => ({
      id: article.id,
      slug: article.slug,
      title: article.title,
      author: article.user.name,
      author_avatar: article.user.profile_image,
      published_at: new Date(article.published_at),
      excerpt: article.description || '',
      content: article.body_html || article.body_markdown || '',
      tags: article.tag_list,
      category: categorizePost(article.tag_list),
      image_url: article.cover_image || `https://api.dicebear.com/7.x/shapes/svg?seed=${article.id}&backgroundColor=6366f1`,
      reading_time: article.reading_time_minutes,
      url: article.url,
    }));

    return cachedPosts;
  } catch (error) {
    console.error('Failed to fetch blog posts:', error);
    return getFallbackPosts();
  }
}

export async function getBlogPostBySlug(slug: string): Promise<BlogPost | undefined> {
  const posts = await getBlogPosts();
  return posts.find((post) => post.slug === slug);
}

export async function getBlogPostsByCategory(category: string): Promise<BlogPost[]> {
  const posts = await getBlogPosts();
  return posts.filter((post) => post.category === category);
}

export async function getBlogCategories(): Promise<string[]> {
  const posts = await getBlogPosts();
  return [...new Set(posts.map((post) => post.category))].sort();
}

export async function getPopularTags(): Promise<string[]> {
  const posts = await getBlogPosts();
  const tagCounts = new Map<string, number>();

  posts.forEach((post) => {
    post.tags.forEach((tag) => {
      tagCounts.set(tag, (tagCounts.get(tag) || 0) + 1);
    });
  });

  return [...tagCounts.entries()]
    .sort((a, b) => b[1] - a[1])
    .slice(0, 10)
    .map(([tag]) => tag);
}

// Fallback posts in case API fails
function getFallbackPosts(): BlogPost[] {
  return [
    {
      id: 1,
      slug: 'getting-started-with-typescript',
      title: 'Getting Started with TypeScript in 2024',
      author: 'Demo Author',
      author_avatar: 'https://api.dicebear.com/7.x/avataaars/svg?seed=demo1',
      published_at: new Date(),
      excerpt: 'A comprehensive guide to getting started with TypeScript for modern web development.',
      content: '<p>TypeScript has become an essential tool for modern web development...</p>',
      tags: ['typescript', 'javascript', 'webdev'],
      category: 'Development',
      image_url: 'https://api.dicebear.com/7.x/shapes/svg?seed=typescript&backgroundColor=6366f1',
      reading_time: 5,
      url: '#',
    },
    {
      id: 2,
      slug: 'introduction-to-astro',
      title: 'Introduction to Astro: The Modern Static Site Generator',
      author: 'Demo Author',
      author_avatar: 'https://api.dicebear.com/7.x/avataaars/svg?seed=demo2',
      published_at: new Date(),
      excerpt: 'Learn how Astro can help you build faster websites with less JavaScript.',
      content: '<p>Astro is a modern static site generator that delivers lightning-fast performance...</p>',
      tags: ['astro', 'webdev', 'javascript'],
      category: 'Development',
      image_url: 'https://api.dicebear.com/7.x/shapes/svg?seed=astro&backgroundColor=6366f1',
      reading_time: 7,
      url: '#',
    },
  ];
}

export function formatDate(date: Date): string {
  return date.toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'long',
    day: 'numeric',
  });
}
