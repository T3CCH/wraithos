// Package system provides SSH service management for WraithOS.
//
// SSH is OFF by default for security. When enabled via the web UI,
// the sshd service is started via OpenRC and the setting is persisted
// to the config disk so it survives reboots.
package system

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/wraithos/wraith-ui/internal/storage"
)

// SSHConfig holds the persisted SSH configuration.
type SSHConfig struct {
	Enabled bool `json:"enabled"`
}

// SSHStatus represents the current state of the SSH service.
type SSHStatus struct {
	Enabled bool `json:"enabled"` // persisted config setting
	Running bool `json:"running"` // whether sshd process is actually running
}

// GetSSHStatus checks both the persisted config and the live service state.
func GetSSHStatus() (*SSHStatus, error) {
	cfg := loadSSHConfig()
	running := isSSHDRunning()
	return &SSHStatus{
		Enabled: cfg.Enabled,
		Running: running,
	}, nil
}

// EnableSSH starts the sshd service and persists the enabled state.
func EnableSSH() error {
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
