// Package docker provides Docker API client operations and compose management.
package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/system"
	"github.com/docker/docker/client"
)

// ContainerInfo holds summary information about a running container.
type ContainerInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Image   string `json:"image"`
	State   string `json:"state"`
	Status  string `json:"status"`
	Created string `json:"created"`
}

// Client wraps the Docker SDK client.
type Client struct {
	cli *client.Client
}

// NewClient creates a new Docker API client using the default socket.
func NewClient() (*Client, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("create docker client: %w", err)
	}
	return &Client{cli: cli}, nil
}

// Close releases the Docker client resources.
func (c *Client) Close() error {
	return c.cli.Close()
}

// Ping checks connectivity to the Docker daemon.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.cli.Ping(ctx)
	return err
}

// Info returns Docker system information.
func (c *Client) Info(ctx context.Context) (*system.Info, error) {
	info, err := c.cli.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("docker info: %w", err)
	}
	return &info, nil
}

// ListContainers returns all containers (running and stopped).
func (c *Client) ListContainers(ctx context.Context) ([]ContainerInfo, error) {
	containers, err := c.cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}

	result := make([]ContainerInfo, 0, len(containers))
	for _, ctr := range containers {
		name := ""
		if len(ctr.Names) > 0 {
			name = ctr.Names[0]
			if len(name) > 0 && name[0] == '/' {
				name = name[1:]
			}
		}

		result = append(result, ContainerInfo{
			ID:      ctr.ID[:12],
			Name:    name,
			Image:   ctr.Image,
			State:   ctr.State,
			Status:  ctr.Status,
			Created: time.Unix(ctr.Created, 0).Format(time.RFC3339),
		})
	}

	return result, nil
}

// ContainerStats returns basic resource usage for a container.
type ContainerStats struct {
	CPUPercent float64 `json:"cpu_percent"`
	MemUsageMB float64 `json:"mem_usage_mb"`
	MemLimitMB float64 `json:"mem_limit_mb"`
}

// GetContainerStats retrieves resource usage for a specific container.
func (c *Client) GetContainerStats(ctx context.Context, containerID string) (*ContainerStats, error) {
	resp, err := c.cli.ContainerStatsOneShot(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("container stats: %w", err)
	}
	defer resp.Body.Close()

	var statsJSON container.StatsResponse
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&statsJSON); err != nil {
		if err == io.EOF {
			return &ContainerStats{}, nil
		}
		return nil, fmt.Errorf("decode stats: %w", err)
	}

	cpuPercent := calculateCPUPercent(&statsJSON)
	memUsage := float64(statsJSON.MemoryStats.Usage) / (1024 * 1024)
	memLimit := float64(statsJSON.MemoryStats.Limit) / (1024 * 1024)

	return &ContainerStats{
		CPUPercent: cpuPercent,
		MemUsageMB: memUsage,
		MemLimitMB: memLimit,
	}, nil
}

func calculateCPUPercent(stats *container.StatsResponse) float64 {
	cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage - stats.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(stats.CPUStats.SystemUsage - stats.PreCPUStats.SystemUsage)

	if systemDelta > 0 && cpuDelta > 0 {
		cpuCount := float64(stats.CPUStats.OnlineCPUs)
		if cpuCount == 0 {
			cpuCount = float64(len(stats.CPUStats.CPUUsage.PercpuUsage))
		}
		return (cpuDelta / systemDelta) * cpuCount * 100.0
	}
	return 0
}
