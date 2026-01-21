export type ReviewType = 'product' | 'service' | 'comment' | 'feedback';
export type Sentiment = 'positive' | 'neutral' | 'negative';

export interface Review {
  id: number;
  type: ReviewType;
  title?: string;
  body: string;
  rating?: number;
  likes: number;
  author: {
    id: number;
    name: string;
    username: string;
    avatar?: string;
  };
  product?: {
    id: number;
    name: string;
    category: string;
  };
  created_at: string;
  verified_purchase?: boolean;
  helpful_count?: number;
}

interface DummyJSONComment {
  id: number;
  body: string;
  postId: number;
  likes: number;
  user: {
    id: number;
    username: string;
    fullName: string;
  };
}

interface DummyJSONCommentsResponse {
  comments: DummyJSONComment[];
  total: number;
}

let cachedReviews: Review[] | null = null;

export async function getReviews(): Promise<Review[]> {
  if (cachedReviews) return cachedReviews;

  try {
    const response = await fetch('https://dummyjson.com/comments?limit=100');

    if (!response.ok) {
      throw new Error(`DummyJSON API error: ${response.status}`);
    }

    const data: DummyJSONCommentsResponse = await response.json();

    // Transform comments into reviews with mixed types
    const apiReviews = data.comments.map((comment, index): Review => {
      const types: ReviewType[] = ['product', 'service', 'comment', 'feedback'];
      const type = types[index % types.length];
      const hasRating = type === 'product' || type === 'service';

      return {
        id: comment.id,
        type,
        title: hasRating ? generateReviewTitle(comment.body) : undefined,
        body: comment.body,
        rating: hasRating ? generateRating(comment.likes) : undefined,
        likes: comment.likes,
        author: {
          id: comment.user.id,
          name: comment.user.fullName,
          username: comment.user.username,
          avatar: `https://dummyjson.com/icon/${comment.user.username}/150`,
        },
        product: type === 'product' ? generateProduct(index) : undefined,
        created_at: generateDate(index),
        verified_purchase: type === 'product' ? Math.random() > 0.3 : undefined,
        helpful_count: Math.floor(Math.random() * 50),
      };
    });

    // Add some more detailed product reviews
    cachedReviews = [...apiReviews, ...getDetailedReviews()];
    return cachedReviews;
  } catch (error) {
    console.error('Failed to fetch reviews:', error);
    return getFallbackReviews();
  }
}

export async function getReviewById(id: number): Promise<Review | undefined> {
  const reviews = await getReviews();
  return reviews.find((review) => review.id === id);
}

export async function getReviewsByType(type: ReviewType): Promise<Review[]> {
  const reviews = await getReviews();
  return reviews.filter((review) => review.type === type);
}

export async function getReviewsBySentiment(sentiment: Sentiment): Promise<Review[]> {
  const reviews = await getReviews();
  return reviews.filter((review) => analyzeSentiment(review) === sentiment);
}

export async function getReviewTypes(): Promise<ReviewType[]> {
  return ['product', 'service', 'comment', 'feedback'];
}

export async function getFeaturedReviews(): Promise<Review[]> {
  const reviews = await getReviews();
  return reviews
    .filter((r) => r.rating && r.rating >= 4)
    .slice(0, 6);
}

export async function getRecentReviews(): Promise<Review[]> {
  const reviews = await getReviews();
  return reviews.slice(0, 8);
}

// Sentiment analysis based on content and rating
export function analyzeSentiment(review: Review): Sentiment {
  const positiveWords = ['great', 'excellent', 'amazing', 'love', 'awesome', 'fantastic', 'perfect', 'wonderful', 'best', 'happy', 'impressed', 'recommend'];
  const negativeWords = ['bad', 'terrible', 'awful', 'hate', 'worst', 'disappointing', 'poor', 'horrible', 'broken', 'waste', 'useless', 'frustrated'];

  const text = (review.body + ' ' + (review.title || '')).toLowerCase();

  let score = 0;

  // Weight by rating if available
  if (review.rating !== undefined) {
    if (review.rating >= 4) score += 2;
    else if (review.rating <= 2) score -= 2;
  }

  // Weight by keywords
  positiveWords.forEach((word) => {
    if (text.includes(word)) score += 1;
  });

  negativeWords.forEach((word) => {
    if (text.includes(word)) score -= 1;
  });

  if (score > 0) return 'positive';
  if (score < 0) return 'negative';
  return 'neutral';
}

export function getSentimentColor(sentiment: Sentiment): string {
  switch (sentiment) {
    case 'positive':
      return 'badge-green';
    case 'neutral':
      return 'badge-gray';
    case 'negative':
      return 'badge-red';
    default:
      return 'badge-gray';
  }
}

export function getTypeColor(type: ReviewType): string {
  switch (type) {
    case 'product':
      return 'badge-blue';
    case 'service':
      return 'badge-purple';
    case 'comment':
      return 'badge-gray';
    case 'feedback':
      return 'badge-yellow';
    default:
      return 'badge-gray';
  }
}

export function formatReviewDate(dateStr: string): string {
  const date = new Date(dateStr);
  return date.toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  });
}

export function formatRelativeTime(dateStr: string): string {
  const date = new Date(dateStr);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24));

  if (diffDays === 0) return 'Today';
  if (diffDays === 1) return 'Yesterday';
  if (diffDays < 7) return `${diffDays} days ago`;
  if (diffDays < 30) return `${Math.floor(diffDays / 7)} weeks ago`;
  if (diffDays < 365) return `${Math.floor(diffDays / 30)} months ago`;
  return `${Math.floor(diffDays / 365)} years ago`;
}

function generateReviewTitle(body: string): string {
  const words = body.split(' ').slice(0, 4).join(' ');
  return words.charAt(0).toUpperCase() + words.slice(1) + '...';
}

function generateRating(likes: number): number {
  // Generate rating influenced by likes
  const base = 3 + (likes / 10);
  const rating = Math.min(5, Math.max(1, base + (Math.random() - 0.5)));
  return Math.round(rating * 2) / 2; // Round to nearest 0.5
}

function generateProduct(index: number): { id: number; name: string; category: string } {
  const products = [
    { id: 1, name: 'Wireless Headphones', category: 'Electronics' },
    { id: 2, name: 'Running Shoes', category: 'Sports' },
    { id: 3, name: 'Coffee Maker', category: 'Kitchen' },
    { id: 4, name: 'Laptop Stand', category: 'Office' },
    { id: 5, name: 'Smart Watch', category: 'Electronics' },
    { id: 6, name: 'Yoga Mat', category: 'Fitness' },
    { id: 7, name: 'Backpack', category: 'Travel' },
    { id: 8, name: 'Water Bottle', category: 'Sports' },
    { id: 9, name: 'Desk Lamp', category: 'Office' },
    { id: 10, name: 'Bluetooth Speaker', category: 'Electronics' },
  ];
  return products[index % products.length];
}

function generateDate(index: number): string {
  const daysAgo = Math.floor(Math.random() * 365);
  const date = new Date();
  date.setDate(date.getDate() - daysAgo);
  return date.toISOString();
}

function getDetailedReviews(): Review[] {
  return [
    {
      id: 1001,
      type: 'product',
      title: 'Best headphones I have ever owned!',
      body: 'These wireless headphones exceeded all my expectations. The sound quality is crystal clear, the noise cancellation is phenomenal, and the battery life lasts all day. I use them for work calls, music, and podcasts. Highly recommend to anyone looking for premium audio quality. The build quality feels premium and they are surprisingly comfortable for long listening sessions.',
      rating: 5,
      likes: 42,
      author: {
        id: 201,
        name: 'Sarah Mitchell',
        username: 'sarahm',
        avatar: 'https://dummyjson.com/icon/sarahm/150',
      },
      product: { id: 1, name: 'Premium Wireless Headphones Pro', category: 'Electronics' },
      created_at: '2024-01-15T10:30:00Z',
      verified_purchase: true,
      helpful_count: 38,
    },
    {
      id: 1002,
      type: 'product',
      title: 'Disappointed with quality',
      body: 'After reading great reviews, I was excited to try this product. Unfortunately, it broke within two weeks of normal use. The customer service was unresponsive and I had to wait over a month for a replacement. Very frustrated with this experience. Would not recommend based on durability issues.',
      rating: 1.5,
      likes: 5,
      author: {
        id: 202,
        name: 'James Wilson',
        username: 'jamesw',
        avatar: 'https://dummyjson.com/icon/jamesw/150',
      },
      product: { id: 3, name: 'Compact Coffee Maker', category: 'Kitchen' },
      created_at: '2024-01-10T14:20:00Z',
      verified_purchase: true,
      helpful_count: 22,
    },
    {
      id: 1003,
      type: 'service',
      title: 'Exceptional customer support',
      body: 'I had an issue with my order and reached out to customer support. They responded within an hour, resolved my problem, and even sent me a discount code for my next purchase. This is how customer service should be done! Their team was professional, friendly, and genuinely cared about my satisfaction.',
      rating: 5,
      likes: 28,
      author: {
        id: 203,
        name: 'Emily Chen',
        username: 'emilyc',
        avatar: 'https://dummyjson.com/icon/emilyc/150',
      },
      created_at: '2024-01-18T09:15:00Z',
      helpful_count: 15,
    },
    {
      id: 1004,
      type: 'feedback',
      title: 'Website needs improvement',
      body: 'The checkout process is confusing and I almost abandoned my cart twice. The mobile version is particularly difficult to navigate. Would appreciate a cleaner interface and faster loading times. The product selection is great, but the user experience needs work.',
      rating: 3,
      likes: 12,
      author: {
        id: 204,
        name: 'Michael Brown',
        username: 'mikeb',
        avatar: 'https://dummyjson.com/icon/mikeb/150',
      },
      created_at: '2024-01-20T16:45:00Z',
      helpful_count: 8,
    },
    {
      id: 1005,
      type: 'comment',
      body: 'Does anyone know if this comes in different colors? Looking for something that matches my home office setup. Thanks in advance for any help!',
      likes: 3,
      author: {
        id: 205,
        name: 'Lisa Taylor',
        username: 'lisat',
        avatar: 'https://dummyjson.com/icon/lisat/150',
      },
      created_at: '2024-01-22T11:00:00Z',
      helpful_count: 2,
    },
    {
      id: 1006,
      type: 'product',
      title: 'Good value for money',
      body: 'For the price point, this is a solid product. It does exactly what it says and nothing more. Build quality is decent, not premium but acceptable. If you are on a budget and need something functional, this is a good choice.',
      rating: 4,
      likes: 18,
      author: {
        id: 206,
        name: 'David Lee',
        username: 'davidl',
        avatar: 'https://dummyjson.com/icon/davidl/150',
      },
      product: { id: 4, name: 'Adjustable Laptop Stand', category: 'Office' },
      created_at: '2024-01-08T13:30:00Z',
      verified_purchase: true,
      helpful_count: 12,
    },
    {
      id: 1007,
      type: 'service',
      title: 'Terrible delivery experience',
      body: 'Package arrived damaged and a week late. No tracking updates were provided and when I called support, I was put on hold for 45 minutes. This is unacceptable. The product itself was fine but the whole experience left a bad taste.',
      rating: 2,
      likes: 8,
      author: {
        id: 207,
        name: 'Anna Martinez',
        username: 'annam',
        avatar: 'https://dummyjson.com/icon/annam/150',
      },
      created_at: '2024-01-05T08:20:00Z',
      helpful_count: 25,
    },
    {
      id: 1008,
      type: 'feedback',
      body: 'I think adding a wishlist feature would be really helpful. Sometimes I want to save items for later without adding them to my cart. Also, comparison tool between similar products would be nice.',
      likes: 15,
      author: {
        id: 208,
        name: 'Kevin Johnson',
        username: 'kevinj',
        avatar: 'https://dummyjson.com/icon/kevinj/150',
      },
      created_at: '2024-01-12T17:10:00Z',
      helpful_count: 11,
    },
  ];
}

function getFallbackReviews(): Review[] {
  return getDetailedReviews();
}
