#!/bin/bash
set -e

REPO="productdevbook/devir"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

info() { echo -e "${GREEN}[INFO]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }

# Detect OS
detect_os() {
    case "$(uname -s)" in
        Linux*)  echo "linux" ;;
        Darwin*) echo "darwin" ;;
        MINGW*|MSYS*|CYGWIN*) echo "windows" ;;
        *) error "Unsupported OS: $(uname -s)" ;;
    esac
}

# Detect architecture
detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64) echo "amd64" ;;
        arm64|aarch64) echo "arm64" ;;
        *) error "Unsupported architecture: $(uname -m)" ;;
    esac
}

# Get latest version
get_latest_version() {
    curl -s "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"v([^"]+)".*/\1/'
}

# Main
main() {
    info "Detecting system..."
    OS=$(detect_os)
    ARCH=$(detect_arch)
    info "OS: $OS, Arch: $ARCH"

    info "Fetching latest version..."
    VERSION=$(get_latest_version)
    if [ -z "$VERSION" ]; then
        error "Could not determine latest version"
    fi
    info "Latest version: v$VERSION"

    # Build download URL
    if [ "$OS" = "windows" ]; then
        FILENAME="devir-${OS}-${ARCH}.zip"
    else
        FILENAME="devir-${OS}-${ARCH}.tar.gz"
    fi
    URL="https://github.com/${REPO}/releases/download/v${VERSION}/${FILENAME}"

    # Create temp directory
    TMP_DIR=$(mktemp -d)
    trap "rm -rf $TMP_DIR" EXIT

    info "Downloading $FILENAME..."
    curl -fsSL "$URL" -o "$TMP_DIR/$FILENAME" || error "Download failed"

    info "Extracting..."
    cd "$TMP_DIR"
    if [ "$OS" = "windows" ]; then
        unzip -q "$FILENAME"
    else
        tar -xzf "$FILENAME"
    fi

    # Install
    BINARY="devir"
    if [ "$OS" = "windows" ]; then
        BINARY="devir.exe"
    fi

    if [ -w "$INSTALL_DIR" ]; then
        info "Installing to $INSTALL_DIR..."
        mv "$BINARY" "$INSTALL_DIR/"
    else
        info "Installing to $INSTALL_DIR (requires sudo)..."
        sudo mv "$BINARY" "$INSTALL_DIR/"
    fi

    # Verify
    if command -v devir &> /dev/null; then
        info "Installation complete!"
        echo ""
        devir -h
    else
        warn "Installed but 'devir' not found in PATH"
        warn "Add $INSTALL_DIR to your PATH"
    fi
}

main "$@"
