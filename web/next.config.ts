import type { NextConfig } from 'next';
import { execSync } from 'child_process';

// Get git commit hash at build time
function getGitCommit(): string {
  try {
    return execSync('git rev-parse --short HEAD').toString().trim();
  } catch {
    return 'unknown';
  }
}

// Get git tag if on a tag, otherwise return empty
function getGitTag(): string {
  try {
    return execSync('git describe --tags --exact-match 2>/dev/null').toString().trim();
  } catch {
    return '';
  }
}

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
  env: {
    NEXT_PUBLIC_GIT_COMMIT: getGitCommit(),
    NEXT_PUBLIC_GIT_TAG: getGitTag(),
    NEXT_PUBLIC_BUILD_TIME: new Date().toISOString(),
  },
};

export default nextConfig;
