package system

import (
	"fmt"
	"net"
	"os/exec"
	"strings"

	"github.com/wraithos/wraith-ui/internal/storage"
)

// NetworkConfig represents the desired network configuration.
type NetworkConfig struct {
	Mode    string `json:"mode"`    // "dhcp" or "static"
	Address string `json:"address"` // CIDR notation, e.g. "192.168.1.100/24"
	Gateway string `json:"gateway"`
	DNS     string `json:"dns"` // comma-separated DNS servers
}

// NetworkStatus represents the current network state.
type NetworkStatus struct {
	Interface string `json:"interface"`
	Mode      string `json:"mode"`
	Address   string `json:"address"`
	Gateway   string `json:"gateway"`
	DNS       string `json:"dns"`
}

// LoadNetworkConfig reads the saved network configuration from disk.
// Returns a default DHCP config if the file does not exist or cannot be read.
func LoadNetworkConfig() *NetworkConfig {
	cfg := &NetworkConfig{Mode: "dhcp"}
	if !storage.Exists(storage.NetworkFile()) {
		return cfg
	}
	if err := storage.ReadJSON(storage.NetworkFile(), cfg); err != nil {
		return &NetworkConfig{Mode: "dhcp"}
	}
	// Normalize: empty mode defaults to DHCP
	if cfg.Mode == "" {
		cfg.Mode = "dhcp"
	}
	return cfg
}

// GetNetworkStatus reads the current network configuration from the system.
func GetNetworkStatus() (*NetworkStatus, error) {
	iface, err := findDefaultInterface()
	if err != nil {
		return nil, err
	}

	addr, err := getInterfaceAddress(iface)
	if err != nil {
		addr = "unknown"
	}

	gw, err := getDefaultGateway()
	if err != nil {
		gw = "unknown"
	}

	dns := getResolvers()

	// Determine mode from the saved config file. The saved config is the
	// source of truth because DHCP clients (udhcpc -q) may exit after
	// obtaining a lease, making process-based detection unreliable.
	saved := LoadNetworkConfig()
	mode := saved.Mode

	return &NetworkStatus{
		Interface: iface,
		Mode:      mode,
		Address:   addr,
		Gateway:   gw,
		DNS:       dns,
	}, nil
}

// ApplyNetworkConfig writes the network configuration and applies it.
func ApplyNetworkConfig(cfg *NetworkConfig) error {
	if err := storage.WriteJSON(storage.NetworkFile(), cfg); err != nil {
		return fmt.Errorf("save network config: %w", err)
	}

	iface, err := findDefaultInterface()
	if err != nil {
		return fmt.Errorf("find interface: %w", err)
	}

	if cfg.Mode == "dhcp" {
		return applyDHCP(iface)
	}
	return applyStatic(iface, cfg)
}

func findDefaultInterface() (string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", fmt.Errorf("list interfaces: %w", err)
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		if len(addrs) > 0 {
			return iface.Name, nil
		}
	}

	// Fall back to first non-loopback interface
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		return iface.Name, nil
	}

	return "", fmt.Errorf("no suitable network interface found")
}

func getInterfaceAddress(name string) (string, error) {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return "", err
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return "", err
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP.To4() != nil {
			return addr.String(), nil
		}
	}
	return "", fmt.Errorf("no IPv4 address on %s", name)
}

func getDefaultGateway() (string, error) {
	out, err := exec.Command("ip", "route", "show", "default").Output()
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(out))
	for i, f := range fields {
		if f == "via" && i+1 < len(fields) {
			return fields[i+1], nil
		}
	}
	return "", fmt.Errorf("no default gateway found")
}

func getResolvers() string {
	data, err := storage.ReadFile("/etc/resolv.conf")
	if err != nil {
		return ""
	}
	var servers []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "nameserver ") {
			servers = append(servers, strings.TrimPrefix(line, "nameserver "))
		}
	}
	return strings.Join(servers, ", ")
}

func isDHCPActive(iface string) bool {
	// Check for dhclient or udhcpc process for this interface
	out, err := exec.Command("pgrep", "-a", "dhc").Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), iface)
}

func applyDHCP(iface string) error {
	// Kill any existing static config, start DHCP client
	exec.Command("ip", "addr", "flush", "dev", iface).Run()

	// Prefer udhcpc (Alpine/BusyBox), fall back to dhclient
	if path, err := exec.LookPath("udhcpc"); err == nil {
		return exec.Command(path, "-i", iface, "-q").Run()
	}
	return exec.Command("dhclient", iface).Run()
}

func applyStatic(iface string, cfg *NetworkConfig) error {
	// Kill DHCP client if running (try both dhclient and udhcpc)
	exec.Command("pkill", "-f", fmt.Sprintf("dhclient.*%s", iface)).Run()
	exec.Command("pkill", "-f", fmt.Sprintf("udhcpc.*%s", iface)).Run()

	// Flush existing addresses
	if err := exec.Command("ip", "addr", "flush", "dev", iface).Run(); err != nil {
		return fmt.Errorf("flush addresses: %w", err)
	}

	// Set static address
	if err := exec.Command("ip", "addr", "add", cfg.Address, "dev", iface).Run(); err != nil {
		return fmt.Errorf("set address: %w", err)
	}

	// Set default gateway
	if cfg.Gateway != "" {
		if err := exec.Command("ip", "route", "add", "default", "via", cfg.Gateway).Run(); err != nil {
			return fmt.Errorf("set gateway: %w", err)
		}
	}

	// Set DNS
	if cfg.DNS != "" {
		var lines []string
		for _, dns := range strings.Split(cfg.DNS, ",") {
			dns = strings.TrimSpace(dns)
			if dns != "" {
				lines = append(lines, "nameserver "+dns)
			}
		}
		content := strings.Join(lines, "\n") + "\n"
		if err := storage.WriteFile("/etc/resolv.conf", []byte(content)); err != nil {
			return fmt.Errorf("write resolv.conf: %w", err)
		}
	}

	return nil
}
