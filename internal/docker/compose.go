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

// Deploy runs docker compose up -d, streaming output to the handler.
func (cm *ComposeManager) Deploy(ctx context.Context, handler OutputHandler) error {
	return cm.runCompose(ctx, handler, "up", "-d", "--remove-orphans")
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
