package api

import (
	"net/http"
	"strings"

	"github.com/wraithos/wraith-ui/internal/system"
)

// networkGetResponse matches the format the frontend expects.
type networkGetResponse struct {
	IP        string   `json:"ip"`
	Gateway   string   `json:"gateway"`
	DNS       []string `json:"dns"`
	DHCP      bool     `json:"dhcp"`
	Interface string   `json:"interface"`
	Mask      string   `json:"mask"`
}

// handleNetworkGet returns the current network configuration
// in the format the frontend JS expects.
func (s *Server) handleNetworkGet(w http.ResponseWriter, r *http.Request) {
	status, err := system.GetNetworkStatus()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := networkGetResponse{
		IP:        status.Address,
		Gateway:   status.Gateway,
		DHCP:      status.Mode == "dhcp",
		Interface: status.Interface,
	}

	if status.DNS != "" {
		for _, d := range strings.Split(status.DNS, ",") {
			d = strings.TrimSpace(d)
			if d != "" {
				resp.DNS = append(resp.DNS, d)
			}
		}
	}
	if resp.DNS == nil {
		resp.DNS = []string{}
	}

	writeOK(w, resp)
}

// networkSetRequest matches what the frontend JS sends.
type networkSetRequest struct {
	DHCP    bool     `json:"dhcp"`
	IP      string   `json:"ip"`
	Mask    string   `json:"mask"`
	Gateway string   `json:"gateway"`
	DNS     []string `json:"dns"`
}

// handleNetworkSet applies a new network configuration.
func (s *Server) handleNetworkSet(w http.ResponseWriter, r *http.Request) {
	var req networkSetRequest
	if err := decodeJSONLenient(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Convert frontend format to system.NetworkConfig
	cfg := system.NetworkConfig{
		Gateway: req.Gateway,
		DNS:     strings.Join(req.DNS, ","),
	}

	if req.DHCP {
		cfg.Mode = "dhcp"
	} else {
		cfg.Mode = "static"
		// Combine IP and mask into CIDR if mask provided
		cfg.Address = req.IP
		if req.Mask != "" && !strings.Contains(req.IP, "/") {
			cidr := maskToCIDR(req.Mask)
			if cidr > 0 {
				cfg.Address = req.IP + "/" + strings.TrimSpace(string(rune('0'+cidr/10))) + string(rune('0'+cidr%10))
			}
		}
	}

	if cfg.Mode == "static" && cfg.Address == "" {
		writeError(w, http.StatusBadRequest, "IP address is required for static mode")
		return
	}

	if err := system.ApplyNetworkConfig(&cfg); err != nil {
		s.Logs.Error("network", "failed to apply network config: %v", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.Logs.Info("network", "network config updated to %s mode", cfg.Mode)
	writeOK(w, map[string]string{"status": "applied"})
}

// maskToCIDR converts a subnet mask string to CIDR prefix length.
func maskToCIDR(mask string) int {
	parts := strings.Split(mask, ".")
	if len(parts) != 4 {
		return 0
	}
	bits := 0
	for _, p := range parts {
		var n int
		for _, c := range p {
			if c >= '0' && c <= '9' {
				n = n*10 + int(c-'0')
			}
		}
		for n > 0 {
			bits += n & 1
			n >>= 1
		}
	}
	return bits
}
