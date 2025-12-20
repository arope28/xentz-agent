#!/bin/bash
# build.sh - Build executables for all platforms and architectures

set -e

VERSION=${VERSION:-$(date +%Y%m%d)}
DIST_DIR="dist"
mkdir -p "$DIST_DIR"

echo "Building xentz-agent for all platforms..."
echo "Version: $VERSION"
echo ""

# macOS - Intel (amd64)
echo "Building for macOS (Intel/amd64)..."
GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o "$DIST_DIR/xentz-agent-darwin-amd64" ./cmd/xentz-agent

# macOS - Apple Silicon (arm64)
echo "Building for macOS (Apple Silicon/arm64)..."
GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o "$DIST_DIR/xentz-agent-darwin-arm64" ./cmd/xentz-agent

# macOS - Universal binary (works on both Intel and Apple Silicon)
echo "Building for macOS (Universal binary)..."
if command -v lipo &> /dev/null; then
    lipo -create -output "$DIST_DIR/xentz-agent-darwin-universal" \
        "$DIST_DIR/xentz-agent-darwin-amd64" \
        "$DIST_DIR/xentz-agent-darwin-arm64"
    echo "  ✓ Universal binary created"
else
    echo "  ⚠ lipo not found, skipping universal binary (macOS only)"
fi

# Windows - amd64
echo "Building for Windows (amd64)..."
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o "$DIST_DIR/xentz-agent-windows-amd64.exe" ./cmd/xentz-agent

# Windows - arm64 (Windows on ARM)
echo "Building for Windows (arm64)..."
GOOS=windows GOARCH=arm64 go build -ldflags="-s -w" -o "$DIST_DIR/xentz-agent-windows-arm64.exe" ./cmd/xentz-agent

# Linux - amd64
echo "Building for Linux (amd64)..."
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o "$DIST_DIR/xentz-agent-linux-amd64" ./cmd/xentz-agent

# Linux - arm64
echo "Building for Linux (arm64)..."
GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o "$DIST_DIR/xentz-agent-linux-arm64" ./cmd/xentz-agent

# Linux - ARMv7 (32-bit, for Raspberry Pi and older ARM devices)
echo "Building for Linux (ARMv7)..."
GOOS=linux GOARCH=arm GOARM=7 go build -ldflags="-s -w" -o "$DIST_DIR/xentz-agent-linux-armv7" ./cmd/xentz-agent

# FreeBSD - amd64 (optional, for completeness)
if command -v go &> /dev/null && go env GOOS | grep -q darwin; then
    echo "Building for FreeBSD (amd64)..."
    GOOS=freebsd GOARCH=amd64 go build -ldflags="-s -w" -o "$DIST_DIR/xentz-agent-freebsd-amd64" ./cmd/xentz-agent || echo "  ⚠ FreeBSD build skipped (may require cross-compilation tools)"
fi

echo ""
echo "Build complete! Executables are in ./$DIST_DIR/"
echo ""
echo "Files created:"
ls -lh "$DIST_DIR" | grep xentz-agent

