#!/bin/bash
# install.sh - Universal installer that detects OS/arch and downloads the correct binary
# Usage: curl -fsSL https://your-domain.com/install.sh | bash

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration - Update these URLs to point to your release binaries
BASE_URL="${XENTZ_AGENT_BASE_URL:-https://github.com/arope28/xentz-agent/releases/latest/download}"
INSTALL_DIR="${HOME}/.local/bin"
BINARY_NAME="xentz-agent"

echo -e "${GREEN}xentz-agent Installer${NC}"
echo "========================"
echo ""

# Detect OS and architecture
detect_platform() {
    local os=""
    local arch=""
    local ext=""

    # Detect OS
    case "$(uname -s)" in
        Linux*)
            os="linux"
            ;;
        Darwin*)
            os="darwin"
            ;;
        *)
            echo -e "${RED}Error: Unsupported operating system: $(uname -s)${NC}"
            exit 1
            ;;
    esac

    # Detect architecture
    case "$(uname -m)" in
        x86_64|amd64)
            arch="amd64"
            ;;
        arm64|aarch64)
            arch="arm64"
            ;;
        armv7l|armv6l)
            arch="armv7"
            ;;
        *)
            echo -e "${RED}Error: Unsupported architecture: $(uname -m)${NC}"
            exit 1
            ;;
    esac

    # Special case: macOS universal binary
    if [ "$os" = "darwin" ] && [ -f "/usr/bin/lipo" ]; then
        # Check if we can use universal binary
        echo -e "${YELLOW}Note: Universal binary available for macOS${NC}"
        # For now, use architecture-specific, but could prefer universal
    fi

    echo "$os-$arch"
}

PLATFORM=$(detect_platform)
OS=$(echo $PLATFORM | cut -d'-' -f1)
ARCH=$(echo $PLATFORM | cut -d'-' -f2)

echo "Detected: $OS ($ARCH)"
echo ""

# Determine binary name
if [ "$OS" = "windows" ]; then
    BINARY_FILE="${BINARY_NAME}-${OS}-${ARCH}.exe"
else
    BINARY_FILE="${BINARY_NAME}-${OS}-${ARCH}"
fi

# For macOS, prefer universal binary if available
if [ "$OS" = "darwin" ]; then
    UNIVERSAL_FILE="${BINARY_NAME}-darwin-universal"
    # Try universal first, fallback to arch-specific
    if curl -fsSL -o /dev/null --head "${BASE_URL}/${UNIVERSAL_FILE}"; then
        BINARY_FILE="$UNIVERSAL_FILE"
        echo "Using universal binary for macOS"
    fi
fi

DOWNLOAD_URL="${BASE_URL}/${BINARY_FILE}"
echo "Downloading from: $DOWNLOAD_URL"
echo ""

# Create install directory
mkdir -p "$INSTALL_DIR"

# Download binary
echo "Downloading xentz-agent..."
if ! curl -fsSL -o "${INSTALL_DIR}/${BINARY_NAME}" "$DOWNLOAD_URL"; then
    echo -e "${RED}Error: Failed to download binary${NC}"
    echo "Please check that the release exists at: $DOWNLOAD_URL"
    exit 1
fi

# Make executable
chmod +x "${INSTALL_DIR}/${BINARY_NAME}"

echo -e "${GREEN}✓ Installation complete!${NC}"
echo ""
echo "Binary installed to: ${INSTALL_DIR}/${BINARY_NAME}"
echo ""

# Check if install directory is in PATH
if [[ ":$PATH:" != *":${INSTALL_DIR}:"* ]]; then
    echo -e "${YELLOW}Note: ${INSTALL_DIR} is not in your PATH${NC}"
    echo "Add this to your ~/.bashrc, ~/.zshrc, or ~/.profile:"
    echo "  export PATH=\"\${HOME}/.local/bin:\$PATH\""
    echo ""
    echo "Or run the agent directly:"
    echo "  ${INSTALL_DIR}/${BINARY_NAME} --help"
else
    echo "You can now run:"
    echo "  ${BINARY_NAME} --help"
fi

echo ""
echo "Next steps:"
echo "  1. Install restic: brew install restic  (macOS) or apt install restic (Linux)"
echo "  2. Run: ${BINARY_NAME} install --repo <your-repo> --password <pwd> --include <paths>"

