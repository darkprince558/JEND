#!/bin/sh
set -e

OWNER="darkprince558"
REPO="jend"
BINARY="jend"
INSTALL_DIR="/usr/local/bin"

# Detect OS
OS="$(uname -s)"
case "${OS}" in
    Linux*)     OS='linux';;
    Darwin*)    OS='darwin';;
    CYGWIN*)    OS='windows';;
    MINGW*)     OS='windows';;
    *)          echo "Unknown OS: ${OS}"; exit 1;;
esac

# Detect Arch
ARCH="$(uname -m)"
case "${ARCH}" in
    x86_64)    ARCH='x86_64';;
    arm64)     ARCH='arm64';;
    aarch64)   ARCH='arm64';;
    i386)      ARCH='i386';;
    *)         echo "Unknown Architecture: ${ARCH}"; exit 1;;
esac

echo "Detected ${OS} ${ARCH}..."

# Get Latest Tag
LATEST_URL="https://api.github.com/repos/${OWNER}/${REPO}/releases/latest"
echo "Fetching latest release info..."
# Ensure we get the browser_download_url matching our OS/Arch
# Naming pattern: jend_Darwin_arm64.tar.gz or jend_Linux_x86_64.tar.gz
# Note: GoReleaser title-cases OS (Darwin/Linux)

OS_TITLE="$(echo "$OS" | awk '{print toupper(substr($0,1,1)) substr($0,2)}')"
ASSET_PATTERN="${BINARY}_${OS_TITLE}_${ARCH}.tar.gz"

DOWNLOAD_URL=$(curl -s $LATEST_URL | grep "browser_download_url" | grep "$ASSET_PATTERN" | cut -d '"' -f 4)

if [ -z "$DOWNLOAD_URL" ]; then
    echo "Error: Could not find release asset for ${ASSET_PATTERN}"
    exit 1
fi

echo "Downloading $DOWNLOAD_URL..."
curl -L -o /tmp/jend.tar.gz "$DOWNLOAD_URL"

echo "Extracting..."
tar -xzf /tmp/jend.tar.gz -C /tmp/ "$BINARY"

echo "Installing to $INSTALL_DIR (requires sudo)..."
sudo mv "/tmp/$BINARY" "$INSTALL_DIR/$BINARY"
chmod +x "$INSTALL_DIR/$BINARY"

echo "Success! Run '$BINARY --help' to get started."
