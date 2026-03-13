package storage

import (
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
