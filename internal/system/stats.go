// Package system provides OS-level stats, network management, and log collection.
package system

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// CPUStats represents CPU utilization.
type CPUStats struct {
	UsagePercent float64 `json:"usage_percent"`
}

// MemStats represents memory utilization.
type MemStats struct {
	TotalMB uint64 `json:"total_mb"`
	UsedMB  uint64 `json:"used_mb"`
	FreeMB  uint64 `json:"free_mb"`
	Percent float64 `json:"percent"`
}

// DiskStats represents disk usage for a mount point.
type DiskStats struct {
	Path    string  `json:"path"`
	TotalMB uint64  `json:"total_mb"`
	UsedMB  uint64  `json:"used_mb"`
	FreeMB  uint64  `json:"free_mb"`
	Percent float64 `json:"percent"`
}

// GetCPUStats reads CPU utilization from /proc/stat.
// Takes two samples 500ms apart to calculate usage.
func GetCPUStats() (*CPUStats, error) {
	idle1, total1, err := readCPUSample()
	if err != nil {
		return nil, err
	}

	time.Sleep(500 * time.Millisecond)

	idle2, total2, err := readCPUSample()
	if err != nil {
		return nil, err
	}

	idleDelta := float64(idle2 - idle1)
	totalDelta := float64(total2 - total1)

	if totalDelta == 0 {
		return &CPUStats{UsagePercent: 0}, nil
	}

	usage := (1.0 - idleDelta/totalDelta) * 100.0
	return &CPUStats{UsagePercent: usage}, nil
}

func readCPUSample() (idle, total uint64, err error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0, 0, fmt.Errorf("open /proc/stat: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 5 {
			return 0, 0, fmt.Errorf("unexpected /proc/stat format")
		}

		var values []uint64
		for _, field := range fields[1:] {
			v, err := strconv.ParseUint(field, 10, 64)
			if err != nil {
				return 0, 0, fmt.Errorf("parse cpu field: %w", err)
			}
			values = append(values, v)
			total += v
		}

		// idle is the 4th field (index 3)
		if len(values) > 3 {
			idle = values[3]
		}
		// iowait is the 5th field (index 4), count as idle
		if len(values) > 4 {
			idle += values[4]
		}

		return idle, total, nil
	}

	return 0, 0, fmt.Errorf("cpu line not found in /proc/stat")
}

// GetMemStats reads memory info from /proc/meminfo.
func GetMemStats() (*MemStats, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return nil, fmt.Errorf("open /proc/meminfo: %w", err)
	}
	defer f.Close()

	info := make(map[string]uint64)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		valStr := strings.TrimSpace(parts[1])
		valStr = strings.TrimSuffix(valStr, " kB")
		valStr = strings.TrimSpace(valStr)

		v, err := strconv.ParseUint(valStr, 10, 64)
		if err != nil {
			continue
		}
		info[key] = v
	}

	totalKB := info["MemTotal"]
	freeKB := info["MemFree"]
	buffersKB := info["Buffers"]
	cachedKB := info["Cached"]
	sreclaimKB := info["SReclaimable"]

	// Available = Free + Buffers + Cached + SReclaimable
	availKB := freeKB + buffersKB + cachedKB + sreclaimKB
	usedKB := totalKB - availKB

	totalMB := totalKB / 1024
	usedMB := usedKB / 1024
	freeMB := availKB / 1024

	var percent float64
	if totalKB > 0 {
		percent = float64(usedKB) / float64(totalKB) * 100.0
	}

	return &MemStats{
		TotalMB: totalMB,
		UsedMB:  usedMB,
		FreeMB:  freeMB,
		Percent: percent,
	}, nil
}

// GetDiskStats returns disk usage for the given mount point using statvfs.
func GetDiskStats(path string) (*DiskStats, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return nil, fmt.Errorf("statfs %s: %w", path, err)
	}

	totalBytes := stat.Blocks * uint64(stat.Bsize)
	freeBytes := stat.Bfree * uint64(stat.Bsize)
	usedBytes := totalBytes - freeBytes

	totalMB := totalBytes / (1024 * 1024)
	usedMB := usedBytes / (1024 * 1024)
	freeMB := freeBytes / (1024 * 1024)

	var percent float64
	if totalBytes > 0 {
		percent = float64(usedBytes) / float64(totalBytes) * 100.0
	}

	return &DiskStats{
		Path:    path,
		TotalMB: totalMB,
		UsedMB:  usedMB,
		FreeMB:  freeMB,
		Percent: percent,
	}, nil
}

// SystemInfo contains OS-level information.
type SystemInfo struct {
	Hostname string `json:"hostname"`
	Kernel   string `json:"kernel"`
	Uptime   string `json:"uptime"`
	Version  string `json:"version"`
}

// GetSystemInfo returns hostname, kernel version, and uptime.
func GetSystemInfo(version string) (*SystemInfo, error) {
	var uts syscall.Utsname
	if err := syscall.Uname(&uts); err != nil {
		return nil, fmt.Errorf("uname: %w", err)
	}

	hostname := charsToString(uts.Nodename[:])
	kernel := charsToString(uts.Release[:])

	// Read uptime from /proc/uptime
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return nil, fmt.Errorf("read /proc/uptime: %w", err)
	}

	fields := strings.Fields(string(data))
	uptimeSecs := 0.0
	if len(fields) > 0 {
		uptimeSecs, _ = strconv.ParseFloat(fields[0], 64)
	}
	uptime := formatUptime(uptimeSecs)

	return &SystemInfo{
		Hostname: hostname,
		Kernel:   kernel,
		Uptime:   uptime,
		Version:  version,
	}, nil
}

// GetArch returns the system architecture string.
func GetArch() string {
	var uts syscall.Utsname
	if err := syscall.Uname(&uts); err != nil {
		return "unknown"
	}
	return charsToString(uts.Machine[:])
}

func charsToString(arr []int8) string {
	b := make([]byte, 0, len(arr))
	for _, c := range arr {
		if c == 0 {
			break
		}
		b = append(b, byte(c))
	}
	return string(b)
}

func formatUptime(seconds float64) string {
	d := int(seconds)
	days := d / 86400
	hours := (d % 86400) / 3600
	mins := (d % 3600) / 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, mins)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, mins)
	}
	return fmt.Sprintf("%dm", mins)
}
