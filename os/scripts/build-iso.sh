#!/bin/bash
# build-iso.sh - Master build script for WraithOS ISO
#
# Builds a bootable ISO image using Alpine's mkimage (atools).
# The ISO contains a squashfs root filesystem that boots entirely
# into RAM via overlayfs + zram.
#
# Prerequisites:
#   - Alpine Linux build host (or Alpine container)
#   - Packages: alpine-sdk, squashfs-tools, grub-efi, mtools, dosfstools
#   - atools (Alpine build tools) cloned to $ATOOLS_DIR
#
# Usage:
#   ./build-iso.sh              # Build with defaults
#   ./build-iso.sh --clean      # Clean build directory first
#   ALPINE_MIRROR=... ./build-iso.sh  # Custom mirror

set -euo pipefail

# --- Configuration ---

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
REPO_ROOT="$(cd "$PROJECT_DIR/.." && pwd)"
BUILD_DIR="$PROJECT_DIR/build"
ATOOLS_DIR="$BUILD_DIR/atools"

ALPINE_BRANCH="${ALPINE_BRANCH:-v3.21}"
ALPINE_MIRROR="${ALPINE_MIRROR:-https://dl-cdn.alpinelinux.org/alpine}"
ALPINE_ARCH="x86_64"

VERSION=$(cat "$PROJECT_DIR/rootfs/usr/share/wraithos/version" 2>/dev/null || echo "0.0.0-dev")
ISO_NAME="wraithos-${VERSION}.iso"

# --- Functions ---

log() {
    echo "==> $*"
}

err() {
    echo "ERROR: $*" >&2
    exit 1
}

check_prerequisites() {
    log "Checking prerequisites..."

    local missing=()

    # Check for required commands
    for cmd in git make mksquashfs grub-mkimage mcopy; do
        if ! command -v "$cmd" >/dev/null 2>&1; then
            missing+=("$cmd")
        fi
    done

    if [ ${#missing[@]} -gt 0 ]; then
        err "Missing required commands: ${missing[*]}
Install them with:
  apk add alpine-sdk squashfs-tools grub-efi mtools dosfstools xorriso"
    fi

    # Must run as root (or in a rootless container with fakeroot)
    if [ "$(id -u)" -ne 0 ] && ! command -v fakeroot >/dev/null 2>&1; then
        err "Must run as root or have fakeroot installed
  apk add fakeroot"
    fi

    log "Prerequisites OK"
}

setup_atools() {
    if [ -d "$ATOOLS_DIR" ]; then
        log "atools already present"
        return
    fi

    log "Cloning Alpine atools..."
    mkdir -p "$BUILD_DIR"
    git clone --depth 1 --branch "$ALPINE_BRANCH" \
        https://gitlab.alpinelinux.org/alpine/aports.git "$BUILD_DIR/aports-tmp"

    # We only need the mkimage scripts from aports/scripts
    mkdir -p "$ATOOLS_DIR"
    cp -a "$BUILD_DIR/aports-tmp/scripts/"* "$ATOOLS_DIR/" 2>/dev/null || true
    rm -rf "$BUILD_DIR/aports-tmp"

    log "atools ready"
}

build_ui_binary() {
    local ui_binary="$REPO_ROOT/wraith-ui"

    if [ -f "$ui_binary" ]; then
        log "Web UI binary already built: $ui_binary"
        return
    fi

    log "Building Go web UI binary..."

    if ! command -v go >/dev/null 2>&1; then
        log "WARNING: Go not available -- skipping UI binary build"
        log "  Build manually: cd $REPO_ROOT && CGO_ENABLED=0 go build -o wraith-ui ./cmd/wraith-ui"
        return
    fi

    (
        cd "$REPO_ROOT"
        # Static binary for Alpine (musl-compatible)
        CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
            go build -ldflags="-s -w -X main.version=${VERSION}" \
            -o wraith-ui ./cmd/wraith-ui
    )

    log "Web UI binary built: $ui_binary"
}

install_profile() {
    log "Installing mkimage profile..."

    # Link our profile into the atools directory
    ln -sf "$PROJECT_DIR/mkimage/wraithos.sh" "$ATOOLS_DIR/mkimg.wraithos.sh"
    ln -sf "$PROJECT_DIR/mkimage/genapkovl-wraithos.sh" "$ATOOLS_DIR/genapkovl-wraithos.sh"

    log "Profile installed"
}

build_iso() {
    log "Building WraithOS ISO (version $VERSION)..."

    mkdir -p "$BUILD_DIR/out"

    local mkimage_cmd="$ATOOLS_DIR/mkimage.sh"

    if [ ! -f "$mkimage_cmd" ]; then
        err "mkimage.sh not found at $mkimage_cmd -- run setup_atools first"
    fi

    chmod +x "$mkimage_cmd"

    # Build the ISO using Alpine's mkimage
    # --profile wraithos uses our wraithos.sh profile
    # --outdir specifies where the ISO lands
    "$mkimage_cmd" \
        --tag "$VERSION" \
        --outdir "$BUILD_DIR/out" \
        --arch "$ALPINE_ARCH" \
        --repository "$ALPINE_MIRROR/$ALPINE_BRANCH/main" \
        --extra-repository "$ALPINE_MIRROR/$ALPINE_BRANCH/community" \
        --profile wraithos

    # Rename to our convention
    local built_iso
    built_iso=$(find "$BUILD_DIR/out" -name "*.iso" -type f | head -1)

    if [ -z "$built_iso" ]; then
        err "ISO build produced no output"
    fi

    mv "$built_iso" "$BUILD_DIR/$ISO_NAME"

    log "ISO built: $BUILD_DIR/$ISO_NAME"
}

print_summary() {
    local iso_path="$BUILD_DIR/$ISO_NAME"

    if [ ! -f "$iso_path" ]; then
        err "ISO not found at $iso_path"
    fi

    local size_bytes size_human sha256 md5

    size_bytes=$(stat -c %s "$iso_path" 2>/dev/null || stat -f %z "$iso_path" 2>/dev/null)
    size_human=$(du -h "$iso_path" | cut -f1)
    sha256=$(sha256sum "$iso_path" | cut -d' ' -f1)
    md5=$(md5sum "$iso_path" | cut -d' ' -f1)

    echo ""
    echo "============================================"
    echo "  WraithOS ISO Build Complete"
    echo "============================================"
    echo "  Version:  $VERSION"
    echo "  File:     $iso_path"
    echo "  Size:     $size_human ($size_bytes bytes)"
    echo "  SHA256:   $sha256"
    echo "  MD5:      $md5"
    echo "============================================"
    echo ""

    # Warn if ISO exceeds 200MB target
    if [ "$size_bytes" -gt 209715200 ]; then
        echo "WARNING: ISO exceeds 200MB target size"
    fi

    # Write checksums file
    {
        echo "sha256:$sha256  $ISO_NAME"
        echo "md5:$md5  $ISO_NAME"
    } > "$BUILD_DIR/$ISO_NAME.checksums"
}

# --- Main ---

case "${1:-}" in
    --clean)
        log "Cleaning build directory..."
        rm -rf "$BUILD_DIR"
        log "Clean complete"
        ;;
    --help|-h)
        echo "Usage: $0 [--clean|--help]"
        echo ""
        echo "Environment variables:"
        echo "  ALPINE_BRANCH   Alpine version branch (default: v3.21)"
        echo "  ALPINE_MIRROR   Alpine mirror URL"
        exit 0
        ;;
esac

check_prerequisites
setup_atools
build_ui_binary
install_profile
build_iso
print_summary
