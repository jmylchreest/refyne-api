export interface Product {
  id: number;
  name: string;
  price: number;
  original_price?: number;
  description: string;
  category: string;
  brand: string;
  images: string[];
  thumbnail: string;
  rating: number;
  reviews_count: number;
  stock: number;
  discount_percentage: number;
  specs: Record<string, string>;
}

interface DummyJSONProduct {
  id: number;
  title: string;
  description: string;
  price: number;
  discountPercentage: number;
  rating: number;
  stock: number;
  brand: string;
  category: string;
  thumbnail: string;
  images: string[];
}

interface DummyJSONResponse {
  products: DummyJSONProduct[];
  total: number;
}

let cachedProducts: Product[] | null = null;

function formatCategory(category: string): string {
  return category
    .split('-')
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
    .join(' ');
}

function generateSpecs(category: string): Record<string, string> {
  const baseSpecs: Record<string, string> = {
    'Warranty': '1 Year',
    'Shipping': 'Free worldwide',
    'Return Policy': '30 days',
  };

  switch (category.toLowerCase()) {
    case 'smartphones':
      return {
        ...baseSpecs,
        'Storage': '128GB',
        'RAM': '8GB',
        'Display': '6.5" AMOLED',
        'Battery': '4500mAh',
      };
    case 'laptops':
      return {
        ...baseSpecs,
        'Processor': 'Intel Core i7',
        'RAM': '16GB',
        'Storage': '512GB SSD',
        'Display': '15.6" FHD',
      };
    case 'fragrances':
      return {
        ...baseSpecs,
        'Volume': '100ml',
        'Type': 'Eau de Parfum',
        'Notes': 'Floral, Woody',
      };
    case 'skincare':
      return {
        ...baseSpecs,
        'Size': '50ml',
        'Skin Type': 'All skin types',
        'Ingredients': 'Natural',
      };
    case 'groceries':
      return {
        ...baseSpecs,
        'Weight': '500g',
        'Organic': 'Yes',
        'Storage': 'Room temperature',
      };
    case 'home-decoration':
      return {
        ...baseSpecs,
        'Material': 'Premium quality',
        'Dimensions': 'Standard size',
        'Care': 'Easy clean',
      };
    default:
      return baseSpecs;
  }
}

export async function getProducts(): Promise<Product[]> {
  if (cachedProducts) return cachedProducts;

  try {
    const response = await fetch('https://dummyjson.com/products?limit=100');

    if (!response.ok) {
      throw new Error(`DummyJSON API error: ${response.status}`);
    }

    const data: DummyJSONResponse = await response.json();

    cachedProducts = data.products.map((product): Product => {
      const originalPrice = product.discountPercentage > 0
        ? Math.round(product.price / (1 - product.discountPercentage / 100))
        : undefined;

      return {
        id: product.id,
        name: product.title,
        price: product.price,
        original_price: originalPrice,
        description: product.description,
        category: formatCategory(product.category),
        brand: product.brand,
        images: product.images,
        thumbnail: product.thumbnail,
        rating: product.rating,
        reviews_count: Math.floor(product.rating * 50 + Math.random() * 200),
        stock: product.stock,
        discount_percentage: product.discountPercentage,
        specs: generateSpecs(product.category),
      };
    });

    return cachedProducts;
  } catch (error) {
    console.error('Failed to fetch products:', error);
    return getFallbackProducts();
  }
}

export async function getProductById(id: number): Promise<Product | undefined> {
  const products = await getProducts();
  return products.find((product) => product.id === id);
}

export async function getProductsByCategory(category: string): Promise<Product[]> {
  const products = await getProducts();
  return products.filter((product) => product.category === category);
}

export async function getProductCategories(): Promise<string[]> {
  const products = await getProducts();
  return [...new Set(products.map((product) => product.category))].sort();
}

export async function getProductBrands(): Promise<string[]> {
  const products = await getProducts();
  return [...new Set(products.map((product) => product.brand))].sort();
}

export async function getFeaturedProducts(): Promise<Product[]> {
  const products = await getProducts();
  return products
    .filter((product) => product.rating >= 4.5)
    .slice(0, 8);
}

function getFallbackProducts(): Product[] {
  return [
    {
      id: 1,
      name: 'Sample Product',
      price: 99.99,
      description: 'A sample product for demonstration purposes.',
      category: 'Electronics',
      brand: 'Demo Brand',
      images: ['https://api.dicebear.com/7.x/shapes/svg?seed=product1'],
      thumbnail: 'https://api.dicebear.com/7.x/shapes/svg?seed=product1',
      rating: 4.5,
      reviews_count: 150,
      stock: 100,
      discount_percentage: 0,
      specs: {
        'Warranty': '1 Year',
        'Shipping': 'Free worldwide',
      },
    },
  ];
}

export function formatPrice(price: number): string {
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
  }).format(price);
}

export function formatRating(rating: number): string {
  return rating.toFixed(1);
}

export function getStockStatus(stock: number): { label: string; color: string } {
  if (stock === 0) {
    return { label: 'Out of Stock', color: 'badge-red' };
  } else if (stock < 10) {
    return { label: 'Low Stock', color: 'badge-yellow' };
  } else {
    return { label: 'In Stock', color: 'badge-green' };
  }
}
