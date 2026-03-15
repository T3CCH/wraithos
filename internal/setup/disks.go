// Package setup provides disk detection, formatting, and mounting for the
// first-run setup wizard. It shells out to lsblk, blkid, mkfs.ext4, and
// mount -- the same tools used by the wraith-disks OpenRC service.
package setup

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/wraithos/wraith-ui/internal/storage"
)

// Labels used by wraith-disks to identify config and cache disks.
const (
	ConfigLabel = "WRAITH-CONFIG"
	CacheLabel  = "WRAITH-CACHE"
	SingleLabel = "WRAITH-SINGLE"
)

// SingleDiskDir is the mount point for a single-disk setup.
// Config and cache subdirectories live beneath this.
const SingleDiskDir = "/wraith/disk"

// validDevicePath matches /dev/[a-z]+[0-9]* (e.g. /dev/xvda, /dev/sdb1).
var validDevicePath = regexp.MustCompile(`^/dev/[a-z]+[0-9]*$`)

// diskMu prevents concurrent disk operations (format, mount, migrate).
var diskMu sync.Mutex

// setupInProgress is protected by diskMu and prevents concurrent
// disk setup requests at the API level.
var setupInProgress bool

// BlockDevice represents a detected block device from lsblk.
type BlockDevice struct {
	Device    string `json:"device"`
	SizeBytes uint64 `json:"sizeBytes"`
	FSType    string `json:"fstype"`
	Label     string `json:"label"`
	InUse     bool   `json:"inUse"`
	HasData   bool   `json:"hasData"`
}

// MountStatus describes whether a wraith mount point is backed by a
// real block device or tmpfs.
type MountStatus struct {
	Mounted    bool   `json:"mounted"`
	Persistent bool   `json:"persistent"`
	Type       string `json:"type"`
	Device     string `json:"device"`
	Label      string `json:"label"`
	SizeBytes  uint64 `json:"sizeBytes"`
	UsedBytes  uint64 `json:"usedBytes"`
}

// DiskSetupResult is returned after formatting/mounting a single disk.
type DiskSetupResult struct {
	Device  string `json:"device"`
	Action  string `json:"action"` // "formatted", "mounted", "skipped"
	Mounted bool   `json:"mounted"`
}

// lsblkOutput is the JSON structure returned by lsblk --json.
type lsblkOutput struct {
	BlockDevices []lsblkDevice `json:"blockdevices"`
}

type lsblkDevice struct {
	Name       string         `json:"name"`
	Size       uint64         `json:"size"`
	Type       string         `json:"type"`
	FSType     *string        `json:"fstype"`
	Label      *string        `json:"label"`
	MountPoint *string        `json:"mountpoint"`
	Children   []lsblkDevice  `json:"children,omitempty"`
}

// systemMountPoints are paths that must never have their backing device
// offered for formatting.
var systemMountPoints = map[string]bool{
	"/":     true,
	"/boot": true,
	"/sys":  true,
	"/proc": true,
	"/dev":  true,
	"/run":  true,
	"/tmp":  true,
}

// DetectDisks runs lsblk and returns available block devices, filtering
// out boot media (sr0, loop*), and devices mounted at system paths.
func DetectDisks() ([]BlockDevice, error) {
	cmd := exec.Command("lsblk", "--json", "--bytes",
		"--output", "NAME,SIZE,TYPE,FSTYPE,LABEL,MOUNTPOINT")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("lsblk: %w", err)
	}

	var parsed lsblkOutput
	if err := json.Unmarshal(output, &parsed); err != nil {
		return nil, fmt.Errorf("parse lsblk output: %w", err)
	}

	var result []BlockDevice
	for _, dev := range parsed.BlockDevices {
		if shouldSkipDevice(dev) {
			continue
		}

		bd := BlockDevice{
			Device:    "/dev/" + dev.Name,
			SizeBytes: dev.Size,
		}
		if dev.FSType != nil {
			bd.FSType = *dev.FSType
			bd.HasData = *dev.FSType != ""
		}
		if dev.Label != nil {
			bd.Label = *dev.Label
		}
		if dev.MountPoint != nil && *dev.MountPoint != "" {
			bd.InUse = true
		}

		result = append(result, bd)
	}

	return result, nil
}

// shouldSkipDevice returns true for devices that must not be offered
// for formatting: loop devices, CD-ROMs, and system-mounted devices.
// Also checks child partitions -- if any child is mounted at a system
// path, the parent device is skipped.
func shouldSkipDevice(dev lsblkDevice) bool {
	// Skip loop devices
	if dev.Type == "loop" || strings.HasPrefix(dev.Name, "loop") {
		return true
	}

	// Skip zram devices (RAM-based compressed block devices used for swap)
	if strings.HasPrefix(dev.Name, "zram") {
		return true
	}

	// Skip CD-ROM / ISO
	if dev.Name == "sr0" || dev.Type == "rom" {
		return true
	}

	// Skip devices mounted at system paths
	if dev.MountPoint != nil && systemMountPoints[*dev.MountPoint] {
		return true
	}

	// Skip partition-type entries (we use whole disks)
	if dev.Type == "part" {
		return true
	}

	// Only include "disk" type devices
	if dev.Type != "disk" {
		return true
	}

	// Skip if any child partition is mounted at a system path
	if hasChildAtSystemMount(dev.Children) {
		return true
	}

	return false
}

// hasChildAtSystemMount recursively checks whether any child device
// (partition) is mounted at a system path.
func hasChildAtSystemMount(children []lsblkDevice) bool {
	for _, child := range children {
		if child.MountPoint != nil && systemMountPoints[*child.MountPoint] {
			return true
		}
		if hasChildAtSystemMount(child.Children) {
			return true
		}
	}
	return false
}

// GetDiskStatus checks /proc/mounts to determine whether /wraith/config
// and /wraith/cache are backed by tmpfs or real block devices.
// For the config disk, the RAM-based architecture means ConfigBase is
// always tmpfs, so we also check ConfigDiskDir for the physical disk.
// In single-disk mode, both config and cache are backed by subdirectories
// of the WRAITH-SINGLE mount via bind mounts.
func GetDiskStatus() (config, cache MountStatus) {
	config = getMountStatus(storage.ConfigBase)

	// With the RAM-based config architecture, ConfigBase is always tmpfs
	// but the physical config disk is mounted at ConfigDiskDir.
	// If ConfigDiskDir has a real device mounted, config is persistent.
	if config.Mounted && !config.Persistent && storage.ConfigDiskDir != "" {
		diskStatus := getMountStatus(storage.ConfigDiskDir)
		if diskStatus.Mounted && diskStatus.Persistent {
			config.Persistent = true
			config.Device = diskStatus.Device
			config.Label = diskStatus.Label
			// Keep the type as "tmpfs (ram-backed)" for clarity
			config.Type = "tmpfs+disk"
		}
	}

	cache = getMountStatus(storage.CacheDisk)

	// Check for single-disk mode: if SingleDiskDir is mounted with
	// WRAITH-SINGLE label, both config and cache are persistent via
	// bind mounts from that single disk.
	if IsSingleDiskMode() {
		singleMs := getMountStatus(SingleDiskDir)
		if !config.Persistent {
			config.Persistent = true
			config.Device = singleMs.Device
			config.Label = SingleLabel
			config.Type = "single-disk"
		}
		if !cache.Persistent {
			cache.Persistent = true
			cache.Device = singleMs.Device
			cache.Label = SingleLabel
			cache.Type = "single-disk"
		}
	}

	return config, cache
}

// getMountStatus parses /proc/mounts for the given mount point.
func getMountStatus(mountpoint string) MountStatus {
	ms := MountStatus{}

	f, err := os.Open("/proc/mounts")
	if err != nil {
		return ms
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}
		// fields: device mountpoint fstype options ...
		if fields[1] != mountpoint {
			continue
		}

		ms.Mounted = true
		ms.Device = fields[0]
		ms.Type = fields[2]
		ms.Persistent = fields[2] != "tmpfs"

		// Try to get label via blkid if it's a real device
		if ms.Persistent {
			ms.Label = getDeviceLabel(fields[0])
		}

		// Get size/used from df
		ms.SizeBytes, ms.UsedBytes = getDiskUsage(mountpoint)
		return ms
	}

	return ms
}

// getDeviceLabel runs blkid to get the LABEL for a device.
func getDeviceLabel(device string) string {
	cmd := exec.Command("blkid", "-s", "LABEL", "-o", "value", device)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// getDiskUsage returns total and used bytes for a mountpoint via df.
func getDiskUsage(mountpoint string) (total, used uint64) {
	cmd := exec.Command("df", "--block-size=1", "--output=size,used", mountpoint)
	out, err := cmd.Output()
	if err != nil {
		return 0, 0
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return 0, 0
	}
	fields := strings.Fields(lines[1])
	if len(fields) < 2 {
		return 0, 0
	}
	fmt.Sscanf(fields[0], "%d", &total)
	fmt.Sscanf(fields[1], "%d", &used)
	return total, used
}

// ValidateDevice checks that a device path is safe to operate on.
func ValidateDevice(device string) error {
	if !validDevicePath.MatchString(device) {
		return fmt.Errorf("invalid device path %q: must match /dev/[a-z]+[0-9]*", device)
	}

	// Reject symlinks that point outside /dev
	resolved, err := filepath.EvalSymlinks(device)
	if err != nil {
		return fmt.Errorf("cannot resolve device path %q: %w", device, err)
	}
	if !strings.HasPrefix(resolved, "/dev/") {
		return fmt.Errorf("device path %q resolves outside /dev: %s", device, resolved)
	}

	// Refuse boot media and virtual devices
	base := filepath.Base(device)
	if base == "sr0" || strings.HasPrefix(base, "loop") || strings.HasPrefix(base, "zram") {
		return fmt.Errorf("refusing to operate on virtual device: %s", device)
	}

	return nil
}

// isDeviceMounted checks /proc/mounts for any mount of the given device.
// Returns the mount point if mounted, or empty string.
func isDeviceMounted(device string) string {
	f, err := os.Open("/proc/mounts")
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 && fields[0] == device {
			return fields[1]
		}
	}
	return ""
}

// isMountpointActive checks /proc/mounts for any entry with the given
// mount point. Used to confirm a lazy unmount has actually completed.
func isMountpointActive(mountpoint string) bool {
	f, err := os.Open("/proc/mounts")
	if err != nil {
		return false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 && fields[1] == mountpoint {
			return true
		}
	}
	return false
}

// AcquireSetup attempts to mark a disk setup as in-progress.
// Returns false if a setup is already running.
func AcquireSetup() bool {
	diskMu.Lock()
	defer diskMu.Unlock()
	if setupInProgress {
		return false
	}
	setupInProgress = true
	return true
}

// ReleaseSetup marks the disk setup as no longer in-progress.
func ReleaseSetup() {
	diskMu.Lock()
	defer diskMu.Unlock()
	setupInProgress = false
}

// FormatDisk formats a device with ext4 and the given label.
// Refuses to format boot media or already-mounted devices.
func FormatDisk(device, label string) error {
	diskMu.Lock()
	defer diskMu.Unlock()

	return formatDiskLocked(device, label)
}

// formatDiskLocked does the actual format work. Caller must hold diskMu.
func formatDiskLocked(device, label string) error {
	if err := ValidateDevice(device); err != nil {
		return err
	}

	if mnt := isDeviceMounted(device); mnt != "" {
		return fmt.Errorf("device %s is currently mounted at %s", device, mnt)
	}

	log.Printf("formatting %s as ext4 with label %s", device, label)
	cmd := exec.Command("mkfs.ext4", "-F", "-L", label, device)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mkfs.ext4 failed: %s: %w", strings.TrimSpace(string(output)), err)
	}

	return nil
}

// unmountAndFormat unmounts a device (if mounted) and formats it atomically
// under diskMu so nothing can remount between unmount and format.
func unmountAndFormat(device, label string) error {
	diskMu.Lock()
	defer diskMu.Unlock()

	// Unmount the device if it's currently mounted anywhere
	if mnt := isDeviceMounted(device); mnt != "" {
		log.Printf("unmounting %s from %s before format", device, mnt)
		umountCmd := exec.Command("umount", device)
		if output, err := umountCmd.CombinedOutput(); err != nil {
			log.Printf("normal unmount failed (%s), trying lazy unmount", strings.TrimSpace(string(output)))
			lazyCmd := exec.Command("umount", "-l", device)
			if output, err := lazyCmd.CombinedOutput(); err != nil {
				return fmt.Errorf("unmount %s: %s: %w", device, strings.TrimSpace(string(output)), err)
			}
			// Wait for lazy unmount to complete
			for i := 0; i < 15; i++ {
				if isDeviceMounted(device) == "" {
					break
				}
				time.Sleep(500 * time.Millisecond)
			}
		}
	}

	return formatDiskLocked(device, label)
}

// MountDisk mounts a device at the given mount point, creating it if needed.
func MountDisk(device, mountpoint string) error {
	if err := ValidateDevice(device); err != nil {
		return err
	}

	if err := os.MkdirAll(mountpoint, 0755); err != nil {
		return fmt.Errorf("create mount point %s: %w", mountpoint, err)
	}

	cmd := exec.Command("mount", "-t", "ext4", "-o", "noatime,errors=remount-ro", device, mountpoint)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mount %s at %s: %s: %w", device, mountpoint, strings.TrimSpace(string(output)), err)
	}

	return nil
}

// MigrateTmpfs copies all files from the current tmpfs mount to a
// temporary mount of the new disk, then swaps the mounts.
// Returns a list of migrated file names.
func MigrateTmpfs(device, mountpoint string) ([]string, error) {
	diskMu.Lock()
	defer diskMu.Unlock()

	tempMount := mountpoint + ".new"

	// Mount the new disk at a temporary location
	if err := os.MkdirAll(tempMount, 0755); err != nil {
		return nil, fmt.Errorf("create temp mount: %w", err)
	}

	cmd := exec.Command("mount", "-t", "ext4", "-o", "noatime,errors=remount-ro", device, tempMount)
	if output, err := cmd.CombinedOutput(); err != nil {
		os.Remove(tempMount)
		return nil, fmt.Errorf("mount new disk at temp: %s: %w", strings.TrimSpace(string(output)), err)
	}

	// Copy files from tmpfs to new disk
	var migrated []string
	entries, err := os.ReadDir(mountpoint)
	if err != nil {
		log.Printf("warning: could not read tmpfs at %s: %v", mountpoint, err)
	} else {
		for _, entry := range entries {
			src := filepath.Join(mountpoint, entry.Name())
			// Use cp -a to preserve permissions and handle directories
			cpCmd := exec.Command("cp", "-a", src, tempMount+"/")
			if output, cpErr := cpCmd.CombinedOutput(); cpErr != nil {
				log.Printf("warning: failed to migrate %s: %s", entry.Name(), strings.TrimSpace(string(output)))
			} else {
				migrated = append(migrated, entry.Name())
			}
		}
	}

	// Unmount the tmpfs
	umountCmd := exec.Command("umount", mountpoint)
	if output, err := umountCmd.CombinedOutput(); err != nil {
		// Try lazy unmount as fallback
		log.Printf("normal unmount failed (%s), trying lazy unmount", strings.TrimSpace(string(output)))
		lazyCmd := exec.Command("umount", "-l", mountpoint)
		if output, err := lazyCmd.CombinedOutput(); err != nil {
			// Unmount temp and bail
			exec.Command("umount", tempMount).Run()
			os.Remove(tempMount)
			return nil, fmt.Errorf("unmount tmpfs at %s: %s: %w", mountpoint, strings.TrimSpace(string(output)), err)
		}

		// After lazy unmount, poll /proc/mounts to confirm the
		// mountpoint is actually gone before proceeding.
		unmounted := false
		for i := 0; i < 5; i++ {
			if !isMountpointActive(mountpoint) {
				unmounted = true
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if !unmounted {
			exec.Command("umount", tempMount).Run()
			os.Remove(tempMount)
			return nil, fmt.Errorf("mountpoint %s still active after lazy unmount", mountpoint)
		}
	}

	// Unmount from temp and mount at final location
	umountTempCmd := exec.Command("umount", tempMount)
	if output, err := umountTempCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("unmount temp at %s: %s: %w", tempMount, strings.TrimSpace(string(output)), err)
	}
	os.Remove(tempMount)

	// Mount at the real mount point
	if err := MountDisk(device, mountpoint); err != nil {
		return nil, fmt.Errorf("final mount: %w", err)
	}

	return migrated, nil
}

// InitConfigLayout creates the directory structure expected on the config disk.
// Mirrors init_config_layout() from /etc/init.d/wraith-disks.
func InitConfigLayout(base string) error {
	dirs := []string{
		filepath.Join(base, "compose"),
		filepath.Join(base, "backup"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("create %s: %w", d, err)
		}
	}

	// Create default config files if they don't exist
	defaults := map[string]string{
		filepath.Join(base, "network.json"): `{"mode":"dhcp"}`,
		filepath.Join(base, "samba.json"):   `{"mounts":[]}`,
	}
	for path, content := range defaults {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				return fmt.Errorf("create %s: %w", path, err)
			}
		}
	}

	return nil
}

// InitCacheLayout creates the directory structure expected on the cache disk.
// Mirrors init_cache_layout() from /etc/init.d/wraith-disks.
func InitCacheLayout(base string) error {
	dirs := []string{
		filepath.Join(base, "docker"),
		filepath.Join(base, "mounts"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("create %s: %w", d, err)
		}
	}
	return nil
}

// StopDocker stops the Docker daemon via the OpenRC service.
// Stops both the wraith-docker compose wrapper and the stock docker service.
func StopDocker() error {
	log.Printf("stopping Docker daemon for cache disk format")

	// Stop compose stack first
	cmd := exec.Command("rc-service", "wraith-docker", "stop")
	cmd.CombinedOutput() // ignore errors -- may not be running

	// Stop the stock docker daemon
	cmd = exec.Command("rc-service", "docker", "stop")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Try direct kill as fallback
		log.Printf("rc-service docker stop failed (%s), trying direct stop", strings.TrimSpace(string(output)))
		exec.Command("pkill", "-TERM", "dockerd").Run()
		time.Sleep(3 * time.Second)
	}
	return nil
}

// StartDocker starts the Docker daemon via the OpenRC service.
func StartDocker() error {
	log.Printf("starting Docker daemon after cache disk format")

	// Start the stock docker service
	cmd := exec.Command("rc-service", "docker", "start")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("start docker: %s: %w", strings.TrimSpace(string(output)), err)
	}

	// Start the wraith-docker compose deployer
	cmd = exec.Command("rc-service", "wraith-docker", "start")
	cmd.CombinedOutput() // ignore errors

	return nil
}

// WipeDisk wipes a wraith disk by unmounting it, reformatting it with the
// correct label, and remounting it with a clean directory layout.
// diskType must be "config", "cache", or "single".
func WipeDisk(diskType string) error {
	var label, mountpoint string
	switch diskType {
	case "config":
		label = ConfigLabel
		mountpoint = storage.ConfigBase
	case "cache":
		label = CacheLabel
		mountpoint = storage.CacheDisk
	case "single":
		label = SingleLabel
		mountpoint = SingleDiskDir
	default:
		return fmt.Errorf("invalid disk type %q: must be \"config\", \"cache\", or \"single\"", diskType)
	}

	// Find the device backing this mount point
	ms := getMountStatus(mountpoint)

	// For config disk with RAM-based architecture, check the physical disk
	if diskType == "config" && (!ms.Persistent || ms.Type == "tmpfs") && storage.ConfigDiskDir != "" {
		ms = getMountStatus(storage.ConfigDiskDir)
	}

	if !ms.Persistent || ms.Device == "" {
		return fmt.Errorf("%s disk is not on a persistent device (currently %s)", diskType, ms.Type)
	}

	device := ms.Device
	if err := ValidateDevice(device); err != nil {
		return err
	}

	// Stop Docker before wiping cache disk
	if diskType == "cache" {
		if err := StopDocker(); err != nil {
			log.Printf("warning: failed to stop docker for cache wipe: %v", err)
		}
	}

	// Unmount and reformat
	if err := unmountAndFormat(device, label); err != nil {
		if diskType == "cache" {
			StartDocker()
		}
		return fmt.Errorf("wipe %s disk: %w", diskType, err)
	}

	// For config disk, also unmount the config-disk sync dir
	if diskType == "config" && storage.ConfigDiskDir != "" {
		if isMountpointActive(storage.ConfigDiskDir) {
			cmd := exec.Command("umount", storage.ConfigDiskDir)
			if output, err := cmd.CombinedOutput(); err != nil {
				log.Printf("warning: unmount %s failed: %s", storage.ConfigDiskDir, strings.TrimSpace(string(output)))
			}
		}
	}

	// Remount at the appropriate location
	if diskType == "config" {
		// Mount at config-disk dir for sync-back
		if storage.ConfigDiskDir != "" {
			if err := MountDisk(device, storage.ConfigDiskDir); err != nil {
				return fmt.Errorf("remount config disk at %s: %w", storage.ConfigDiskDir, err)
			}
			if err := InitConfigLayout(storage.ConfigDiskDir); err != nil {
				return fmt.Errorf("init config layout: %w", err)
			}
			// Sync current RAM config to the fresh disk
			cpCmd := exec.Command("cp", "-a", mountpoint+"/.", storage.ConfigDiskDir+"/")
			if output, cpErr := cpCmd.CombinedOutput(); cpErr != nil {
				log.Printf("warning: config sync after wipe failed: %s", strings.TrimSpace(string(output)))
			}
		}
	} else {
		// Cache disk: mount directly
		if err := MountDisk(device, mountpoint); err != nil {
			StartDocker()
			return fmt.Errorf("remount cache disk: %w", err)
		}
		if err := InitCacheLayout(mountpoint); err != nil {
			StartDocker()
			return fmt.Errorf("init cache layout: %w", err)
		}
		if err := StartDocker(); err != nil {
			log.Printf("warning: failed to restart docker after cache wipe: %v", err)
		}
	}

	return nil
}

// NeedsDiskSetup returns true if either wraith mount point is on tmpfs.
func NeedsDiskSetup() bool {
	config, cache := GetDiskStatus()
	return !config.Persistent || !cache.Persistent
}

// SetupDisk handles the full workflow for a single disk: validate, format
// (if needed), migrate tmpfs content, mount, and initialize layout.
// label should be ConfigLabel or CacheLabel.
func SetupDisk(device, label string) (*DiskSetupResult, []string, error) {
	if err := ValidateDevice(device); err != nil {
		return nil, nil, err
	}

	// Check if already has the correct label (no format needed)
	existingLabel := getDeviceLabel(device)

	mountpoint := storage.ConfigBase
	if label == CacheLabel {
		mountpoint = storage.CacheDisk
	}

	result := &DiskSetupResult{
		Device: device,
	}

	var migrated []string

	if existingLabel == label {
		// Already labeled correctly -- just mount
		result.Action = "mounted"

		// Check if mountpoint is currently tmpfs and migrate
		ms := getMountStatus(mountpoint)
		if ms.Mounted && !ms.Persistent {
			var err error
			migrated, err = MigrateTmpfs(device, mountpoint)
			if err != nil {
				return nil, nil, fmt.Errorf("migrate tmpfs for %s: %w", label, err)
			}
		} else if !ms.Mounted {
			// Verify the device is not already mounted elsewhere (e.g.
			// a WRAITH-labeled boot device). Also refuse if any child
			// partition is at a system mount point.
			if mnt := isDeviceMounted(device); mnt != "" {
				return nil, nil, fmt.Errorf("device %s is already mounted at %s", device, mnt)
			}
			if err := MountDisk(device, mountpoint); err != nil {
				return nil, nil, err
			}
		}
	} else {
		// Need to format
		result.Action = "formatted"

		// If formatting cache disk, stop Docker first
		if label == CacheLabel {
			if err := StopDocker(); err != nil {
				log.Printf("warning: failed to stop docker: %v", err)
			}
		}

		// Unmount and format under one lock so nothing can remount between
		if err := unmountAndFormat(device, label); err != nil {
			if label == CacheLabel {
				StartDocker()
			}
			return nil, nil, err
		}

		// Check if mountpoint is currently tmpfs and need migration
		ms := getMountStatus(mountpoint)

		if ms.Mounted && !ms.Persistent {
			// Migrate tmpfs content to the new disk
			var err error
			migrated, err = MigrateTmpfs(device, mountpoint)
			if err != nil {
				if label == CacheLabel {
					StartDocker()
				}
				return nil, nil, fmt.Errorf("migrate tmpfs for %s: %w", label, err)
			}
		} else {
			// Mount directly (already formatted above)
			if err := MountDisk(device, mountpoint); err != nil {
				if label == CacheLabel {
					StartDocker()
				}
				return nil, nil, err
			}
		}

		// Restart Docker if we stopped it
		if label == CacheLabel {
			if err := StartDocker(); err != nil {
				log.Printf("warning: failed to restart docker: %v", err)
			}
		}
	}

	// Initialize directory layout
	if label == ConfigLabel {
		if err := InitConfigLayout(mountpoint); err != nil {
			return nil, nil, fmt.Errorf("init config layout: %w", err)
		}

		// Also mount at ConfigDiskDir for sync-back support.
		// After reboot, the init script will handle this, but we set it up
		// now so that config writes sync to disk immediately.
		if storage.ConfigDiskDir != "" && !isMountpointActive(storage.ConfigDiskDir) {
			if err := MountDisk(device, storage.ConfigDiskDir); err != nil {
				log.Printf("warning: could not mount config disk at %s for sync-back: %v",
					storage.ConfigDiskDir, err)
			} else {
				// Initialize layout on the physical disk too
				InitConfigLayout(storage.ConfigDiskDir)
				// Copy current config to physical disk
				cpCmd := exec.Command("cp", "-a", mountpoint+"/.", storage.ConfigDiskDir+"/")
				if output, cpErr := cpCmd.CombinedOutput(); cpErr != nil {
					log.Printf("warning: initial config sync failed: %s", strings.TrimSpace(string(output)))
				} else {
					log.Printf("config synced to physical disk at %s", storage.ConfigDiskDir)
				}
			}
		}
	} else {
		if err := InitCacheLayout(mountpoint); err != nil {
			return nil, nil, fmt.Errorf("init cache layout: %w", err)
		}
	}

	result.Mounted = true
	return result, migrated, nil
}

// SetupSingleDisk handles the full workflow for a single-disk setup where
// one device serves both config and cache roles using subdirectories.
// The disk is formatted with the WRAITH-SINGLE label and mounted at
// SingleDiskDir with config/ and cache/ subdirectories. Bind mounts map
// these to the standard paths so existing code works transparently.
func SetupSingleDisk(device string) (*DiskSetupResult, []string, error) {
	if err := ValidateDevice(device); err != nil {
		return nil, nil, err
	}

	result := &DiskSetupResult{
		Device: device,
	}

	existingLabel := getDeviceLabel(device)
	var migrated []string

	// Stop Docker before any cache-related operations
	if err := StopDocker(); err != nil {
		log.Printf("warning: failed to stop docker: %v", err)
	}

	if existingLabel == SingleLabel {
		// Already labeled correctly -- just mount
		result.Action = "mounted"

		if mnt := isDeviceMounted(device); mnt != "" {
			// Already mounted somewhere; if not at SingleDiskDir, unmount first
			if mnt != SingleDiskDir {
				cmd := exec.Command("umount", device)
				if output, err := cmd.CombinedOutput(); err != nil {
					StartDocker()
					return nil, nil, fmt.Errorf("unmount %s from %s: %s: %w",
						device, mnt, strings.TrimSpace(string(output)), err)
				}
			}
		}

		if !isMountpointActive(SingleDiskDir) {
			if err := MountDisk(device, SingleDiskDir); err != nil {
				StartDocker()
				return nil, nil, fmt.Errorf("mount single disk: %w", err)
			}
		}
	} else {
		// Need to format
		result.Action = "formatted"

		if err := unmountAndFormat(device, SingleLabel); err != nil {
			StartDocker()
			return nil, nil, err
		}

		if err := MountDisk(device, SingleDiskDir); err != nil {
			StartDocker()
			return nil, nil, fmt.Errorf("mount single disk: %w", err)
		}
	}

	// Create the subdirectory layout
	configDir := filepath.Join(SingleDiskDir, "config")
	cacheDir := filepath.Join(SingleDiskDir, "cache")

	if err := os.MkdirAll(configDir, 0755); err != nil {
		StartDocker()
		return nil, nil, fmt.Errorf("create config subdir: %w", err)
	}
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		StartDocker()
		return nil, nil, fmt.Errorf("create cache subdir: %w", err)
	}

	// Initialize layouts inside subdirectories
	if err := InitConfigLayout(configDir); err != nil {
		StartDocker()
		return nil, nil, fmt.Errorf("init config layout on single disk: %w", err)
	}
	if err := InitCacheLayout(cacheDir); err != nil {
		StartDocker()
		return nil, nil, fmt.Errorf("init cache layout on single disk: %w", err)
	}

	// Migrate existing tmpfs content from the standard config path
	configMs := getMountStatus(storage.ConfigBase)
	if configMs.Mounted && !configMs.Persistent {
		entries, err := os.ReadDir(storage.ConfigBase)
		if err == nil {
			for _, entry := range entries {
				src := filepath.Join(storage.ConfigBase, entry.Name())
				cpCmd := exec.Command("cp", "-a", src, configDir+"/")
				if output, cpErr := cpCmd.CombinedOutput(); cpErr != nil {
					log.Printf("warning: failed to migrate config %s: %s",
						entry.Name(), strings.TrimSpace(string(output)))
				} else {
					migrated = append(migrated, entry.Name())
				}
			}
		}

		// Unmount tmpfs at ConfigBase so we can bind mount over it
		umountCmd := exec.Command("umount", storage.ConfigBase)
		if output, err := umountCmd.CombinedOutput(); err != nil {
			lazyCmd := exec.Command("umount", "-l", storage.ConfigBase)
			if output, err := lazyCmd.CombinedOutput(); err != nil {
				log.Printf("warning: unmount config tmpfs: %s: %v",
					strings.TrimSpace(string(output)), err)
			}
			// Wait for lazy unmount
			for i := 0; i < 10; i++ {
				if !isMountpointActive(storage.ConfigBase) {
					break
				}
				time.Sleep(200 * time.Millisecond)
			}
		} else {
			_ = output
		}
	}

	// Unmount existing cache tmpfs if present
	cacheMs := getMountStatus(storage.CacheDisk)
	if cacheMs.Mounted && !cacheMs.Persistent {
		umountCmd := exec.Command("umount", storage.CacheDisk)
		if output, err := umountCmd.CombinedOutput(); err != nil {
			lazyCmd := exec.Command("umount", "-l", storage.CacheDisk)
			if output, err := lazyCmd.CombinedOutput(); err != nil {
				log.Printf("warning: unmount cache tmpfs: %s: %v",
					strings.TrimSpace(string(output)), err)
			}
			for i := 0; i < 10; i++ {
				if !isMountpointActive(storage.CacheDisk) {
					break
				}
				time.Sleep(200 * time.Millisecond)
			}
		} else {
			_ = output
		}
	}

	// Set up bind mounts so existing paths work transparently:
	//   /wraith/config-disk -> /wraith/disk/config  (physical config for sync-back)
	//   /wraith/cache       -> /wraith/disk/cache   (Docker data)

	// Bind mount config subdir to ConfigDiskDir for sync-back
	if storage.ConfigDiskDir != "" {
		if err := os.MkdirAll(storage.ConfigDiskDir, 0755); err != nil {
			log.Printf("warning: create %s: %v", storage.ConfigDiskDir, err)
		}
		if !isMountpointActive(storage.ConfigDiskDir) {
			cmd := exec.Command("mount", "--bind", configDir, storage.ConfigDiskDir)
			if output, err := cmd.CombinedOutput(); err != nil {
				log.Printf("warning: bind mount config-disk: %s: %v",
					strings.TrimSpace(string(output)), err)
			} else {
				log.Printf("bind mounted %s -> %s", configDir, storage.ConfigDiskDir)
			}
		}
	}

	// Bind mount cache subdir to CacheDisk path
	if err := os.MkdirAll(storage.CacheDisk, 0755); err != nil {
		log.Printf("warning: create %s: %v", storage.CacheDisk, err)
	}
	if !isMountpointActive(storage.CacheDisk) {
		cmd := exec.Command("mount", "--bind", cacheDir, storage.CacheDisk)
		if output, err := cmd.CombinedOutput(); err != nil {
			StartDocker()
			return nil, nil, fmt.Errorf("bind mount cache: %s: %w",
				strings.TrimSpace(string(output)), err)
		}
		log.Printf("bind mounted %s -> %s", cacheDir, storage.CacheDisk)
	}

	// Re-mount ConfigBase as tmpfs if it was unmounted (RAM-based config arch)
	if !isMountpointActive(storage.ConfigBase) {
		if err := os.MkdirAll(storage.ConfigBase, 0755); err != nil {
			log.Printf("warning: create %s: %v", storage.ConfigBase, err)
		}
		cmd := exec.Command("mount", "-t", "tmpfs", "-o", "size=64M", "tmpfs", storage.ConfigBase)
		if output, err := cmd.CombinedOutput(); err != nil {
			log.Printf("warning: remount config tmpfs: %s: %v",
				strings.TrimSpace(string(output)), err)
		}
		// Copy config from disk into RAM
		cpCmd := exec.Command("cp", "-a", configDir+"/.", storage.ConfigBase+"/")
		if output, err := cpCmd.CombinedOutput(); err != nil {
			log.Printf("warning: copy config to RAM: %s: %v",
				strings.TrimSpace(string(output)), err)
		}
	}

	// Restart Docker now that cache is bind-mounted
	if err := StartDocker(); err != nil {
		log.Printf("warning: failed to restart docker: %v", err)
	}

	result.Mounted = true
	return result, migrated, nil
}

// IsSingleDiskMode checks whether the system is running in single-disk mode
// by looking for a mounted WRAITH-SINGLE labeled device.
func IsSingleDiskMode() bool {
	ms := getMountStatus(SingleDiskDir)
	return ms.Mounted && ms.Persistent
}

// DiskExpandInfo describes a disk that can be expanded.
type DiskExpandInfo struct {
	Device     string `json:"device"`
	Label      string `json:"label"`
	Role       string `json:"role"`       // "config", "cache", or "single"
	Mountpoint string `json:"mountpoint"`
	FSBytes    uint64 `json:"fsBytes"`     // current filesystem size
	DevBytes   uint64 `json:"deviceBytes"` // block device size
	GrowBytes  uint64 `json:"growBytes"`   // available to grow
}

// minExpandThreshold is the minimum difference between device size and
// filesystem size (in bytes) before we offer expansion. Avoids noise
// from alignment overhead and metadata differences.
const minExpandThreshold = 100 * 1024 * 1024 // 100 MB

// CheckDiskExpandable checks if a block device is larger than its ext4
// filesystem, indicating the disk was resized in the hypervisor but the
// filesystem hasn't been grown yet.
func CheckDiskExpandable(device, mountpoint string) (expandable bool, fsBytes, devBytes uint64, err error) {
	if err := ValidateDevice(device); err != nil {
		return false, 0, 0, err
	}

	// Get block device size via lsblk
	cmd := exec.Command("lsblk", "-bndo", "SIZE", device)
	out, err := cmd.Output()
	if err != nil {
		return false, 0, 0, fmt.Errorf("lsblk %s: %w", device, err)
	}
	fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &devBytes)

	// Get filesystem size via df (reports the actual filesystem size, not device)
	dfCmd := exec.Command("df", "--block-size=1", "--output=size", mountpoint)
	dfOut, err := dfCmd.Output()
	if err != nil {
		return false, 0, 0, fmt.Errorf("df %s: %w", mountpoint, err)
	}
	lines := strings.Split(strings.TrimSpace(string(dfOut)), "\n")
	if len(lines) >= 2 {
		fmt.Sscanf(strings.TrimSpace(lines[1]), "%d", &fsBytes)
	}

	if devBytes > fsBytes && (devBytes-fsBytes) > minExpandThreshold {
		return true, fsBytes, devBytes, nil
	}
	return false, fsBytes, devBytes, nil
}

// ExpandDisk runs resize2fs on a mounted ext4 device to grow the filesystem
// to fill the entire block device. ext4 supports online resize (no unmount
// needed for grow operations).
func ExpandDisk(device string) error {
	diskMu.Lock()
	defer diskMu.Unlock()

	if err := ValidateDevice(device); err != nil {
		return err
	}

	// Verify the device is mounted (resize2fs works online for ext4 grow)
	mnt := isDeviceMounted(device)
	if mnt == "" {
		return fmt.Errorf("device %s is not mounted", device)
	}

	// Verify this is a wraith-managed disk by checking its label
	label := getDeviceLabel(device)
	if label != ConfigLabel && label != CacheLabel && label != SingleLabel {
		return fmt.Errorf("device %s has label %q: not a wraith-managed disk", device, label)
	}

	log.Printf("expanding ext4 filesystem on %s (mounted at %s, label %s)", device, mnt, label)

	cmd := exec.Command("resize2fs", device)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("resize2fs %s: %s: %w", device, strings.TrimSpace(string(output)), err)
	}

	log.Printf("filesystem expanded on %s: %s", device, strings.TrimSpace(string(output)))
	return nil
}

// GetExpandableDisks checks all wraith-managed mounted disks and returns
// those where the block device is larger than the filesystem.
func GetExpandableDisks() []DiskExpandInfo {
	var results []DiskExpandInfo

	type diskCheck struct {
		role       string
		mountpoint string
	}

	var checks []diskCheck

	if IsSingleDiskMode() {
		ms := getMountStatus(SingleDiskDir)
		if ms.Mounted && ms.Persistent && ms.Device != "" {
			checks = append(checks, diskCheck{role: "single", mountpoint: SingleDiskDir})
		}
	} else {
		// Check config disk
		configMs, cacheMs := GetDiskStatus()
		if configMs.Persistent && configMs.Device != "" {
			mp := storage.ConfigBase
			if storage.ConfigDiskDir != "" {
				mp = storage.ConfigDiskDir
			}
			checks = append(checks, diskCheck{role: "config", mountpoint: mp})
		}
		if cacheMs.Persistent && cacheMs.Device != "" {
			checks = append(checks, diskCheck{role: "cache", mountpoint: storage.CacheDisk})
		}
	}

	for _, chk := range checks {
		ms := getMountStatus(chk.mountpoint)
		if !ms.Mounted || !ms.Persistent || ms.Device == "" {
			continue
		}

		expandable, fsBytes, devBytes, err := CheckDiskExpandable(ms.Device, chk.mountpoint)
		if err != nil {
			log.Printf("check expand %s (%s): %v", ms.Device, chk.role, err)
			continue
		}
		if !expandable {
			continue
		}

		results = append(results, DiskExpandInfo{
			Device:     ms.Device,
			Label:      ms.Label,
			Role:       chk.role,
			Mountpoint: chk.mountpoint,
			FSBytes:    fsBytes,
			DevBytes:   devBytes,
			GrowBytes:  devBytes - fsBytes,
		})
	}

	return results
}
