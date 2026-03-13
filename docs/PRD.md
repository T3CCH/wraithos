# WraithOS - Product Requirements Document

**Document Status:** Approved
**Version:** 1.0
**Date:** 2026-03-12
**Author:** T3CCH
**Repository:** git@github.com:T3CCH/wraithos.git

---

## Table of Contents

1. [Executive Summary](#1-executive-summary)
2. [Problem Statement](#2-problem-statement)
3. [Goals and Non-Goals](#3-goals-and-non-goals)
4. [Target Users](#4-target-users)
5. [System Architecture](#5-system-architecture)
6. [Functional Requirements](#6-functional-requirements)
7. [Non-Functional Requirements](#7-non-functional-requirements)
8. [Update and Backup Strategy](#8-update-and-backup-strategy)
9. [Future Roadmap](#9-future-roadmap)
10. [Technical Decisions Log](#10-technical-decisions-log)
11. [Success Criteria](#11-success-criteria)
12. [Phasing](#12-phasing)

---

## 1. Executive Summary

WraithOS is a minimal, RAM-based Linux operating system purpose-built to run Docker containers at RAM speed. Built on Alpine Linux, it boots from an ISO image that loads entirely into RAM, creating an immutable base layer with ephemeral overlay storage. The entire OS footprint targets approximately 512MB of RAM (before containers), with zram compression reducing actual physical memory usage to approximately 150-200MB.

The system is designed for single-purpose virtual machines: each WraithOS instance runs exactly one docker-compose stack, managed through a lightweight Go-based web UI on port 82. Configuration persists on a dedicated ext4 disk, while Docker images and volumes live on a separate cache disk. This separation means the OS itself is stateless and disposable -- updates are performed by swapping the ISO and rebooting.

WraithOS exists because running full Linux distributions as Docker hosts in VM environments is wasteful. Most of the OS is unused overhead. WraithOS strips that away, delivering a Docker runtime that boots in seconds, runs at memory speed, and can be reprovisioned from scratch in minutes.

---

## 2. Problem Statement

### Current Pain Points

**Bloated Docker Hosts:** Running Ubuntu Server, Debian, or similar distributions as Docker hosts in virtualized environments wastes significant resources. A typical Ubuntu Server install consumes 1-2GB of RAM and 10-25GB of disk before a single container runs. Most of those packages, services, and libraries are never used.

**Slow Boot and Recovery:** Traditional Linux distributions take 30-60 seconds to boot. When a VM needs reprovisioning or disaster recovery, the full OS installation process adds significant downtime.

**Configuration Drift:** Long-running Linux hosts accumulate configuration changes, installed packages, and state that make them difficult to reproduce. "Works on my machine" extends to "works on this specific VM that was set up 18 months ago."

**Management Overhead:** Each Docker host VM requires its own patching cycle, security updates, user management, and monitoring infrastructure. This does not scale when managing dozens of single-purpose VMs.

**Disk I/O Bottleneck:** Traditional disk-based operating systems create I/O contention between the OS, Docker's storage driver, and running containers. In virtualized environments with shared storage, this contention compounds.

### What WraithOS Solves

- Eliminates OS bloat by running entirely in RAM from a minimal Alpine base
- Reduces boot time to seconds (ISO loads to RAM, overlayfs mounts, services start)
- Eliminates configuration drift through immutable OS layer (changes vanish on reboot)
- Simplifies management through a purpose-built web UI (one compose stack per VM)
- Removes disk I/O bottleneck for OS operations (everything runs from RAM)
- Makes reprovisioning trivial (boot from ISO, point to config disk, done)

---

## 3. Goals and Non-Goals

### Goals (V1)

| ID | Goal | Measurable Target |
|----|------|-------------------|
| G1 | Minimal RAM footprint | OS + Docker daemon under 512MB before containers |
| G2 | Fast boot | ISO-to-running-containers in under 60 seconds |
| G3 | Immutable OS layer | All OS changes are ephemeral; reboot restores pristine state |
| G4 | Web-based management | Single-binary Go web UI on port 82 for all admin tasks |
| G5 | Docker-compose native | First-class support for single docker-compose stack per VM |
| G6 | Persistent configuration | Config survives reboots on dedicated ext4 disk |
| G7 | Network file access | Mount remote Samba/CIFS shares into containers |
| G8 | Simple updates | Swap ISO, reboot. No in-place package upgrades |
| G9 | Basic monitoring | CPU, RAM, disk, and container status dashboard |
| G10 | Firewall | nftables-based network security |

### Non-Goals (Explicitly Out of Scope)

| ID | Non-Goal | Rationale |
|----|----------|-----------|
| NG1 | General-purpose Linux distribution | WraithOS is single-purpose: run Docker containers |
| NG2 | Multi-stack orchestration | One VM = one compose stack. Use multiple VMs for multiple stacks |
| NG3 | Kubernetes / Swarm support | Out of scope. Docker Compose only |
| NG4 | GUI desktop environment | Headless only. Web UI for management |
| NG5 | Package manager for user software | No apk/apt for end users. OS image is immutable |
| NG6 | Legacy BIOS boot | UEFI only. Legacy systems are out of scope |
| NG7 | NFS server functionality | WraithOS mounts remote shares (client). It does not serve them |
| NG8 | Built-in container registry | Use external registries (Docker Hub, GHCR, private) |
| NG9 | Advanced log aggregation | Basic ring buffer + optional syslog forward. Use ELK/Loki externally |
| NG10 | Cluster management | Each WraithOS instance is independent |

---

## 4. Target Users

### Primary: Homelab Operators and Small Infrastructure Teams

- Run multiple VMs for different services (media server, home automation, dev environments)
- Want lightweight Docker hosts without the overhead of full Linux installs
- Value simplicity and reproducibility over flexibility
- Comfortable with docker-compose but prefer a web UI over SSH for daily management

### Secondary: Edge Computing and Appliance Builders

- Need a minimal OS to run a specific containerized application
- Want fast boot, small footprint, and easy reprovisioning
- May deploy dozens of identical instances via PXE (future phase)

### Anti-Users (Not Designed For)

- Users who need a general-purpose Linux workstation
- Teams requiring Kubernetes orchestration
- Environments that require legacy BIOS boot support
- Users who expect to install arbitrary packages on the host OS

---

## 5. System Architecture

### High-Level Architecture

```
+------------------------------------------------------------------+
|                        WraithOS VM                                |
|                                                                   |
|  +------------------------------------------------------------+  |
|  |                    RAM (Memory)                             |  |
|  |                                                             |  |
|  |  +------------------+  +-----------------------------+     |  |
|  |  |   squashfs       |  |      overlayfs (tmpfs)      |     |  |
|  |  |   (read-only     |  |      (ephemeral writes)     |     |  |
|  |  |    base image)   |  |                             |     |  |
|  |  +--------+---------+  +-------------+---------------+     |  |
|  |           |                          |                      |  |
|  |           +----------+---------------+                      |  |
|  |                      |                                      |  |
|  |              +-------v--------+                             |  |
|  |              |   Merged Root  |   zram compressed           |  |
|  |              |   Filesystem   |   (~150-200MB actual)       |  |
|  |              +-------+--------+                             |  |
|  |                      |                                      |  |
|  +------------------------------------------------------------+  |
|                         |                                         |
|           +-------------+-------------+                           |
|           |                           |                           |
|  +--------v---------+    +-----------v-----------+                |
|  |  OpenRC Init      |    |   Docker Daemon       |               |
|  |  (Alpine default) |    |   (overlay2 driver)   |               |
|  +--------+----------+    +-----------+-----------+               |
|           |                           |                           |
|  +--------v---------+    +-----------v-----------+                |
|  |  Web UI (Go)      |    |   docker-compose      |               |
|  |  Port 82           |    |   stack (single)      |               |
|  +-------------------+    +-----------------------+               |
|                                                                   |
|  +---------------------------+  +-----------------------------+   |
|  |   Config Disk (ext4)      |  |   Docker Cache Disk (ext4)  |   |
|  |   ~100MB                  |  |   Size varies               |   |
|  |                           |  |                              |   |
|  |   - wraithos.conf         |  |   - /var/lib/docker/         |   |
|  |   - credentials           |  |     (images, layers)         |   |
|  |   - docker-compose.yml    |  |   - container volumes        |   |
|  |   - network settings      |  |   - mounted Samba data       |   |
|  |   - TLS certificates      |  |                              |   |
|  +---------------------------+  +-----------------------------+   |
+------------------------------------------------------------------+
```

### Disk Layout

```
+---------------------+     +------------------------+
|   ISO / Boot Media  |     |   Virtual Disk 1       |
|                     |     |   "Config Disk"        |
|   UEFI bootable     |     |   ext4, ~100MB         |
|   Contains:         |     |                        |
|   - GRUB/systemd-   |     |   /mnt/config/         |
|     boot             |     |     credentials.bcrypt |
|   - squashfs root   |     |     compose.yml        |
|   - kernel + initrd |     |     network.conf       |
|                     |     |     wraithos.conf       |
+---------------------+     |     tls/               |
                             +------------------------+

                             +------------------------+
                             |   Virtual Disk 2       |
                             |   "Docker Cache Disk"  |
                             |   ext4, size varies    |
                             |                        |
                             |   /var/lib/docker/     |
                             |     overlay2/          |
                             |     volumes/           |
                             |   /mnt/samba/          |
                             |     (mount points)     |
                             +------------------------+
```

### Boot Flow

```
UEFI Firmware
    |
    v
GRUB / systemd-boot (from ISO)
    |
    v
Load kernel + initrd into RAM
    |
    v
initrd mounts squashfs into RAM
    |
    v
overlayfs merges squashfs (lower) + tmpfs (upper)
    |
    v
zram compresses RAM filesystem (~500MB -> ~150-200MB actual)
    |
    v
Pivot root to merged filesystem
    |
    v
OpenRC init starts
    |
    +---> Mount config disk (ext4) at /mnt/config
    |
    +---> Mount docker cache disk (ext4) at /var/lib/docker
    |
    +---> Configure networking (DHCP or static from config)
    |
    +---> Start Docker daemon (overlay2 storage driver)
    |
    +---> Start web management UI (Go binary, port 82)
    |
    +---> Mount Samba shares (if configured)
    |
    +---> Start docker-compose stack (if auto-start enabled)
    |
    v
System Ready
```

---

## 6. Functional Requirements

### 6.1 Boot System

**Priority: P0 (Must Have)**

| ID | Requirement | Details |
|----|-------------|---------|
| BOOT-1 | UEFI boot only | No legacy BIOS support. GRUB or systemd-boot as bootloader |
| BOOT-2 | ISO-to-RAM loading | Entire squashfs root image loads into RAM at boot |
| BOOT-3 | squashfs base layer | Read-only compressed filesystem as the immutable OS layer |
| BOOT-4 | overlayfs merge | tmpfs upper layer merged with squashfs lower layer for writable root |
| BOOT-5 | zram compression | Compress RAM filesystem to reduce physical memory usage (~500MB logical to ~150-200MB physical) |
| BOOT-6 | OpenRC init | Use Alpine's default init system. No systemd |
| BOOT-7 | Auto-mount config disk | Detect and mount config disk (ext4) on boot at /mnt/config |
| BOOT-8 | Auto-mount cache disk | Detect and mount docker cache disk (ext4) on boot at /var/lib/docker |
| BOOT-9 | Boot time target | ISO to system-ready in under 60 seconds |
| BOOT-10 | Graceful missing disk | Boot to web UI even if config or cache disks are missing (first-run setup mode) |

**Acceptance Criteria:**
- System boots from ISO without requiring installed disk OS
- After reboot, all OS-level changes from the previous session are gone
- zram is active and measurably reducing RAM usage
- Config and cache disks are mounted automatically when present

### 6.2 Storage System

**Priority: P0 (Must Have)**

| ID | Requirement | Details |
|----|-------------|---------|
| STOR-1 | Config disk filesystem | ext4, approximately 100MB |
| STOR-2 | Config disk contents | Credentials (bcrypt), docker-compose.yml, network config, TLS certs, wraithos.conf |
| STOR-3 | Docker cache disk filesystem | ext4, user-determined size |
| STOR-4 | Docker cache disk contents | Docker image cache (overlay2), container volumes, Samba mount data |
| STOR-5 | Docker storage driver | overlay2 on ext4 cache disk |
| STOR-6 | Volume default location | Container volumes default to cache disk |
| STOR-7 | RAM disk option | Optional RAM-backed tmpfs volumes for ephemeral container data |
| STOR-8 | Disk detection | Auto-detect config and cache disks by label or UUID |

**Acceptance Criteria:**
- Config disk survives reboots with all settings intact
- Docker images persist on cache disk across reboots
- New WraithOS instance can be pointed at an existing config disk and resume operation

### 6.3 Web Management UI

**Priority: P0 (Must Have)**

| ID | Requirement | Details |
|----|-------------|---------|
| UI-1 | Technology | Go single static binary, zero runtime dependencies |
| UI-2 | Port | Listens on port 82 |
| UI-3 | Authentication | File-based auth with bcrypt password hash stored on config disk |
| UI-4 | Session management | Session cookies for authenticated access |
| UI-5 | First-run setup | On first boot (no credentials file), prompt to create admin credentials |
| UI-6 | Dashboard | Overview showing CPU, RAM, disk usage, container status |
| UI-7 | Compose editor | Web-based YAML editor for docker-compose.yml with syntax highlighting |
| UI-8 | Compose controls | Start, stop, restart, pull, and update docker-compose stack |
| UI-9 | Container logs | View container logs from the web UI |
| UI-10 | Samba mount management | Add, remove, and configure Samba/CIFS mount points |
| UI-11 | Network configuration | View current IP, switch between DHCP and static IP |
| UI-12 | TLS support | HTTPS for the web UI using certificates on config disk |

**Acceptance Criteria:**
- Web UI loads in a browser without installing any client-side dependencies
- Admin can create credentials on first boot through the web interface
- Compose stack can be fully managed (edit, start, stop, update) without SSH access
- Dashboard auto-refreshes and shows real-time system metrics

### 6.4 Docker Integration

**Priority: P0 (Must Have)**

| ID | Requirement | Details |
|----|-------------|---------|
| DOCK-1 | Compose-only model | Docker Compose is the only supported deployment method. No raw docker run |
| DOCK-2 | Single stack per VM | Each WraithOS instance runs exactly one docker-compose stack |
| DOCK-3 | Compose file location | docker-compose.yml stored on config disk |
| DOCK-4 | Image caching | Docker images cached on docker cache disk (persist across reboots) |
| DOCK-5 | Volume management | Volumes stored on cache disk by default, optional RAM disk |
| DOCK-6 | Auto-start | Option to automatically start the compose stack on boot |
| DOCK-7 | Pull updates | Ability to pull latest images and recreate containers from web UI |
| DOCK-8 | Environment variables | Support for .env file alongside docker-compose.yml on config disk |

**Acceptance Criteria:**
- Docker daemon starts automatically on boot with overlay2 driver
- Compose stack starts automatically (if configured) after Docker daemon is ready
- Images survive reboot (cached on disk)
- User can edit compose file and deploy changes entirely through web UI

### 6.5 Networking

**Priority: P0 (Must Have)**

| ID | Requirement | Details |
|----|-------------|---------|
| NET-1 | Default DHCP | Boot with DHCP by default on first run |
| NET-2 | Static IP override | Configure static IP, gateway, DNS through web UI |
| NET-3 | Network config persistence | Network settings stored on config disk, applied on boot |
| NET-4 | Firewall | nftables-based firewall with sensible defaults |
| NET-5 | Default firewall rules | Allow: port 82 (web UI), Docker published ports. Block: all other inbound |
| NET-6 | DNS configuration | Configurable DNS servers (default to DHCP-provided) |

**Acceptance Criteria:**
- System obtains an IP address via DHCP on first boot without configuration
- Static IP settings persist across reboots
- nftables rules are active and blocking unexpected inbound traffic
- Docker container port mappings work correctly through the firewall

### 6.6 Samba Integration

**Priority: P1 (Should Have)**

| ID | Requirement | Details |
|----|-------------|---------|
| SMB-1 | Client only | WraithOS mounts remote Samba shares. It does not serve shares |
| SMB-2 | cifs-utils | Use cifs-utils for mounting CIFS/SMB shares |
| SMB-3 | Web UI controls | Add, edit, remove Samba mount configurations from the web UI |
| SMB-4 | Credentials storage | Samba credentials stored securely on config disk |
| SMB-5 | Mount into containers | Samba mounts available as bind mounts for Docker containers |
| SMB-6 | Auto-mount on boot | Configured Samba shares mount automatically on boot |
| SMB-7 | Mount point location | Samba shares mounted under /mnt/samba/ on the cache disk |

**Acceptance Criteria:**
- Remote Samba share can be configured and mounted entirely through web UI
- Mounted share is accessible as a volume inside Docker containers
- Samba mounts reconnect automatically on boot
- Credentials are not exposed in plain text in any config file

### 6.7 Logging

**Priority: P1 (Should Have)**

| ID | Requirement | Details |
|----|-------------|---------|
| LOG-1 | Hybrid logging | RAM ring buffer for local viewing + optional external syslog |
| LOG-2 | Ring buffer | Fixed-size in-memory log buffer viewable from web UI |
| LOG-3 | Syslog forwarding | Optional configuration to forward logs to external syslog server |
| LOG-4 | Container logs | Docker container logs viewable through web UI (docker logs API) |
| LOG-5 | Log rotation | Ring buffer self-manages size. No disk log rotation needed |
| LOG-6 | Boot logs | Capture boot sequence logs for troubleshooting |

**Acceptance Criteria:**
- System logs are viewable in the web UI without SSH access
- Log ring buffer does not grow unbounded (fixed memory allocation)
- Optional syslog forwarding works when configured
- Container logs are accessible per-container in the web UI

### 6.8 Monitoring

**Priority: P1 (Should Have)**

| ID | Requirement | Details |
|----|-------------|---------|
| MON-1 | Data sources | /proc filesystem for host metrics, Docker API for container metrics |
| MON-2 | CPU metrics | Current CPU usage percentage |
| MON-3 | Memory metrics | Total RAM, used RAM, available RAM, zram compression ratio |
| MON-4 | Disk metrics | Config disk and cache disk usage (used/total/percentage) |
| MON-5 | Container metrics | Per-container status (running/stopped/error), CPU, memory usage |
| MON-6 | Dashboard display | Real-time dashboard in the web UI with auto-refresh |
| MON-7 | No external dependencies | No Prometheus, Grafana, or other monitoring stack required |

**Acceptance Criteria:**
- Dashboard loads within 2 seconds and shows current system state
- Metrics update automatically without manual page refresh
- All monitoring data comes from local sources (no external agents)
- Dashboard works on mobile browsers (responsive layout)

---

## 7. Non-Functional Requirements

### 7.1 Performance

| ID | Requirement | Target |
|----|-------------|--------|
| PERF-1 | OS RAM footprint | Under 512MB (OS + Docker daemon, before containers) |
| PERF-2 | Physical RAM with zram | ~150-200MB actual physical RAM for OS layer |
| PERF-3 | Boot time | Under 60 seconds from power-on to system-ready |
| PERF-4 | Web UI response time | Page loads under 500ms on local network |
| PERF-5 | ISO image size | Under 200MB |
| PERF-6 | Docker daemon start | Docker ready within 15 seconds of init |

### 7.2 Security

| ID | Requirement | Details |
|----|-------------|---------|
| SEC-1 | Security posture | Moderate (not hardened for hostile networks, but not wide open) |
| SEC-2 | Firewall | nftables with default-deny inbound policy |
| SEC-3 | Non-root services | Web UI and Docker containers run as non-root where possible |
| SEC-4 | TLS for web UI | HTTPS support with user-provided or self-signed certificates |
| SEC-5 | Credential storage | bcrypt hashed passwords, no plain text credentials |
| SEC-6 | Immutable OS | squashfs base layer cannot be modified at runtime |
| SEC-7 | Minimal attack surface | No unnecessary packages, services, or open ports |

### 7.3 Reliability

| ID | Requirement | Details |
|----|-------------|---------|
| REL-1 | Stateless recovery | Any WraithOS instance can be rebuilt from ISO + config disk |
| REL-2 | Config disk resilience | Config disk is the single source of truth; losing it means reconfiguration only |
| REL-3 | Docker image recovery | Images are re-pullable from registries if cache disk is lost |
| REL-4 | Graceful degradation | System boots to web UI even if Docker daemon fails |
| REL-5 | No single point of failure | No dependency on external services for basic operation |

### 7.4 Maintainability

| ID | Requirement | Details |
|----|-------------|---------|
| MAINT-1 | ISO-based updates | Replace ISO, reboot. No in-place package management |
| MAINT-2 | Build tooling | Alpine mkimage (atools) for reproducible ISO builds |
| MAINT-3 | Single binary web UI | Go binary with embedded assets. No npm, no bundlers at runtime |
| MAINT-4 | Minimal dependencies | musl libc (Alpine), no glibc compatibility layer |

---

## 8. Update and Backup Strategy

### Update Process

```
Current State                    Update Process
+------------------+            +------------------+
| Running WraithOS |  ------>   | 1. Download new  |
| (ISO v1.0)       |            |    ISO (v1.1)    |
+------------------+            +------------------+
                                         |
                                         v
                                +------------------+
                                | 2. Replace ISO   |
                                |    in VM config  |
                                +------------------+
                                         |
                                         v
                                +------------------+
                                | 3. Reboot VM     |
                                +------------------+
                                         |
                                         v
                                +------------------+
                                | 4. New OS loads   |
                                |    from RAM       |
                                | 5. Config disk    |
                                |    auto-mounts    |
                                | 6. Stack restarts |
                                +------------------+
```

**Key properties:**
- OS updates never touch the config disk or cache disk
- Rollback is trivial: swap back to the old ISO and reboot
- Config and data survive all updates
- Docker images persist on cache disk (no re-pull needed unless cache disk is wiped)

### Backup Strategy

| What | How | Frequency |
|------|-----|-----------|
| Configuration | Export config disk contents via web UI (zip/tar archive) | On-demand or scheduled |
| Docker Compose file | Included in config export | With config |
| Credentials | Included in config export (hashed) | With config |
| Docker images | Not backed up (re-pullable from registries) | N/A |
| Container volumes | User responsibility (bind mount to Samba share or external storage) | User-defined |
| OS itself | ISO file (versioned, downloadable) | Per release |

**What is NOT backed up:**
- Docker images (re-pullable)
- Ephemeral RAM data (by design)
- Container runtime state (recreated from compose file)

---

## 9. Future Roadmap

### V2 Features (Post-V1)

| ID | Feature | Description | Priority |
|----|---------|-------------|----------|
| FUT-1 | PXE boot (iPXE) | Network boot via iPXE chainloading. Boot WraithOS without local ISO | High |
| FUT-2 | Remote compose URL | Pull docker-compose.yml from a remote URL (HTTP/HTTPS) on boot | High |
| FUT-3 | Multi-NIC support | Configure multiple network interfaces through web UI | Medium |
| FUT-4 | Webhook notifications | HTTP webhooks for container state changes (start/stop/crash) | Medium |
| FUT-5 | API endpoint | REST API for programmatic management (in addition to web UI) | Medium |
| FUT-6 | SSH toggle | Optional SSH server (disabled by default, enable via web UI) | Low |
| FUT-7 | Custom firewall rules | User-defined nftables rules through web UI | Low |
| FUT-8 | NFS client support | Mount NFS shares in addition to Samba/CIFS | Low |

### Long-Term Vision

- Fleet management: central dashboard to monitor multiple WraithOS instances
- Template library: pre-built compose stacks for common applications
- Auto-update: schedule ISO updates with automatic reboot windows

---

## 10. Technical Decisions Log

All decisions made during the requirements interview phase with rationale.

| # | Decision | Chosen Option | Alternatives Considered | Rationale |
|---|----------|---------------|------------------------|-----------|
| 1 | Base distribution | Alpine Linux | Void, Buildroot, custom | musl libc, ~5MB base, existing Docker support, apk for build-time packages, large community |
| 2 | Init system | OpenRC | systemd, runit, s6 | Alpine default, simple, well-documented, no binary logs |
| 3 | RAM filesystem | squashfs + overlayfs + zram | tmpfs only, ramfs | squashfs provides compression, overlayfs provides write layer, zram reduces physical RAM |
| 4 | Boot mode | UEFI only | UEFI + BIOS, BIOS only | Modern VMs all support UEFI. Eliminates legacy complexity |
| 5 | Bootloader | GRUB or systemd-boot | SYSLINUX, rEFInd | Both are mature UEFI bootloaders. Decision deferred to implementation |
| 6 | Web UI language | Go | Rust, Python+Flask, Node.js | Single static binary, zero runtime deps, excellent HTTP stdlib, fast compilation |
| 7 | Web UI port | 82 | 80, 443, 8080, 9090 | Avoids conflict with containerized web apps on 80/443. Close to 80 for memorability |
| 8 | Authentication | File-based bcrypt + session cookies | OAuth, LDAP, PAM | Simple, no external dependencies, appropriate for single-user/small-team management |
| 9 | Docker model | Compose-only, single stack | Raw docker run, multiple stacks, Podman | Compose is the standard for declarative container management. Single stack enforces simplicity |
| 10 | Storage layout | Two ext4 disks (config + cache) | Single disk with partitions, btrfs, ZFS | ext4 is universal, two disks allow independent management, no kernel module complexity |
| 11 | Docker storage driver | overlay2 | devicemapper, btrfs, zfs | overlay2 is Docker's recommended driver, works on ext4, best performance |
| 12 | Network default | DHCP with static override | Static only, NetworkManager | DHCP works out of the box. Static override for production use |
| 13 | Firewall | nftables | iptables, ufw | nftables is the modern Linux firewall. iptables is legacy |
| 14 | Samba approach | cifs-utils client only | Samba server, NFS | Client-only keeps WraithOS simple. Mounting remote shares is the use case, not serving |
| 15 | Logging | Hybrid RAM ring buffer + optional syslog | journald, file-based, ELK | RAM buffer is fast and bounded. Syslog forward for those who need it. No disk writes |
| 16 | Monitoring | /proc + Docker API (built-in) | Prometheus node_exporter, Telegraf | Zero additional dependencies. Dashboard reads from kernel and Docker directly |
| 17 | Update mechanism | ISO swap + reboot | In-place apt/apk upgrade, A/B partitions | Immutable OS philosophy. Swap and reboot is atomic and rollback-safe |
| 18 | Backup scope | Config-only export | Full disk image, Docker volume backup | Config is small and portable. Images are re-pullable. Volumes are user-managed |
| 19 | Build tooling | Alpine mkimage (atools) | Packer, custom scripts, Buildroot | Native Alpine tool for ISO creation. Well-documented, reproducible |
| 20 | RAM target | 512MB for OS + Docker daemon | 256MB, 1GB | 512MB provides headroom for Docker daemon and OS services. zram compresses to ~150-200MB actual |

---

## 11. Success Criteria

### V1 is "Done" When

| # | Criterion | Verification Method |
|---|-----------|-------------------|
| SC-1 | ISO boots in a UEFI VM and loads entirely into RAM | Boot test on QEMU/KVM and VMware/Proxmox |
| SC-2 | OS + Docker daemon uses under 512MB RAM | Measure with `free -m` after boot, before containers |
| SC-3 | zram is active and compressing RAM | Verify with `zramctl` showing compression ratio |
| SC-4 | Config disk mounts automatically and persists settings | Reboot test: settings survive, OS changes do not |
| SC-5 | Docker cache disk stores images across reboots | Pull image, reboot, verify image still present |
| SC-6 | Web UI is accessible on port 82 | Browser test from another machine on the network |
| SC-7 | First-run credential creation works | Boot fresh (no config disk data), create admin through web UI |
| SC-8 | Compose stack can be edited and deployed from web UI | Edit compose YAML in browser, click deploy, verify containers start |
| SC-9 | Samba share can be mounted and used by containers | Mount remote share via web UI, reference as volume in compose |
| SC-10 | nftables firewall is active with default rules | Port scan from external machine shows only 82 + docker ports |
| SC-11 | Dashboard shows CPU, RAM, disk, container status | Visual inspection of dashboard accuracy |
| SC-12 | ISO can be swapped and rebooted without losing config | Replace ISO, reboot, verify all settings and compose stack intact |
| SC-13 | Config export produces a restorable archive | Export config, wipe config disk, import, verify restoration |
| SC-14 | Boot-to-ready time under 60 seconds | Timed boot test |
| SC-15 | System runs stable for 72 hours under load | Deploy a compose stack, let it run, verify no crashes or memory leaks |

---

## 12. Phasing

### Phase 1: V1 Core (MVP)

**Goal:** Bootable, manageable WraithOS that runs Docker Compose stacks.

| Component | Scope |
|-----------|-------|
| Boot system | UEFI boot, squashfs+overlayfs+zram, OpenRC init |
| Storage | Config disk + cache disk auto-detection and mounting |
| Docker | Daemon with overlay2, single compose stack, auto-start |
| Web UI | Authentication, dashboard, compose editor, compose controls |
| Networking | DHCP + static IP, nftables with defaults |
| Samba | CIFS mount via web UI, expose to containers |
| Logging | RAM ring buffer viewable in web UI |
| Monitoring | Basic dashboard (CPU/RAM/disk/containers) |
| Updates | ISO swap + reboot |
| Backup | Config-only export via web UI |
| Build | Alpine mkimage ISO build pipeline |

**Estimated Effort:** This is the full V1 implementation.

### Phase 2: V2 Network Boot and Automation

**Goal:** Enable hands-free provisioning and remote configuration.

| Component | Scope |
|-----------|-------|
| PXE boot | iPXE chainloading support (boot without local ISO) |
| Remote compose | Pull docker-compose.yml from HTTP/HTTPS URL on boot |
| API | REST API for programmatic management |
| Webhooks | HTTP notifications for container state changes |

**Prerequisite:** V1 complete and stable.

### Phase 3: Future Enhancements

**Goal:** Quality-of-life improvements based on real-world usage.

| Component | Scope |
|-----------|-------|
| Multi-NIC | Multiple network interface configuration |
| SSH toggle | Optional SSH server, disabled by default |
| Custom firewall | User-defined nftables rules via web UI |
| NFS client | Mount NFS shares alongside Samba |
| Fleet management | Central dashboard for multiple WraithOS instances |
| Template library | Pre-built compose stacks for common applications |
| Auto-update | Scheduled ISO updates with reboot windows |

**Prerequisite:** V2 complete. Prioritization driven by user feedback.

---

## Appendix A: Glossary

| Term | Definition |
|------|-----------|
| squashfs | Compressed, read-only filesystem commonly used in live Linux distributions |
| overlayfs | Union filesystem that merges a read-only lower layer with a writable upper layer |
| zram | Linux kernel feature that creates compressed RAM-based block devices |
| tmpfs | Temporary filesystem stored in RAM (used as overlayfs upper layer) |
| OpenRC | Init system used by Alpine Linux and Gentoo |
| overlay2 | Docker's default storage driver, uses Linux overlayfs |
| nftables | Modern Linux kernel packet filtering framework (replacement for iptables) |
| cifs-utils | Linux utilities for mounting SMB/CIFS network shares |
| UEFI | Unified Extensible Firmware Interface (modern BIOS replacement) |
| mkimage | Alpine Linux tool for creating bootable ISO images |
| bcrypt | Password hashing algorithm designed for secure credential storage |

## Appendix B: Open Questions

None at this time. All requirements have been clarified through the interview process. Questions may arise during implementation and will be documented here.

---

*This document is the source of truth for the WraithOS project. All implementation decisions should reference this PRD. Changes to requirements should be reflected here before implementation begins.*
