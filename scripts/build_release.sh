#!/bin/bash
set -e

APP_NAME="jend"
VERSION="0.1.0"
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE=$(date +%FT%T%z)

# LDFLAGS to inject variables into root.go
LDFLAGS="-s -w -X main.version=$VERSION -X main.commit=$COMMIT -X main.date=$DATE"

# Output directory
mkdir -p dist

echo "Building JEND v$VERSION ($COMMIT)..."

# 1. MacOS (Apple Silicon)
echo "Building Darwin ARM64..."
GOOS=darwin GOARCH=arm64 go build -ldflags "$LDFLAGS" -o dist/${APP_NAME}-darwin-arm64 ./cmd/jend

# 2. MacOS (Intel)
echo "Building Darwin AMD64..."
GOOS=darwin GOARCH=amd64 go build -ldflags "$LDFLAGS" -o dist/${APP_NAME}-darwin-amd64 ./cmd/jend

# 3. Linux (AMD64)
echo "Building Linux AMD64..."
GOOS=linux GOARCH=amd64 go build -ldflags "$LDFLAGS" -o dist/${APP_NAME}-linux-amd64 ./cmd/jend

# 4. Windows (AMD64)
echo "Building Windows AMD64..."
GOOS=windows GOARCH=amd64 go build -ldflags "$LDFLAGS" -o dist/${APP_NAME}-windows-amd64.exe ./cmd/jend

echo "Build complete! Artifacts in dist/"
ls -lh dist/
