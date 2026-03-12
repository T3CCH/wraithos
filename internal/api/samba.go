package api

import (
	"net/http"
	"strings"

	"github.com/wraithos/wraith-ui/internal/system"
)

// handleSambaList returns all configured Samba mounts.
func (s *Server) handleSambaList(w http.ResponseWriter, r *http.Request) {
	mounts, err := s.Samba.ListMounts()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if mounts == nil {
		mounts = []system.SambaMount{}
	}
	writeOK(w, map[string]interface{}{"mounts": mounts})
}

// handleSambaAdd creates a new Samba mount configuration.
func (s *Server) handleSambaAdd(w http.ResponseWriter, r *http.Request) {
	var mount system.SambaMount
	if err := decodeJSONLenient(r, &mount); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	result, err := s.Samba.AddMount(mount)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.Logs.Info("samba", "added mount %s (//%s/%s)", result.ID, mount.Server, mount.Share)
	writeJSON(w, http.StatusCreated, result)
}

// handleSambaByID routes DELETE /api/samba/mounts/{id},
// POST /api/samba/mounts/{id}/mount, and POST /api/samba/mounts/{id}/unmount.
func (s *Server) handleSambaByID(w http.ResponseWriter, r *http.Request) {
	// Parse the ID and optional action from the URL path.
	// Path format: /api/samba/mounts/{id}[/mount|/unmount]
	path := strings.TrimPrefix(r.URL.Path, "/api/samba/mounts/")
	parts := strings.SplitN(path, "/", 2)

	if len(parts) == 0 || parts[0] == "" {
		writeError(w, http.StatusBadRequest, "mount ID required")
		return
	}

	id := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	switch {
	case r.Method == http.MethodDelete && action == "":
		s.handleSambaRemove(w, r, id)
	case r.Method == http.MethodPost && action == "mount":
		s.handleSambaMount(w, r, id)
	case r.Method == http.MethodPost && action == "unmount":
		s.handleSambaUnmount(w, r, id)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleMountsByID routes requests under /api/mounts/{id} (frontend path).
func (s *Server) handleMountsByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/mounts/")
	parts := strings.SplitN(path, "/", 2)

	if len(parts) == 0 || parts[0] == "" {
		writeError(w, http.StatusBadRequest, "mount ID required")
		return
	}

	id := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	switch {
	case r.Method == http.MethodDelete && action == "":
		s.handleSambaRemove(w, r, id)
	case r.Method == http.MethodPost && action == "mount":
		s.handleSambaMount(w, r, id)
	case r.Method == http.MethodPost && action == "unmount":
		s.handleSambaUnmount(w, r, id)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleSambaRemove(w http.ResponseWriter, r *http.Request, id string) {
	if err := s.Samba.RemoveMount(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	s.Logs.Info("samba", "removed mount %s", id)
	writeOK(w, map[string]string{"status": "removed"})
}

func (s *Server) handleSambaMount(w http.ResponseWriter, r *http.Request, id string) {
	if err := s.Samba.Mount(id); err != nil {
		s.Logs.Error("samba", "mount %s failed: %v", id, err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.Logs.Info("samba", "mounted %s", id)
	writeOK(w, map[string]string{"status": "mounted"})
}

func (s *Server) handleSambaUnmount(w http.ResponseWriter, r *http.Request, id string) {
	if err := s.Samba.Unmount(id); err != nil {
		s.Logs.Error("samba", "unmount %s failed: %v", id, err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.Logs.Info("samba", "unmounted %s", id)
	writeOK(w, map[string]string{"status": "unmounted"})
}
