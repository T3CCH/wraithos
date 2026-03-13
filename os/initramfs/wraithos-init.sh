#!/bin/sh
# wraithos-init.sh - Custom initramfs hook for WraithOS
#
# Boot flow:
#   1. Load squashfs root image into RAM
#   2. Create zram device (compressed RAM block device)
#   3. Mount overlayfs: lower=squashfs, upper=zram-backed tmpfs
#   4. switch_root to the overlay (pivot_root doesn't work from initramfs)
#   5. Hand off to OpenRC init
#
# This script is called as an initramfs init hook. It expects the
# squashfs image to be at /boot/wraithos.sfs on the boot media.

# Minimal PATH for initramfs environment
export PATH="/usr/bin:/usr/sbin:/bin:/sbin"

# Alpine's initramfs only symlinks a few busybox applets.
# Install all applets so mount, mkdir, sleep, etc. are available.
/bin/busybox --install -s /bin 2>/dev/null
/bin/busybox --install -s /sbin 2>/dev/null

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
# Returns 0 on success, 1 on failure. Failure is non-fatal --
# the caller falls back to tmpfs if zram is unavailable.
setup_zram() {
    log "Setting up zram device..."

    # Load LZ4 compression modules required by zram
    modprobe lz4 2>/dev/null || true
    modprobe lz4_compress 2>/dev/null || true
    modprobe lz4hc 2>/dev/null || true
    modprobe lz4hc_compress 2>/dev/null || true

    # Load zram module if needed
    modprobe zram num_devices=1 2>/dev/null || true

    # Wait briefly for device node
    local i=0
    while [ ! -e /dev/zram0 ] && [ "$i" -lt 10 ]; do
        sleep 0.1
        i=$((i + 1))
    done
    if [ ! -e /dev/zram0 ]; then
        log "WARNING: zram device not available -- falling back to tmpfs"
        return 1
    fi

    # Size the zram device. Use 512MB by default -- this is the
    # compressed capacity, so actual RAM usage will be much less.
    local zram_size_mb=512
    if ! echo "${zram_size_mb}M" > /sys/block/zram0/disksize 2>/dev/null; then
        log "WARNING: Failed to set zram disk size -- falling back to tmpfs"
        return 1
    fi

    # Format and mount
    if ! mkfs.ext4 -q -L wraith-overlay /dev/zram0; then
        log "WARNING: Failed to format zram device -- falling back to tmpfs"
        return 1
    fi

    mkdir -p "$ZRAM_MNT"
    if ! mount /dev/zram0 "$ZRAM_MNT"; then
        log "WARNING: Failed to mount zram device -- falling back to tmpfs"
        return 1
    fi

    log "zram0: ${zram_size_mb}MB compressed block device ready"
    return 0
}

# Create overlayfs combining the read-only squashfs with a
# writable upper layer. Uses zram if available, tmpfs otherwise.
mount_overlay() {
    local upper_base="$1"   # mount point for the upper layer backing store

    log "Mounting overlayfs (upper on $upper_base)..."

    mkdir -p "$OVERLAY_MNT" "$upper_base/upper" "$upper_base/work"

    mount -t overlay overlay \
        -o "lowerdir=$SFS_MNT,upperdir=$upper_base/upper,workdir=$upper_base/work" \
        "$OVERLAY_MNT" \
        || die "Failed to mount overlayfs"

    log "Overlay filesystem ready"
}

# Set up a tmpfs fallback when zram is not available.
# Less memory-efficient (no compression) but always works.
setup_tmpfs_fallback() {
    log "Setting up tmpfs overlay upper layer..."

    mkdir -p "$ZRAM_MNT"
    mount -t tmpfs -o size=512M tmpfs "$ZRAM_MNT" \
        || die "Failed to mount tmpfs fallback"

    log "tmpfs: 512MB uncompressed overlay ready (no zram compression)"
}

# Switch root from initramfs to the overlay filesystem,
# then exec into the real init system (OpenRC).
# Note: pivot_root does not work from initramfs (rootfs).
# switch_root deletes initramfs contents, moves the new root
# to /, chroots, and execs the target init.
switch_and_exec() {
    log "Switching root to overlay filesystem..."

    # Verify the target init exists in the overlay
    if [ ! -x "$OVERLAY_MNT/sbin/init" ]; then
        die "No /sbin/init found in overlay root"
    fi

    # Move pseudo-filesystems into the new root so they
    # survive the switch. switch_root expects them there.
    mkdir -p "$OVERLAY_MNT/proc" "$OVERLAY_MNT/sys" "$OVERLAY_MNT/dev"
    mount --move /proc "$OVERLAY_MNT/proc" 2>/dev/null || true
    mount --move /sys "$OVERLAY_MNT/sys" 2>/dev/null || true
    mount --move /dev "$OVERLAY_MNT/dev" 2>/dev/null || true

    log "Boot complete -- handing off to OpenRC init"

    # switch_root will:
    #   1. Delete everything in the initramfs (free memory)
    #   2. Mount-move the new root to /
    #   3. chroot into it
    #   4. exec /sbin/init
    exec switch_root "$OVERLAY_MNT" /sbin/init
}

# --- Main ---

mount_pseudo_fs

# On UEFI systems (esp. Xen), VGA text console may not work.
# Load simpledrm/simplefb to enable framebuffer console.
log "Initializing console..."
modprobe simpledrm 2>/dev/null || modprobe simplefb 2>/dev/null || true
# Brief pause for framebuffer to initialize
sleep 0.2

log "WraithOS boot starting..."
log "RAM-based Alpine Linux for Docker containers"

# Load essential kernel modules for boot
log "Loading kernel modules..."
modprobe loop 2>/dev/null || log "loop: built-in or not found"
modprobe squashfs 2>/dev/null || log "squashfs: built-in or not found"
modprobe overlay 2>/dev/null || log "overlay: built-in or not found"
modprobe cdrom 2>/dev/null || log "cdrom: not found"
modprobe sr_mod 2>/dev/null || log "sr_mod: not found"
modprobe isofs 2>/dev/null || log "isofs: built-in or not found"
modprobe ext4 2>/dev/null || log "ext4: built-in or not found"

# Give devices a moment to settle
sleep 1
mdev -s 2>/dev/null || true

find_boot_media
mount_squashfs

if setup_zram; then
    mount_overlay "$ZRAM_MNT"
else
    setup_tmpfs_fallback
    mount_overlay "$ZRAM_MNT"
fi

switch_and_exec
