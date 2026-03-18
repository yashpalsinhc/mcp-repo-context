#!/usr/bin/env bash
# Install mcp-repo-context from GitHub Releases (versioned binaries with checksums).
# Usage: curl -sSL https://raw.githubusercontent.com/yashpalsinhc/mcp-repo-context/main/scripts/install.sh | bash
#    or: ./install.sh [VERSION] [DEST_DIR]
#    or: VERSION=v1.0.0 DEST=/usr/local/bin ./install.sh
#
# Requires: curl, tar (or unzip on Windows), sha256sum (or shasum on macOS)

set -e

REPO="${MCP_REPO_CONTEXT_REPO:-yashpalsinhc/mcp-repo-context}"
VERSION="${1:-${VERSION:-latest}}"
DEST="${2:-${DEST:-./bin}}"

# Resolve latest tag
get_latest_tag() {
  curl -sSL "https://api.github.com/repos/${REPO}/releases/latest" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p'
}

if [ "$VERSION" = "latest" ]; then
  VERSION="$(get_latest_tag)"
fi

# Normalize version (strip leading v if present for asset names)
VER_STRIP="${VERSION#v}"

# Detect OS and arch
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$OS" in
  linux)   GOOS=linux ;;
  darwin)  GOOS=darwin ;;
  *)       echo "Unsupported OS: $OS" >&2; exit 1 ;;
esac
# Currently only Linux binaries are published via GoReleaser; darwin/windows: build from source
if [ "$GOOS" != "linux" ]; then
  echo "Pre-built binaries are only published for Linux. On $GOOS please build from source: go build -o mcp-server ./cmd/mcp-server" >&2
  exit 1
fi
case "$ARCH" in
  x86_64|amd64) GOARCH=amd64 ;;
  aarch64|arm64) GOARCH=arm64 ;;
  *)       echo "Unsupported arch: $ARCH" >&2; exit 1 ;;
esac

ASSET_NAME="mcp-repo-context_${VER_STRIP}_${GOOS}_${GOARCH}.tar.gz"
CHECKSUMS_URL="https://github.com/${REPO}/releases/download/${VERSION}/checksums.txt"
ASSET_URL="https://github.com/${REPO}/releases/download/${VERSION}/${ASSET_NAME}"

echo "Installing mcp-repo-context ${VERSION} (${GOOS}/${GOARCH}) to ${DEST}"

mkdir -p "$DEST"
TMPDIR="$(mktemp -d)"
trap "rm -rf '$TMPDIR'" EXIT

# Download checksums and asset
curl -sSL -o "$TMPDIR/checksums.txt" "$CHECKSUMS_URL"
curl -sSL -o "$TMPDIR/$ASSET_NAME" "$ASSET_URL"

# Verify checksum
if command -v sha256sum >/dev/null 2>&1; then
  (cd "$TMPDIR" && sha256sum -c --ignore-missing checksums.txt)
elif command -v shasum >/dev/null 2>&1; then
  (cd "$TMPDIR" && shasum -a 256 -c --ignore-missing checksums.txt)
else
  echo "Warning: sha256sum/shasum not found; skipping checksum verification" >&2
fi

# Extract
(cd "$TMPDIR" && tar -xzf "$ASSET_NAME")
mv "$TMPDIR/mcp-server" "$DEST/mcp-server"
chmod +x "$DEST/mcp-server"

echo "Installed: $DEST/mcp-server"
"$DEST/mcp-server" -version 2>/dev/null || true
