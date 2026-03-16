// Package system provides SSH service management for WraithOS.
//
// SSH is OFF by default for security. When enabled via the web UI,
// the sshd service is started via OpenRC and the setting is persisted
// to the config disk so it survives reboots.
package system

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/wraithos/wraith-ui/internal/storage"
)

// SSHConfig holds the persisted SSH configuration.
type SSHConfig struct {
	Enabled bool `json:"enabled"`
}

// SSHStatus represents the current state of the SSH service.
type SSHStatus struct {
	Enabled   bool `json:"enabled"`   // persisted config setting
	Running   bool `json:"running"`   // whether sshd process is actually running
	Installed bool `json:"installed"` // whether openssh-server is available
}

// GetSSHStatus checks both the persisted config and the live service state.
func GetSSHStatus() (*SSHStatus, error) {
	cfg := loadSSHConfig()
	running := isSSHDRunning()
	installed := isSSHDInstalled()
	return &SSHStatus{
		Enabled:   cfg.Enabled,
		Running:   running,
		Installed: installed,
	}, nil
}

// EnableSSH starts the sshd service and persists the enabled state.
func EnableSSH() error {
	// Check that openssh-server is installed
	if !isSSHDInstalled() {
		return fmt.Errorf("openssh-server is not installed; install it with: apk add openssh-server")
	}

	// Generate host keys if they don't exist (required for sshd to start)
	if err := ensureSSHHostKeys(); err != nil {
		return fmt.Errorf("generate host keys: %w", err)
	}

	// Start sshd via OpenRC
	cmd := exec.Command("rc-service", "sshd", "start")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("start sshd: %s: %w",
			strings.TrimSpace(string(output)), err)
	}

	// Add to default runlevel so it starts on boot (if rc-update is available)
	addCmd := exec.Command("rc-update", "add", "sshd", "default")
	if output, err := addCmd.CombinedOutput(); err != nil {
		// Non-fatal: the service is running, just won't auto-start on boot
		// via OpenRC runlevel (config persistence handles this instead)
		_ = output
	}

	// Persist to config disk
	cfg := SSHConfig{Enabled: true}
	if err := storage.WriteJSON(storage.SSHFile(), &cfg); err != nil {
		return fmt.Errorf("save ssh config: %w", err)
	}

	return nil
}

// DisableSSH stops the sshd service and persists the disabled state.
func DisableSSH() error {
	// Stop sshd via OpenRC
	cmd := exec.Command("rc-service", "sshd", "stop")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("stop sshd: %s: %w",
			strings.TrimSpace(string(output)), err)
	}

	// Remove from default runlevel
	delCmd := exec.Command("rc-update", "del", "sshd", "default")
	if output, err := delCmd.CombinedOutput(); err != nil {
		_ = output // Non-fatal
	}

	// Persist to config disk
	cfg := SSHConfig{Enabled: false}
	if err := storage.WriteJSON(storage.SSHFile(), &cfg); err != nil {
		return fmt.Errorf("save ssh config: %w", err)
	}

	return nil
}

// StartSSHIfEnabled checks the persisted config and starts sshd if enabled.
// Call this on startup (e.g., in main.go) to restore SSH state after reboot.
func StartSSHIfEnabled() {
	cfg := loadSSHConfig()
	if !cfg.Enabled {
		return
	}

	if !isSSHDInstalled() {
		fmt.Println("ssh: auto-start skipped: openssh-server not installed")
		return
	}

	// Generate host keys if needed (may be missing after ISO re-flash)
	if err := ensureSSHHostKeys(); err != nil {
		fmt.Printf("ssh: host key generation failed: %v\n", err)
	}

	cmd := exec.Command("rc-service", "sshd", "start")
	if output, err := cmd.CombinedOutput(); err != nil {
		fmt.Printf("ssh: auto-start failed: %s: %v\n",
			strings.TrimSpace(string(output)), err)
	}
}

// loadSSHConfig reads the persisted SSH config. Returns disabled if not found.
func loadSSHConfig() SSHConfig {
	var cfg SSHConfig
	if err := storage.ReadJSON(storage.SSHFile(), &cfg); err != nil {
		// File doesn't exist or is corrupt -- default to disabled
		return SSHConfig{Enabled: false}
	}
	return cfg
}

// isSSHDRunning checks if the sshd service is currently running.
func isSSHDRunning() bool {
	cmd := exec.Command("rc-service", "sshd", "status")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	// OpenRC status output contains "started" when running
	return strings.Contains(strings.ToLower(string(output)), "started")
}

// isSSHDInstalled checks if the openssh-server init script exists.
func isSSHDInstalled() bool {
	_, err := os.Stat("/etc/init.d/sshd")
	return err == nil
}

// ensureSSHHostKeys generates SSH host keys if they don't already exist.
// On Alpine Linux, sshd will fail to start without host keys.
func ensureSSHHostKeys() error {
	keyTypes := []struct {
		algo string
		file string
	}{
		{"rsa", "/etc/ssh/ssh_host_rsa_key"},
		{"ecdsa", "/etc/ssh/ssh_host_ecdsa_key"},
		{"ed25519", "/etc/ssh/ssh_host_ed25519_key"},
	}

	// Ensure /etc/ssh directory exists
	if err := os.MkdirAll("/etc/ssh", 0755); err != nil {
		return fmt.Errorf("create /etc/ssh: %w", err)
	}

	for _, kt := range keyTypes {
		if _, err := os.Stat(kt.file); err == nil {
			continue // key already exists
		}

		// Ensure parent directory exists (should already from above)
		dir := filepath.Dir(kt.file)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create dir %s: %w", dir, err)
		}

		cmd := exec.Command("ssh-keygen", "-t", kt.algo, "-f", kt.file, "-N", "")
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("generate %s key: %s: %w",
				kt.algo, strings.TrimSpace(string(output)), err)
		}
	}

	return nil
}
