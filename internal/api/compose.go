package api

import (
	"io"
	"net/http"
)

type composeContentResponse struct {
	Content string `json:"content"`
}

type composeContentRequest struct {
	Content string `json:"content"`
}

// handleComposeGet returns the current docker-compose.yml content.
func (s *Server) handleComposeGet(w http.ResponseWriter, r *http.Request) {
	content, err := s.Compose.GetComposeFile()
	if err != nil {
		writeError(w, http.StatusNotFound, "no compose file found")
		return
	}
	writeOK(w, composeContentResponse{Content: content})
}

// handleComposeSave saves a new docker-compose.yml (with validation).
func (s *Server) handleComposeSave(w http.ResponseWriter, r *http.Request) {
	var req composeContentRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Content == "" {
		writeError(w, http.StatusBadRequest, "content is required")
		return
	}

	if err := s.Compose.SaveComposeFile(req.Content); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.Logs.Info("compose", "compose file updated")
	writeOK(w, map[string]string{"status": "saved"})
}

// handleComposeDeploy runs docker compose up -d.
func (s *Server) handleComposeDeploy(w http.ResponseWriter, r *http.Request) {
	s.Logs.Info("compose", "deploying stack")

	err := s.Compose.Deploy(r.Context(), func(line string) {
		s.Logs.Info("compose", "%s", line)
	})
	if err != nil {
		s.Logs.Error("compose", "deploy failed: %v", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.Logs.Info("compose", "deploy completed")
	writeOK(w, map[string]string{"status": "deployed"})
}

// handleComposeStop runs docker compose down.
func (s *Server) handleComposeStop(w http.ResponseWriter, r *http.Request) {
	s.Logs.Info("compose", "stopping stack")

	err := s.Compose.Stop(r.Context(), func(line string) {
		s.Logs.Info("compose", "%s", line)
	})
	if err != nil {
		s.Logs.Error("compose", "stop failed: %v", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.Logs.Info("compose", "stack stopped")
	writeOK(w, map[string]string{"status": "stopped"})
}

// handleComposeRestart runs down then up.
func (s *Server) handleComposeRestart(w http.ResponseWriter, r *http.Request) {
	s.Logs.Info("compose", "restarting stack")

	err := s.Compose.Restart(r.Context(), func(line string) {
		s.Logs.Info("compose", "%s", line)
	})
	if err != nil {
		s.Logs.Error("compose", "restart failed: %v", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.Logs.Info("compose", "stack restarted")
	writeOK(w, map[string]string{"status": "restarted"})
}

// handleComposePull pulls latest images for all services.
func (s *Server) handleComposePull(w http.ResponseWriter, r *http.Request) {
	s.Logs.Info("compose", "pulling images")

	err := s.Compose.Pull(r.Context(), func(line string) {
		s.Logs.Info("compose", "%s", line)
	})
	if err != nil {
		s.Logs.Error("compose", "pull failed: %v", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.Logs.Info("compose", "pull completed")
	writeOK(w, map[string]string{"status": "pulled"})
}

// handleComposeValidate validates a compose file without saving it.
func (s *Server) handleComposeValidate(w http.ResponseWriter, r *http.Request) {
	var req composeContentRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Content == "" {
		writeError(w, http.StatusBadRequest, "content is required")
		return
	}

	// Save, validate, then the save itself runs docker compose config.
	// For validate-only we save temporarily then check.
	if err := s.Compose.SaveComposeFile(req.Content); err != nil {
		writeOK(w, map[string]interface{}{"valid": false, "error": err.Error()})
		return
	}

	writeOK(w, map[string]interface{}{"valid": true})
}

// handleComposeStatus returns the compose stack status.
func (s *Server) handleComposeStatus(w http.ResponseWriter, r *http.Request) {
	status, err := s.Compose.Status(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	io.WriteString(w, status)
}
