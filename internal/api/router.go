// Package api provides the HTTP router, middleware, and API handlers.
package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/wraithos/wraith-ui/internal/auth"
	"github.com/wraithos/wraith-ui/internal/docker"
	"github.com/wraithos/wraith-ui/internal/system"
)

// Server holds all dependencies for the API handlers.
type Server struct {
	Auth     *auth.Manager
	Docker   *docker.Client
	Compose  *docker.ComposeManager
	Samba    *system.SambaManager
	Logs     *system.LogCollector
	Version  string
	Mux      *http.ServeMux
}

// NewServer creates a new API server with all routes registered.
func NewServer(
	authMgr *auth.Manager,
	dockerClient *docker.Client,
	composeMgr *docker.ComposeManager,
	sambaMgr *system.SambaManager,
	logCollector *system.LogCollector,
	version string,
	staticFS http.FileSystem,
) *Server {
	s := &Server{
		Auth:    authMgr,
		Docker:  dockerClient,
		Compose: composeMgr,
		Samba:   sambaMgr,
		Logs:    logCollector,
		Version: version,
		Mux:     http.NewServeMux(),
	}

	s.registerRoutes(staticFS)
	return s
}

func (s *Server) registerRoutes(staticFS http.FileSystem) {
	// Auth endpoints (no auth middleware required)
	s.Mux.HandleFunc("POST /api/auth/setup", s.handleAuthSetup)
	s.Mux.HandleFunc("POST /api/auth/login", s.handleAuthLogin)
	s.Mux.HandleFunc("POST /api/auth/logout", s.handleAuthLogout)
	s.Mux.HandleFunc("GET /api/auth/status", s.handleAuthStatus)
	s.Mux.HandleFunc("PUT /api/auth/password", s.requireAuth(s.handlePasswordChange))

	// Dashboard / system status (frontend calls GET /api/system/status)
	s.Mux.HandleFunc("GET /api/system/status", s.requireAuth(s.handleDashboard))
	s.Mux.HandleFunc("GET /api/dashboard", s.requireAuth(s.handleDashboard))

	// Compose file endpoints (frontend uses /api/compose/file)
	s.Mux.HandleFunc("GET /api/compose/file", s.requireAuth(s.handleComposeGet))
	s.Mux.HandleFunc("PUT /api/compose/file", s.requireAuth(s.handleComposeSave))
	s.Mux.HandleFunc("POST /api/compose/validate", s.requireAuth(s.handleComposeValidate))

	// Compose action endpoints
	s.Mux.HandleFunc("POST /api/compose/deploy", s.requireAuth(s.handleComposeDeploy))
	s.Mux.HandleFunc("POST /api/compose/start", s.requireAuth(s.handleComposeDeploy))
	s.Mux.HandleFunc("POST /api/compose/stop", s.requireAuth(s.handleComposeStop))
	s.Mux.HandleFunc("POST /api/compose/restart", s.requireAuth(s.handleComposeRestart))
	s.Mux.HandleFunc("POST /api/compose/pull", s.requireAuth(s.handleComposePull))
	s.Mux.HandleFunc("GET /api/compose/status", s.requireAuth(s.handleComposeStatus))

	// Compose settings endpoints
	s.Mux.HandleFunc("GET /api/compose/settings", s.requireAuth(s.handleComposeSettingsGet))
	s.Mux.HandleFunc("PUT /api/compose/settings", s.requireAuth(s.handleComposeSettingsSet))

	// WebSocket endpoints for compose terminal
	s.Mux.HandleFunc("/api/compose/terminal", s.requireAuth(s.handleComposeTerminal))
	s.Mux.HandleFunc("/api/compose/deploy/ws", s.requireAuth(s.handleComposeTerminal))

	// Mount endpoints (frontend uses /api/mounts)
	s.Mux.HandleFunc("GET /api/mounts", s.requireAuth(s.handleSambaList))
	s.Mux.HandleFunc("POST /api/mounts", s.requireAuth(s.handleSambaAdd))
	s.Mux.Handle("/api/mounts/", s.requireAuthHandler(http.HandlerFunc(s.handleMountsByID)))

	// Network endpoints
	s.Mux.HandleFunc("GET /api/network", s.requireAuth(s.handleNetworkGet))
	s.Mux.HandleFunc("PUT /api/network", s.requireAuth(s.handleNetworkSet))

	// Setup wizard endpoints
	s.Mux.HandleFunc("GET /api/setup/status", s.requireAuth(s.handleSetupStatus))
	s.Mux.HandleFunc("POST /api/setup/disks", s.requireAuth(s.handleSetupDisks))
	s.Mux.HandleFunc("POST /api/setup/rescan", s.requireAuth(s.handleSetupRescan))
	s.Mux.HandleFunc("POST /api/setup/wipe", s.requireAuth(s.handleDiskWipe))

	// Timezone endpoints
	s.Mux.HandleFunc("GET /api/system/timezone", s.requireAuth(s.handleTimezoneGet))
	s.Mux.HandleFunc("PUT /api/system/timezone", s.requireAuth(s.handleTimezoneSet))

	// Reboot endpoint
	s.Mux.HandleFunc("POST /api/system/reboot", s.requireAuth(s.handleReboot))

	// System endpoints
	s.Mux.HandleFunc("GET /api/system/info", s.requireAuth(s.handleSystemInfo))
	s.Mux.HandleFunc("GET /api/system/logs", s.requireAuth(s.handleSystemLogs))
	s.Mux.HandleFunc("GET /api/system/backup", s.requireAuth(s.handleSystemBackup))
	s.Mux.HandleFunc("POST /api/system/restore", s.requireAuth(s.handleSystemRestore))

	// SSH service endpoints
	s.Mux.HandleFunc("GET /api/system/ssh", s.requireAuth(s.handleSSHGet))
	s.Mux.HandleFunc("PUT /api/system/ssh", s.requireAuth(s.handleSSHSet))

	// Disk expand endpoints
	s.Mux.HandleFunc("GET /api/system/disks/expandable", s.requireAuth(s.handleExpandableDisks))
	s.Mux.HandleFunc("POST /api/system/disks/expand", s.requireAuth(s.handleExpandDisk))

	// Docker network management endpoints
	s.Mux.HandleFunc("GET /api/docker/networks", s.requireAuth(s.handleDockerNetworkList))
	s.Mux.HandleFunc("POST /api/docker/networks", s.requireAuth(s.handleDockerNetworkCreate))
	s.Mux.HandleFunc("DELETE /api/docker/networks", s.requireAuth(s.handleDockerNetworkDelete))

	// Docker system prune endpoint
	s.Mux.HandleFunc("POST /api/docker/prune", s.requireAuth(s.handleDockerPrune))

	// File manager endpoints
	s.Mux.HandleFunc("GET /api/files/roots", s.requireAuth(s.handleFileRoots))
	s.Mux.HandleFunc("GET /api/files/list", s.requireAuth(s.handleFileList))
	s.Mux.HandleFunc("GET /api/files/download", s.requireAuth(s.handleFileDownload))
	s.Mux.HandleFunc("POST /api/files/upload", s.requireAuth(s.handleFileUpload))
	s.Mux.HandleFunc("POST /api/files/mkdir", s.requireAuth(s.handleFileMkdir))
	s.Mux.HandleFunc("POST /api/files/delete", s.requireAuth(s.handleFileDelete))
	s.Mux.HandleFunc("POST /api/files/move", s.requireAuth(s.handleFileMove))
	s.Mux.HandleFunc("POST /api/files/copy", s.requireAuth(s.handleFileCopy))
	s.Mux.HandleFunc("POST /api/files/rename", s.requireAuth(s.handleFileRename))

	// Static file serving (frontend)
	if staticFS != nil {
		fileServer := http.FileServer(staticFS)
		s.Mux.Handle("/", s.spaHandler(fileServer))
	}
}

// spaHandler wraps the file server to serve index.html for SPA routes.
func (s *Server) spaHandler(fileServer http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Don't intercept API routes
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}

		// Try to serve the file directly
		fileServer.ServeHTTP(w, r)
	})
}

// requireAuth is middleware that checks for a valid session.
func (s *Server) requireAuth(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := auth.GetSessionToken(r)
		if token == "" {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}

		if _, valid := s.Auth.ValidateSession(token); !valid {
			writeError(w, http.StatusUnauthorized, "session expired or invalid")
			return
		}

		handler(w, r)
	}
}

// requireAuthHandler wraps an http.Handler with auth middleware.
func (s *Server) requireAuthHandler(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := auth.GetSessionToken(r)
		if token == "" {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}

		if _, valid := s.Auth.ValidateSession(token); !valid {
			writeError(w, http.StatusUnauthorized, "session expired or invalid")
			return
		}

		handler.ServeHTTP(w, r)
	})
}

// loggingMiddleware logs each request.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s", r.RemoteAddr, r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

// Handler returns the top-level HTTP handler with middleware applied.
func (s *Server) Handler() http.Handler {
	return loggingMiddleware(s.Mux)
}

// --- JSON response helpers ---

type errorResponse struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("error encoding json response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}

func writeOK(w http.ResponseWriter, data interface{}) {
	writeJSON(w, http.StatusOK, data)
}

func decodeJSON(r *http.Request, dest interface{}) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(dest)
}

func decodeJSONLenient(r *http.Request, dest interface{}) error {
	return json.NewDecoder(r.Body).Decode(dest)
}
