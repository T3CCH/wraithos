package system

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/wraithos/wraith-ui/internal/docker"
	"github.com/wraithos/wraith-ui/internal/storage"
)

// MountWatchdog monitors mounts flagged as "required for Docker" and
// automatically re-mounts them and restarts the Docker compose stack
// if they become unmounted. Also supports per-stack mount requirements.
type MountWatchdog struct {
	samba    *SambaManager
	logs     *LogCollector
	interval time.Duration
	cancel   context.CancelFunc
}

// NewMountWatchdog creates a watchdog that checks mount status periodically.
func NewMountWatchdog(samba *SambaManager, logs *LogCollector) *MountWatchdog {
	return &MountWatchdog{
		samba:    samba,
		logs:     logs,
		interval: 30 * time.Second,
	}
}

// Start begins the watchdog loop in a background goroutine.
func (w *MountWatchdog) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	go w.run(ctx)
	w.logs.Info("watchdog", "mount watchdog started (interval: %v)", w.interval)
}

// Stop signals the watchdog loop to exit.
func (w *MountWatchdog) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
}

func (w *MountWatchdog) run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.check()
		}
	}
}

func (w *MountWatchdog) check() {
	mounts, err := w.samba.GetDockerRequiredMounts()
	if err != nil {
		w.logs.Warn("watchdog", "failed to get docker-required mounts: %v", err)
		return
	}

	if len(mounts) == 0 {
		return
	}

	var remounted []string
	remountedNames := make(map[string]bool)
	for _, m := range mounts {
		if m.Mounted {
			continue
		}

		w.logs.Warn("watchdog", "docker-required mount %s (%s) is unmounted, attempting re-mount",
			m.ID, m.MountPoint)

		if err := w.samba.Mount(m.ID); err != nil {
			w.logs.Error("watchdog", "failed to re-mount %s: %v", m.ID, err)
			continue
		}

		w.logs.Info("watchdog", "re-mounted %s at %s", m.ID, m.MountPoint)
		remounted = append(remounted, m.ID)
		// Extract mount name from path (e.g., /remotemounts/media -> media)
		mountName := filepath.Base(m.MountPoint)
		remountedNames[mountName] = true
	}

	if len(remounted) > 0 {
		w.logs.Info("watchdog", "re-mounted %d docker-required mount(s), restarting affected stacks",
			len(remounted))
		w.restartAffectedStacks(remountedNames)
	}
}

// restartAffectedStacks restarts stacks that depend on the given mount names.
// Falls back to restarting the legacy compose stack if no per-stack config exists.
func (w *MountWatchdog) restartAffectedStacks(remountedNames map[string]bool) {
	// Check per-stack mount requirements
	cfg, err := docker.LoadStacksConfig()
	if err == nil && len(cfg.Stacks) > 0 {
		restarted := 0
		for name, stack := range cfg.Stacks {
			if len(stack.RequiredMounts) == 0 {
				continue
			}
			// Check if any of this stack's required mounts were just remounted
			needsRestart := false
			for _, reqMount := range stack.RequiredMounts {
				if remountedNames[reqMount] {
					needsRestart = true
					break
				}
			}
			if !needsRestart {
				continue
			}

			w.logs.Info("watchdog", "restarting stack %s after mount recovery", name)
			w.restartStack(name)
			restarted++
		}
		if restarted > 0 {
			w.logs.Info("watchdog", "restarted %d stack(s) after mount recovery", restarted)
			return
		}
	}

	// Fallback: restart legacy compose stack
	w.restartLegacyStack()
}

// restartStack restarts a single named stack.
func (w *MountWatchdog) restartStack(name string) {
	dir := filepath.Join(storage.AppsDir(), name)
	composeFile := filepath.Join(dir, "docker-compose.yml")

	if !storage.Exists(composeFile) {
		w.logs.Warn("watchdog", "stack %s has no compose file, skipping restart", name)
		return
	}

	// docker compose down
	downCmd := exec.Command("docker", "compose", "-f", composeFile, "-p", name,
		"--project-directory", dir, "down")
	if output, err := downCmd.CombinedOutput(); err != nil {
		w.logs.Error("watchdog", "stack %s compose down failed: %s: %v",
			name, strings.TrimSpace(string(output)), err)
	}

	// docker compose up -d
	upCmd := exec.Command("docker", "compose", "-f", composeFile, "-p", name,
		"--project-directory", dir, "up", "-d", "--remove-orphans")
	if output, err := upCmd.CombinedOutput(); err != nil {
		w.logs.Error("watchdog", "stack %s compose up failed: %s: %v",
			name, strings.TrimSpace(string(output)), err)
		return
	}

	w.logs.Info("watchdog", "stack %s restarted after mount recovery", name)
}

// restartLegacyStack restarts the legacy single compose stack.
func (w *MountWatchdog) restartLegacyStack() {
	composeFile := storage.ComposeFile()

	if !storage.Exists(composeFile) {
		w.logs.Warn("watchdog", "no compose file found, skipping stack restart")
		return
	}

	// docker compose down
	downCmd := exec.Command("docker", "compose", "-f", composeFile, "down")
	if output, err := downCmd.CombinedOutput(); err != nil {
		w.logs.Error("watchdog", "docker compose down failed: %s: %v",
			strings.TrimSpace(string(output)), err)
	}

	// docker compose up -d
	upCmd := exec.Command("docker", "compose", "-f", composeFile, "up", "-d", "--remove-orphans")
	if output, err := upCmd.CombinedOutput(); err != nil {
		w.logs.Error("watchdog", "docker compose up failed: %s: %v",
			strings.TrimSpace(string(output)), err)
		return
	}

	w.logs.Info("watchdog", "docker compose stack restarted after mount recovery")
}

// WatchdogStatus returns the current state of docker-required mounts.
type WatchdogStatus struct {
	Mounts []WatchdogMountStatus `json:"mounts"`
}

// WatchdogMountStatus represents the watchdog view of a single mount.
type WatchdogMountStatus struct {
	ID         string `json:"id"`
	MountPoint string `json:"mountpoint"`
	Server     string `json:"server"`
	Share      string `json:"share"`
	Mounted    bool   `json:"mounted"`
}

// Status returns the current watchdog status for API consumers.
func (w *MountWatchdog) Status() (*WatchdogStatus, error) {
	mounts, err := w.samba.GetDockerRequiredMounts()
	if err != nil {
		return nil, fmt.Errorf("get docker-required mounts: %w", err)
	}

	status := &WatchdogStatus{}
	for _, m := range mounts {
		status.Mounts = append(status.Mounts, WatchdogMountStatus{
			ID:         m.ID,
			MountPoint: m.MountPoint,
			Server:     m.Server,
			Share:      m.Share,
			Mounted:    m.Mounted,
		})
	}
	return status, nil
}
