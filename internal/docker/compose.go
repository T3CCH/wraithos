package docker

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync/atomic"

	"github.com/wraithos/wraith-ui/internal/storage"
)

// ComposeManager handles docker compose operations by shelling out
// to the docker compose CLI (the SDK does not support compose directly).
type ComposeManager struct {
	opRunning int32 // atomic flag: 1 if a compose operation is in progress
}

// NewComposeManager creates a new compose manager.
func NewComposeManager() *ComposeManager {
	return &ComposeManager{}
}

// GetComposeFile reads the current docker-compose.yml content.
func (cm *ComposeManager) GetComposeFile() (string, error) {
	data, err := storage.ReadFile(storage.ComposeFile())
	if err != nil {
		return "", fmt.Errorf("read compose file: %w", err)
	}
	return string(data), nil
}

// SaveComposeFile writes the docker-compose.yml content.
// Performs basic YAML validation by running docker compose config.
func (cm *ComposeManager) SaveComposeFile(content string) error {
	// Write the file first
	if err := storage.WriteFile(storage.ComposeFile(), []byte(content)); err != nil {
		return fmt.Errorf("write compose file: %w", err)
	}

	// Validate with docker compose config
	cmd := exec.Command("docker", "compose", "-f", storage.ComposeFile(), "config", "--quiet")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("invalid compose file: %s", strings.TrimSpace(string(output)))
	}

	return nil
}

// OutputHandler receives streaming output from compose commands.
type OutputHandler func(line string)

// DeployEvent carries structured deploy progress for the frontend.
type DeployEvent struct {
	Type    string `json:"type"`              // "phase", "pull_progress", "service", "output", "error", "warning", "success", "pull", "complete"
	Phase   string `json:"phase,omitempty"`   // "pull", "create", "start", "done"
	Service string `json:"service,omitempty"` // service or image name
	Status  string `json:"status,omitempty"`  // "pulling", "pulled", "creating", "created", "starting", "started", "error", "exists"
	Line    string `json:"line,omitempty"`    // raw output line
	Success bool   `json:"success,omitempty"` // for complete events
	Error   string `json:"error,omitempty"`   // error message
}

// DeployEventHandler receives structured deploy events.
type DeployEventHandler func(event DeployEvent)

// Deploy runs docker compose up -d, streaming output to the handler.
func (cm *ComposeManager) Deploy(ctx context.Context, handler OutputHandler) error {
	return cm.runCompose(ctx, handler, "up", "-d", "--remove-orphans")
}

// DeployFull runs a phased deployment: pull images first, then create/start containers.
// Sends structured events for each phase so the frontend can show granular progress.
func (cm *ComposeManager) DeployFull(ctx context.Context, eventHandler DeployEventHandler) error {
	// Use atomic flag to prevent concurrent operations
	if !atomic.CompareAndSwapInt32(&cm.opRunning, 0, 1) {
		return fmt.Errorf("another compose operation is already in progress")
	}
	defer atomic.StoreInt32(&cm.opRunning, 0)

	// Phase 1: Pull images
	eventHandler(DeployEvent{Type: "phase", Phase: "pull", Status: "starting"})

	pullErr := cm.runComposeNoLock(ctx, func(line string) {
		event := parsePullLine(line)
		eventHandler(event)
	}, "pull")

	if pullErr != nil {
		// Pull failure is not fatal if images are already available locally.
		// Send warning but continue to up.
		eventHandler(DeployEvent{
			Type:  "warning",
			Phase: "pull",
			Line:  fmt.Sprintf("Pull encountered issues: %v (continuing with available images)", pullErr),
		})
	}

	eventHandler(DeployEvent{Type: "phase", Phase: "pull", Status: "done"})

	// Phase 2: Create and start containers
	eventHandler(DeployEvent{Type: "phase", Phase: "create", Status: "starting"})

	upErr := cm.runComposeNoLock(ctx, func(line string) {
		event := parseUpLine(line)
		eventHandler(event)
	}, "up", "-d", "--remove-orphans")

	if upErr != nil {
		eventHandler(DeployEvent{Type: "phase", Phase: "create", Status: "error", Error: upErr.Error()})
		return upErr
	}

	eventHandler(DeployEvent{Type: "phase", Phase: "create", Status: "done"})
	return nil
}

// runComposeNoLock executes a docker compose command without the atomic lock
// (caller is responsible for locking).
func (cm *ComposeManager) runComposeNoLock(ctx context.Context, handler OutputHandler, args ...string) error {
	fullArgs := append([]string{"compose", "-f", storage.ComposeFile()}, args...)
	cmd := exec.CommandContext(ctx, "docker", fullArgs...)

	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		pw.Close()
		return fmt.Errorf("start docker compose %s: %w", args[0], err)
	}

	go func() {
		cmd.Wait()
		pw.Close()
	}()

	scanner := bufio.NewScanner(pr)
	for scanner.Scan() {
		if handler != nil {
			handler(scanner.Text())
		}
	}

	if cmd.ProcessState != nil && !cmd.ProcessState.Success() {
		return fmt.Errorf("docker compose %s failed: exit code %d", args[0], cmd.ProcessState.ExitCode())
	}

	return nil
}

// parsePullLine parses docker compose pull output into structured events.
// Docker compose pull outputs lines like:
//
//	"[+] Pulling 3/5"
//	" ✔ redis Pulled   2.1s"
//	" ⠋ nginx Pulling  fs layer..."
//	"5d20c808ce19: Downloading [==>  ]  1.2MB/50MB"
//	"5d20c808ce19: Pull complete"
func parsePullLine(line string) DeployEvent {
	trimmed := strings.TrimSpace(line)

	// Service pulled successfully: " ✔ redis Pulled"
	if strings.Contains(trimmed, "Pulled") {
		svc := extractServiceFromPullLine(trimmed)
		return DeployEvent{Type: "pull_progress", Phase: "pull", Service: svc, Status: "pulled", Line: line}
	}

	// Service pulling: " ⠋ nginx Pulling"
	if strings.Contains(trimmed, "Pulling") {
		svc := extractServiceFromPullLine(trimmed)
		return DeployEvent{Type: "pull_progress", Phase: "pull", Service: svc, Status: "pulling", Line: line}
	}

	// Layer download progress: "abc123: Downloading [==>  ] 1.2MB/50MB"
	if strings.Contains(trimmed, "Downloading") || strings.Contains(trimmed, "Extracting") {
		return DeployEvent{Type: "pull_progress", Phase: "pull", Status: "downloading", Line: line}
	}

	// Layer complete
	if strings.Contains(trimmed, "Pull complete") || strings.Contains(trimmed, "Already exists") {
		return DeployEvent{Type: "pull_progress", Phase: "pull", Status: "layer_done", Line: line}
	}

	// Overall pull progress: "[+] Pulling 3/5"
	if strings.HasPrefix(trimmed, "[+]") {
		return DeployEvent{Type: "pull_progress", Phase: "pull", Status: "progress", Line: line}
	}

	// Fall back to raw output
	return DeployEvent{Type: classifyLineType(line), Phase: "pull", Line: line}
}

// parseUpLine parses docker compose up output into structured events.
// Docker compose up -d outputs lines like:
//
//	"[+] Running 3/3"
//	" ✔ Container myapp-redis-1  Created   0.1s"
//	" ✔ Container myapp-web-1    Started   0.3s"
//	" ✔ Network myapp_default    Created   0.0s"
func parseUpLine(line string) DeployEvent {
	trimmed := strings.TrimSpace(line)

	// Container/Network created
	if strings.Contains(trimmed, "Created") {
		svc := extractContainerFromUpLine(trimmed)
		return DeployEvent{Type: "service", Phase: "create", Service: svc, Status: "created", Line: line}
	}

	// Container started
	if strings.Contains(trimmed, "Started") {
		svc := extractContainerFromUpLine(trimmed)
		return DeployEvent{Type: "service", Phase: "start", Service: svc, Status: "started", Line: line}
	}

	// Container/resource already running
	if strings.Contains(trimmed, "Running") && !strings.HasPrefix(trimmed, "[+]") {
		svc := extractContainerFromUpLine(trimmed)
		return DeployEvent{Type: "service", Phase: "start", Service: svc, Status: "exists", Line: line}
	}

	// Overall progress
	if strings.HasPrefix(trimmed, "[+]") {
		return DeployEvent{Type: "output", Phase: "create", Line: line}
	}

	return DeployEvent{Type: classifyLineType(line), Phase: "create", Line: line}
}

// extractServiceFromPullLine extracts the service name from a pull line.
// Input like " ✔ redis Pulled" or " ⠋ nginx Pulling" -> "redis" or "nginx"
func extractServiceFromPullLine(line string) string {
	// Remove spinner/check characters and leading whitespace
	cleaned := strings.TrimSpace(line)
	// Remove common prefix characters (✔, ✗, spinner chars)
	for _, prefix := range []string{"✔", "✗", "⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"} {
		cleaned = strings.TrimPrefix(cleaned, prefix)
	}
	cleaned = strings.TrimSpace(cleaned)
	parts := strings.Fields(cleaned)
	if len(parts) > 0 {
		return parts[0]
	}
	return "unknown"
}

// extractContainerFromUpLine extracts container/resource name from a compose up line.
// Input like " ✔ Container myapp-redis-1  Created" -> "myapp-redis-1"
func extractContainerFromUpLine(line string) string {
	cleaned := strings.TrimSpace(line)
	for _, prefix := range []string{"✔", "✗", "⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"} {
		cleaned = strings.TrimPrefix(cleaned, prefix)
	}
	cleaned = strings.TrimSpace(cleaned)
	parts := strings.Fields(cleaned)
	// "Container myapp-redis-1 Created" or "Network myapp_default Created"
	if len(parts) >= 2 {
		return parts[1] // the name is the second field after "Container"/"Network"
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return "unknown"
}

// classifyLineType categorizes a line for basic color-coding (used as fallback).
func classifyLineType(line string) string {
	lower := strings.ToLower(line)
	if strings.Contains(lower, "error") || strings.Contains(lower, "fatal") {
		return "error"
	}
	if strings.Contains(lower, "warning") || strings.Contains(lower, "warn") {
		return "warning"
	}
	if strings.Contains(lower, "pulling") || strings.Contains(lower, "download") ||
		strings.Contains(lower, "extracting") || strings.Contains(lower, "waiting") {
		return "pull"
	}
	if strings.Contains(lower, "created") || strings.Contains(lower, "started") ||
		strings.Contains(lower, "running") || strings.Contains(lower, "done") {
		return "success"
	}
	return "output"
}

// ListServices returns the service names defined in the compose file.
func (cm *ComposeManager) ListServices(ctx context.Context) ([]string, error) {
	cmd := exec.CommandContext(ctx, "docker", "compose", "-f", storage.ComposeFile(), "config", "--services")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("list services: %s: %w", strings.TrimSpace(string(output)), err)
	}
	var services []string
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			services = append(services, line)
		}
	}
	return services, nil
}

// ListImages returns the image names used by compose services.
func (cm *ComposeManager) ListImages(ctx context.Context) ([]string, error) {
	cmd := exec.CommandContext(ctx, "docker", "compose", "-f", storage.ComposeFile(), "config", "--images")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("list images: %s: %w", strings.TrimSpace(string(output)), err)
	}
	var images []string
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			images = append(images, line)
		}
	}
	return images, nil
}

// Stop runs docker compose down, streaming output to the handler.
func (cm *ComposeManager) Stop(ctx context.Context, handler OutputHandler) error {
	return cm.runCompose(ctx, handler, "down")
}

// Restart runs docker compose down followed by up -d.
func (cm *ComposeManager) Restart(ctx context.Context, handler OutputHandler) error {
	if err := cm.runCompose(ctx, handler, "down"); err != nil {
		return err
	}
	return cm.runCompose(ctx, handler, "up", "-d", "--remove-orphans")
}

// Pull pulls the latest images for all services.
func (cm *ComposeManager) Pull(ctx context.Context, handler OutputHandler) error {
	return cm.runCompose(ctx, handler, "pull")
}

// Status returns the compose stack status.
func (cm *ComposeManager) Status(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", "compose", "-f", storage.ComposeFile(), "ps", "--format", "json")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("compose status: %s: %w", strings.TrimSpace(string(output)), err)
	}
	return string(output), nil
}

// runCompose executes a docker compose command, streaming stdout/stderr
// line by line to the provided handler.
func (cm *ComposeManager) runCompose(ctx context.Context, handler OutputHandler, args ...string) error {
	// Use atomic flag instead of mutex to avoid blocking during long operations
	if !atomic.CompareAndSwapInt32(&cm.opRunning, 0, 1) {
		return fmt.Errorf("another compose operation is already in progress")
	}
	defer atomic.StoreInt32(&cm.opRunning, 0)

	fullArgs := append([]string{"compose", "-f", storage.ComposeFile()}, args...)
	cmd := exec.CommandContext(ctx, "docker", fullArgs...)

	// Merge stdout and stderr into a single pipe for unified streaming
	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		pw.Close()
		return fmt.Errorf("start docker compose %s: %w", args[0], err)
	}

	// Close the write end when the command finishes so the scanner exits
	go func() {
		cmd.Wait()
		pw.Close()
	}()

	// Stream output line by line
	scanner := bufio.NewScanner(pr)
	for scanner.Scan() {
		if handler != nil {
			handler(scanner.Text())
		}
	}

	// cmd.Wait() already called in goroutine; check ProcessState for error
	if cmd.ProcessState != nil && !cmd.ProcessState.Success() {
		return fmt.Errorf("docker compose %s failed: exit code %d", args[0], cmd.ProcessState.ExitCode())
	}

	return nil
}

// Logs retrieves recent logs for the compose stack.
func (cm *ComposeManager) Logs(ctx context.Context, lines int) (string, error) {
	tailArg := fmt.Sprintf("%d", lines)
	cmd := exec.CommandContext(ctx, "docker", "compose", "-f", storage.ComposeFile(), "logs", "--tail", tailArg, "--no-color")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("compose logs: %w", err)
	}
	return string(output), nil
}

// StreamLogs streams compose logs to the provided writer until context cancellation.
func (cm *ComposeManager) StreamLogs(ctx context.Context, w io.Writer) error {
	cmd := exec.CommandContext(ctx, "docker", "compose", "-f", storage.ComposeFile(), "logs", "--follow", "--no-color")
	cmd.Stdout = w
	cmd.Stderr = w
	return cmd.Run()
}
