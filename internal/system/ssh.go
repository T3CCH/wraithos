// Package system provides SSH service management for WraithOS.
//
// SSH is OFF by default for security. When enabled via the web UI,
// the sshd service is started via OpenRC and the setting is persisted
// to the config disk so it survives reboots.
package system

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

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

	// Ensure PermitRootLogin is enabled (Alpine defaults to prohibit-password)
	configurePermitRootLogin()

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

	// Start idle monitor to auto-stop sshd after 30 min of no sessions
	StartSSHIdleMonitor()

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

	// Ensure PermitRootLogin is enabled
	configurePermitRootLogin()

	cmd := exec.Command("rc-service", "sshd", "start")
	if output, err := cmd.CombinedOutput(); err != nil {
		fmt.Printf("ssh: auto-start failed: %s: %v\n",
			strings.TrimSpace(string(output)), err)
		return
	}

	// Start idle monitor to auto-stop sshd after 30 min of no sessions
	StartSSHIdleMonitor()
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

// ensureSSHHostKeys restores persisted host keys from the config disk, or
// generates new ones and saves them. This prevents "host key changed"
// warnings after reboots (since /etc/ssh is on the volatile overlay).
func ensureSSHHostKeys() error {
	keyDir := "/etc/ssh"
	persistDir := filepath.Join(storage.ConfigBase, "ssh_host_keys")

	keyTypes := []struct {
		algo string
		file string
	}{
		{"rsa", "ssh_host_rsa_key"},
		{"ecdsa", "ssh_host_ecdsa_key"},
		{"ed25519", "ssh_host_ed25519_key"},
	}

	os.MkdirAll(keyDir, 0755)
	os.MkdirAll(persistDir, 0700)

	for _, kt := range keyTypes {
		keyPath := filepath.Join(keyDir, kt.file)
		persistPath := filepath.Join(persistDir, kt.file)

		// Check persistent storage first -- restore if available
		if _, err := os.Stat(persistPath); err == nil {
			exec.Command("cp", "-a", persistPath, keyPath).Run()
			exec.Command("cp", "-a", persistPath+".pub", keyPath+".pub").Run()
			continue
		}

		// Check if key exists in /etc/ssh already (e.g. from ISO)
		if _, err := os.Stat(keyPath); err == nil {
			// Save to persistent storage for future reboots
			exec.Command("cp", "-a", keyPath, persistPath).Run()
			exec.Command("cp", "-a", keyPath+".pub", persistPath+".pub").Run()
			continue
		}

		// Generate new key
		cmd := exec.Command("ssh-keygen", "-t", kt.algo, "-f", keyPath, "-N", "")
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("generate %s key: %s: %w",
				kt.algo, strings.TrimSpace(string(output)), err)
		}

		// Save to persistent storage
		exec.Command("cp", "-a", keyPath, persistPath).Run()
		exec.Command("cp", "-a", keyPath+".pub", persistPath+".pub").Run()
	}

	// Sync the persistent key directory to the physical config disk
	// so keys survive hard reboots.
	storage.SyncConfigFile(persistDir)
	for _, kt := range keyTypes {
		storage.SyncConfigFile(filepath.Join(persistDir, kt.file))
		storage.SyncConfigFile(filepath.Join(persistDir, kt.file+".pub"))
	}

	return nil
}

// configurePermitRootLogin ensures sshd_config allows root password login.
// Alpine's default is "#PermitRootLogin prohibit-password" which blocks
// password auth for root.
func configurePermitRootLogin() {
	sshConfigPath := "/etc/ssh/sshd_config"
	data, err := os.ReadFile(sshConfigPath)
	if err != nil {
		log.Printf("ssh: could not read sshd_config: %v", err)
		return
	}

	content := string(data)

	// Replace any existing PermitRootLogin line (commented or not)
	if strings.Contains(content, "PermitRootLogin") {
		lines := strings.Split(content, "\n")
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "PermitRootLogin") || strings.HasPrefix(trimmed, "#PermitRootLogin") {
				lines[i] = "PermitRootLogin yes"
			}
		}
		content = strings.Join(lines, "\n")
	} else {
		content += "\nPermitRootLogin yes\n"
	}

	if err := os.WriteFile(sshConfigPath, []byte(content), 0644); err != nil {
		log.Printf("ssh: could not write sshd_config: %v", err)
	}
}

// StartSSHIdleMonitor begins monitoring for idle SSH sessions. If no active
// SSH sessions are detected for 30 minutes, sshd is automatically stopped.
// This is a security measure to avoid leaving SSH open indefinitely.
func StartSSHIdleMonitor() {
	go func() {
		idleSince := time.Now()
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			if !isSSHDRunning() {
				return // sshd was stopped externally, exit monitor
			}

			// Count active SSH sessions (sshd forks with "sshd: user@" for each)
			out, err := exec.Command("sh", "-c", "pgrep -c -f 'sshd:.*@'").Output()
			sessions := 0
			if err == nil {
				fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &sessions)
			}

			if sessions > 0 {
				idleSince = time.Now()
			} else if time.Since(idleSince) > 30*time.Minute {
				log.Printf("ssh: no active sessions for 30 minutes, stopping sshd")
				DisableSSH()
				return
			}
		}
	}()
}
