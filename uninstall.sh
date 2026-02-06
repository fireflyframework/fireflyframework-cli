#!/usr/bin/env bash
# Firefly Framework CLI Uninstaller
# Usage: bash uninstall.sh

set -euo pipefail

BINARY="flywork"
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
NC='\033[0m'

info()    { printf "${CYAN}  ℹ ${NC}%s\n" "$1"; }
success() { printf "${GREEN}  ✓ ${NC}%s\n" "$1"; }
warn()    { printf "${YELLOW}  ! ${NC}%s\n" "$1"; }

echo ""
info "Firefly Framework CLI Uninstaller"
echo ""

# ── Find and remove binary ───────────────────────────────────────────────────

BIN_PATH="$(command -v "$BINARY" 2>/dev/null || true)"

if [ -n "$BIN_PATH" ]; then
    BIN_PATH="$(readlink -f "$BIN_PATH" 2>/dev/null || echo "$BIN_PATH")"
    info "Found binary at: $BIN_PATH"
    rm -f "$BIN_PATH"
    success "Removed $BIN_PATH"
else
    # Check common locations
    for dir in "/usr/local/bin" "$HOME/.flywork/bin"; do
        if [ -f "$dir/$BINARY" ]; then
            rm -f "$dir/$BINARY"
            success "Removed $dir/$BINARY"
        fi
    done
fi

# ── Remove ~/.flywork directory ──────────────────────────────────────────────

FLYWORK_HOME="$HOME/.flywork"

if [ -d "$FLYWORK_HOME" ]; then
    printf "  ${YELLOW}?${NC} Remove ${FLYWORK_HOME} and all data? [y/N]: "
    read -r answer
    if [ "$answer" = "y" ] || [ "$answer" = "Y" ]; then
        rm -rf "$FLYWORK_HOME"
        success "Removed $FLYWORK_HOME"
    else
        info "Kept $FLYWORK_HOME"
    fi
fi

# ── Clean PATH entries from shell RC files ───────────────────────────────────

for rc in "$HOME/.zshrc" "$HOME/.bashrc" "$HOME/.profile"; do
    if [ -f "$rc" ] && grep -q "flywork" "$rc" 2>/dev/null; then
        # Remove lines containing flywork PATH and the comment above
        sed -i.bak '/# Firefly Framework CLI/d; /\.flywork\/bin/d' "$rc"
        rm -f "${rc}.bak"
        info "Cleaned PATH entry from $rc"
    fi
done

echo ""
success "Flywork CLI uninstalled."
