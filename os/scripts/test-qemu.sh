#!/bin/bash
# test-qemu.sh - Boot WraithOS ISO in QEMU for testing
#
# Creates virtual config and cache disks, boots the ISO with UEFI
# (OVMF firmware), and forwards port 82 for web UI access.
#
# Prerequisites:
#   - qemu-system-x86_64
#   - OVMF UEFI firmware (usually in /usr/share/OVMF/)
#
# Usage:
#   ./test-qemu.sh                    # Boot latest build
#   ./test-qemu.sh path/to/image.iso  # Boot specific ISO
#   ./test-qemu.sh --create-disks     # Only create test disks

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
BUILD_DIR="$PROJECT_DIR/build"
TEST_DIR="$BUILD_DIR/test-vm"

# VM settings
VM_RAM="1024"          # MB -- enough for OS + a small container
VM_CPUS="2"
VM_CONFIG_DISK_SIZE="2G"   # Smaller than real for testing
VM_CACHE_DISK_SIZE="10G"

# OVMF firmware paths (varies by distro)
OVMF_PATHS=(
    "/usr/share/OVMF/OVMF_CODE.fd"
    "/usr/share/edk2/x64/OVMF_CODE.fd"
    "/usr/share/edk2-ovmf/OVMF_CODE.fd"
    "/usr/share/qemu/OVMF_CODE.fd"
    "/usr/share/ovmf/OVMF_CODE.fd"
)

log() {
    echo "[test-qemu] $*"
}

err() {
    echo "ERROR: $*" >&2
    exit 1
}

find_ovmf() {
    for path in "${OVMF_PATHS[@]}"; do
        if [ -f "$path" ]; then
            echo "$path"
            return 0
        fi
    done

    err "OVMF firmware not found. Install it:
  Alpine: apk add ovmf
  Debian/Ubuntu: apt install ovmf
  Fedora: dnf install edk2-ovmf"
}

find_iso() {
    local iso="${1:-}"

    if [ -n "$iso" ] && [ -f "$iso" ]; then
        echo "$iso"
        return 0
    fi

    # Find the most recently built ISO
    local latest
    latest=$(find "$BUILD_DIR" -maxdepth 1 -name "wraithos-*.iso" -type f \
        | sort -V | tail -1)

    if [ -z "$latest" ]; then
        err "No ISO found in $BUILD_DIR -- run build-iso.sh first"
    fi

    echo "$latest"
}

create_test_disks() {
    mkdir -p "$TEST_DIR"

    local config_disk="$TEST_DIR/config.qcow2"
    local cache_disk="$TEST_DIR/cache.qcow2"

    if [ ! -f "$config_disk" ]; then
        log "Creating config disk ($VM_CONFIG_DISK_SIZE)..."
        qemu-img create -f qcow2 "$config_disk" "$VM_CONFIG_DISK_SIZE"

        # Format the disk image with the expected label.
        # We use a temporary NBD mount to format it.
        if command -v nbdkit >/dev/null 2>&1 || [ -e /dev/nbd0 ]; then
            log "Note: Config disk created but not formatted"
            log "  It will be formatted on first boot if needed,"
            log "  or format manually with label WRAITH-CONFIG"
        fi
    else
        log "Config disk already exists: $config_disk"
    fi

    if [ ! -f "$cache_disk" ]; then
        log "Creating cache disk ($VM_CACHE_DISK_SIZE)..."
        qemu-img create -f qcow2 "$cache_disk" "$VM_CACHE_DISK_SIZE"
        log "Note: Cache disk created but not formatted"
        log "  Format manually with label WRAITH-CACHE"
    else
        log "Cache disk already exists: $cache_disk"
    fi

    log "Test disks ready in $TEST_DIR"
}

boot_vm() {
    local iso="$1"
    local ovmf="$2"

    log "Booting WraithOS in QEMU..."
    log "  ISO:    $iso"
    log "  RAM:    ${VM_RAM}MB"
    log "  CPUs:   $VM_CPUS"
    log "  OVMF:   $ovmf"
    log ""
    log "  Web UI: http://localhost:8082 (forwarded from guest port 82)"
    log "  SSH:    ssh -p 2222 localhost (if enabled)"
    log ""
    log "  Press Ctrl+A, X to quit QEMU"
    echo ""

    qemu-system-x86_64 \
        -name "wraithos-test" \
        -machine q35,accel=kvm:tcg \
        -cpu host \
        -m "$VM_RAM" \
        -smp "$VM_CPUS" \
        -drive "if=pflash,format=raw,readonly=on,file=$ovmf" \
        -cdrom "$iso" \
        -boot d \
        -drive "file=$TEST_DIR/config.qcow2,format=qcow2,if=virtio,id=config" \
        -drive "file=$TEST_DIR/cache.qcow2,format=qcow2,if=virtio,id=cache" \
        -netdev user,id=net0,hostfwd=tcp::8082-:82,hostfwd=tcp::2222-:22 \
        -device virtio-net-pci,netdev=net0 \
        -display none \
        -serial mon:stdio \
        -no-reboot
}

# --- Main ---

case "${1:-}" in
    --create-disks)
        create_test_disks
        exit 0
        ;;
    --help|-h)
        echo "Usage: $0 [ISO_PATH] [--create-disks|--help]"
        echo ""
        echo "Boots WraithOS in QEMU with UEFI firmware."
        echo "Port 82 (web UI) is forwarded to localhost:8082."
        exit 0
        ;;
esac

# Check for qemu
command -v qemu-system-x86_64 >/dev/null 2>&1 \
    || err "qemu-system-x86_64 not found. Install: apk add qemu-system-x86_64"

ovmf=$(find_ovmf)
iso=$(find_iso "${1:-}")
create_test_disks
boot_vm "$iso" "$ovmf"
