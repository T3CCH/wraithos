#!/bin/bash
# upload-xcp.sh - Upload WraithOS ISO to XCP-ng host
#
# Transfers the built ISO to an XCP-ng server's ISO storage
# repository so it can be used to create VMs.
#
# Usage:
#   ./upload-xcp.sh xcphost.local              # Upload latest build
#   ./upload-xcp.sh xcphost.local path/to.iso  # Upload specific ISO
#   XCP_USER=admin ./upload-xcp.sh xcphost.local

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
BUILD_DIR="$PROJECT_DIR/build"

# XCP-ng defaults
XCP_USER="${XCP_USER:-root}"
XCP_ISO_DIR="${XCP_ISO_DIR:-/var/opt/xen/ISO_Store}"

log() {
    echo "[upload-xcp] $*"
}

err() {
    echo "ERROR: $*" >&2
    exit 1
}

# --- Main ---

if [ $# -lt 1 ]; then
    echo "Usage: $0 <xcp-host> [iso-path]"
    echo ""
    echo "Environment variables:"
    echo "  XCP_USER     SSH user (default: root)"
    echo "  XCP_ISO_DIR  ISO storage path (default: /var/opt/xen/ISO_Store)"
    exit 1
fi

XCP_HOST="$1"
ISO_PATH="${2:-}"

# Find ISO if not specified
if [ -z "$ISO_PATH" ]; then
    ISO_PATH=$(find "$BUILD_DIR" -maxdepth 1 -name "wraithos-*.iso" -type f \
        | sort -V | tail -1)
    [ -z "$ISO_PATH" ] && err "No ISO found in $BUILD_DIR -- run build-iso.sh first"
fi

[ -f "$ISO_PATH" ] || err "ISO not found: $ISO_PATH"

ISO_FILENAME=$(basename "$ISO_PATH")
ISO_SIZE=$(du -h "$ISO_PATH" | cut -f1)

log "Uploading to XCP-ng host..."
log "  Host:   $XCP_HOST"
log "  User:   $XCP_USER"
log "  ISO:    $ISO_FILENAME ($ISO_SIZE)"
log "  Dest:   $XCP_ISO_DIR/$ISO_FILENAME"

# Upload via SCP
scp "$ISO_PATH" "${XCP_USER}@${XCP_HOST}:${XCP_ISO_DIR}/${ISO_FILENAME}"

# Verify the upload and rescan the ISO SR
log "Verifying upload..."
# shellcheck disable=SC2029
ssh "${XCP_USER}@${XCP_HOST}" "
    ls -lh '${XCP_ISO_DIR}/${ISO_FILENAME}' && \
    echo 'Upload verified' && \
    xe sr-list type=iso 2>/dev/null | head -5 || true
"

log "Upload complete: $ISO_FILENAME -> $XCP_HOST:$XCP_ISO_DIR/"
log ""
log "To create a VM with this ISO in Xen Orchestra or xe CLI:"
log "  1. Go to New VM -> select the ISO"
log "  2. Set RAM >= 1024MB, add config disk (100MB) and cache disk"
log "  3. Label disks: WRAITH-CONFIG and WRAITH-CACHE after formatting"
