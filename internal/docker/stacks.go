package docker

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/wraithos/wraith-ui/internal/storage"
)

// Stack represents a docker compose stack's persistent metadata.
type Stack struct {
	Name           string   `json:"name"`
	RequiredMounts []string `json:"requiredMounts,omitempty"`
	CreatedAt      string   `json:"createdAt"`
}

// StacksConfig is the on-disk stacks.json structure.
type StacksConfig struct {
	Stacks map[string]Stack `json:"stacks"`
}

// StackStatus is the API response for a single stack with live status.
type StackStatus struct {
	Name           string            `json:"name"`
	Status         string            `json:"status"` // running, stopped, partial, unknown
	Containers     []ContainerStatus `json:"containers"`
	CreatedAt      string            `json:"createdAt"`
	RequiredMounts []string          `json:"requiredMounts,omitempty"`
}

// ContainerStatus describes a single container within a stack.
type ContainerStatus struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Image   string `json:"image"`
	Ports   string `json:"ports"`
	Started string `json:"started"`
}

// StackDetail is the full response for GET /api/stacks/{name}.
type StackDetail struct {
	Name           string            `json:"name"`
	Status         string            `json:"status"`
	Containers     []ContainerStatus `json:"containers"`
	Compose        string            `json:"compose"`
	Env            string            `json:"env"`
	CreatedAt      string            `json:"createdAt"`
	RequiredMounts []string          `json:"requiredMounts,omitempty"`
}

// StackManager handles multi-stack docker compose operations.
type StackManager struct {
	opRunning int32 // atomic flag per-op
}

// NewStackManager creates a new stack manager.
func NewStackManager() *StackManager {
	return &StackManager{}
}

var validStackName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// ValidateStackName checks that a stack name is valid.
func ValidateStackName(name string) error {
	if name == "" {
		return fmt.Errorf("stack name is required")
	}
	if len(name) > 64 {
		return fmt.Errorf("stack name must be 64 characters or fewer")
	}
	if !validStackName.MatchString(name) {
		return fmt.Errorf("stack name must be lowercase alphanumeric and hyphens, starting with a letter or digit")
	}
	return nil
}

// stackDir returns the directory for a named stack.
func stackDir(name string) string {
	return filepath.Join(storage.AppsDir(), name)
}

// stackComposeFile returns the compose file path for a stack.
func stackComposeFile(name string) string {
	return filepath.Join(stackDir(name), "docker-compose.yml")
}

// stackEnvFile returns the .env file path for a stack.
func stackEnvFile(name string) string {
	return filepath.Join(stackDir(name), ".env")
}

// composeBaseArgs returns the common docker compose arguments for a stack,
// including -f, -p, --project-directory, and --env-file (if .env exists).
func composeBaseArgs(name string) []string {
	args := []string{"compose", "-f", stackComposeFile(name),
		"-p", name, "--project-directory", stackDir(name)}
	envFile := stackEnvFile(name)
	if _, err := os.Stat(envFile); err == nil {
		args = append(args, "--env-file", envFile)
	}
	return args
}

// stacksConfigFile returns the path to stacks.json.
func stacksConfigFile() string {
	return filepath.Join(storage.ConfigBase, "stacks.json")
}

// LoadStacksConfig reads stacks.json from disk.
func LoadStacksConfig() (*StacksConfig, error) {
	cfg := &StacksConfig{Stacks: make(map[string]Stack)}
	path := stacksConfigFile()
	if !storage.Exists(path) {
		return cfg, nil
	}
	if err := storage.ReadJSON(path, cfg); err != nil {
		return cfg, fmt.Errorf("read stacks config: %w", err)
	}
	if cfg.Stacks == nil {
		cfg.Stacks = make(map[string]Stack)
	}
	return cfg, nil
}

// SaveStacksConfig writes stacks.json to disk.
func SaveStacksConfig(cfg *StacksConfig) error {
	return storage.WriteJSON(stacksConfigFile(), cfg)
}

// ListStacks scans /dockerapps/*/docker-compose.yml and returns status for each.
func (sm *StackManager) ListStacks(ctx context.Context) ([]StackStatus, error) {
	cfg, _ := LoadStacksConfig()

	appsDir := storage.AppsDir()
	entries, err := os.ReadDir(appsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []StackStatus{}, nil
		}
		return nil, fmt.Errorf("read apps dir: %w", err)
	}

	var stacks []StackStatus
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		composeFile := stackComposeFile(name)
		if !storage.Exists(composeFile) {
			continue
		}

		status := sm.getStackStatus(ctx, name)

		// Merge metadata from config
		if meta, ok := cfg.Stacks[name]; ok {
			status.CreatedAt = meta.CreatedAt
			status.RequiredMounts = meta.RequiredMounts
		}

		stacks = append(stacks, status)
	}

	if stacks == nil {
		stacks = []StackStatus{}
	}
	return stacks, nil
}

// getStackStatus checks the running state of a stack.
func (sm *StackManager) getStackStatus(ctx context.Context, name string) StackStatus {
	status := StackStatus{
		Name:       name,
		Status:     "unknown",
		Containers: []ContainerStatus{},
	}

	psArgs := append(composeBaseArgs(name), "ps", "--format", "json")
	cmd := exec.CommandContext(ctx, "docker", psArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		status.Status = "stopped"
		return status
	}

	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		status.Status = "stopped"
		return status
	}

	// Docker compose ps --format json outputs one JSON object per line
	var containers []ContainerStatus
	running := 0
	total := 0

	for _, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var raw map[string]interface{}
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}
		total++

		cs := ContainerStatus{
			ID:      jsonStr(raw, "ID"),
			Name:    jsonStr(raw, "Name"),
			Status:  jsonStr(raw, "State"),
			Image:   jsonStr(raw, "Image"),
			Ports:   jsonStr(raw, "Ports"),
			Started: jsonStr(raw, "Status"),
		}
		if cs.Status == "running" {
			running++
		}
		containers = append(containers, cs)
	}

	status.Containers = containers
	if total == 0 {
		status.Status = "stopped"
	} else if running == total {
		status.Status = "running"
	} else if running == 0 {
		status.Status = "stopped"
	} else {
		status.Status = "partial"
	}

	return status
}

func jsonStr(m map[string]interface{}, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return fmt.Sprintf("%v", v)
	}
	return s
}

// CreateStack creates a new stack directory with compose and optional .env files.
func (sm *StackManager) CreateStack(name, composeYAML, envContent string) error {
	if err := ValidateStackName(name); err != nil {
		return err
	}

	dir := stackDir(name)
	if storage.Exists(dir) {
		return fmt.Errorf("stack %q already exists", name)
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create stack directory: %w", err)
	}

	// Write compose file
	if err := storage.WriteFile(stackComposeFile(name), []byte(composeYAML)); err != nil {
		os.RemoveAll(dir)
		return fmt.Errorf("write compose file: %w", err)
	}

	// Write .env if provided
	if envContent != "" {
		if err := storage.WriteFile(stackEnvFile(name), []byte(envContent)); err != nil {
			os.RemoveAll(dir)
			return fmt.Errorf("write env file: %w", err)
		}
	}

	// Add to stacks config
	cfg, _ := LoadStacksConfig()
	cfg.Stacks[name] = Stack{
		Name:      name,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := SaveStacksConfig(cfg); err != nil {
		// Non-fatal, stack files are already created
		return fmt.Errorf("save stacks config: %w", err)
	}

	return nil
}

// DeleteStack stops and removes a stack, then deletes its directory.
func (sm *StackManager) DeleteStack(ctx context.Context, name string) error {
	if err := ValidateStackName(name); err != nil {
		return err
	}

	dir := stackDir(name)
	if !storage.Exists(dir) {
		return fmt.Errorf("stack %q does not exist", name)
	}

	// Try to stop the stack first (ignore errors, it may not be running)
	if storage.Exists(stackComposeFile(name)) {
		downArgs := append(composeBaseArgs(name), "down", "--remove-orphans")
		cmd := exec.CommandContext(ctx, "docker", downArgs...)
		cmd.CombinedOutput() // ignore errors
	}

	// Remove directory
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("remove stack directory: %w", err)
	}

	// Remove from config
	cfg, _ := LoadStacksConfig()
	delete(cfg.Stacks, name)
	SaveStacksConfig(cfg) // ignore errors

	return nil
}

// GetStack returns full detail for a single stack.
func (sm *StackManager) GetStack(ctx context.Context, name string) (*StackDetail, error) {
	if err := ValidateStackName(name); err != nil {
		return nil, err
	}

	dir := stackDir(name)
	if !storage.Exists(dir) {
		return nil, fmt.Errorf("stack %q does not exist", name)
	}

	status := sm.getStackStatus(ctx, name)

	compose := ""
	if data, err := os.ReadFile(stackComposeFile(name)); err == nil {
		compose = string(data)
	}

	env := ""
	if data, err := os.ReadFile(stackEnvFile(name)); err == nil {
		env = string(data)
	}

	cfg, _ := LoadStacksConfig()
	detail := &StackDetail{
		Name:       name,
		Status:     status.Status,
		Containers: status.Containers,
		Compose:    compose,
		Env:        env,
	}

	if meta, ok := cfg.Stacks[name]; ok {
		detail.CreatedAt = meta.CreatedAt
		detail.RequiredMounts = meta.RequiredMounts
	}

	return detail, nil
}

// SaveCompose writes a docker-compose.yml for a stack with YAML validation.
func (sm *StackManager) SaveCompose(name, content string) error {
	if err := ValidateStackName(name); err != nil {
		return err
	}

	path := stackComposeFile(name)
	dir := stackDir(name)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create stack directory: %w", err)
	}

	if err := storage.WriteFile(path, []byte(content)); err != nil {
		return fmt.Errorf("write compose file: %w", err)
	}

	// Validate with project-directory and env-file so compose can resolve .env variables
	validateArgs := append(composeBaseArgs(name), "config", "--quiet")
	cmd := exec.Command("docker", validateArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("invalid compose file: %s", strings.TrimSpace(string(output)))
	}

	return nil
}

// SaveEnv writes the .env file for a stack.
func (sm *StackManager) SaveEnv(name, content string) error {
	if err := ValidateStackName(name); err != nil {
		return err
	}
	return storage.WriteFile(stackEnvFile(name), []byte(content))
}

// UpdateMounts updates the required mounts for a stack.
func (sm *StackManager) UpdateMounts(name string, mounts []string) error {
	if err := ValidateStackName(name); err != nil {
		return err
	}

	cfg, err := LoadStacksConfig()
	if err != nil {
		return err
	}

	stack, ok := cfg.Stacks[name]
	if !ok {
		// Auto-create entry if stack dir exists
		if !storage.Exists(stackDir(name)) {
			return fmt.Errorf("stack %q does not exist", name)
		}
		stack = Stack{
			Name:      name,
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
		}
	}
	stack.RequiredMounts = mounts
	cfg.Stacks[name] = stack
	return SaveStacksConfig(cfg)
}

// RunForStack executes a docker compose command for a specific stack,
// streaming output line by line through the handler.
func (sm *StackManager) RunForStack(ctx context.Context, name string, handler OutputHandler, args ...string) error {
	if !atomic.CompareAndSwapInt32(&sm.opRunning, 0, 1) {
		return fmt.Errorf("another stack operation is already in progress")
	}
	defer atomic.StoreInt32(&sm.opRunning, 0)

	return sm.runForStackNoLock(ctx, name, handler, args...)
}

// runForStackNoLock runs compose commands without the atomic lock.
func (sm *StackManager) runForStackNoLock(ctx context.Context, name string, handler OutputHandler, args ...string) error {
	fullArgs := append(composeBaseArgs(name), args...)
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

// Start runs docker compose up -d for a stack.
func (sm *StackManager) Start(ctx context.Context, name string, handler OutputHandler) error {
	return sm.RunForStack(ctx, name, handler, "up", "-d", "--remove-orphans")
}

// Stop runs docker compose down for a stack.
func (sm *StackManager) Stop(ctx context.Context, name string, handler OutputHandler) error {
	return sm.RunForStack(ctx, name, handler, "down", "--remove-orphans")
}

// Restart stops then starts a stack.
func (sm *StackManager) Restart(ctx context.Context, name string, handler OutputHandler) error {
	if !atomic.CompareAndSwapInt32(&sm.opRunning, 0, 1) {
		return fmt.Errorf("another stack operation is already in progress")
	}
	defer atomic.StoreInt32(&sm.opRunning, 0)

	if err := sm.runForStackNoLock(ctx, name, handler, "down", "--remove-orphans"); err != nil {
		return err
	}
	return sm.runForStackNoLock(ctx, name, handler, "up", "-d", "--remove-orphans")
}

// Pull pulls latest images for a stack.
func (sm *StackManager) Pull(ctx context.Context, name string, handler OutputHandler) error {
	return sm.RunForStack(ctx, name, handler, "pull")
}

// Deploy runs a phased deploy: pull then up.
func (sm *StackManager) Deploy(ctx context.Context, name string, handler OutputHandler) error {
	if !atomic.CompareAndSwapInt32(&sm.opRunning, 0, 1) {
		return fmt.Errorf("another stack operation is already in progress")
	}
	defer atomic.StoreInt32(&sm.opRunning, 0)

	// Phase 1: Pull
	if handler != nil {
		handler("--- Pulling images ---")
	}
	pullErr := sm.runForStackNoLock(ctx, name, handler, "pull")
	if pullErr != nil && handler != nil {
		handler(fmt.Sprintf("Pull warning: %v (continuing with available images)", pullErr))
	}

	// Phase 2: Up
	if handler != nil {
		handler("--- Starting containers ---")
	}
	return sm.runForStackNoLock(ctx, name, handler, "up", "-d", "--remove-orphans")
}

// RestartContainer restarts a single container by name.
func (sm *StackManager) RestartContainer(ctx context.Context, containerName string) error {
	cmd := exec.CommandContext(ctx, "docker", "restart", containerName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("restart container: %s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}

// StreamLogs streams compose logs for a stack via a writer.
func (sm *StackManager) StreamLogs(ctx context.Context, name string, container string, w io.Writer) error {
	args := append(composeBaseArgs(name), "logs", "--follow", "--no-color", "--tail", "100")
	if container != "" && container != "all" {
		args = append(args, container)
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = w
	cmd.Stderr = w
	return cmd.Run()
}
