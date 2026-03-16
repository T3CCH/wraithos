package api

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/wraithos/wraith-ui/internal/docker"
)

// handleStackList returns all stacks with their current status.
func (s *Server) handleStackList(w http.ResponseWriter, r *http.Request) {
	stacks, err := s.Stacks.ListStacks(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, map[string]interface{}{"stacks": stacks})
}

// handleStackCreate creates a new stack from a compose file and optional .env.
func (s *Server) handleStackCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string `json:"name"`
		Compose string `json:"compose"`
		Env     string `json:"env"`
	}
	if err := decodeJSONLenient(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Compose == "" {
		writeError(w, http.StatusBadRequest, "compose content is required")
		return
	}
	if err := docker.ValidateStackName(req.Name); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.Stacks.CreateStack(req.Name, req.Compose, req.Env); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	s.Logs.Info("stacks", "created stack %s", req.Name)
	writeOK(w, map[string]string{"status": "created", "name": req.Name})
}

// handleStackGet returns full detail for a single stack.
func (s *Server) handleStackGet(w http.ResponseWriter, r *http.Request, name string) {
	detail, err := s.Stacks.GetStack(r.Context(), name)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeOK(w, detail)
}

// handleStackUpdate updates a stack's compose and/or env files.
func (s *Server) handleStackUpdate(w http.ResponseWriter, r *http.Request, name string) {
	var req struct {
		Compose string `json:"compose"`
		Env     string `json:"env"`
	}
	if err := decodeJSONLenient(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Compose != "" {
		if err := s.Stacks.SaveCompose(name, req.Compose); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	if req.Env != "" {
		if err := s.Stacks.SaveEnv(name, req.Env); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	s.Logs.Info("stacks", "updated stack %s", name)
	writeOK(w, map[string]string{"status": "updated"})
}

// handleStackDelete deletes a stack and its directory.
func (s *Server) handleStackDelete(w http.ResponseWriter, r *http.Request, name string) {
	if err := s.Stacks.DeleteStack(r.Context(), name); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.Logs.Info("stacks", "deleted stack %s", name)
	writeOK(w, map[string]string{"status": "deleted"})
}

// handleStackAction dispatches start/stop/restart/pull actions for a stack with SSE streaming.
func (s *Server) handleStackAction(w http.ResponseWriter, r *http.Request, name, action string) {
	// Check required mounts before start/deploy/restart
	if action == "start" || action == "deploy" || action == "restart" {
		cfg, _ := docker.LoadStacksConfig()
		if stack, ok := cfg.Stacks[name]; ok && len(stack.RequiredMounts) > 0 {
			mounts, err := s.Samba.ListMounts()
			if err == nil {
				mounted := make(map[string]bool)
				for _, m := range mounts {
					if m.Mounted {
						mounted[filepath.Base(m.MountPoint)] = true
					}
				}
				var missing []string
				for _, req := range stack.RequiredMounts {
					if !mounted[req] {
						missing = append(missing, req)
					}
				}
				if len(missing) > 0 {
					writeError(w, http.StatusPreconditionFailed,
						fmt.Sprintf("Required mounts not connected: %s. Connect them in Network Mounts before starting.",
							strings.Join(missing, ", ")))
					return
				}
			}
		}
	}

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

	s.Logs.Info("stacks", "%s: %s started (SSE)", name, action)

	outputHandler := func(line string) {
		s.Logs.Info("stacks", "%s: %s", name, line)
		sseEvent(w, flusher, "message", map[string]string{
			"type": classifyLine(line),
			"line": line,
		})
	}

	var err error
	switch action {
	case "start":
		err = s.Stacks.Start(r.Context(), name, outputHandler)
	case "stop":
		err = s.Stacks.Stop(r.Context(), name, outputHandler)
	case "restart":
		err = s.Stacks.Restart(r.Context(), name, outputHandler)
	case "pull":
		err = s.Stacks.Pull(r.Context(), name, outputHandler)
	case "deploy":
		err = s.Stacks.Deploy(r.Context(), name, outputHandler)
	default:
		sseEvent(w, flusher, "message", map[string]interface{}{
			"type":    "complete",
			"success": false,
			"error":   "unknown action: " + action,
		})
		return
	}

	if err != nil {
		s.Logs.Error("stacks", "%s: %s failed: %v", name, action, err)
		sseEvent(w, flusher, "message", map[string]interface{}{
			"type":    "complete",
			"success": false,
			"error":   err.Error(),
		})
	} else {
		s.Logs.Info("stacks", "%s: %s completed", name, action)
		sseEvent(w, flusher, "message", map[string]interface{}{
			"type":    "complete",
			"success": true,
		})
	}
}

// handleStackMounts updates the required mounts list for a stack.
func (s *Server) handleStackMounts(w http.ResponseWriter, r *http.Request, name string) {
	var req struct {
		Mounts []string `json:"mounts"`
	}
	if err := decodeJSONLenient(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := s.Stacks.UpdateMounts(name, req.Mounts); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.Logs.Info("stacks", "%s: updated required mounts: %v", name, req.Mounts)
	writeOK(w, map[string]string{"status": "updated"})
}

// handleStackTerminal streams docker compose output over a WebSocket for stack actions.
func (s *Server) handleStackTerminal(w http.ResponseWriter, r *http.Request, name string) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("stack websocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	type wsMessage struct {
		Action string `json:"action"`
	}

	for {
		var msg wsMessage
		if err := conn.ReadJSON(&msg); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("stack websocket read error: %v", err)
			}
			return
		}

		ctx, cancel := context.WithCancel(r.Context())

		outputHandler := func(line string) {
			if writeErr := conn.WriteJSON(map[string]string{
				"type": "output",
				"data": line,
			}); writeErr != nil {
				log.Printf("stack websocket write error: %v", writeErr)
				cancel()
			}
		}

		var cmdErr error
		switch msg.Action {
		case "start":
			cmdErr = s.Stacks.Start(ctx, name, outputHandler)
		case "stop":
			cmdErr = s.Stacks.Stop(ctx, name, outputHandler)
		case "restart":
			cmdErr = s.Stacks.Restart(ctx, name, outputHandler)
		case "pull":
			cmdErr = s.Stacks.Pull(ctx, name, outputHandler)
		case "deploy":
			cmdErr = s.Stacks.Deploy(ctx, name, outputHandler)
		default:
			conn.WriteJSON(map[string]string{
				"type": "error",
				"data": "unknown action: " + msg.Action,
			})
			cancel()
			continue
		}

		if cmdErr != nil {
			conn.WriteJSON(map[string]string{
				"type": "error",
				"data": cmdErr.Error(),
			})
		} else {
			conn.WriteJSON(map[string]string{
				"type": "done",
				"data": msg.Action + " completed",
			})
		}

		cancel()
	}
}

// handleStackLogs streams live container logs for a stack over WebSocket.
func (s *Server) handleStackLogs(w http.ResponseWriter, r *http.Request, name string) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("stack logs websocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	container := r.URL.Query().Get("container")

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Monitor for client disconnect
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				cancel()
				return
			}
		}
	}()

	// Use io.Pipe to stream log output to WebSocket line by line
	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()
		s.Stacks.StreamLogs(ctx, name, container, pw)
	}()

	scanner := bufio.NewScanner(pr)
	for scanner.Scan() {
		line := scanner.Text()
		if err := conn.WriteJSON(map[string]string{
			"type": "log",
			"data": line,
		}); err != nil {
			return
		}
	}
}

// handleStackContainerRestart restarts a single container.
func (s *Server) handleStackContainerRestart(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Container string `json:"container"`
	}
	if err := decodeJSONLenient(r, &req); err != nil || req.Container == "" {
		writeError(w, http.StatusBadRequest, "container name is required")
		return
	}

	if err := s.Stacks.RestartContainer(r.Context(), req.Container); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.Logs.Info("stacks", "restarted container %s", req.Container)
	writeOK(w, map[string]string{"status": "restarted"})
}

// handleStacksRoute is the catch-all router for /api/stacks/ paths.
// It extracts the stack name and action from the URL.
func (s *Server) handleStacksRoute(w http.ResponseWriter, r *http.Request) {
	// Strip prefix to get "name" or "name/action"
	path := strings.TrimPrefix(r.URL.Path, "/api/stacks/")
	path = strings.TrimSuffix(path, "/")

	if path == "" {
		s.handleStackList(w, r)
		return
	}

	parts := strings.SplitN(path, "/", 2)
	name := parts[0]

	if err := docker.ValidateStackName(name); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if len(parts) == 1 {
		// /api/stacks/{name}
		switch r.Method {
		case http.MethodGet:
			s.handleStackGet(w, r, name)
		case http.MethodPut:
			s.handleStackUpdate(w, r, name)
		case http.MethodDelete:
			s.handleStackDelete(w, r, name)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	action := parts[1]

	// WebSocket endpoints (no method prefix in Go 1.22 mux for WebSocket)
	if action == "terminal" {
		s.handleStackTerminal(w, r, name)
		return
	}
	if action == "logs" {
		s.handleStackLogs(w, r, name)
		return
	}

	// Action endpoints require POST or PUT
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	switch action {
	case "start", "stop", "restart", "pull", "deploy":
		s.handleStackAction(w, r, name, action)
	case "mounts":
		s.handleStackMounts(w, r, name)
	default:
		writeError(w, http.StatusNotFound, "unknown action: "+action)
	}
}
