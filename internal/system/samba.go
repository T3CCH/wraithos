package system

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/wraithos/wraith-ui/internal/storage"
)

var validServerName = regexp.MustCompile(`^[\w.\-]+$`)
var validMountName = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// SambaMount represents a configured network mount (CIFS or NFS).
type SambaMount struct {
	ID             string `json:"id"`
	Type           string `json:"type,omitempty"` // "cifs" (default) or "nfs"
	Server         string `json:"server"`
	Share          string `json:"share"`
	MountPoint     string `json:"mountpoint"`
	Username       string `json:"username"`
	Password       string `json:"password,omitempty"`
	Options        string `json:"options,omitempty"`
	Mounted        bool   `json:"mounted"`
	DockerRequired bool   `json:"dockerRequired,omitempty"` // auto-remount + restart Docker stack
}

// MountType returns the mount type, defaulting to "cifs" for backward compatibility.
func (m *SambaMount) MountType() string {
	if m.Type == "nfs" {
		return "nfs"
	}
	return "cifs"
}

// SambaConfig holds all configured mounts.
type SambaConfig struct {
	Mounts []SambaMount `json:"mounts"`
}

// SambaManager manages network mounts (CIFS and NFS).
type SambaManager struct {
	mu sync.Mutex
}

// NewSambaManager creates a new network mount manager.
func NewSambaManager() *SambaManager {
	return &SambaManager{}
}

// ListMounts returns all configured mounts with current mount status.
func (m *SambaManager) ListMounts() ([]SambaMount, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg, err := m.loadConfig()
	if err != nil {
		return nil, err
	}

	// Update mounted status from system
	mounted := getActiveMounts()
	for i := range cfg.Mounts {
		cfg.Mounts[i].Mounted = mounted[cfg.Mounts[i].MountPoint]
		// Never expose passwords in list responses
		cfg.Mounts[i].Password = ""
	}

	return cfg.Mounts, nil
}

// AddMount adds a new network mount configuration.
// The MountPoint field is treated as a mount name (e.g. "media") and the
// actual mount path is constructed as /remotemounts/<name>.
func (m *SambaManager) AddMount(mount SambaMount) (*SambaMount, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if mount.Server == "" || mount.Share == "" {
		return nil, fmt.Errorf("server and share are required")
	}

	// Validate mount type
	mountType := mount.MountType()
	mount.Type = mountType

	// Validate mount name (sent via MountPoint field from frontend)
	mountName := mount.MountPoint
	if mountName == "" {
		return nil, fmt.Errorf("mount name is required")
	}
	if !validMountName.MatchString(mountName) {
		return nil, fmt.Errorf("invalid mount name: must contain only alphanumeric characters, hyphens, or underscores")
	}

	// Construct the actual mount point from the name
	mount.MountPoint = filepath.Join(storage.MountsDir(), mountName)

	// Validate server name to prevent injection
	if !validServerName.MatchString(mount.Server) {
		return nil, fmt.Errorf("invalid server name: must contain only alphanumeric, dot, hyphen, or underscore characters")
	}

	// Validate credentials (only relevant for CIFS)
	if mountType == "cifs" {
		if err := validateCredential(mount.Username, "username"); err != nil {
			return nil, err
		}
		if err := validateCredential(mount.Password, "password"); err != nil {
			return nil, err
		}
	} else {
		// NFS does not use credentials
		mount.Username = ""
		mount.Password = ""
	}

	cfg, err := m.loadConfig()
	if err != nil {
		return nil, err
	}

	// Generate ID from server and share
	mount.ID = fmt.Sprintf("%s_%s", sanitizeID(mount.Server), sanitizeID(mount.Share))

	// Check for duplicate
	for _, existing := range cfg.Mounts {
		if existing.ID == mount.ID {
			return nil, fmt.Errorf("mount %s already exists", mount.ID)
		}
	}

	// Check for duplicate mount point
	for _, existing := range cfg.Mounts {
		if existing.MountPoint == mount.MountPoint {
			return nil, fmt.Errorf("mount name %q is already in use", mountName)
		}
	}

	cfg.Mounts = append(cfg.Mounts, mount)
	if err := storage.WriteJSON(storage.SambaFile(), cfg); err != nil {
		return nil, fmt.Errorf("save config: %w", err)
	}

	// Don't return the password
	result := mount
	result.Password = ""
	return &result, nil
}

// RemoveMount removes a mount configuration and unmounts if active.
func (m *SambaManager) RemoveMount(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg, err := m.loadConfig()
	if err != nil {
		return err
	}

	found := false
	var updated []SambaMount
	for _, mount := range cfg.Mounts {
		if mount.ID == id {
			found = true
			// Unmount if currently mounted
			if isMounted(mount.MountPoint) {
				exec.Command("umount", mount.MountPoint).Run()
			}
			continue
		}
		updated = append(updated, mount)
	}

	if !found {
		return fmt.Errorf("mount %s not found", id)
	}

	cfg.Mounts = updated
	return storage.WriteJSON(storage.SambaFile(), cfg)
}

// Mount activates a configured mount.
func (m *SambaManager) Mount(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg, err := m.loadConfig()
	if err != nil {
		return err
	}

	mount, err := m.findMount(cfg, id)
	if err != nil {
		return err
	}

	// Create mount point directory
	if err := os.MkdirAll(mount.MountPoint, 0755); err != nil {
		return fmt.Errorf("create mount point: %w", err)
	}

	switch mount.MountType() {
	case "nfs":
		return m.mountNFS(mount)
	default:
		return m.mountCIFS(mount)
	}
}

// mountCIFS mounts a CIFS/SMB share.
func (m *SambaManager) mountCIFS(mount *SambaMount) error {
	source := fmt.Sprintf("//%s/%s", mount.Server, mount.Share)
	opts := "iocharset=utf8"

	var credFile *os.File
	if mount.Username != "" {
		// Write credentials to a temp file with restricted permissions
		var err error
		credFile, err = os.CreateTemp("", "cifs-cred-*")
		if err != nil {
			return fmt.Errorf("create credentials file: %w", err)
		}
		defer os.Remove(credFile.Name())
		defer credFile.Close()

		if err := os.Chmod(credFile.Name(), 0600); err != nil {
			return fmt.Errorf("chmod credentials file: %w", err)
		}

		credContent := fmt.Sprintf("username=%s\n", mount.Username)
		if mount.Password != "" {
			credContent += fmt.Sprintf("password=%s\n", mount.Password)
		}
		if _, err := credFile.WriteString(credContent); err != nil {
			return fmt.Errorf("write credentials file: %w", err)
		}
		credFile.Close()

		opts += ",credentials=" + credFile.Name()
	} else {
		opts += ",guest"
	}

	if mount.Options != "" {
		opts += "," + mount.Options
	}

	cmd := exec.Command("mount.cifs", source, mount.MountPoint, "-o", opts)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mount.cifs failed: %s: %w", strings.TrimSpace(string(output)), err)
	}

	return nil
}

// mountNFS mounts an NFS export.
func (m *SambaManager) mountNFS(mount *SambaMount) error {
	source := fmt.Sprintf("%s:%s", mount.Server, mount.Share)
	opts := "noatime,nfsvers=4"

	if mount.Options != "" {
		opts += "," + mount.Options
	}

	cmd := exec.Command("mount", "-t", "nfs", source, mount.MountPoint, "-o", opts)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mount -t nfs failed: %s: %w", strings.TrimSpace(string(output)), err)
	}

	return nil
}

// Unmount deactivates a configured mount.
func (m *SambaManager) Unmount(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg, err := m.loadConfig()
	if err != nil {
		return err
	}

	mount, err := m.findMount(cfg, id)
	if err != nil {
		return err
	}

	if !isMounted(mount.MountPoint) {
		return fmt.Errorf("mount %s is not currently mounted", id)
	}

	output, err := exec.Command("umount", mount.MountPoint).CombinedOutput()
	if err != nil {
		return fmt.Errorf("umount failed: %s: %w", strings.TrimSpace(string(output)), err)
	}

	return nil
}

// UpdateDockerRequired sets the "required for Docker" flag on a mount.
func (m *SambaManager) UpdateDockerRequired(id string, required bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg, err := m.loadConfig()
	if err != nil {
		return err
	}

	found := false
	for i := range cfg.Mounts {
		if cfg.Mounts[i].ID == id {
			cfg.Mounts[i].DockerRequired = required
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("mount %s not found", id)
	}

	return storage.WriteJSON(storage.SambaFile(), cfg)
}

// GetDockerRequiredMounts returns all mounts flagged as required for Docker.
// Caller should check Mounted status against the active mount table.
func (m *SambaManager) GetDockerRequiredMounts() ([]SambaMount, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg, err := m.loadConfig()
	if err != nil {
		return nil, err
	}

	mounted := getActiveMounts()
	var result []SambaMount
	for _, mount := range cfg.Mounts {
		if mount.DockerRequired {
			mount.Mounted = mounted[mount.MountPoint]
			mount.Password = "" // never expose passwords
			result = append(result, mount)
		}
	}
	return result, nil
}

func (m *SambaManager) loadConfig() (*SambaConfig, error) {
	cfg := &SambaConfig{}
	if !storage.Exists(storage.SambaFile()) {
		return cfg, nil
	}
	if err := storage.ReadJSON(storage.SambaFile(), cfg); err != nil {
		return nil, fmt.Errorf("read samba config: %w", err)
	}
	return cfg, nil
}

func (m *SambaManager) findMount(cfg *SambaConfig, id string) (*SambaMount, error) {
	for i := range cfg.Mounts {
		if cfg.Mounts[i].ID == id {
			return &cfg.Mounts[i], nil
		}
	}
	return nil, fmt.Errorf("mount %s not found", id)
}

func getActiveMounts() map[string]bool {
	result := make(map[string]bool)
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return result
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && (fields[2] == "cifs" || fields[2] == "nfs" || fields[2] == "nfs4") {
			result[fields[1]] = true
		}
	}
	return result
}

func isMounted(path string) bool {
	mounts := getActiveMounts()
	return mounts[path]
}

func validateCredential(value, field string) error {
	if value == "" {
		return nil
	}
	if strings.ContainsAny(value, ",\n\r") {
		return fmt.Errorf("invalid %s: must not contain commas or newlines", field)
	}
	return nil
}

func sanitizeID(s string) string {
	s = strings.ReplaceAll(s, ".", "_")
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, " ", "_")
	return strings.ToLower(s)
}
