#!/bin/sh
# genapkovl-wraithos.sh - APK overlay generator for WraithOS
#
# Called by mkimage to create the overlay tarball that gets
# unpacked on top of the base Alpine rootfs during ISO build.
# This injects our custom init scripts, config, and services.
#
# Usage: Called automatically by mkimage.sh -- not run directly.
#   Receives $1 = temporary rootfs directory

set -e

OVERLAY_DIR="$1"

if [ -z "$OVERLAY_DIR" ]; then
    echo "Usage: $0 <overlay-directory>" >&2
    exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOTFS_SRC="$SCRIPT_DIR/../rootfs"

# Copy the entire rootfs overlay tree
if [ -d "$ROOTFS_SRC" ]; then
    cp -a "$ROOTFS_SRC/"* "$OVERLAY_DIR/"
fi

# Ensure correct permissions on service scripts
chmod 755 "$OVERLAY_DIR"/etc/init.d/wraith-* 2>/dev/null || true
chmod 755 "$OVERLAY_DIR"/etc/init.d/xe-daemon 2>/dev/null || true
chmod 755 "$OVERLAY_DIR"/etc/local.d/*.start 2>/dev/null || true

# Enable services in the default runlevel
mkdir -p "$OVERLAY_DIR/etc/runlevels/default"
for svc in wraith-disks wraith-network wraith-docker wraith-samba wraith-ui xe-daemon local; do
    ln -sf "/etc/init.d/$svc" "$OVERLAY_DIR/etc/runlevels/default/$svc" 2>/dev/null || true
done

echo "[genapkovl] WraithOS overlay generated"
