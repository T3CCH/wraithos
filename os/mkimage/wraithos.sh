# wraithos.sh - Alpine mkimage profile for WraithOS
#
# This profile builds a UEFI-bootable ISO containing:
#   - Alpine Linux with virt kernel (VM-optimized)
#   - Docker + docker-compose
#   - Networking (nftables, cifs-utils, nfs-utils)
#   - squashfs root filesystem for RAM-based boot
#   - GRUB EFI bootloader
#
# Usage: Called by mkimage.sh (alpine atools) -- not run directly.
#   ./mkimage.sh --profile wraithos --arch x86_64
#
# The profile functions follow Alpine mkimage conventions:
#   profile_wraithos()       - Declare profile metadata
#   build_wraithos()         - Customize the root filesystem
#   section_wraithos_grub()  - GRUB configuration

profile_wraithos() {
    title="WraithOS"
    desc="Minimal RAM-based Alpine Linux for Docker containers"
    profile_abbrev="wraithos"
    image_ext="iso"
    arch="x86_64"

    # VM-optimized kernel (smaller, faster for virtualization)
    kernel_flavors="virt"

    # Use GRUB for UEFI boot
    boot_addons=""
    initfs_features="squashfs overlay zram"
    grub_mod="part_gpt part_msdos fat iso9660 normal boot linux loopback squash4"

    # Core Alpine + WraithOS requirements
    apks="
        alpine-base
        openrc
        busybox-openrc

        linux-virt
        linux-firmware-none

        docker
        docker-cli-compose

        nftables
        cifs-utils
        nfs-utils
        e2fsprogs
        squashfs-tools
        zram-init

        grub-efi
        efibootmgr
        mkinitfs

        blkid
        util-linux
        lsblk

        ca-certificates
        curl
        tzdata
    "

    # Packages only needed during ISO build, not in the final image
    apks_dev=""

    local _k
    for _k in $kernel_flavors; do
        apks="$apks linux-$_k"
    done
}

build_wraithos() {
    local _srcdir="${startdir:-.}"
    local _rootfs="$DESTDIR"

    msg "Installing WraithOS overlay files..."

    # Copy rootfs overlay (init scripts, config, etc.)
    if [ -d "$_srcdir/../rootfs" ]; then
        cp -a "$_srcdir/../rootfs/"* "$_rootfs/"
    fi

    # Install the web UI binary if it exists
    local _ui_binary="$_srcdir/../../wraith-ui"
    if [ -f "$_ui_binary" ]; then
        install -m 755 "$_ui_binary" "$_rootfs/usr/bin/wraith-ui"
        msg "Installed wraith-ui binary"
    else
        msg "WARNING: wraith-ui binary not found at $_ui_binary"
        msg "  Build it first: cd ../.. && go build -o wraith-ui ./cmd/wraith-ui"
    fi

    # Install custom initramfs hook
    if [ -d "$_srcdir/../initramfs" ]; then
        install -m 755 "$_srcdir/../initramfs/wraithos-init.sh" \
            "$_rootfs/usr/share/mkinitfs/wraithos-init.sh"
    fi

    # Make init scripts executable
    chmod 755 "$_rootfs"/etc/init.d/wraith-* 2>/dev/null || true
    chmod 755 "$_rootfs"/etc/local.d/*.start 2>/dev/null || true

    # Enable WraithOS services at default runlevel
    for svc in docker wraith-disks wraith-network wraith-docker wraith-samba wraith-ui; do
        mkdir -p "$_rootfs/etc/runlevels/default"
        ln -sf "/etc/init.d/$svc" "$_rootfs/etc/runlevels/default/$svc"
    done

    # Enable local.d for firstboot script
    mkdir -p "$_rootfs/etc/runlevels/default"
    ln -sf /etc/init.d/local "$_rootfs/etc/runlevels/default/local"

    # Enable core services
    for svc in devfs dmesg hwdrivers mdev sysfs; do
        mkdir -p "$_rootfs/etc/runlevels/sysinit"
        ln -sf "/etc/init.d/$svc" "$_rootfs/etc/runlevels/sysinit/$svc"
    done

    for svc in modules sysctl hostname bootmisc syslog cgroups; do
        mkdir -p "$_rootfs/etc/runlevels/boot"
        ln -sf "/etc/init.d/$svc" "$_rootfs/etc/runlevels/boot/$svc"
    done

    for svc in mount-ro killprocs savecache; do
        mkdir -p "$_rootfs/etc/runlevels/shutdown"
        ln -sf "/etc/init.d/$svc" "$_rootfs/etc/runlevels/shutdown/$svc"
    done

    # Set hostname
    echo "wraithos" > "$_rootfs/etc/hostname"

    # Configure /etc/hosts
    cat > "$_rootfs/etc/hosts" <<EOF
127.0.0.1   localhost wraithos
::1         localhost wraithos
EOF

    # Minimal fstab -- real mounts happen via wraith-disks service
    cat > "$_rootfs/etc/fstab" <<EOF
# WraithOS - disks mounted dynamically by wraith-disks service
tmpfs   /tmp    tmpfs   nosuid,nodev,noatime    0 0
tmpfs   /run    tmpfs   nosuid,nodev,mode=755   0 0
EOF

    # Set timezone to UTC
    cp "$_rootfs/usr/share/zoneinfo/UTC" "$_rootfs/etc/localtime"
    echo "UTC" > "$_rootfs/etc/timezone"

    # Set root password for console access (debug/emergency)
    # Password: wraithos
    local _hash
    _hash=$(openssl passwd -6 "wraithos")
    # Use awk instead of sed to avoid $ interpolation issues in hash
    awk -v hash="$_hash" 'BEGIN{FS=OFS=":"} /^root:/ {$2=hash} 1' "$_rootfs/etc/shadow" > "$_rootfs/etc/shadow.tmp"
    mv "$_rootfs/etc/shadow.tmp" "$_rootfs/etc/shadow"
    chmod 640 "$_rootfs/etc/shadow"

    msg "WraithOS overlay installed"
}

# GRUB configuration for UEFI boot.
# Loads the kernel and initramfs, passing boot parameters for
# the squashfs+overlayfs RAM boot.
section_wraithos_grub() {
    local _flavor="$1"
    local _kernel="vmlinuz-$_flavor"
    local _initrd="initramfs-$_flavor"

    cat <<EOF

set timeout=3
set default=0

menuentry "WraithOS" {
    linux /boot/$_kernel modules=loop,squashfs,overlay,lz4,lz4_compress,lz4hc,zram quiet
    initrd /boot/$_initrd
}

menuentry "WraithOS (verbose boot)" {
    linux /boot/$_kernel modules=loop,squashfs,overlay,lz4,lz4_compress,lz4hc,zram loglevel=7
    initrd /boot/$_initrd
}
EOF
}
