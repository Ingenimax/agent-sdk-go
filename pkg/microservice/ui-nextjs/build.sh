#!/bin/bash

# Build script for Next.js UI to be embedded in Go binary

set -e

echo "🔧 Building Next.js UI for Go embedding..."

# Install dependencies
echo "📦 Installing dependencies..."
npm install

# Type check
echo "🔍 Type checking..."
npm run type-check

# Lint code
echo "🧹 Linting..."
npm run lint

# Build for production
echo "🏗️  Building for production..."
npm run build

echo "✅ Build complete! Files generated in out/ directory"
echo "🚀 Ready for Go binary embedding"

# List the generated files
echo ""
echo "Generated files:"
ls -la out/
