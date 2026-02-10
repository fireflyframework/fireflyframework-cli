#!/usr/bin/env bash
# Firefly Framework CLI Installer
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/fireflyframework/fireflyframework-cli/main/install.sh | bash
#   FLYWORK_VERSION=v26.01.01 bash install.sh
#   FLYWORK_INSTALL_DIR=/custom/path bash install.sh

set -euo pipefail

REPO="fireflyframework/fireflyframework-cli"
BINARY="flywork"

# ── Colors ───────────────────────────────────────────────────────────────────

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

info()    { printf "${CYAN}  ℹ ${NC}%s\n" "$1"; }
success() { printf "${GREEN}  ✓ ${NC}%s\n" "$1"; }
warn()    { printf "${YELLOW}  ! ${NC}%s\n" "$1"; }
error()   { printf "${RED}  ✗ ${NC}%s\n" "$1"; exit 1; }

# ── Banner ───────────────────────────────────────────────────────────────────

printf "${BOLD}${CYAN}"
cat << 'EOF'
  _____.__                _____.__
_/ ____\__|______   _____/ ____\  | ___.__.
\   __\|  \_  __ \_/ __ \   __\|  |<   |  |
 |  |  |  ||  | \/\  ___/|  |  |  |_\___  |
 |__|  |__||__|    \___  >__|  |____/ ____|
                       \/           \/
  _____                                                 __
_/ ____\___________    _____   ______  _  _____________|  | __
\   __\\_  __ \__  \  /     \_/ __ \ \/ \/ /  _ \_  __ \  |/ /
 |  |   |  | \// __ \|  Y Y  \  ___/\     (  <_> )  | \/    <
 |__|   |__|  (____  /__|_|  /\___  >\/\_/ \____/|__|  |__|_ \
                   \/      \/     \/                        \/
EOF
printf "${NC}\n"
info "Firefly Framework CLI Installer"
echo ""

# ── Detect platform ──────────────────────────────────────────────────────────

detect_os() {
    local os
    os="$(uname -s | tr '[:upper:]' '[:lower:]')"
    case "$os" in
        darwin) echo "darwin" ;;
        linux)  echo "linux" ;;
        *)      error "Unsupported OS: $os (use install.ps1 for Windows)" ;;
    esac
}

detect_arch() {
    local arch
    arch="$(uname -m)"
    case "$arch" in
        x86_64|amd64)   echo "amd64" ;;
        aarch64|arm64)  echo "arm64" ;;
        *)              error "Unsupported architecture: $arch" ;;
    esac
}

OS="$(detect_os)"
ARCH="$(detect_arch)"
info "Detected platform: ${OS}/${ARCH}"

# ── Resolve version ──────────────────────────────────────────────────────────

if [ -z "${FLYWORK_VERSION:-}" ]; then
    info "Fetching latest release..."
    FLYWORK_VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')"
    if [ -z "$FLYWORK_VERSION" ]; then
        error "Could not determine latest version"
    fi
fi
# Ensure version starts with 'v' (matches Makefile archive naming)
case "$FLYWORK_VERSION" in v*) ;; *) FLYWORK_VERSION="v${FLYWORK_VERSION}" ;; esac
info "Version: ${FLYWORK_VERSION}"

# ── Resolve install directory ────────────────────────────────────────────────

if [ -z "${FLYWORK_INSTALL_DIR:-}" ]; then
    if [ -w "/usr/local/bin" ]; then
        FLYWORK_INSTALL_DIR="/usr/local/bin"
    else
        FLYWORK_INSTALL_DIR="${HOME}/.flywork/bin"
    fi
fi

mkdir -p "$FLYWORK_INSTALL_DIR"
info "Install directory: ${FLYWORK_INSTALL_DIR}"

# ── Download ─────────────────────────────────────────────────────────────────

ARCHIVE_NAME="${BINARY}-${FLYWORK_VERSION}-${OS}-${ARCH}.tar.gz"
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${FLYWORK_VERSION}/${ARCHIVE_NAME}"

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

info "Downloading ${ARCHIVE_NAME}..."
if ! curl -fsSL -o "${TMP_DIR}/${ARCHIVE_NAME}" "$DOWNLOAD_URL"; then
    error "Download failed — check that version ${FLYWORK_VERSION} exists for ${OS}/${ARCH}"
fi

# ── Extract & install ────────────────────────────────────────────────────────

info "Extracting..."
tar -xzf "${TMP_DIR}/${ARCHIVE_NAME}" -C "$TMP_DIR"

# Find the binary inside the extracted directory
EXTRACTED_BIN="$(find "$TMP_DIR" -name "$BINARY" -type f | head -1)"
if [ -z "$EXTRACTED_BIN" ]; then
    error "Binary not found in archive"
fi

chmod +x "$EXTRACTED_BIN"
mv "$EXTRACTED_BIN" "${FLYWORK_INSTALL_DIR}/${BINARY}"
success "Installed ${BINARY} to ${FLYWORK_INSTALL_DIR}/${BINARY}"

# ── Update PATH if needed ───────────────────────────────────────────────────

add_to_path() {
    local dir="$1"
    local shell_rc

    case "${SHELL:-/bin/bash}" in
        */zsh)  shell_rc="$HOME/.zshrc" ;;
        */bash)
            if [ -f "$HOME/.bashrc" ]; then
                shell_rc="$HOME/.bashrc"
            else
                shell_rc="$HOME/.profile"
            fi
            ;;
        *)      shell_rc="$HOME/.profile" ;;
    esac

    if ! echo "$PATH" | tr ':' '\n' | grep -q "^${dir}$"; then
        local export_line="export PATH=\"${dir}:\$PATH\""
        if [ -f "$shell_rc" ] && grep -qF "$dir" "$shell_rc" 2>/dev/null; then
            return
        fi
        echo "" >> "$shell_rc"
        echo "# Firefly Framework CLI" >> "$shell_rc"
        echo "$export_line" >> "$shell_rc"
        warn "Added ${dir} to PATH in ${shell_rc} — restart your shell or run: source ${shell_rc}"
    fi
}

if ! echo "$PATH" | tr ':' '\n' | grep -q "^${FLYWORK_INSTALL_DIR}$"; then
    add_to_path "$FLYWORK_INSTALL_DIR"
    export PATH="${FLYWORK_INSTALL_DIR}:$PATH"
fi

# ── Verify ───────────────────────────────────────────────────────────────────

echo ""
if command -v "$BINARY" &>/dev/null; then
    success "Installation complete!"
    echo ""
    "$BINARY" version
else
    warn "Installed, but '${BINARY}' is not in PATH yet. Restart your shell or run:"
    echo "  export PATH=\"${FLYWORK_INSTALL_DIR}:\$PATH\""
fi

echo ""
printf "${CYAN}  ℹ ${NC}Get started: ${BOLD}flywork setup${NC}\n"
