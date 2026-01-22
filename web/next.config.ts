import type { NextConfig } from 'next';

const nextConfig: NextConfig = {
  reactStrictMode: true,
  images: {
    unoptimized: true, // Disable image optimization to exclude sharp from bundle
  },
  experimental: {
    // Optimize imports for packages that don't tree-shake well
    optimizePackageImports: [
      'lucide-react',
      'recharts',
      'date-fns',
      '@radix-ui/react-icons',
    ],
  },
};

export default nextConfig;
