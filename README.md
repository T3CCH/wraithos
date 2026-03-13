# WraithOS

**A lightweight, RAM-based operating system for running Docker containers.**

WraithOS is designed to run on dedicated hardware or VMs as a minimal Docker host. Boot it from an ISO, configure it through a web browser, and deploy Docker containers without managing a full operating system.

## What Is WraithOS?

Think of WraithOS as a purpose-built appliance, not a general-purpose operating system. It's built on Alpine Linux and runs entirely from RAM (about 512MB), making it blazing fast. All your configuration and data gets saved to a dedicated disk, so even though the OS vanishes when you reboot, your settings and containers stay intact.

**Key features:**
- Boots in seconds (under 60 seconds from power-on to ready)
- Runs entirely in RAM for speed
- Manage everything through a web browser on port 82 (no SSH required)
- One Docker Compose stack per instance (keep it simple, use multiple instances for multiple apps)
- Automatically mounts network shares (Samba/SMB) for your containers
- Survives reboots with persistent configuration on a dedicated disk

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
- **System Status**: CPU usage, memory, and disk space
- **Uptime**: How long your system has been running since the last reboot
- **Container Status**: Whether your Docker containers are running, stopped, or crashed

### Setting Up Your First Container

1. Click **Compose Editor** in the sidebar
2. Write or paste your Docker Compose file in the YAML editor
3. Click **Save**
4. Click **Start** to launch your containers

WraithOS uses Docker Compose, so if you've used Compose before, you're already familiar with the syntax. If you haven't, here's a minimal example to run something like Nginx:

```yaml
version: '3'
services:
  web:
    image: nginx:latest
    ports:
      - "80:80"
      - "443:443"
```

The Compose Editor includes syntax highlighting and live validation to catch errors before you deploy.

### Updating Your Stack

1. Go to **Compose Editor**
2. Make changes to your YAML
3. Click **Save** and then **Restart** (or **Update** to pull the latest images)
4. Watch the progress in the terminal below the editor

### Mounting Network Shares

If you have a Samba/SMB server on your network (like a NAS), you can mount it:

1. Go to **Network Mounts**
2. Click **Add Mount**
3. Enter:
   - Server address (e.g., `192.168.1.10`)
   - Share name (e.g., `media`, `backups`)
   - Username and password (if required)
4. Click **Mount**

The share will be available inside your containers as a volume. Reference it in your Compose file like any other Docker volume.

### Network Configuration

By default, WraithOS gets an IP address from your router (DHCP).

To set a static IP:

1. Go to **Network**
2. Click **Switch to Static IP**
3. Enter your IP address, subnet mask, gateway, and DNS servers
4. Click **Apply**

Settings survive reboots, so you only need to do this once.

### Backup Your Configuration

Your settings are stored on a dedicated disk, but it's good practice to back them up:

1. Go to **Settings**
2. Click **Export Configuration**
3. Download the ZIP file containing all your settings, credentials, and Compose file

To restore later, just upload the file in the same location.

### Reboot or Wipe Disks

In **Settings**, you can:
- **Reboot** the system
- **Erase Configuration Disk** (delete all settings, useful for starting fresh)
- **Erase Docker Disk** (delete all images and container data)

Use these carefully — erasure is permanent.

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

### One Compose Stack Per Instance

WraithOS is designed to run exactly one Docker Compose stack. This enforces simplicity: one VM, one app.

Want to run multiple services? Either:
- Add them all to the same Compose file, or
- Spin up multiple WraithOS instances (one per service)

This approach scales better than "one giant Linux box with everything."

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

## Command Reference (Web UI)

| Page | What You Can Do |
|------|-----------------|
| **Dashboard** | View system metrics, container status, and uptime |
| **Compose Editor** | Write and deploy your Docker Compose stack |
| **Network Mounts** | Add and manage Samba/SMB shares for your containers |
| **Network** | View your IP address and switch between DHCP and static IP |
| **Settings** | Reboot, backup/restore configuration, wipe disks, view logs, change password |

## Limitations

- **One Compose stack per instance**: No multi-stack orchestration on a single WraithOS instance
- **No SSH by default**: This is intentional. The OS is immutable; SSH access isn't useful
- **No package manager**: You can't install additional OS packages. Use container volumes for data; containers for software
- **Headless only**: No GUI desktop; web UI only
- **UEFI only**: Legacy BIOS systems are not supported

## Getting Help

Check the [Issues](https://github.com/T3CCH/wraithos/issues) on GitHub if you encounter problems. Before opening an issue:

1. Try restarting WraithOS (**Settings** → **Reboot**)
2. Check the system logs (**Settings** → **View Logs**)
3. Verify your network connection and disk space
4. Include your Compose file (without secrets) and any error messages in your issue report

## License

WraithOS is open source under the MIT License. See LICENSE file for details.

---

**Ready to get started?** Download the ISO, boot it up, and navigate to port 82. Your first container is just a few clicks away.
