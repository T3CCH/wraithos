#!/bin/sh
# wraithos-init.sh - Custom initramfs hook for WraithOS
#
# Boot flow:
#   1. Load squashfs root image into RAM
#   2. Create zram device (compressed RAM block device)
#   3. Mount overlayfs: lower=squashfs, upper=zram-backed tmpfs
#   4. Pivot root to the overlay
#   5. Hand off to OpenRC init
#
# This script is called as an initramfs init hook. It expects the
# squashfs image to be at /boot/wraithos.sfs on the boot media.

set -e

# Minimal PATH for initramfs environment
export PATH="/usr/bin:/usr/sbin:/bin:/sbin"

# Mount points used during boot
BOOT_MNT="/mnt/boot"
SFS_MNT="/mnt/squashfs"
ZRAM_MNT="/mnt/zram"
OVERLAY_MNT="/mnt/overlay"
WORK_DIR="/mnt/overlay-work"

log() {
    echo "[wraithos-init] $*"
}

die() {
    log "FATAL: $*"
    log "Dropping to emergency shell"
    exec /bin/sh
}

# Mount essential pseudo-filesystems if not already present
mount_pseudo_fs() {
    [ -d /proc/1 ] || mount -t proc none /proc
    [ -d /sys/class ] || mount -t sysfs none /sys
    [ -e /dev/null ] || mount -t devtmpfs none /dev
}

# Find and mount the boot media containing the squashfs image.
# Searches for a partition with the squashfs file.
find_boot_media() {
    log "Searching for boot media..."

    mkdir -p "$BOOT_MNT"

    # Try each block device looking for our squashfs image
    for dev in /dev/sr0 /dev/vda /dev/sda /dev/nvme0n1; do
        [ -b "$dev" ] || continue
        if mount -o ro "$dev" "$BOOT_MNT" 2>/dev/null; then
            if [ -f "$BOOT_MNT/boot/wraithos.sfs" ]; then
                log "Found boot media at $dev"
                return 0
            fi
            umount "$BOOT_MNT"
        fi

        # Check partitions (e.g., /dev/sda1, /dev/vda1)
        for part in "${dev}"[0-9] "${dev}p"[0-9]; do
            [ -b "$part" ] || continue
            if mount -o ro "$part" "$BOOT_MNT" 2>/dev/null; then
                if [ -f "$BOOT_MNT/boot/wraithos.sfs" ]; then
                    log "Found boot media at $part"
                    return 0
                fi
                umount "$BOOT_MNT"
            fi
        done
    done

    die "Could not find boot media with wraithos.sfs"
}

# Mount the squashfs root image
mount_squashfs() {
    log "Mounting squashfs root image..."
    mkdir -p "$SFS_MNT"
    mount -t squashfs -o ro,loop "$BOOT_MNT/boot/wraithos.sfs" "$SFS_MNT" \
        || die "Failed to mount squashfs image"
    log "Squashfs mounted ($(du -sh "$SFS_MNT" 2>/dev/null | cut -f1) root filesystem)"
}

# Set up a zram device for the overlay upper layer.
# zram provides compressed RAM storage -- a 500MB OS image
# uses roughly 150-200MB of actual RAM after compression.
setup_zram() {
    log "Setting up zram device..."

    # Load zram module if needed
    modprobe zram num_devices=1 2>/dev/null || true

    # Wait briefly for device node
    local i=0
    while [ ! -e /dev/zram0 ] && [ "$i" -lt 10 ]; do
        sleep 0.1
        i=$((i + 1))
    done
    [ -e /dev/zram0 ] || die "zram device not available"

    # Size the zram device. Use 512MB by default -- this is the
    # compressed capacity, so actual RAM usage will be much less.
    local zram_size_mb=512
    echo "${zram_size_mb}M" > /sys/block/zram0/disksize \
        || die "Failed to set zram disk size"

    # Format and mount
    mkfs.ext4 -q -L wraith-overlay /dev/zram0 \
        || die "Failed to format zram device"

    mkdir -p "$ZRAM_MNT"
    mount /dev/zram0 "$ZRAM_MNT" \
        || die "Failed to mount zram device"

    log "zram0: ${zram_size_mb}MB compressed block device ready"
}

# Create overlayfs combining the read-only squashfs with the
# writable zram-backed upper layer.
mount_overlay() {
    log "Mounting overlayfs..."

    mkdir -p "$OVERLAY_MNT" "$ZRAM_MNT/upper" "$ZRAM_MNT/work"

    mount -t overlay overlay \
        -o "lowerdir=$SFS_MNT,upperdir=$ZRAM_MNT/upper,workdir=$ZRAM_MNT/work" \
        "$OVERLAY_MNT" \
        || die "Failed to mount overlayfs"

    log "Overlay filesystem ready"
}

# Pivot root from initramfs to the overlay filesystem,
# then exec into the real init system (OpenRC).
pivot_and_exec() {
    log "Pivoting root to overlay filesystem..."

    # Prepare target filesystem for pivot
    mkdir -p "$OVERLAY_MNT/mnt/initramfs"

    # Move pseudo-filesystems into the new root
    mount --move /proc "$OVERLAY_MNT/proc" 2>/dev/null || true
    mount --move /sys "$OVERLAY_MNT/sys" 2>/dev/null || true
    mount --move /dev "$OVERLAY_MNT/dev" 2>/dev/null || true

    # Pivot root
    cd "$OVERLAY_MNT" || die "Cannot cd to overlay mount"
    pivot_root . mnt/initramfs || die "pivot_root failed"

    # Clean up initramfs mounts inside new root
    # These are no longer needed after pivot
    umount -l /mnt/initramfs/mnt/boot 2>/dev/null || true

    log "Boot complete -- handing off to OpenRC init"

    # Execute the real init
    exec /sbin/init "$@"
}

# --- Main ---

log "WraithOS boot starting..."
log "RAM-based Alpine Linux for Docker containers"

mount_pseudo_fs
find_boot_media
mount_squashfs
setup_zram
mount_overlay
pivot_and_exec "$@"
