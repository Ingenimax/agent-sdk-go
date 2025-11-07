import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // Enable static export for embedding in Go binary
  output: 'export',

  // Disable image optimization for static export
  images: {
    unoptimized: true,
  },

  // Set base path for serving from Go server
  basePath: '',

  // Ensure trailing slash is not added
  trailingSlash: false,

  // Disable server features for static export
  typescript: {
    ignoreBuildErrors: false,
  },

  // Optimize for production
  experimental: {
    optimizePackageImports: ['lucide-react'],
  },
};

export default nextConfig;
