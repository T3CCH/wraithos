package api

import (
	"net/http"
	"strings"

	"github.com/wraithos/wraith-ui/internal/docker"
)

// handleDockerNetworkList returns all Docker networks.
func (s *Server) handleDockerNetworkList(w http.ResponseWriter, r *http.Request) {
	if s.Docker == nil {
		writeError(w, http.StatusServiceUnavailable, "Docker is not available")
		return
	}

	networks, err := s.Docker.ListNetworks(r.Context())
	if err != nil {
		s.Logs.Warn("docker-networks", "failed to list networks: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list networks: "+err.Error())
		return
	}

	writeOK(w, map[string]interface{}{"networks": networks})
}

// handleDockerNetworkCreate creates a new Docker network.
func (s *Server) handleDockerNetworkCreate(w http.ResponseWriter, r *http.Request) {
	if s.Docker == nil {
		writeError(w, http.StatusServiceUnavailable, "Docker is not available")
		return
	}

	var req docker.CreateNetworkOptions
	if err := decodeJSONLenient(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate name
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "network name is required")
		return
	}

	// Default driver to bridge
	if req.Driver == "" {
		req.Driver = "bridge"
	}

	// Validate driver
	validDrivers := map[string]bool{
		"bridge": true, "host": true, "overlay": true,
		"macvlan": true, "ipvlan": true, "none": true,
	}
	if !validDrivers[req.Driver] {
		writeError(w, http.StatusBadRequest, "unsupported network driver: "+req.Driver)
		return
	}

	id, err := s.Docker.CreateNetwork(r.Context(), req)
	if err != nil {
		s.Logs.Warn("docker-networks", "failed to create network %q: %v", req.Name, err)
		writeError(w, http.StatusInternalServerError, "failed to create network: "+err.Error())
		return
	}

	s.Logs.Info("docker-networks", "created network %q (driver=%s, id=%s)", req.Name, req.Driver, id[:12])
	writeOK(w, map[string]interface{}{
		"id":   id[:12],
		"name": req.Name,
	})
}

// handleDockerNetworkDelete deletes a Docker network.
func (s *Server) handleDockerNetworkDelete(w http.ResponseWriter, r *http.Request) {
	if s.Docker == nil {
		writeError(w, http.StatusServiceUnavailable, "Docker is not available")
		return
	}

	var req struct {
		ID string `json:"id"`
	}
	if err := decodeJSONLenient(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.ID = strings.TrimSpace(req.ID)
	if req.ID == "" {
		writeError(w, http.StatusBadRequest, "network ID is required")
		return
	}

	if err := s.Docker.RemoveNetwork(r.Context(), req.ID); err != nil {
		s.Logs.Warn("docker-networks", "failed to remove network %s: %v", req.ID, err)
		writeError(w, http.StatusInternalServerError, "failed to remove network: "+err.Error())
		return
	}

	s.Logs.Info("docker-networks", "removed network %s", req.ID)
	writeOK(w, map[string]string{"status": "removed"})
}
