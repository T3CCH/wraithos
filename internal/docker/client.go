// Package docker provides Docker API client operations and compose management.
package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/system"
	"github.com/docker/docker/client"
)

// ContainerInfo holds summary information about a running container.
type ContainerInfo struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Image   string   `json:"image"`
	State   string   `json:"state"`
	Status  string   `json:"status"`
	Created string   `json:"created"`
	Ports   []string `json:"ports"`
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

		ports := formatPorts(ctr.Ports)

		result = append(result, ContainerInfo{
			ID:      ctr.ID[:12],
			Name:    name,
			Image:   ctr.Image,
			State:   ctr.State,
			Status:  ctr.Status,
			Created: time.Unix(ctr.Created, 0).Format(time.RFC3339),
			Ports:   ports,
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

// formatPorts converts Docker port bindings into human-readable strings like "0.0.0.0:8080->80/tcp".
func formatPorts(ports []types.Port) []string {
	result := make([]string, 0, len(ports))
	seen := make(map[string]bool)
	for _, p := range ports {
		var s string
		if p.PublicPort > 0 {
			s = fmt.Sprintf("%s:%d->%d/%s", p.IP, p.PublicPort, p.PrivatePort, p.Type)
		} else {
			s = fmt.Sprintf("%d/%s", p.PrivatePort, p.Type)
		}
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	sort.Strings(result)
	return result
}

// NetworkInfo holds summary information about a Docker network.
type NetworkInfo struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Driver     string            `json:"driver"`
	Scope      string            `json:"scope"`
	Internal   bool              `json:"internal"`
	Containers int               `json:"containers"`
	Subnet     string            `json:"subnet,omitempty"`
	Gateway    string            `json:"gateway,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
}

// ListNetworks returns all Docker networks.
func (c *Client) ListNetworks(ctx context.Context) ([]NetworkInfo, error) {
	networks, err := c.cli.NetworkList(ctx, network.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list networks: %w", err)
	}

	result := make([]NetworkInfo, 0, len(networks))
	for _, n := range networks {
		info := NetworkInfo{
			ID:         n.ID[:12],
			Name:       n.Name,
			Driver:     n.Driver,
			Scope:      n.Scope,
			Internal:   n.Internal,
			Containers: len(n.Containers),
			Labels:     n.Labels,
		}
		if len(n.IPAM.Config) > 0 {
			info.Subnet = n.IPAM.Config[0].Subnet
			info.Gateway = n.IPAM.Config[0].Gateway
		}
		result = append(result, info)
	}

	return result, nil
}

// CreateNetworkOptions holds parameters for creating a Docker network.
type CreateNetworkOptions struct {
	Name       string            `json:"name"`
	Driver     string            `json:"driver"`
	Subnet     string            `json:"subnet,omitempty"`
	Gateway    string            `json:"gateway,omitempty"`
	IPRange    string            `json:"ipRange,omitempty"`
	Internal   bool              `json:"internal"`
	ParentIface string           `json:"parent,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
}

// CreateNetwork creates a new Docker network.
func (c *Client) CreateNetwork(ctx context.Context, opts CreateNetworkOptions) (string, error) {
	createOpts := network.CreateOptions{
		Driver:   opts.Driver,
		Internal: opts.Internal,
		Labels:   opts.Labels,
		Options:  map[string]string{},
	}

	// macvlan/ipvlan require parent interface
	if opts.ParentIface != "" {
		createOpts.Options["parent"] = opts.ParentIface
	}

	if opts.Subnet != "" {
		ipamConfig := network.IPAMConfig{
			Subnet: opts.Subnet,
		}
		if opts.Gateway != "" {
			ipamConfig.Gateway = opts.Gateway
		}
		if opts.IPRange != "" {
			ipamConfig.IPRange = opts.IPRange
		}
		createOpts.IPAM = &network.IPAM{
			Config: []network.IPAMConfig{ipamConfig},
		}
	}

	resp, err := c.cli.NetworkCreate(ctx, opts.Name, createOpts)
	if err != nil {
		return "", fmt.Errorf("create network: %w", err)
	}
	return resp.ID, nil
}

// RemoveNetwork deletes a Docker network by ID or name.
func (c *Client) RemoveNetwork(ctx context.Context, networkID string) error {
	if err := c.cli.NetworkRemove(ctx, networkID); err != nil {
		return fmt.Errorf("remove network: %w", err)
	}
	return nil
}

// ImageInfo holds summary information about a Docker image.
type ImageInfo struct {
	ID      string   `json:"id"`
	Tags    []string `json:"tags"`
	Size    int64    `json:"size"`
	Created string   `json:"created"`
	InUse   bool     `json:"inUse"`
}

// ListImages returns all Docker images with usage info.
// Images currently used by any container (running or stopped) are marked as in-use.
func (c *Client) ListImages(ctx context.Context) ([]ImageInfo, uint64, error) {
	images, err := c.cli.ImageList(ctx, image.ListOptions{All: false})
	if err != nil {
		return nil, 0, fmt.Errorf("list images: %w", err)
	}

	// Build a set of image IDs currently in use by containers
	containers, err := c.cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, 0, fmt.Errorf("list containers for image usage: %w", err)
	}
	usedImages := make(map[string]bool)
	for _, ctr := range containers {
		usedImages[ctr.ImageID] = true
	}

	result := make([]ImageInfo, 0, len(images))
	var reclaimable uint64
	for _, img := range images {
		tags := img.RepoTags
		if tags == nil {
			tags = []string{"<none>:<none>"}
		}
		inUse := usedImages[img.ID]
		if !inUse {
			reclaimable += uint64(img.Size)
		}
		result = append(result, ImageInfo{
			ID:      img.ID,
			Tags:    tags,
			Size:    img.Size,
			Created: time.Unix(img.Created, 0).Format(time.RFC3339),
			InUse:   inUse,
		})
	}

	// Sort: in-use first, then by size descending
	sort.Slice(result, func(i, j int) bool {
		if result[i].InUse != result[j].InUse {
			return result[i].InUse
		}
		return result[i].Size > result[j].Size
	})

	return result, reclaimable, nil
}

// PruneResult holds the result of a Docker system prune.
type PruneResult struct {
	ContainersDeleted []string `json:"containersDeleted"`
	ImagesDeleted     int      `json:"imagesDeleted"`
	NetworksDeleted   []string `json:"networksDeleted"`
	SpaceReclaimed    uint64   `json:"spaceReclaimed"`
}

// SystemPrune removes all unused containers, networks, images (including unreferenced).
func (c *Client) SystemPrune(ctx context.Context) (*PruneResult, error) {
	result := &PruneResult{}

	emptyFilter := filters.NewArgs()

	// Prune stopped containers
	containerReport, err := c.cli.ContainersPrune(ctx, emptyFilter)
	if err != nil {
		return nil, fmt.Errorf("prune containers: %w", err)
	}
	result.ContainersDeleted = containerReport.ContainersDeleted
	result.SpaceReclaimed += containerReport.SpaceReclaimed

	// Prune unused networks
	networkReport, err := c.cli.NetworksPrune(ctx, emptyFilter)
	if err != nil {
		return nil, fmt.Errorf("prune networks: %w", err)
	}
	result.NetworksDeleted = networkReport.NetworksDeleted

	// Prune all unused images (not just dangling, equivalent to -a)
	// Use dangling=false filter to remove all unused images (like docker system prune -a)
	allImagesFilter := filters.NewArgs(filters.Arg("dangling", "false"))
	imageReport, err := c.cli.ImagesPrune(ctx, allImagesFilter)
	if err != nil {
		return nil, fmt.Errorf("prune images: %w", err)
	}
	result.ImagesDeleted = len(imageReport.ImagesDeleted)
	result.SpaceReclaimed += imageReport.SpaceReclaimed

	// Prune build cache
	buildReport, err := c.cli.BuildCachePrune(ctx, types.BuildCachePruneOptions{All: true})
	if err != nil {
		// Build cache prune may not be available, don't fail
	} else {
		result.SpaceReclaimed += buildReport.SpaceReclaimed
	}

	if result.ContainersDeleted == nil {
		result.ContainersDeleted = []string{}
	}
	if result.NetworksDeleted == nil {
		result.NetworksDeleted = []string{}
	}

	return result, nil
}
