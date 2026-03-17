package api

import (
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/wraithos/wraith-ui/internal/docker"
	"github.com/wraithos/wraith-ui/internal/setup"
	"github.com/wraithos/wraith-ui/internal/storage"
	"github.com/wraithos/wraith-ui/internal/system"
)

// systemStats matches what the frontend JS expects in d.system.
type systemStats struct {
	CPUPercent      float64 `json:"cpuPercent"`
	RAMUsed         uint64  `json:"ramUsed"`
	RAMTotal        uint64  `json:"ramTotal"`
	ConfigDiskUsed  uint64  `json:"configDiskUsed"`
	ConfigDiskTotal uint64  `json:"configDiskTotal"`
	CacheDiskUsed   uint64  `json:"cacheDiskUsed"`
	CacheDiskTotal  uint64  `json:"cacheDiskTotal"`
	ConfigDiskType  string  `json:"configDiskType"`
	CacheDiskType   string  `json:"cacheDiskType"`
	Uptime          float64 `json:"uptime"`
}

// networkInfo matches what the frontend JS expects in d.network.
type networkInfo struct {
	IP        string   `json:"ip"`
	Gateway   string   `json:"gateway"`
	DNS       []string `json:"dns"`
	DHCP      bool     `json:"dhcp"`
	Interface string   `json:"interface"`
}

// containerInfo matches what the frontend JS expects in d.containers[].
type containerInfo struct {
	Name   string   `json:"name"`
	Image  string   `json:"image"`
	State  string   `json:"state"`
	Uptime float64  `json:"uptime"`
	Ports  []string `json:"ports"`
	Stack  string   `json:"stack,omitempty"`
}

type dashboardResponse struct {
	System      systemStats      `json:"system"`
	Network     networkInfo      `json:"network"`
	Containers  []containerInfo  `json:"containers"`
	Images      []docker.ImageInfo `json:"images"`
	Reclaimable uint64           `json:"reclaimable"`
}

// handleDashboard returns system stats, network info, and container list
// in the format the frontend expects from GET /api/system/status.
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	resp := dashboardResponse{}

	// CPU stats (takes ~500ms due to sampling)
	cpu, err := system.GetCPUStats()
	if err != nil {
		s.Logs.Warn("dashboard", "failed to get CPU stats: %v", err)
	} else {
		resp.System.CPUPercent = cpu.UsagePercent
	}

	// Memory stats (convert MB to bytes for frontend)
	mem, err := system.GetMemStats()
	if err != nil {
		s.Logs.Warn("dashboard", "failed to get memory stats: %v", err)
	} else {
		resp.System.RAMUsed = mem.UsedMB * 1024 * 1024
		resp.System.RAMTotal = mem.TotalMB * 1024 * 1024
	}

	// Config disk usage (convert MB to bytes for frontend)
	configDisk, err := system.GetDiskStats(storage.ConfigBase)
	if err != nil {
		s.Logs.Warn("dashboard", "failed to get config disk stats: %v", err)
	} else {
		resp.System.ConfigDiskUsed = configDisk.UsedMB * 1024 * 1024
		resp.System.ConfigDiskTotal = configDisk.TotalMB * 1024 * 1024
	}

	// Cache disk usage (convert MB to bytes for frontend)
	cacheDisk, err := system.GetDiskStats(storage.CacheDisk)
	if err != nil {
		s.Logs.Warn("dashboard", "failed to get cache disk stats: %v", err)
	} else {
		resp.System.CacheDiskUsed = cacheDisk.UsedMB * 1024 * 1024
		resp.System.CacheDiskTotal = cacheDisk.TotalMB * 1024 * 1024
	}

	// Disk persistence type (tmpfs vs ext4)
	configStatus, cacheStatus := setup.GetDiskStatus()
	resp.System.ConfigDiskType = configStatus.Type
	resp.System.CacheDiskType = cacheStatus.Type
	if resp.System.ConfigDiskType == "" {
		resp.System.ConfigDiskType = "unknown"
	}
	if resp.System.CacheDiskType == "" {
		resp.System.CacheDiskType = "unknown"
	}

	// Uptime in seconds
	sysInfo, err := system.GetSystemInfo(s.Version)
	if err != nil {
		s.Logs.Warn("dashboard", "failed to get system info: %v", err)
	} else {
		resp.System.Uptime = parseUptimeSeconds(sysInfo.Uptime)
	}

	// Network status
	netStatus, err := system.GetNetworkStatus()
	if err != nil {
		s.Logs.Warn("dashboard", "failed to get network status: %v", err)
	} else {
		resp.Network.IP = netStatus.Address
		resp.Network.Gateway = netStatus.Gateway
		resp.Network.Interface = netStatus.Interface
		resp.Network.DHCP = netStatus.Mode == "dhcp"
		if netStatus.DNS != "" {
			for _, d := range strings.Split(netStatus.DNS, ",") {
				d = strings.TrimSpace(d)
				if d != "" {
					resp.Network.DNS = append(resp.Network.DNS, d)
				}
			}
		}
		if resp.Network.DNS == nil {
			resp.Network.DNS = []string{}
		}
	}

	// Container list
	if s.Docker != nil {
		containers, err := s.Docker.ListContainers(r.Context())
		if err != nil {
			s.Logs.Warn("dashboard", "failed to list containers: %v", err)
		} else {
			resp.Containers = make([]containerInfo, 0, len(containers))
			for _, c := range containers {
				ports := c.Ports
				if ports == nil {
					ports = []string{}
				}
				resp.Containers = append(resp.Containers, containerInfo{
					Name:  c.Name,
					Image: c.Image,
					State: c.State,
					Ports: ports,
					Stack: c.Stack,
				})
			}
		}
	}
	if resp.Containers == nil {
		resp.Containers = []containerInfo{}
	}

	// Docker images with usage info
	if s.Docker != nil {
		images, reclaimable, err := s.Docker.ListImages(r.Context())
		if err != nil {
			s.Logs.Warn("dashboard", "failed to list images: %v", err)
		} else {
			resp.Images = images
			resp.Reclaimable = reclaimable
		}
	}
	if resp.Images == nil {
		resp.Images = []docker.ImageInfo{}
	}

	writeOK(w, resp)
}

// parseUptimeSeconds reads /proc/uptime and returns seconds as a float.
func parseUptimeSeconds(_ string) float64 {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0
	}
	secs, _ := strconv.ParseFloat(fields[0], 64)
	return secs
}
