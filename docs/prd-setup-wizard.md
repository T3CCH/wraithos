# WraithOS First-Run Setup Wizard - Product Requirements Document

**Document Status:** Draft
**Version:** 0.2
**Date:** 2026-03-13
**Author:** T3CCH
**Parent PRD:** [WraithOS PRD v1.0](./PRD.md)
**Affected Components:** wraith-ui (Go backend + JS frontend), wraith-disks (OpenRC service)

---

## Table of Contents

1. [Problem Statement](#1-problem-statement)
2. [Goals and Success Criteria](#2-goals-and-success-criteria)
3. [User Stories](#3-user-stories)
4. [Wizard Flow Design](#4-wizard-flow-design)
5. [Functional Requirements](#5-functional-requirements)
6. [API Design](#6-api-design)
7. [Technical Constraints](#7-technical-constraints)
8. [Security Considerations](#8-security-considerations)
9. [Non-Requirements (Out of Scope)](#9-non-requirements-out-of-scope)
10. [Resolved Questions](#10-resolved-questions)
11. [Compose Editor Enhancements](#11-compose-editor-enhancements)
12. [Implementation Notes](#12-implementation-notes)

---

## 1. Problem Statement

### Current State

When a new WraithOS VM boots for the first time, disks must be manually formatted and labeled from the console using commands like `mkfs.ext4 -L WRAITH-CONFIG /dev/xvda`. This requires:

- SSH or console access to the VM
- Knowledge of Linux disk commands (`lsblk`, `blkid`, `mkfs.ext4`)
- Knowledge of which labels to use (`WRAITH-CONFIG`, `WRAITH-CACHE`)
- Understanding of the two-disk architecture

Without formatted disks, the `wraith-disks` OpenRC service falls back to tmpfs mounts (see `/etc/init.d/wraith-disks`, lines 77-81 and 93-97). The system works, but **nothing persists across reboots** -- no compose files, no credentials, no Docker images.

### The Gap

The existing web UI already handles first-run **password creation** (the login page detects `needsSetup` via `GET /api/auth/status` and switches to a "Set Up WraithOS" form). But disk setup has no web UI equivalent. A user who boots WraithOS and sets their password through the browser has no way to know that their configuration is living on volatile tmpfs and will vanish on reboot.

### Impact

- Users lose all configuration on first reboot if they did not manually format disks
- The "zero SSH required" philosophy (PRD goal G4) is broken for initial setup
- First-run experience is confusing: the dashboard shows disk stats but does not indicate whether disks are persistent or tmpfs

---

## 2. Goals and Success Criteria

### Goals

| ID | Goal | Measurable Target |
|----|------|-------------------|
| SW-G1 | Eliminate console requirement for disk setup | User can go from fresh boot to fully persistent system using only the web UI |
| SW-G2 | Prevent silent data loss | System clearly communicates when running on tmpfs (non-persistent) |
| SW-G3 | Guide new users through initial configuration | Step-by-step wizard covers disks, network, and timezone |
| SW-G4 | Allow re-running disk setup | Disk management available from Settings page after initial wizard |
| SW-G5 | Handle edge cases gracefully | System behaves predictably with 0, 1, or 2 disks attached |

### Success Criteria

| # | Criterion | Verification |
|---|-----------|-------------|
| SC-1 | Fresh boot with unformatted disks shows setup wizard after password creation | Boot test with raw disks attached |
| SC-2 | Wizard correctly detects unformatted block devices | Verify with both raw and pre-formatted disks |
| SC-3 | User can assign and format disks as config/cache through the wizard | Format disks, reboot, verify persistence |
| SC-4 | Dashboard indicates tmpfs fallback status with a warning banner | Boot without disks, check dashboard |
| SC-5 | Disk management is accessible from Settings after initial setup | Navigate to Settings, manage disks |
| SC-6 | System works with only a config disk (no cache disk) | Boot with one disk, verify partial persistence |
| SC-7 | Formatting requires explicit confirmation (destructive action) | UI test: confirm dialog before format |

---

## 3. User Stories

### US-1: First-time setup with two disks

> As a homelab operator who just booted a fresh WraithOS VM with two blank virtual disks, I want the web UI to guide me through formatting and assigning those disks so that my configuration and Docker data persist across reboots without needing SSH access.

### US-2: Running on tmpfs without knowing

> As a user who set up my password and deployed a compose stack, I want the dashboard to clearly warn me that I am running on tmpfs and will lose everything on reboot, so that I can set up persistent disks before it is too late.

### US-3: Single disk available

> As a user with only one virtual disk attached, I want the wizard to let me assign it as the config disk (higher priority for persistence) and understand that Docker data will still be volatile.

### US-4: Re-running disk setup

> As a user who added a cache disk to my VM after initial setup, I want to format and configure it through the Settings page without having to start over.

### US-5: Already-formatted disks

> As a user re-provisioning a VM with disks that already have WRAITH-CONFIG and WRAITH-CACHE labels, I want the wizard to detect them and skip formatting, just confirming the mount points.

---

## 4. Wizard Flow Design

### Trigger Conditions

The setup wizard modal appears when **all** of these are true:

1. User has completed password setup (authenticated)
2. At least one of the wraith disks is mounted on tmpfs (detected by checking if `/wraith/config` or `/wraith/cache` is a tmpfs mount)

If both disks are already on real block devices (ext4), the wizard does not appear.

### Wizard Steps

```
Step 1: Welcome / Status Overview
    |
    v
Step 2: Disk Detection & Assignment
    |
    v
Step 3: Confirm & Format
    |
    v
Step 4: Network Configuration (optional, skippable)
    |
    v
Step 5: Timezone Selection (optional, skippable)
    |
    v
Step 6: Summary & Reboot Prompt
```

### Step Details

#### Step 1: Welcome / Status Overview

Display the current system state:

- Config disk: "Persistent (WRAITH-CONFIG on /dev/xvda)" or "Temporary (tmpfs -- will not survive reboot)"
- Cache disk: "Persistent (WRAITH-CACHE on /dev/xvdb)" or "Temporary (tmpfs -- will not survive reboot)"
- If both disks are already persistent, show a "Setup Complete" message and close the wizard

Purpose: Inform the user of what is and is not persistent.

#### Step 2: Disk Detection & Assignment

- Backend scans for available block devices using `lsblk` (excluding boot media, loop devices, mounted disks)
- Display a list of detected disks with: device path, size, current label (if any), current filesystem (if any)
- User assigns each disk a role: **Config Disk**, **Cache Disk**, or **Skip**
- If no unformatted disks are detected, display instructions for attaching disks via XCP-ng/hypervisor and a "Rescan" button

**Automatic detection of existing disks (no reformat needed):**

The wizard must detect and auto-recognize disks that are already prepared for WraithOS. This is critical for re-provisioning VMs, recovering from reboots, and migrating disks between hosts. Detection happens in priority order:

1. **WRAITH-labeled disks:** If a disk already has the label `WRAITH-CONFIG` or `WRAITH-CACHE` (detected via `blkid`), pre-select its role and display it with a green checkmark: "Already formatted -- will mount without reformatting." These disks skip the format step entirely.
2. **Existing ext4 filesystems:** If a disk has an ext4 filesystem but no WRAITH label, display it with a yellow warning icon: "This disk has an existing ext4 filesystem. It may contain data from another use." The user must explicitly choose to format it (see OQ-5 resolution below). Do not offer re-labeling without format.
3. **Other filesystems (ntfs, xfs, btrfs, etc.):** Display with a warning: "This disk has a [type] filesystem and must be formatted as ext4 before use."
4. **Raw/unformatted disks:** Display as available for formatting with no warnings.

**Config disk sizing note:** The config disk stores only text configuration files (wraithos.conf, compose files, credentials, network settings) and logs to forward. A 100MB disk is sufficient. The wizard should accept any disk size but display a hint: "Config disk only needs ~100MB for configuration files."

**Validation rules:**
- A disk can only be assigned one role
- Config and cache must always be separate physical disks (no partitioning)
- At least one disk should be assigned (warn if skipping all, but allow it)

#### Step 3: Confirm & Format

- Show a summary of what will happen: "Format /dev/xvda as ext4 with label WRAITH-CONFIG"
- For already-labeled disks: "Mount existing WRAITH-CONFIG disk at /wraith/config (no formatting needed)"
- **For disks with existing data (any filesystem detected):** Show an elevated warning: "WARNING: This disk contains an existing [fstype] filesystem and may have data on it. Formatting will permanently erase all data. This cannot be undone." Require the user to type "FORMAT" to proceed (not just a checkbox)
- **For raw/unformatted disks:** Standard destructive action warning: "WARNING: Formatting will erase all data on this disk. This cannot be undone." Require confirmation checkbox before proceeding
- Show progress: formatting, labeling, mounting, initializing directory structure
- **Docker stop for cache disk:** If the cache disk is being formatted, Docker will be stopped first. Show a warning: "Docker will be temporarily stopped while the cache disk is formatted. Running containers will be interrupted." After format+mount, Docker is restarted automatically
- After successful format+mount, attempt hot-remount and restart of affected services. Display the mount result to the user

#### Step 4: Network Configuration (Optional)

- Pre-filled with current network settings (from `GET /api/network`)
- DHCP toggle (already implemented in the network page)
- Static IP, gateway, DNS fields
- "Skip" button to keep current settings

This step is included because network configuration is a common first-run task and the user is already in a setup flow. It reuses the existing network API endpoints.

#### Step 5: Timezone Selection (Optional)

- Dropdown of common timezones (or a searchable list)
- Default to UTC
- "Skip" button to keep UTC

#### Step 6: Summary & Reboot Prompt

- Show what was configured: disks, network, timezone
- If disks were formatted: **"A reboot is recommended so all services start with persistent storage, but is not required if hot-remount succeeded."** Offer both a "Reboot Now" button and a "Continue Without Reboot" button. The setup endpoint attempts hot-remount first; if it succeeds, indicate that reboot is optional
- If only network/timezone changed: "Settings applied. No reboot required."
- "Close" button to dismiss the wizard and go to the dashboard

---

## 5. Functional Requirements

### 5.1 Disk Detection API

**Priority: P0 (Must Have)**

| ID | Requirement | Details |
|----|-------------|---------|
| DISK-1 | List block devices | Enumerate available block devices via `lsblk --json --bytes --output NAME,SIZE,TYPE,FSTYPE,LABEL,MOUNTPOINT` |
| DISK-2 | Filter boot media | Exclude devices that are: loop devices, the boot ISO/CD-ROM (`/dev/sr0`), already mounted at `/` or system mount points |
| DISK-3 | Detect existing labels | Identify disks already labeled `WRAITH-CONFIG` or `WRAITH-CACHE` and auto-recognize them without reformatting |
| DISK-3a | Detect existing filesystems | Identify disks with existing ext4 (or other) filesystems via `blkid`. Report filesystem type, label, and UUID. Disks with existing data require explicit format confirmation |
| DISK-3b | Auto-recognize WRAITH disks | Disks with WRAITH-CONFIG or WRAITH-CACHE labels are pre-selected for their role and skip the format step. No user action needed beyond confirming the mount |
| DISK-4 | Report mount status | For each wraith mount point (`/wraith/config`, `/wraith/cache`), report whether it is backed by a real block device or tmpfs |
| DISK-5 | Rescan capability | Allow re-scanning block devices (for hot-added disks in XCP-ng) |

### 5.2 Disk Formatting API

**Priority: P0 (Must Have)**

| ID | Requirement | Details |
|----|-------------|---------|
| FMT-1 | Format with ext4 | Execute `mkfs.ext4 -L <LABEL> <device>` where label is `WRAITH-CONFIG` or `WRAITH-CACHE` |
| FMT-2 | Mount after format | After formatting, mount the disk at the appropriate mount point (`/wraith/config` or `/wraith/cache`) |
| FMT-3 | Initialize layout | After mounting, run the directory initialization (equivalent to `init_config_layout` or `init_cache_layout` from the wraith-disks service) |
| FMT-4 | Unmount tmpfs first | If the mount point currently has a tmpfs mount, unmount it before mounting the real disk. Migrate any existing tmpfs content (auth.json, network.json, compose files) to the new disk |
| FMT-5 | Refuse to format mounted disks | Do not format a disk that is currently mounted anywhere |
| FMT-6 | Refuse to format boot media | Never format a device that contains the boot squashfs |
| FMT-7 | Report progress | Stream or poll format progress (config disk is ~100MB and formats instantly; cache disk may be larger and take several seconds) |

### 5.3 Disk Status on Dashboard

**Priority: P0 (Must Have)**

| ID | Requirement | Details |
|----|-------------|---------|
| DASH-1 | Tmpfs warning banner | When config or cache disk is on tmpfs, show a persistent warning banner at the top of the dashboard: "Configuration is running on temporary storage. Data will not survive reboot. [Set up disks]" |
| DASH-2 | Disk type indicator | In the disk stats cards, indicate whether each disk is "Persistent (ext4)" or "Temporary (tmpfs)" |
| DASH-3 | Setup wizard link | The warning banner should link to the setup wizard (or disk management in Settings) |

### 5.4 Setup Wizard UI

**Priority: P0 (Must Have)**

| ID | Requirement | Details |
|----|-------------|---------|
| WIZ-1 | Modal overlay | Wizard appears as a full-screen modal overlay on top of the main dashboard |
| WIZ-2 | Step navigation | Forward/back navigation with step indicators (dots or numbered steps) |
| WIZ-3 | Auto-trigger on every login while on tmpfs | Wizard auto-opens on every login while any disk is on tmpfs. A "Don't show again" option persists as a browser cookie (cannot persist server-side on tmpfs). Cookie name: `wraith-wizard-dismissed` |
| WIZ-4 | Manual trigger | "Set Up Disks" button in Settings page and in the dashboard warning banner |
| WIZ-5 | Dismissible | User can close the wizard at any step (with a warning if disks are not configured) |
| WIZ-6 | No page reload | Wizard operates as a client-side modal, no full page reloads between steps |
| WIZ-7 | Mobile responsive | Wizard must be usable on mobile browsers (responsive layout) |

### 5.5 Settings Page Disk Management

**Priority: P1 (Should Have)**

| ID | Requirement | Details |
|----|-------------|---------|
| SET-1 | Disk status section | Show current disk assignments, mount status, and usage in Settings |
| SET-2 | Format new disk | Allow formatting a newly-attached disk from Settings (same flow as wizard Step 2-3) |
| SET-3 | Rescan disks | Button to rescan for newly-attached block devices |

### 5.6 Timezone Configuration

**Priority: P2 (Nice to Have)**

| ID | Requirement | Details |
|----|-------------|---------|
| TZ-1 | Set timezone | Configure system timezone via `ln -sf /usr/share/zoneinfo/<TZ> /etc/localtime` |
| TZ-2 | Persist timezone | Store timezone setting on config disk, apply on boot |
| TZ-3 | Timezone list | Provide a list of available timezones from `/usr/share/zoneinfo` |

---

## 6. API Design

### New Endpoints

All new endpoints require authentication (`requireAuth` middleware).

#### `GET /api/setup/status`

Returns the current setup state. Used by the frontend to determine whether to show the wizard.

```json
{
  "configDisk": {
    "mounted": true,
    "persistent": false,
    "type": "tmpfs",
    "device": "",
    "label": "",
    "sizeBytes": 67108864,
    "usedBytes": 4096
  },
  "cacheDisk": {
    "mounted": true,
    "persistent": false,
    "type": "tmpfs",
    "device": "",
    "label": "",
    "sizeBytes": 1073741824,
    "usedBytes": 0
  },
  "needsDiskSetup": true,
  "availableDisks": [
    {
      "device": "/dev/xvda",
      "sizeBytes": 104857600,
      "fstype": "",
      "label": "",
      "inUse": false,
      "hasData": false
    },
    {
      "device": "/dev/xvdb",
      "sizeBytes": 21474836480,
      "fstype": "",
      "label": "",
      "inUse": false,
      "hasData": false
    }
  ]
}
```

#### `POST /api/setup/disks`

Format and mount disks. This is the destructive action endpoint.

**Request:**
```json
{
  "configDisk": "/dev/xvda",
  "cacheDisk": "/dev/xvdb",
  "confirmFormat": true
}
```

Either field can be `""` (empty string) to skip that disk. If a disk already has the correct label, it will be mounted without reformatting.

**Response (success):**
```json
{
  "status": "complete",
  "configDisk": {
    "device": "/dev/xvda",
    "action": "formatted",
    "mounted": true
  },
  "cacheDisk": {
    "device": "/dev/xvdb",
    "action": "formatted",
    "mounted": true
  },
  "rebootRecommended": true,
  "hotRemountSuccess": true,
  "migratedFiles": ["auth.json", "network.json"]
}
```

**Response (error):**
```json
{
  "error": "device /dev/xvda is currently mounted at /mnt/something"
}
```

#### `POST /api/setup/rescan`

Re-scan for block devices. Returns the same `availableDisks` array as `GET /api/setup/status`.

```json
{
  "availableDisks": [...]
}
```

#### `GET /api/system/timezone`

Returns the current timezone.

```json
{
  "timezone": "UTC",
  "available": ["US/Eastern", "US/Central", "US/Mountain", "US/Pacific", "Europe/London", "..."]
}
```

#### `PUT /api/system/timezone`

Set the timezone.

```json
{
  "timezone": "US/Eastern"
}
```

#### `POST /api/system/reboot`

Trigger a system reboot (with a short delay to allow the HTTP response to be sent).

```json
{
  "status": "rebooting",
  "delay": 3
}
```

### Modified Endpoints

#### `GET /api/auth/status` (Existing)

Add a `needsDiskSetup` field to the existing response so the login page can inform the user early:

```json
{
  "needsSetup": false,
  "loggedIn": true,
  "version": "0.1.0",
  "needsDiskSetup": true
}
```

#### `GET /api/system/status` (Existing Dashboard)

Add disk persistence type to the system stats:

```json
{
  "system": {
    "configDiskType": "tmpfs",
    "cacheDiskType": "tmpfs",
    "...existing fields..."
  }
}
```

---

## 7. Technical Constraints

### Operating Environment

- The web UI runs as root (necessary for `mkfs.ext4`, `mount`, etc.)
- `e2fsprogs` is already included in the ISO (provides `mkfs.ext4`)
- `blkid`, `lsblk`, and `util-linux` are already included in the ISO
- Disk operations must not interfere with running containers. **Docker must be stopped before formatting the cache disk** (Docker stores images/containers on the cache mount). The format sequence for cache disk is: stop Docker, format, mount, restart Docker. Formatting the config disk does not require stopping Docker

### XCP-ng Disk Naming

- On XCP-ng (Xen), virtual disks appear as `/dev/xvda`, `/dev/xvdb`, etc.
- On QEMU/KVM, they appear as `/dev/vda`, `/dev/vdb` or `/dev/sda`, `/dev/sdb`
- The wizard must not hardcode device names; always enumerate dynamically

### tmpfs Migration

When replacing a tmpfs mount with a real disk, any files already written to the tmpfs (e.g., `auth.json` from the password setup step) must be copied to the new disk before the tmpfs is unmounted. The migration sequence is:

1. Format the new disk
2. Mount the new disk at a temporary mount point
3. Copy contents from current tmpfs mount to the new disk
4. Unmount the tmpfs
5. Mount the new disk at the final mount point
6. Initialize directory layout if needed

### Concurrency

- Only one disk setup operation should run at a time (use a mutex in the Go backend)
- The frontend should disable the "Format" button while an operation is in progress

---

## 8. Security Considerations

### Destructive Operations

| Risk | Mitigation |
|------|-----------|
| Accidental formatting of wrong disk | Filter out boot media, mounted system disks, and require explicit user confirmation |
| Formatting a disk with existing data | Show disk size, current label, and filesystem type. Display elevated warning for disks with existing filesystems. Require typing "FORMAT" (not just checkbox) for disks with existing data |
| Unauthorized disk operations | All setup endpoints require authentication (session cookie) |
| Race condition: format while Docker is writing | Check if cache disk is in use by Docker before formatting; stop Docker daemon first if needed |

### Boot Media Protection

The following devices must never be offered for formatting:

- Any device mounted at `/` or containing the boot squashfs
- `/dev/sr0` (CD-ROM / ISO)
- Loop devices (`/dev/loop*`)
- The device containing the boot media (detected by checking what is mounted at the boot mount point)

### Input Validation

- Device paths must match the pattern `/dev/[a-z]+[0-9]*` (e.g., `/dev/xvda`, `/dev/sdb1`)
- Reject device paths that resolve to symlinks pointing outside `/dev/`
- The `confirmFormat` field must be `true` or the request is rejected

---

## 9. Non-Requirements (Out of Scope)

| ID | Non-Requirement | Rationale |
|----|-----------------|-----------|
| NR-1 | Disk partitioning | WraithOS uses whole-disk ext4, not partitions. Config and cache are always separate physical disks. Config is tiny (~100MB) and does not share a disk with cache. No GPT/MBR management, no partitioning |
| NR-2 | RAID configuration | Single-disk-per-role. RAID is a hypervisor concern |
| NR-3 | Filesystem choice | Always ext4. No btrfs/XFS/ZFS options |
| NR-4 | Disk encryption | Out of scope for V1. May revisit in V2 |
| NR-5 | LVM support | Whole-disk only. No logical volumes |
| NR-6 | Resizing disks | Resize at the hypervisor level, then reformat or use `resize2fs` manually |
| NR-7 | Hostname configuration in wizard | Can be added later; hostname is not critical for first run |
| NR-8 | TLS certificate setup in wizard | Already handled separately; too complex for a first-run flow |
| NR-9 | Compose file upload in wizard | The wizard gets storage working; compose editing is the existing dashboard feature |

---

## 10. Resolved Questions

| # | Question | Decision | Notes |
|---|----------|----------|-------|
| OQ-1 | Should the wizard auto-trigger only on the very first login, or every login while disks are on tmpfs? | **RESOLVED: Show on every login while on tmpfs, with "Don't show again" cookie.** The wizard appears on each login when any disk is mounted on tmpfs. A "Don't show again" option sets a browser cookie (`wraith-wizard-dismissed`) to suppress it. Cookie-based because server-side state cannot persist on tmpfs. Cookie resets on browser clear, which is acceptable -- the wizard is non-intrusive and dismissible. | Updated WIZ-3 |
| OQ-2 | Should Docker daemon be stopped before formatting the cache disk? | **RESOLVED: Yes, stop Docker before formatting cache disk.** Stop Docker, format the cache disk, mount it, restart Docker. The wizard UI must warn the user: "Docker will be temporarily stopped. Running containers will be interrupted." This applies only to the cache disk -- formatting the config disk does not require stopping Docker. | Updated Step 3 |
| OQ-3 | Should the reboot after disk setup be mandatory or optional? | **RESOLVED: Reboot is recommended but optional. Try hot-remount first.** The `POST /api/setup/disks` endpoint attempts hot-remount and restarts affected services (wraith-disks, wraith-docker). If hot-remount succeeds, the user can continue without rebooting. The wizard shows both "Reboot Now" and "Continue Without Reboot" options. Reboot is recommended if hot-remount encounters errors. | Updated Step 6, API response includes `hotRemountSuccess` field |
| OQ-4 | Should we support formatting a single disk for both config and cache (partitioning)? | **RESOLVED: No. Config and cache are always separate disks.** Config disk is tiny (~100MB, stores only text configs and log-forwarding settings). There is no scenario where partitioning makes sense. One disk = assign as config. Cache remains on tmpfs until a second disk is attached. No partitioning support, no shared-disk support. | Updated NR-1, Step 2 validation rules |
| OQ-5 | What happens if a disk has data but no WRAITH label? | **RESOLVED: Show warning for disks with existing data, require explicit format confirmation.** Disks with any existing filesystem (ext4, ntfs, xfs, etc.) display an elevated warning showing the filesystem type and a note that data may exist. The user must type "FORMAT" to confirm (stronger confirmation than the checkbox used for raw disks). Do not offer re-labeling without format -- filesystem structure may be incompatible with WraithOS layout expectations. | Updated Step 2, Step 3, Security section |

---

## 11. Compose Editor Enhancements

These enhancements apply to the existing compose editor page (not the setup wizard itself). They improve the user experience after disk setup is complete by helping users correctly configure Docker volume paths and catch YAML errors early.

### 11.1 Volume Path Hint After Disk Setup

**Priority: P0 (Must Have)**

| ID | Requirement | Details |
|----|-------------|---------|
| COMP-1 | Show cache mount path hint | After disks are formatted and mounted, the compose editor page displays an informational banner at the top of the editor: **"Cache disk mounted at `/wraith/cache/mounts` -- use this as your volume path in compose files."** This helps users know exactly where to map their Docker volumes |
| COMP-2 | Conditional display | The hint only appears when the cache disk is mounted on a real block device (not tmpfs). When running on tmpfs, show instead: "Cache disk is on temporary storage. Volumes will not persist across reboots." |
| COMP-3 | Dismissible hint | The hint can be dismissed for the current session (not permanently -- it is useful reference information) |
| COMP-4 | Example snippet | Include a small expandable example showing how to use the path in a compose file: `volumes: - /wraith/cache/mounts/myapp:/data` |

**API change:** The `GET /api/setup/status` response (or a lightweight `GET /api/disks/status` variant) provides the mount path and persistence status so the compose editor page can render the appropriate hint.

### 11.2 Client-Side YAML Syntax Validation

**Priority: P1 (Should Have)**

| ID | Requirement | Details |
|----|-------------|---------|
| YAML-1 | Real-time YAML syntax checking | Add client-side YAML parsing to the compose editor textarea. As the user types (debounced, ~500ms after last keystroke), parse the content and display syntax errors inline |
| YAML-2 | Error display | Show YAML syntax errors below the textarea with line number and error message. Example: "Line 12: bad indentation of a mapping entry" |
| YAML-3 | Visual error indicator | Highlight the error line in the textarea (or show a red marker in the line gutter if using a code editor component) |
| YAML-4 | Lightweight parser | Use a lightweight JavaScript YAML parser. Recommended: [js-yaml](https://github.com/nodeca/js-yaml) (~60KB minified) loaded from a vendored copy in `web/static/js/vendor/`. Do not use a CDN -- WraithOS runs air-gapped |
| YAML-5 | Complement backend validation | Client-side YAML validation catches syntax errors (missing colons, bad indentation, invalid characters). The existing backend validation via `docker compose config` (in `internal/docker/compose.go` `SaveComposeFile`) catches semantic errors (invalid service names, unknown keys, port conflicts). Both validations remain -- client-side is for fast feedback, backend is the authoritative check |
| YAML-6 | Valid YAML indicator | When YAML parses successfully, show a green checkmark or "Valid YAML syntax" indicator. This does not mean the compose file is valid Docker Compose -- only that the YAML syntax is correct |
| YAML-7 | No external dependencies | The YAML parser must be vendored (bundled) in the WraithOS static assets. No CDN links, no npm runtime dependencies. The `js-yaml` library should be downloaded and placed in `web/static/js/vendor/js-yaml.min.js` |

**Implementation approach:**

```javascript
// In wraith.js or a new compose-editor.js file
// Load js-yaml from vendored copy
// On textarea input (debounced 500ms):
try {
    jsyaml.load(textareaValue);
    showValidIndicator();
} catch (e) {
    showError(e.mark.line + 1, e.message);
}
```

The backend `POST /api/compose/validate` endpoint (handled by `handleComposeValidate` in `internal/api/compose.go`) continues to perform the authoritative Docker Compose validation. The client-side check is purely for fast YAML syntax feedback.

---

## 12. Implementation Notes

### Existing Code Integration Points

**Backend (Go):**

- New package: `internal/setup/` containing disk detection and formatting logic
- New API handlers registered in `internal/api/router.go` under the `/api/setup/` prefix
- Disk detection: shell out to `lsblk --json` and `blkid` (same tools used by `wraith-disks` service)
- Formatting: shell out to `mkfs.ext4 -L <label> <device>` (requires root, which wraith-ui already runs as)
- Mount status: check `/proc/mounts` to determine what is backing each mount point
- The `storage.ConfigBase` and `storage.CacheDisk` variables (in `internal/storage/paths.go`) define the mount points to check

**Frontend (JavaScript):**

- New file: `web/static/js/setup-wizard.js` (or inline in `wraith.js`)
- Wizard modal triggered from `wraith.js` after dashboard load if `needsDiskSetup` is true
- Reuse existing UI patterns: form cards, buttons, toast notifications from `wraith.js`
- Reuse existing CSS from `wraith.css` (cards, forms, buttons, modals)
- Dashboard modification: add tmpfs warning banner to `pgDash()` function in `wraith.js`

**Service layer:**

- The `wraith-disks` init script (`/etc/init.d/wraith-disks`) handles boot-time disk detection; the web UI setup wizard handles runtime disk setup. They use the same labels and mount points but operate at different times
- After the wizard formats and mounts disks, a reboot will cause `wraith-disks` to find the labeled disks and mount them normally

### File Listing (Estimated Changes)

```
New files:
  internal/setup/disks.go              -- Disk detection, formatting, mounting logic
  internal/setup/timezone.go           -- Timezone list and set
  internal/api/setup.go                -- HTTP handlers for /api/setup/* endpoints
  internal/api/system_reboot.go        -- Reboot endpoint handler
  web/static/js/setup-wizard.js        -- Wizard UI (or added to wraith.js)
  web/static/js/vendor/js-yaml.min.js  -- Vendored js-yaml library for client-side YAML validation

Modified files:
  internal/api/router.go          -- Register new routes
  internal/api/auth.go            -- Add needsDiskSetup to auth status response
  internal/api/dashboard.go       -- Add disk type (tmpfs vs ext4) to dashboard response
  web/static/js/wraith.js         -- Dashboard tmpfs warning, wizard trigger, compose editor
                                      volume path hint, YAML validation integration
  web/static/css/wraith.css       -- Wizard modal styles, YAML error/valid indicators,
                                      volume path hint banner styles
```

---

*This PRD extends the [WraithOS PRD v1.0](./PRD.md). It addresses requirement BOOT-10 ("Boot to web UI even if config or cache disks are missing -- first-run setup mode") and fills the gap between boot-time disk detection and web-UI-driven disk configuration.*
