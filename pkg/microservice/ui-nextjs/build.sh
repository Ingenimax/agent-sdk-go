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

# Copy built files to the correct location for Go embedding
echo "📁 Copying files to dist directory..."
if [ -d "../ui/dist" ]; then
    rm -rf ../ui/dist/*
else
    mkdir -p ../ui/dist
fi

# Copy all files from out directory to dist
cp -r out/* ../ui/dist/

echo "✅ Build complete! Files copied to ../ui/dist/"
echo "🚀 Ready for Go binary embedding"

# List the generated files
echo ""
echo "Generated files:"
ls -la ../ui/dist/