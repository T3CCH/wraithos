package storage

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// SyncConfigFile copies a single file from the RAM config (ConfigBase) to
// the physical config disk (ConfigDiskDir). It is a no-op if the physical
// disk is not mounted or if the path is not under ConfigBase.
//
// This should be called after any config write so that the physical disk
// stays in sync with the RAM copy.
func SyncConfigFile(path string) {
	if ConfigDiskDir == "" {
		return
	}

	// Only sync files that live under ConfigBase
	rel, err := filepath.Rel(ConfigBase, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return
	}

	// Check if the physical config disk is mounted
	if !isMounted(ConfigDiskDir) {
		return
	}

	dest := filepath.Join(ConfigDiskDir, rel)

	// Ensure destination directory exists
	destDir := filepath.Dir(dest)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		log.Printf("sync-config: create dir %s: %v", destDir, err)
		return
	}

	// Use cp -a to preserve permissions and handle all file types
	cmd := exec.Command("cp", "-a", path, dest)
	if output, err := cmd.CombinedOutput(); err != nil {
		log.Printf("sync-config: copy %s -> %s: %s: %v",
			path, dest, strings.TrimSpace(string(output)), err)
	}
}

// SyncAll copies the entire ConfigBase directory to the physical config disk
// (ConfigDiskDir). This ensures all config files are persisted before
// operations like reboot. Returns an error if the sync fails.
//
// This is more thorough than SyncConfigFile: it catches any files that may
// have been written outside the normal WriteJSON/WriteFile paths.
//
// Uses cp -a (available via busybox on Alpine) rather than rsync, since
// rsync is not included in the WraithOS base image.
func SyncAll() error {
	if ConfigDiskDir == "" {
		return fmt.Errorf("config disk directory not configured")
	}

	if !isMounted(ConfigDiskDir) {
		// Config disk not mounted -- nothing to sync to. This is not an
		// error; the system may be running on tmpfs without a persistent disk.
		log.Printf("sync-all: config disk not mounted at %s, skipping", ConfigDiskDir)
		return nil
	}

	src := strings.TrimRight(ConfigBase, "/")
	dst := strings.TrimRight(ConfigDiskDir, "/")

	// Remove existing content on the config disk and replace with current
	// RAM config. This is a two-step process:
	//   1. Remove old files from the destination (except lost+found)
	//   2. Copy current config files
	//
	// We use a shell command to handle the glob and avoid removing the
	// destination directory itself (which is a mount point).
	cleanCmd := exec.Command("sh", "-c",
		fmt.Sprintf(`find %q -mindepth 1 -maxdepth 1 ! -name lost+found -exec rm -rf {} +`, dst))
	if output, err := cleanCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("clean config disk %s: %s: %w",
			dst, strings.TrimSpace(string(output)), err)
	}

	// Copy all config files to the config disk, preserving permissions
	// The trailing /. copies the contents of src into dst
	cpCmd := exec.Command("cp", "-a", src+"/.", dst+"/")
	if output, err := cpCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("copy config %s -> %s: %s: %w",
			src, dst, strings.TrimSpace(string(output)), err)
	}

	// Force filesystem sync to flush buffers to physical disk
	syncCmd := exec.Command("sync")
	if syncOut, syncErr := syncCmd.CombinedOutput(); syncErr != nil {
		log.Printf("sync-all: sync command failed: %s: %v",
			strings.TrimSpace(string(syncOut)), syncErr)
		// Non-fatal: cp already wrote the data
	}

	log.Printf("sync-all: config synced to %s", dst)
	return nil
}

// isMounted checks /proc/mounts for an active mount at the given path.
func isMounted(mountpoint string) bool {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == mountpoint {
			return true
		}
	}
	return false
}
