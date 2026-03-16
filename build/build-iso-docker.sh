#!/bin/sh
# build-iso-docker.sh - Build WraithOS ISO inside Alpine Docker container
#
# This script runs inside the Alpine container and produces a bootable
# UEFI ISO with a squashfs root filesystem.
#
# Output: /out/wraithos-${VERSION}.iso

set -eu

VERSION=$(cat /build/os/rootfs/usr/share/wraithos/version 2>/dev/null || echo "0.0.0-dev")
ALPINE_MIRROR="https://dl-cdn.alpinelinux.org/alpine"
ALPINE_BRANCH="v3.21"
ARCH="x86_64"

WORK="/work"
ROOTFS="$WORK/rootfs"
ISODIR="$WORK/iso"
OUTDIR="/out"

log() { echo "==> $*"; }
err() { echo "ERROR: $*" >&2; exit 1; }

# -------------------------------------------------------
# Phase 1: Create Alpine rootfs with apk
# -------------------------------------------------------
create_rootfs() {
    log "Creating Alpine rootfs..."
    mkdir -p "$ROOTFS"

    # Initialize apk database in the rootfs
    apk add --root "$ROOTFS" --initdb --no-cache \
        --repository "$ALPINE_MIRROR/$ALPINE_BRANCH/main" \
        --repository "$ALPINE_MIRROR/$ALPINE_BRANCH/community" \
        --arch "$ARCH" \
        --keys-dir /etc/apk/keys \
        alpine-base \
        openrc \
        busybox-openrc \
        linux-virt \
        linux-firmware-none \
        docker \
        docker-cli-compose \
        nftables \
        cifs-utils \
        nfs-utils \
        e2fsprogs \
        squashfs-tools \
        zram-init \
        blkid \
        util-linux \
        lsblk \
        ca-certificates \
        curl \
        tzdata \
        mkinitfs \
        openssh-server \
        xe-guest-utilities \
        open-vm-tools \
        qemu-guest-agent

    log "Alpine rootfs created ($(du -sh "$ROOTFS" | cut -f1))"
}

# -------------------------------------------------------
# Phase 2: Install WraithOS overlay files
# -------------------------------------------------------
install_overlay() {
    log "Installing WraithOS overlay..."

    # Copy rootfs overlay (init scripts, config, etc.)
    if [ -d /build/os/rootfs ]; then
        cp -a /build/os/rootfs/* "$ROOTFS/"
    fi

    # Install wraith-ui binary
    if [ -f /build/wraith-ui ]; then
        install -m 755 /build/wraith-ui "$ROOTFS/usr/bin/wraith-ui"
        log "Installed wraith-ui binary"
    else
        err "wraith-ui binary not found"
    fi

    # Install custom initramfs hook
    if [ -f /build/os/initramfs/wraithos-init.sh ]; then
        mkdir -p "$ROOTFS/usr/share/mkinitfs"
        install -m 755 /build/os/initramfs/wraithos-init.sh \
            "$ROOTFS/usr/share/mkinitfs/wraithos-init.sh"
    fi

    # Make init scripts executable
    chmod 755 "$ROOTFS"/etc/init.d/wraith-* 2>/dev/null || true
    chmod 755 "$ROOTFS"/etc/init.d/xe-daemon 2>/dev/null || true
    chmod 755 "$ROOTFS"/etc/local.d/*.start 2>/dev/null || true

    # Enable WraithOS services at default runlevel
    mkdir -p "$ROOTFS/etc/runlevels/default"
    for svc in docker wraith-disks wraith-network wraith-docker wraith-samba wraith-ui local xe-daemon open-vm-tools qemu-guest-agent; do
        ln -sf "/etc/init.d/$svc" "$ROOTFS/etc/runlevels/default/$svc" 2>/dev/null || true
    done

    # Enable core services
    mkdir -p "$ROOTFS/etc/runlevels/sysinit"
    for svc in devfs dmesg hwdrivers mdev sysfs; do
        ln -sf "/etc/init.d/$svc" "$ROOTFS/etc/runlevels/sysinit/$svc" 2>/dev/null || true
    done

    mkdir -p "$ROOTFS/etc/runlevels/boot"
    for svc in modules sysctl hostname bootmisc syslog cgroups; do
        ln -sf "/etc/init.d/$svc" "$ROOTFS/etc/runlevels/boot/$svc" 2>/dev/null || true
    done

    mkdir -p "$ROOTFS/etc/runlevels/shutdown"
    for svc in mount-ro killprocs savecache; do
        ln -sf "/etc/init.d/$svc" "$ROOTFS/etc/runlevels/shutdown/$svc" 2>/dev/null || true
    done

    # Set hostname
    echo "wraithos" > "$ROOTFS/etc/hostname"

    # Configure /etc/hosts
    cat > "$ROOTFS/etc/hosts" <<EOF
127.0.0.1   localhost wraithos
::1         localhost wraithos
EOF

    # Minimal fstab
    cat > "$ROOTFS/etc/fstab" <<EOF
# WraithOS - disks mounted dynamically by wraith-disks service
tmpfs   /tmp    tmpfs   nosuid,nodev,noatime    0 0
tmpfs   /run    tmpfs   nosuid,nodev,mode=755   0 0
EOF

    # Set timezone to UTC
    if [ -f "$ROOTFS/usr/share/zoneinfo/UTC" ]; then
        cp "$ROOTFS/usr/share/zoneinfo/UTC" "$ROOTFS/etc/localtime"
    fi
    echo "UTC" > "$ROOTFS/etc/timezone"

    # Set root password for console access (debug/emergency)
    # Password: wraithos
    local _hash
    _hash=$(openssl passwd -6 "wraithos")
    awk -v hash="$_hash" 'BEGIN{FS=OFS=":"} /^root:/ {$2=hash} 1' "$ROOTFS/etc/shadow" > "$ROOTFS/etc/shadow.tmp"
    mv "$ROOTFS/etc/shadow.tmp" "$ROOTFS/etc/shadow"
    chmod 640 "$ROOTFS/etc/shadow"

    log "WraithOS overlay installed"
}

# -------------------------------------------------------
# Phase 3: Extract kernel and build initramfs
# -------------------------------------------------------
prepare_boot_files() {
    log "Preparing boot files..."

    mkdir -p "$WORK/boot"

    # Find the virt kernel
    KERNEL_VERSION=$(ls "$ROOTFS/lib/modules/" | head -1)
    log "Kernel version: $KERNEL_VERSION"

    # Copy kernel
    if [ -f "$ROOTFS/boot/vmlinuz-virt" ]; then
        cp "$ROOTFS/boot/vmlinuz-virt" "$WORK/boot/vmlinuz"
        log "Kernel copied: vmlinuz-virt"
    else
        VMLINUZ=$(find "$ROOTFS/boot" -name "vmlinuz-*" -type f | head -1)
        if [ -n "$VMLINUZ" ]; then
            cp "$VMLINUZ" "$WORK/boot/vmlinuz"
            log "Kernel copied: $(basename "$VMLINUZ")"
        else
            err "No kernel found in rootfs"
        fi
    fi

    # Unpack Alpine's initramfs, replace init with ours, repack
    local REPACK_DIR="$WORK/initramfs-repack"
    mkdir -p "$REPACK_DIR"
    cd "$REPACK_DIR"
    zcat "$ROOTFS/boot/initramfs-virt" | cpio -id 2>/dev/null

    # Replace Alpine's init with our custom WraithOS init
    cp /build/os/initramfs/wraithos-init.sh "$REPACK_DIR/init"
    chmod 755 "$REPACK_DIR/init"

    # Add mount points our init needs
    mkdir -p "$REPACK_DIR/mnt/boot" "$REPACK_DIR/mnt/squashfs" "$REPACK_DIR/mnt/zram" "$REPACK_DIR/mnt/overlay" "$REPACK_DIR/mnt/overlay-work"

    # Copy essential kernel modules that aren't in Alpine's default initramfs
    local KVER=$(ls "$ROOTFS/lib/modules/" | head -1)
    local SRC_MOD="$ROOTFS/lib/modules/$KVER"
    local DST_MOD="$REPACK_DIR/lib/modules/$KVER"
    mkdir -p "$DST_MOD/kernel/fs" "$DST_MOD/kernel/drivers/block" "$DST_MOD/kernel/drivers/scsi" "$DST_MOD/kernel/lib" "$DST_MOD/kernel/crypto"

    # Filesystem modules: squashfs, overlayfs, ext4, isofs
    cp -a "$SRC_MOD/kernel/fs/squashfs" "$DST_MOD/kernel/fs/" 2>/dev/null || true
    cp -a "$SRC_MOD/kernel/fs/overlayfs" "$DST_MOD/kernel/fs/" 2>/dev/null || true
    cp -a "$SRC_MOD/kernel/fs/ext4" "$DST_MOD/kernel/fs/" 2>/dev/null || true
    cp -a "$SRC_MOD/kernel/fs/isofs" "$DST_MOD/kernel/fs/" 2>/dev/null || true

    # LZ4 compression modules (required by zram)
    cp -a "$SRC_MOD/kernel/lib/lz4" "$DST_MOD/kernel/lib/" 2>/dev/null || true
    cp -a "$SRC_MOD/kernel/crypto/lz4.ko"* "$DST_MOD/kernel/crypto/" 2>/dev/null || true
    cp -a "$SRC_MOD/kernel/crypto/lz4hc.ko"* "$DST_MOD/kernel/crypto/" 2>/dev/null || true
    # Also try individual .ko files in case they're not in subdirectories
    for lz4mod in lz4_compress lz4_decompress lz4 lz4hc lz4hc_compress; do
        find "$SRC_MOD" -name "${lz4mod}.ko*" 2>/dev/null | while read -r f; do
            rel_path="${f#$SRC_MOD/}"
            target_dir=$(dirname "$DST_MOD/$rel_path")
            mkdir -p "$target_dir"
            cp -a "$f" "$DST_MOD/$rel_path" 2>/dev/null || true
        done
    done

    # Block device modules: loop, zram
    cp -a "$SRC_MOD/kernel/drivers/block/loop.ko"* "$DST_MOD/kernel/drivers/block/" 2>/dev/null || true
    cp -a "$SRC_MOD/kernel/drivers/block/zram" "$DST_MOD/kernel/drivers/block/" 2>/dev/null || true

    # SCSI/CDROM for ISO boot
    cp -a "$SRC_MOD/kernel/drivers/scsi/sr_mod.ko"* "$DST_MOD/kernel/drivers/scsi/" 2>/dev/null || true

    # Regenerate modules.dep for our added modules
    depmod -b "$REPACK_DIR" "$KVER" 2>/dev/null || true

    log "Added kernel modules: squashfs, overlay, ext4, loop, zram, lz4, sr_mod, isofs"

    # Repack
    find . | sort | cpio -o -H newc 2>/dev/null | gzip -1 > "$WORK/boot/initramfs"
    cd /
    log "Initramfs repacked with wraithos init"
    log "Boot files ready"
}

build_mkinitfs_initramfs() {
    log "Building initramfs (Alpine base + WraithOS overlay)..."

    # Use Alpine's pre-built initramfs-virt as base (known working format)
    # Then overlay our custom /init using Linux's concatenated cpio feature
    if [ ! -f "$ROOTFS/boot/initramfs-virt" ]; then
        err "Alpine initramfs-virt not found"
    fi

    # Create overlay directory with our custom init and mount points
    local OVERLAY_DIR="$WORK/initramfs-overlay"
    mkdir -p "$OVERLAY_DIR"

    # Our custom init replaces Alpine's /init
    cp /build/os/initramfs/wraithos-init.sh "$OVERLAY_DIR/init"
    chmod 755 "$OVERLAY_DIR/init"

    # Mount points needed by our init
    mkdir -p "$OVERLAY_DIR/mnt/boot"
    mkdir -p "$OVERLAY_DIR/mnt/squashfs"
    mkdir -p "$OVERLAY_DIR/mnt/zram"
    mkdir -p "$OVERLAY_DIR/mnt/overlay"
    mkdir -p "$OVERLAY_DIR/mnt/overlay-work"
    mkdir -p "$OVERLAY_DIR/newroot"

    # Build overlay cpio archive
    local OVERLAY_CPIO="$WORK/initramfs-overlay.cpio"
    (cd "$OVERLAY_DIR" && find . | cpio -o -H newc 2>/dev/null) > "$OVERLAY_CPIO"

    # Concatenate: Alpine base (gzip) + our overlay (gzip)
    # Linux kernel merges concatenated cpio archives, later files win
    cp "$ROOTFS/boot/initramfs-virt" "$WORK/boot/initramfs"
    gzip -c "$OVERLAY_CPIO" >> "$WORK/boot/initramfs"

    initramfs_size=$(du -sh "$WORK/boot/initramfs" | cut -f1)
    log "Initramfs built (Alpine base + wraithos overlay): $initramfs_size"
}

build_initramfs() {
    log "Building custom initramfs..."

    local INITRAMFS_DIR="$WORK/initramfs"
    mkdir -p "$INITRAMFS_DIR"

    # Create initramfs directory structure (no brace expansion - ash doesn't support it)
    mkdir -p "$INITRAMFS_DIR/bin"
    mkdir -p "$INITRAMFS_DIR/sbin"
    mkdir -p "$INITRAMFS_DIR/dev"
    mkdir -p "$INITRAMFS_DIR/proc"
    mkdir -p "$INITRAMFS_DIR/sys"
    mkdir -p "$INITRAMFS_DIR/mnt"
    mkdir -p "$INITRAMFS_DIR/etc"
    mkdir -p "$INITRAMFS_DIR/lib"
    mkdir -p "$INITRAMFS_DIR/usr/bin"
    mkdir -p "$INITRAMFS_DIR/usr/sbin"
    mkdir -p "$INITRAMFS_DIR/newroot"
    mkdir -p "$INITRAMFS_DIR/tmp"
    mkdir -p "$INITRAMFS_DIR/mnt/boot"
    mkdir -p "$INITRAMFS_DIR/mnt/squashfs"
    mkdir -p "$INITRAMFS_DIR/mnt/zram"
    mkdir -p "$INITRAMFS_DIR/mnt/overlay"
    mkdir -p "$INITRAMFS_DIR/mnt/overlay-work"

    # Copy busybox and musl dynamic linker from rootfs
    cp "$ROOTFS/bin/busybox" "$INITRAMFS_DIR/bin/busybox"
    chmod 755 "$INITRAMFS_DIR/bin/busybox"

    # Copy musl dynamic linker (busybox is dynamically linked against it)
    if [ -f "$ROOTFS/lib/ld-musl-x86_64.so.1" ]; then
        cp "$ROOTFS/lib/ld-musl-x86_64.so.1" "$INITRAMFS_DIR/lib/"
    elif [ -L "$ROOTFS/lib/ld-musl-x86_64.so.1" ]; then
        # Copy the target of the symlink
        cp -L "$ROOTFS/lib/ld-musl-x86_64.so.1" "$INITRAMFS_DIR/lib/"
    fi

    # Also copy libc (musl) -- same file, but some programs reference it differently
    if [ -f "$ROOTFS/lib/libc.musl-x86_64.so.1" ]; then
        cp -L "$ROOTFS/lib/libc.musl-x86_64.so.1" "$INITRAMFS_DIR/lib/" 2>/dev/null || true
    fi

    # Create busybox symlinks for essential commands
    for cmd in sh mount umount mkdir echo cat ls cp mv rm ln \
               switch_root pivot_root sleep mdev \
               modprobe insmod lsmod losetup blkid \
               mknod chmod chown chroot sed awk grep \
               dmesg findfs mkfs.ext4; do
        ln -sf /bin/busybox "$INITRAMFS_DIR/bin/$cmd"
    done

    # Also link to sbin
    for cmd in modprobe insmod switch_root pivot_root mdev blkid losetup; do
        ln -sf /bin/busybox "$INITRAMFS_DIR/sbin/$cmd"
    done

    # Copy kernel modules we need
    local MODDIR="$INITRAMFS_DIR/lib/modules/$KERNEL_VERSION"
    mkdir -p "$MODDIR"

    # Copy essential kernel modules from rootfs
    local SRC_MODDIR="$ROOTFS/lib/modules/$KERNEL_VERSION"
    if [ -d "$SRC_MODDIR" ]; then
        # Copy specific module directories we need
        for mod_path in \
            "kernel/fs/squashfs" \
            "kernel/fs/overlayfs" \
            "kernel/fs/ext4" \
            "kernel/fs/fat" \
            "kernel/fs/nls" \
            "kernel/fs/isofs" \
            "kernel/fs/fuse" \
            "kernel/drivers/block/zram" \
            "kernel/drivers/block/loop.ko.gz" \
            "kernel/drivers/block/loop.ko" \
            "kernel/lib" \
            "kernel/lib/lz4" \
            "kernel/crypto" \
            "kernel/crypto/lz4.ko" \
            "kernel/crypto/lz4.ko.gz" \
            "kernel/crypto/lz4hc.ko" \
            "kernel/crypto/lz4hc.ko.gz" \
            "kernel/drivers/virtio" \
            "kernel/drivers/scsi" \
            "kernel/drivers/ata" \
            "kernel/drivers/cdrom" \
            "kernel/net/bridge" \
            "kernel/net/netfilter" \
            "kernel/net/ipv4/netfilter" \
            ; do
            if [ -e "$SRC_MODDIR/$mod_path" ]; then
                target_dir=$(dirname "$MODDIR/$mod_path")
                mkdir -p "$target_dir"
                cp -a "$SRC_MODDIR/$mod_path" "$MODDIR/$mod_path" 2>/dev/null || true
            fi
        done

        # Copy any LZ4 compression modules (required by zram)
        for f in $(find "$SRC_MODDIR" -name "lz4*" -name "*.ko*" 2>/dev/null); do
            rel_path="${f#$SRC_MODDIR/}"
            target_dir=$(dirname "$MODDIR/$rel_path")
            mkdir -p "$target_dir"
            cp -a "$f" "$MODDIR/$rel_path" 2>/dev/null || true
        done

        # Also copy any virtio net module
        for f in $(find "$SRC_MODDIR" -name "virtio_net*" -o -name "virtio_pci*" -o -name "virtio_ring*" -o -name "virtio.ko*" 2>/dev/null); do
            rel_path="${f#$SRC_MODDIR/}"
            target_dir=$(dirname "$MODDIR/$rel_path")
            mkdir -p "$target_dir"
            cp -a "$f" "$MODDIR/$rel_path" 2>/dev/null || true
        done

        # Copy modules.dep and related files
        for f in modules.dep modules.dep.bin modules.alias modules.alias.bin \
                 modules.symbols modules.symbols.bin modules.order modules.builtin \
                 modules.builtin.bin modules.builtin.modinfo; do
            if [ -f "$SRC_MODDIR/$f" ]; then
                cp "$SRC_MODDIR/$f" "$MODDIR/$f" 2>/dev/null || true
            fi
        done

        # Regenerate modules.dep for our subset
        depmod -b "$INITRAMFS_DIR" "$KERNEL_VERSION" 2>/dev/null || true
    fi

    # Copy our custom init script
    cp /build/os/initramfs/wraithos-init.sh "$INITRAMFS_DIR/init"
    chmod 755 "$INITRAMFS_DIR/init"

    # Create device nodes
    mknod "$INITRAMFS_DIR/dev/console" c 5 1 2>/dev/null || true
    mknod "$INITRAMFS_DIR/dev/null" c 1 3 2>/dev/null || true
    mknod "$INITRAMFS_DIR/dev/tty" c 5 0 2>/dev/null || true
    mknod "$INITRAMFS_DIR/dev/tty0" c 4 0 2>/dev/null || true
    mknod "$INITRAMFS_DIR/dev/tty1" c 4 1 2>/dev/null || true
    mknod "$INITRAMFS_DIR/dev/zero" c 1 5 2>/dev/null || true
    mknod "$INITRAMFS_DIR/dev/random" c 1 8 2>/dev/null || true
    mknod "$INITRAMFS_DIR/dev/urandom" c 1 9 2>/dev/null || true

    # Pack initramfs as cpio+gzip
    (cd "$INITRAMFS_DIR" && find . | cpio -o -H newc 2>/dev/null | gzip -9) > "$WORK/boot/initramfs"

    initramfs_size=$(du -sh "$WORK/boot/initramfs" | cut -f1)
    log "Initramfs built: $initramfs_size"
}

# -------------------------------------------------------
# Phase 4: Create squashfs image of rootfs
# -------------------------------------------------------
create_squashfs() {
    log "Creating squashfs image..."

    # Clean up unnecessary files from rootfs before squashing
    rm -rf "$ROOTFS/var/cache/apk"/* 2>/dev/null || true
    rm -rf "$ROOTFS/usr/share/man" 2>/dev/null || true
    rm -rf "$ROOTFS/usr/share/doc" 2>/dev/null || true

    # Remove boot directory from squashfs (kernel/initramfs are separate on ISO)
    rm -rf "$ROOTFS/boot" 2>/dev/null || true

    mksquashfs "$ROOTFS" "$WORK/boot/wraithos.sfs" \
        -comp xz \
        -b 256K \
        -Xbcj x86 \
        -noappend \
        -no-xattrs \
        -quiet

    sfs_size=$(du -sh "$WORK/boot/wraithos.sfs" | cut -f1)
    log "Squashfs image: $sfs_size"
}

# -------------------------------------------------------
# Phase 5: Create UEFI boot via GRUB
# -------------------------------------------------------
create_efi_image() {
    log "Creating UEFI boot image..."

    EFI_DIR="$WORK/efi"
    mkdir -p "$EFI_DIR/EFI/BOOT"
    mkdir -p "$EFI_DIR/boot/grub"

    # Create the main GRUB menu configuration (lives on ISO at /boot/grub/grub.cfg)
    cat > "$EFI_DIR/boot/grub/grub.cfg" <<'GRUBCFG'
set timeout=3
set default=0

menuentry "WraithOS" {
	linux	/boot/vmlinuz console=tty0 modules=loop,squashfs,overlay,lz4,lz4_compress,lz4hc,zram loglevel=7
	initrd	/boot/initramfs
}

menuentry "WraithOS (quiet)" {
	linux	/boot/vmlinuz console=tty0 modules=loop,squashfs,overlay,lz4,lz4_compress,lz4hc,zram quiet
	initrd	/boot/initramfs
}
GRUBCFG

    # Build GRUB EFI image with grub-mkimage (Alpine-style)
    # Uses -p /boot/grub so GRUB looks for its config on the ISO filesystem
    grub-mkimage \
        -o "$EFI_DIR/EFI/BOOT/BOOTX64.EFI" \
        -O x86_64-efi \
        -p /boot/grub \
        boot linux normal configfile \
        part_gpt part_msdos fat iso9660 \
        loopback search search_fs_uuid search_fs_file search_label \
        gfxterm all_video efi_gop efi_uga \
        ls cat echo test true

    log "GRUB EFI image created"

    # Create a small EFI boot image (FAT) containing ONLY the GRUB binary
    # Alpine style: no kernel/initramfs in the FAT image -- GRUB reads them
    # from the ISO9660 filesystem via its built-in iso9660 driver
    efi_size_kb=2048  # 2MB is plenty for GRUB binary only
    dd if=/dev/zero of="$WORK/efiboot.img" bs=1024 count=$efi_size_kb 2>/dev/null
    mkfs.vfat "$WORK/efiboot.img" >/dev/null 2>&1

    # Copy EFI bootloader into the FAT image
    mmd -i "$WORK/efiboot.img" ::/EFI
    mmd -i "$WORK/efiboot.img" ::/EFI/BOOT
    mcopy -i "$WORK/efiboot.img" "$EFI_DIR/EFI/BOOT/BOOTX64.EFI" ::/EFI/BOOT/BOOTX64.EFI

    log "EFI boot image created (${efi_size_kb}KB)"
}

# -------------------------------------------------------
# Phase 6: Assemble ISO with xorriso
# -------------------------------------------------------
assemble_iso() {
    log "Assembling ISO image..."

    mkdir -p "$ISODIR/boot/grub"

    # Copy boot files into ISO tree
    cp "$WORK/boot/vmlinuz" "$ISODIR/boot/"
    cp "$WORK/boot/initramfs" "$ISODIR/boot/"
    cp "$WORK/boot/wraithos.sfs" "$ISODIR/boot/"

    # Copy GRUB config
    cp "$EFI_DIR/boot/grub/grub.cfg" "$ISODIR/boot/grub/"

    # Copy GRUB font if available
    if [ -f /usr/share/grub/unicode.pf2 ]; then
        cp /usr/share/grub/unicode.pf2 "$ISODIR/boot/grub/"
    fi

    # Copy GRUB modules for x86_64-efi so grub can find them at boot
    if [ -d /usr/lib/grub/x86_64-efi ]; then
        cp -a /usr/lib/grub/x86_64-efi "$ISODIR/boot/grub/"
    fi

    # Place efiboot.img at /boot/grub/efi.img (Alpine convention)
    cp "$WORK/efiboot.img" "$ISODIR/boot/grub/efi.img"

    # Create the ISO (Alpine-style xorriso invocation)
    ISO_FILE="$OUTDIR/wraithos-${VERSION}.iso"

    xorriso -as mkisofs \
        -o "$ISO_FILE" \
        -R -J \
        -V "WRAITHOS" \
        -eltorito-alt-boot \
        -e '/boot/grub/efi.img' \
        -no-emul-boot \
        -boot-load-size 2880 \
        -isohybrid-gpt-basdat \
        "$ISODIR"

    if [ ! -f "$ISO_FILE" ]; then
        err "ISO file was not created"
    fi

    log "ISO assembled: $ISO_FILE"
}

# -------------------------------------------------------
# Phase 7: Print summary
# -------------------------------------------------------
print_summary() {
    ISO_FILE="$OUTDIR/wraithos-${VERSION}.iso"

    size_bytes=$(stat -c %s "$ISO_FILE" 2>/dev/null || wc -c < "$ISO_FILE")
    size_human=$(du -h "$ISO_FILE" | cut -f1)
    sha256sum_val=$(sha256sum "$ISO_FILE" | cut -d' ' -f1)

    echo ""
    echo "============================================"
    echo "  WraithOS ISO Build Complete"
    echo "============================================"
    echo "  Version:  $VERSION"
    echo "  File:     $ISO_FILE"
    echo "  Size:     $size_human ($size_bytes bytes)"
    echo "  SHA256:   $sha256sum_val"
    echo "============================================"
    echo ""

    if [ "$size_bytes" -gt 209715200 ]; then
        echo "WARNING: ISO exceeds 200MB target size"
    fi
}

# --- Main ---

log "WraithOS ISO Builder (Docker)"
log "Version: $VERSION"
log "Alpine: $ALPINE_BRANCH ($ARCH)"

mkdir -p "$WORK" "$OUTDIR"

create_rootfs
install_overlay
prepare_boot_files
create_squashfs
create_efi_image
assemble_iso
print_summary

log "Done!"
