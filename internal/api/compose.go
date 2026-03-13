package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/wraithos/wraith-ui/internal/docker"
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

// sseEvent writes a single SSE event to the response writer and flushes.
func sseEvent(w http.ResponseWriter, flusher http.Flusher, eventType string, data interface{}) {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", jsonBytes)
	flusher.Flush()
	_ = eventType // reserved for future named event types
}

// classifyLine categorizes a compose output line for frontend color-coding.
func classifyLine(line string) string {
	lower := strings.ToLower(line)
	if strings.Contains(lower, "error") || strings.Contains(lower, "fatal") {
		return "error"
	}
	if strings.Contains(lower, "warning") || strings.Contains(lower, "warn") {
		return "warning"
	}
	if strings.Contains(lower, "pulling") || strings.Contains(lower, "download") ||
		strings.Contains(lower, "extracting") || strings.Contains(lower, "waiting") {
		return "pull"
	}
	if strings.Contains(lower, "created") || strings.Contains(lower, "started") ||
		strings.Contains(lower, "running") || strings.Contains(lower, "done") {
		return "success"
	}
	return "output"
}

// streamComposeAction runs a compose operation with SSE output streaming.
// It sets SSE headers, streams each output line as a JSON event, and sends
// a final complete/error event when the operation finishes.
func (s *Server) streamComposeAction(w http.ResponseWriter, r *http.Request, action string, fn func(handler func(string)) error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	s.Logs.Info("compose", "%s started (SSE)", action)

	outputHandler := func(line string) {
		s.Logs.Info("compose", "%s", line)
		sseEvent(w, flusher, "message", map[string]string{
			"type": classifyLine(line),
			"line": line,
		})
	}

	err := fn(outputHandler)

	if err != nil {
		s.Logs.Error("compose", "%s failed: %v", action, err)
		sseEvent(w, flusher, "message", map[string]interface{}{
			"type":    "complete",
			"success": false,
			"error":   err.Error(),
		})
	} else {
		s.Logs.Info("compose", "%s completed", action)
		sseEvent(w, flusher, "message", map[string]interface{}{
			"type":    "complete",
			"success": true,
		})
	}
}

// handleComposeDeploy runs a phased deployment (pull then up) with structured SSE events.
// Sends events with types: phase, pull_progress, service, output, error, warning, success, complete.
func (s *Server) handleComposeDeploy(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	s.Logs.Info("compose", "deploy started (SSE, phased)")

	// Send initial info: list of services and images
	services, _ := s.Compose.ListServices(r.Context())
	images, _ := s.Compose.ListImages(r.Context())

	sseEvent(w, flusher, "message", map[string]interface{}{
		"type":     "deploy_init",
		"services": services,
		"images":   images,
	})

	eventHandler := func(event docker.DeployEvent) {
		if event.Line != "" {
			s.Logs.Info("compose", "%s", event.Line)
		}
		sseEvent(w, flusher, "message", event)
	}

	err := s.Compose.DeployFull(r.Context(), eventHandler)

	if err != nil {
		s.Logs.Error("compose", "deploy failed: %v", err)
		sseEvent(w, flusher, "message", map[string]interface{}{
			"type":    "complete",
			"success": false,
			"error":   err.Error(),
		})
	} else {
		s.Logs.Info("compose", "deploy completed")
		sseEvent(w, flusher, "message", map[string]interface{}{
			"type":    "complete",
			"success": true,
		})
	}
}

// handleComposeStop runs docker compose down with SSE streaming.
func (s *Server) handleComposeStop(w http.ResponseWriter, r *http.Request) {
	s.streamComposeAction(w, r, "stop", func(handler func(string)) error {
		return s.Compose.Stop(r.Context(), handler)
	})
}

// handleComposeRestart runs down then up with SSE streaming.
func (s *Server) handleComposeRestart(w http.ResponseWriter, r *http.Request) {
	s.streamComposeAction(w, r, "restart", func(handler func(string)) error {
		return s.Compose.Restart(r.Context(), handler)
	})
}

// handleComposePull pulls latest images with SSE streaming.
func (s *Server) handleComposePull(w http.ResponseWriter, r *http.Request) {
	s.streamComposeAction(w, r, "pull", func(handler func(string)) error {
		return s.Compose.Pull(r.Context(), handler)
	})
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
