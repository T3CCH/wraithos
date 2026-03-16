# WraithOS

**A lightweight, single-binary Docker container host OS that boots from ISO.**

WraithOS is designed for self-hosters running minimal Docker hosts on XCP-ng, QEMU, or other hypervisors. Boot it from an ISO, configure it through a web browser, and deploy Docker containers without managing a full operating system.

## What Is WraithOS?

Think of WraithOS as a purpose-built appliance, not a general-purpose operating system. It's built on Alpine Linux and runs entirely from RAM (about 512MB), making it blazing fast. A single statically-linked Go binary handles the entire web UI, API, and system management. All your configuration and data gets saved to a dedicated disk, so even though the OS vanishes when you reboot, your settings and containers stay intact.

**Key features:**
- Boots in seconds (under 60 seconds from power-on to ready)
- Runs entirely in RAM for speed and immutability
- Single 20MB binary embeds all static assets (no separate frontend build)
- Manage everything through a web browser on port 82
- Multi-stack Docker Compose management (Portainer-like card grid UI)
- Live container logs via WebSocket per stack
- Automatically mounts network shares (Samba/CIFS and NFS) for your containers
- Survives reboots with persistent configuration on a dedicated disk
- Optional SSH access with auto-shutdown after 30min idle

**Who is this for?**
- Homelab enthusiasts running media servers, home automation, or dev environments
- Sysadmins wanting lightweight Docker hosts without bloated Linux distributions
- Anyone building appliances or edge computing devices that need minimal overhead

## Getting Started

### What You'll Need

- A computer or VM with UEFI firmware (all modern systems have this)
- At least 1GB of RAM (512MB recommended minimum)
- Two storage devices or partitions:
  - A boot device with the WraithOS ISO
  - A configuration disk (~100MB) to store your settings and Docker configuration
  - (Optional) A separate cache disk for Docker images and container data

### Installation

1. **Download the ISO** from the latest release and flash it to a USB drive or attach it to your VM
2. **Boot from the ISO** and select UEFI boot mode if prompted
3. **Open your browser** and navigate to `http://[your-device-ip]:82`
4. **Run the setup wizard** to configure:
   - Which disks to use for configuration and Docker data
   - Your network settings (DHCP or static IP)
   - Your timezone
   - Admin password for the web interface

That's it. WraithOS is now running.

## Using WraithOS

Everything you do in WraithOS happens through the web interface. There's no command line needed for normal operation.

### The Dashboard

When you log in, you see the Dashboard. It shows:
- **System Status**: CPU usage, memory, and disk space (real-time metrics)
- **Docker Images**: Manage and view all images (size, layers, creation date)
- **Uptime**: How long your system has been running since the last reboot
- **Stack Status**: Overview of all deployed Docker Compose stacks

### Multi-Stack Management

WraithOS supports multiple independent Docker Compose stacks, each with its own configuration, volumes, and environment. The **Stacks** view displays them as a card grid (similar to Portainer), where you can:

1. Click **+** to create a new stack
2. Enter a name and your Docker Compose YAML
3. Configure per-stack mount requirements (which network shares this stack needs)
4. Manage environment variables (`.env` file editor)
5. View live container logs via WebSocket
6. Start, stop, restart, or delete each stack independently

Each stack runs in its own namespace with its own Docker Compose project.

### Setting Up Your First Stack

1. Click **+ Create Stack** in the Stacks view
2. Enter a name (e.g., "media-server", "home-assistant")
3. Write or paste your Docker Compose YAML in the editor
4. Optionally specify which network mounts this stack requires
5. Click **Save**
6. Click **Start** to launch your containers

WraithOS uses Docker Compose, so if you've used Compose before, you're already familiar with the syntax. If you haven't, here's a minimal example:

```yaml
version: '3'
services:
  web:
    image: nginx:latest
    ports:
      - "80:80"
      - "443:443"
```

The Compose Editor includes syntax highlighting and live validation.

### Docker Run to Compose Converter

Have a Docker run command you want to convert to Compose? Use the built-in converter:
1. Go to **Tools** → **Docker Run Converter**
2. Paste your `docker run ...` command
3. The converter transforms it to Compose YAML (client-side, no external service)

### Live Container Logs

Each stack has real-time logs for its containers:
1. Open a stack
2. Click **Logs** at the top
3. Select which container to view (or "All" for combined output)
4. Logs stream live via WebSocket and update in real-time

### Environment Variables (.env)

Configure per-stack environment variables:
1. Open a stack
2. Click **.env**
3. Edit variables and click **Save**
4. Restart the stack to apply changes

### Updating Your Stack

1. Go to the stack you want to update
2. Edit the Compose YAML
3. Click **Save**
4. Click **Restart** to restart containers, or **Update** to pull new images
5. Watch the progress in the logs view

### Mounting Network Shares

If you have a Samba/SMB or NFS server on your network (like a NAS), you can mount it:

1. Go to **Network Mounts**
2. Click **Add Mount**
3. Choose the protocol (CIFS/Samba or NFS) and enter:
   - Server address (e.g., `192.168.1.10`)
   - Share name (e.g., `media`, `backups`)
   - Username and password (if CIFS; NFS typically doesn't require auth)
4. Optionally mark as "required for Docker" so the watchdog auto-remounts if it fails
5. Optionally associate with specific stacks so only those stacks mount it
6. Click **Mount**

The share will be available inside your containers as a volume at `/remotemounts/<name>`. Reference it in your Compose file like any other Docker volume.

You can also browse, upload, download, and manage files in mounted shares using the **File Manager** at **Tools** → **File Manager** (scoped to `/dockerapps` for app data and `/remotemounts` for network shares).

### Network Configuration

By default, WraithOS gets an IP address from your router (DHCP).

To configure your network:

1. Go to **Settings** → **Network**
2. View current IP and interface info
3. For static IP:
   - Click **Switch to Static**
   - Enter your IP address, subnet mask, gateway, and DNS servers
   - Click **Apply**
4. For VLAN configuration:
   - Click **VLAN Setup**
   - Enter VLAN ID and parent interface
   - Configure DHCP or static IP for the VLAN
5. For headless setup (no web UI available), SSH into the device and run:
   ```
   wraith-setup
   ```
   This console wizard guides network config, hostname, and timezone without the web UI

Settings survive reboots, so you only need to configure once.

### SSH Access

SSH is OFF by default. To enable:

1. Go to **Settings** → **SSH**
2. Click **Enable SSH**
3. SSH will be available on port 22 with root password: `wraithos`

When enabled, SSH automatically shuts down after 30 minutes of inactivity for security. You can toggle it back off at any time.

Default credentials:
- **Root password**: `wraithos`
- **Web UI port**: 82
- **SSH port**: 22 (if enabled)

### System Administration

#### Backup Your Configuration

Your settings are stored on a dedicated disk, but it's good practice to back them up:

1. Go to **Settings** → **Backup & Restore**
2. Click **Export Configuration**
3. Download the ZIP file containing all your settings, credentials, Compose files, and stack configs

To restore later, just upload the file in the same location.

#### Disk Management

WraithOS supports single-disk and dual-disk modes:
- **Single-disk**: Boot device stores OS, config, and Docker data
- **Dual-disk**: Separate disks for config and Docker cache (recommended for production)

To expand a disk:
1. Go to **Settings** → **Disk Management**
2. Select the disk to expand
3. Click **Expand** (the filesystem will grow to use the new space)

#### Docker Network Management

Advanced Docker networking:
1. Go to **Settings** → **Docker Networks**
2. View, create, and delete custom Docker networks
3. Supported drivers: bridge, overlay, macvlan, ipvlan, host, none

#### Docker System Prune

Free up disk space by removing unused Docker objects:
1. Go to **Settings** → **System Prune**
2. Click **Prune** to remove stopped containers, dangling images, unused networks, and build cache
3. View how much space was reclaimed

#### Reboot or Wipe Disks

In **Settings**, you can:
- **Reboot** the system
- **Erase Configuration Disk** (delete all settings, useful for starting fresh)
- **Erase Docker Disk** (delete all images and container data)

Use these carefully — erasure is permanent.

#### Timezone

Set your timezone for correct log timestamps and scheduled tasks:
1. Go to **Settings** → **Timezone**
2. Select your timezone from the list
3. Changes take effect immediately

## Important Concepts

### The OS Disappears on Reboot

Here's how WraithOS works: Every time you boot it, the entire operating system loads fresh from the ISO into RAM. Any OS-level changes (new packages, modified system files, etc.) vanish on reboot. This is by design — it keeps WraithOS simple, fast, and immune to configuration drift.

**What survives reboots:**
- Your configuration disk (settings, network config, credentials)
- Your Docker Compose file
- Your Docker images and container data (on the cache disk)
- Your mounted network shares

**What does NOT survive reboots:**
- OS-level changes (you can't SSH and install packages)
- Ephemeral container data (if your container doesn't write to a mounted volume, it's gone)

If you need to customize the OS itself, WraithOS isn't the right tool. But if you want a simple, reproducible Docker host, this is a feature, not a bug.

### Multiple Stacks Per Instance

WraithOS supports multiple independent Docker Compose stacks running simultaneously. Each stack has its own configuration, volumes, and environment variables. This allows you to:

- Run multiple services on a single WraithOS instance
- Manage each stack independently (start, stop, restart, delete)
- Specify per-stack mount requirements (only mount what you need)
- View logs per-stack
- Use separate .env files for each stack

For simpler deployments, you can still add all services to a single Compose file.

### Security

WraithOS has a firewall that blocks all inbound traffic by default, except:
- Port 82 (the web UI)
- Ports your containers expose (you define these in your Compose file)

Your admin password for the web UI is hashed with bcrypt — we never store it in plain text. If someone gets your configuration disk, they can't read your password.

## Troubleshooting

### I Can't Access the Web UI

1. Check that the system is running (look for the console or VM logs)
2. Find the IP address from your router or the system's network setup
3. Try `http://[ip-address]:82` (not HTTPS by default)
4. If it's still not working, open the **Settings** panel and note the system's uptime — if it's low, it's still booting

### My Containers Won't Start

1. Go to the **Dashboard** and check the container status
2. Look at the logs in the **Compose Editor** (there's a terminal window below the editor)
3. Common issues:
   - **Image not found**: Make sure you spelled the image name correctly and it's available on Docker Hub or your registry
   - **Port already in use**: Another container is using the same port; check your Compose file
   - **Out of disk space**: Check the dashboard disk usage; your cache disk might be full

### My Network Share Won't Mount

1. Double-check the server address, share name, and credentials
2. Make sure the server is reachable on your network
3. Try mounting it from another device first to verify the server is working
4. Check the system logs in **Settings** → **View Logs**

### Rebooting Lost My Data

This is by design for the OS, but not for your data:
- OS-level changes are intentionally ephemeral (they come back on reboot)
- Configuration and Docker data persist

If you lost something, check:
1. **Dashboard** → **Disk Usage**: Is your cache disk still there?
2. **Network Mounts**: Are your mounted shares still connected?
3. **Compose Editor**: Is your stack file still there?

If your cache disk was wiped, your Docker images are gone, but you can re-pull them from Docker Hub or your registry. It'll take a few minutes.

## Updating WraithOS

Updates to WraithOS are simple:

1. Download a new ISO from the releases page
2. Replace the boot device or VM ISO with the new one
3. Reboot

Your configuration and Docker data automatically survive the update because they're on a separate disk.

To roll back, just boot from the old ISO again.

## Architecture

WraithOS consists of:

- **Single statically-linked Go binary** (`wraith-ui`) that embeds all web assets
- **Alpine Linux kernel** (minimal, ~512MB in RAM)
- **Squashfs + tmpfs overlay** for immutable root filesystem and fast in-memory operation
- **Labeled ext4 disk** for persistent configuration and Docker data
- **nftables firewall** auto-configured for security (ports 82 and exposed container ports only)
- **OpenRC init system** for service management

The entire OS is stateless and ephemeral; only data on the config disk survives reboots.

## Web UI Reference

| Section | Features |
|---------|----------|
| **Dashboard** | System metrics (CPU, memory, disk), Docker image list, stack overview, uptime |
| **Stacks** | Create, manage, and delete Docker Compose stacks; view per-stack logs and .env files |
| **File Manager** | Browse, upload, download, and manage files in `/dockerapps` and `/remotemounts` |
| **Network Mounts** | Add/manage Samba (CIFS) and NFS shares; mark as "docker-required" for auto-remount |
| **Network** | Configure DHCP/static IP, VLAN setup, view network interfaces |
| **Tools** | Docker Run to Compose converter, container logs, system metrics |
| **Settings** | Backup/restore, disk expansion, network config, SSH toggle, timezone, password change, system prune, reboot |

## Features at a Glance

- **Multi-stack Docker Compose management** with per-stack .env files and mount requirements
- **Live WebSocket logs** for real-time container output
- **Docker Run to Compose converter** (client-side, no external API)
- **File Manager** scoped to `/dockerapps` and `/remotemounts` with upload/download
- **Samba/CIFS and NFS mounts** with auto-remount watchdog for failed mounts
- **DHCP and static IP** with optional VLAN support
- **Optional SSH** with auto-shutdown after 30min idle
- **Docker network management** (bridge, overlay, macvlan, ipvlan)
- **Docker system prune** to free up disk space
- **Disk expansion** for growing filesystems
- **Console setup wizard** (`wraith-setup`) for headless deployments
- **Timezone configuration** for correct logging and scheduling
- **Backup/restore** of all configuration as ZIP file
- **nftables firewall** auto-configured to block all inbound except port 82 and exposed ports
- **Single 20MB binary** with all assets embedded
- **Boots in under 60 seconds** from ISO
- **Immutable OS** — configuration persists, but OS changes don't

## Limitations

- **No package manager**: You can't install additional OS packages. Use container volumes for data; containers for software
- **Headless only**: No GUI desktop; web UI only
- **UEFI only**: Legacy BIOS systems are not supported
- **Single binary architecture**: All UI and API logic in one process (simplifies deployment, limits extensibility)

## Console Setup Wizard

For headless deployments (no web UI available), use the console setup wizard to configure network, hostname, and timezone without booting into the OS:

```bash
wraith-setup
```

This command is available at the console (TTY) or via SSH and guides you through:
- Network configuration (DHCP or static IP, optional VLAN)
- Hostname
- Timezone
- Initial admin password

Run this after first boot or whenever you need to reconfigure without the web UI.

## Getting Help

Check the [Issues](https://github.com/T3CCH/wraithos/issues) on GitHub if you encounter problems. Before opening an issue:

1. Try restarting WraithOS (**Settings** → **Reboot**)
2. Check the system logs (**Settings** → **View Logs**)
3. Verify your network connection and disk space
4. For headless setup, use the console wizard: `wraith-setup`
5. Include your Compose file (without secrets) and any error messages in your issue report

## Building from Source

Want to build WraithOS yourself? Here's how.

### Prerequisites

- **Go 1.24+** (optional; use containerized build if not available)
- **Docker** (required for the ISO builder)
- **QEMU** (optional, for local testing)

### Containerized Build (no local Go required)

The simplest way to build without installing Go locally:

```bash
# Build the Go binary in a container
docker run --rm -v $(pwd):/build -w /build golang:1.24-alpine sh -c \
  "GOTOOLCHAIN=auto CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags='-s -w' -o build/wraith-ui ./cmd/wraith-ui"

# Build the ISO using the builder container
docker build -f build/Dockerfile.iso-builder -t wraithos-builder .
docker run --rm --privileged -v $(pwd)/build:/out wraithos-builder
```

The finished ISO appears in `build/`.

### Local Build (with Go installed)

```bash
make build-iso
```

This will:
1. Compile the `wraith-ui` Go binary (statically linked, no CGO)
2. Run the Alpine-based ISO builder to produce a bootable image

The finished ISO lands in `build/`.

### Test in QEMU

```bash
make test-qemu
```

### Other Targets

| Command | Description |
|---------|-------------|
| `make build-ui` | Build just the Go binary |
| `make lint` | Lint shell scripts with shellcheck |
| `make upload XCP_HOST=<host>` | Upload ISO to an XCP-ng hypervisor |
| `make clean` | Remove build artifacts |

### Build Output

- **`wraith-ui`** — The single binary (statically linked, ~20MB)
- **`wraithos.iso`** — Bootable ISO with squashfs + tmpfs overlay

## License

WraithOS is open source under the MIT License. See LICENSE file for details.

---

**Ready to get started?** Download the ISO, boot it up, and navigate to port 82. Your first container is just a few clicks away.
