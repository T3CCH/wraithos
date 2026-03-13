package api

import (
	"net/http"
	"strings"

	"github.com/wraithos/wraith-ui/internal/auth"
	"github.com/wraithos/wraith-ui/internal/setup"
)

type setupRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type authStatusResponse struct {
	NeedsSetup     bool   `json:"needsSetup"`
	LoggedIn       bool   `json:"loggedIn"`
	Version        string `json:"version,omitempty"`
	NeedsDiskSetup bool   `json:"needsDiskSetup"`
}

// handleAuthSetup creates the initial admin credentials (first boot only).
func (s *Server) handleAuthSetup(w http.ResponseWriter, r *http.Request) {
	if !s.Auth.NeedsSetup() {
		writeError(w, http.StatusConflict, "setup already completed")
		return
	}

	var req setupRequest
	if err := decodeJSONLenient(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Default username to "admin" when frontend doesn't send one
	if req.Username == "" {
		req.Username = "admin"
	}

	if err := s.Auth.Setup(req.Username, req.Password); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.Logs.Info("auth", "initial setup completed for user %q", req.Username)
	writeOK(w, map[string]string{"status": "setup complete"})
}

// clientIP extracts the client IP from the request, stripping the port.
func clientIP(r *http.Request) string {
	ip := r.RemoteAddr
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		ip = ip[:idx]
	}
	return strings.TrimPrefix(strings.TrimSuffix(ip, "]"), "[")
}

// handleAuthLogin verifies credentials and returns a session cookie.
func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	// Rate limit by IP
	ip := clientIP(r)
	if err := s.Auth.CheckRateLimit(ip); err != nil {
		writeError(w, http.StatusTooManyRequests, err.Error())
		return
	}

	var req loginRequest
	if err := decodeJSONLenient(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// When frontend sends only password, use empty username which
	// auth.Login will match against the stored username.
	token, err := s.Auth.Login(req.Username, req.Password)
	if err != nil {
		s.Auth.RecordFailedLogin(ip)
		s.Logs.Warn("auth", "failed login attempt for user %q", req.Username)
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	auth.SetSessionCookie(w, token, s.Auth.SecureCookies)
	s.Logs.Info("auth", "user %q logged in", req.Username)
	writeOK(w, map[string]string{"status": "logged in"})
}

// handleAuthLogout clears the session.
func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	token := auth.GetSessionToken(r)
	if token != "" {
		s.Auth.Logout(token)
	}
	auth.ClearSessionCookie(w)
	writeOK(w, map[string]string{"status": "logged out"})
}

// handleAuthStatus reports whether setup is needed or the user is logged in.
func (s *Server) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	needsSetup := s.Auth.NeedsSetup()

	loggedIn := false
	token := auth.GetSessionToken(r)
	if token != "" {
		_, loggedIn = s.Auth.ValidateSession(token)
	}

	writeOK(w, authStatusResponse{
		NeedsSetup:     needsSetup,
		LoggedIn:       loggedIn,
		Version:        s.Version,
		NeedsDiskSetup: setup.NeedsDiskSetup(),
	})
}
