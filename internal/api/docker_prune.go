package api

import (
	"net/http"
)

// handleDockerPrune runs a full Docker system prune (containers, images, networks, build cache).
func (s *Server) handleDockerPrune(w http.ResponseWriter, r *http.Request) {
	if s.Docker == nil {
		writeError(w, http.StatusServiceUnavailable, "Docker is not available")
		return
	}

	s.Logs.Info("docker", "system prune requested")

	result, err := s.Docker.SystemPrune(r.Context())
	if err != nil {
		s.Logs.Warn("docker", "system prune failed: %v", err)
		writeError(w, http.StatusInternalServerError, "system prune failed: "+err.Error())
		return
	}

	s.Logs.Info("docker", "system prune complete: %d containers, %d images, %d networks removed, %d bytes reclaimed",
		len(result.ContainersDeleted), result.ImagesDeleted, len(result.NetworksDeleted), result.SpaceReclaimed)

	writeOK(w, result)
}
