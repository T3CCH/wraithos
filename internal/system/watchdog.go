package system

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/wraithos/wraith-ui/internal/storage"
)

// MountWatchdog monitors mounts flagged as "required for Docker" and
// automatically re-mounts them and restarts the Docker compose stack
// if they become unmounted.
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
	}

	if len(remounted) > 0 {
		w.logs.Info("watchdog", "re-mounted %d docker-required mount(s), restarting compose stack",
			len(remounted))
		w.restartDockerStack()
	}
}

// restartDockerStack restarts the docker compose stack using the compose file
// at the standard path.
func (w *MountWatchdog) restartDockerStack() {
	composeFile := storage.ComposeFile()

	// Check if compose file exists before trying to restart
	if !storage.Exists(composeFile) {
		w.logs.Warn("watchdog", "no compose file found, skipping stack restart")
		return
	}

	// docker compose down
	downCmd := exec.Command("docker", "compose", "-f", composeFile, "down")
	if output, err := downCmd.CombinedOutput(); err != nil {
		w.logs.Error("watchdog", "docker compose down failed: %s: %v",
			strings.TrimSpace(string(output)), err)
		// Continue to try 'up' anyway
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
