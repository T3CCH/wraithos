// Package storage provides configuration disk operations and standard paths.
package storage

import "path/filepath"

// Default paths matching the OS boot scripts (see os/rootfs/etc/conf.d/wraith).
var (
	// ConfigBase is the mount point for the live config (tmpfs in RAM).
	// All Go code reads/writes here. Changes are synced back to the
	// physical config disk at ConfigDiskDir.
	ConfigBase = "/wraith/config"

	// ConfigDiskDir is the mount point for the physical config disk.
	// Used only for sync-back; the Go app does not read/write here directly.
	ConfigDiskDir = "/wraith/config-disk"

	// CacheDisk is the mount point for the Docker cache / volumes disk.
	CacheDisk = "/wraith/cache"
)

// Derived paths are computed from ConfigBase and CacheDisk.
// Call these functions instead of using constants so overrides take effect.

// ComposeDir returns the directory holding docker-compose stack files.
func ComposeDir() string { return filepath.Join(ConfigBase, "compose") }

// ComposeFile returns the path to the main docker-compose file.
func ComposeFile() string { return filepath.Join(ComposeDir(), "docker-compose.yml") }

// MountsDir returns the base directory for network mount points.
func MountsDir() string { return "/mnt" }

// AuthFile returns the path to the bcrypt-hashed credentials file.
func AuthFile() string { return filepath.Join(ConfigBase, "auth.json") }

// SambaFile returns the path to the Samba mount configuration file.
func SambaFile() string { return filepath.Join(ConfigBase, "samba.json") }

// NetworkFile returns the path to the network configuration file.
func NetworkFile() string { return filepath.Join(ConfigBase, "network.json") }
