// Package auth provides password hashing, session management, and cookie handling.
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/wraithos/wraith-ui/internal/storage"
	"golang.org/x/crypto/bcrypt"
)

const (
	sessionCookieName = "wraith_session"
	sessionTTL        = 24 * time.Hour
	bcryptCost        = 12
	minPasswordLen    = 8
	maxLoginAttempts  = 10
	loginWindow       = time.Minute
)

// Credentials stored on the config disk.
type Credentials struct {
	Username string `json:"username"`
	Hash     string `json:"hash"`
}

// Session represents an active login session.
type Session struct {
	Token     string
	Username  string
	ExpiresAt time.Time
}

// Manager handles authentication and session state.
// Sessions are stored in memory only (RAM-based OS).
type Manager struct {
	mu            sync.RWMutex
	sessions      map[string]*Session
	SecureCookies bool // Set to true when TLS is configured

	loginMu       sync.Mutex
	loginAttempts map[string][]time.Time // IP -> timestamps of failed attempts
}

// NewManager creates a new auth manager.
func NewManager() *Manager {
	return &Manager{
		sessions:      make(map[string]*Session),
		loginAttempts: make(map[string][]time.Time),
	}
}

// CheckRateLimit returns an error if the IP has exceeded the login attempt limit.
func (m *Manager) CheckRateLimit(ip string) error {
	m.loginMu.Lock()
	defer m.loginMu.Unlock()

	now := time.Now()
	cutoff := now.Add(-loginWindow)

	// Filter to only recent attempts
	attempts := m.loginAttempts[ip]
	recent := attempts[:0]
	for _, t := range attempts {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	m.loginAttempts[ip] = recent

	if len(recent) >= maxLoginAttempts {
		return fmt.Errorf("too many login attempts, try again later")
	}
	return nil
}

// RecordFailedLogin records a failed login attempt for rate limiting.
func (m *Manager) RecordFailedLogin(ip string) {
	m.loginMu.Lock()
	defer m.loginMu.Unlock()
	m.loginAttempts[ip] = append(m.loginAttempts[ip], time.Now())
}

// CleanLoginAttempts removes old login attempt records.
func (m *Manager) CleanLoginAttempts() {
	m.loginMu.Lock()
	defer m.loginMu.Unlock()
	cutoff := time.Now().Add(-loginWindow)
	for ip, attempts := range m.loginAttempts {
		recent := attempts[:0]
		for _, t := range attempts {
			if t.After(cutoff) {
				recent = append(recent, t)
			}
		}
		if len(recent) == 0 {
			delete(m.loginAttempts, ip)
		} else {
			m.loginAttempts[ip] = recent
		}
	}
}

// NeedsSetup returns true if no credentials file exists yet (first boot).
func (m *Manager) NeedsSetup() bool {
	return !storage.Exists(storage.AuthFile())
}

// Setup creates the initial admin credentials. Only works if no credentials exist.
func (m *Manager) Setup(username, password string) error {
	if !m.NeedsSetup() {
		return fmt.Errorf("setup already completed")
	}

	if username == "" || password == "" {
		return fmt.Errorf("username and password are required")
	}

	if len(password) < minPasswordLen {
		return fmt.Errorf("password must be at least %d characters", minPasswordLen)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	creds := Credentials{
		Username: username,
		Hash:     string(hash),
	}

	return storage.WriteJSON(storage.AuthFile(), &creds)
}

// Login verifies credentials and creates a new session. Returns the session token.
func (m *Manager) Login(username, password string) (string, error) {
	var creds Credentials
	if err := storage.ReadJSON(storage.AuthFile(), &creds); err != nil {
		return "", fmt.Errorf("read credentials: %w", err)
	}

	// When username is empty (frontend sends only password), use stored username
	if username == "" {
		username = creds.Username
	}
	if username != creds.Username {
		return "", fmt.Errorf("invalid credentials")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(creds.Hash), []byte(password)); err != nil {
		return "", fmt.Errorf("invalid credentials")
	}

	token, err := generateToken()
	if err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}

	m.mu.Lock()
	m.sessions[token] = &Session{
		Token:     token,
		Username:  username,
		ExpiresAt: time.Now().Add(sessionTTL),
	}
	m.mu.Unlock()

	return token, nil
}

// Logout invalidates the session associated with the given token.
func (m *Manager) Logout(token string) {
	m.mu.Lock()
	delete(m.sessions, token)
	m.mu.Unlock()
}

// ValidateSession checks if a token corresponds to a valid, non-expired session.
func (m *Manager) ValidateSession(token string) (*Session, bool) {
	m.mu.RLock()
	sess, ok := m.sessions[token]
	m.mu.RUnlock()

	if !ok {
		return nil, false
	}

	if time.Now().After(sess.ExpiresAt) {
		m.Logout(token)
		return nil, false
	}

	return sess, true
}

// ChangePassword updates the stored password for the admin user.
func (m *Manager) ChangePassword(currentPassword, newPassword string) error {
	var creds Credentials
	if err := storage.ReadJSON(storage.AuthFile(), &creds); err != nil {
		return fmt.Errorf("read credentials: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(creds.Hash), []byte(currentPassword)); err != nil {
		return fmt.Errorf("current password is incorrect")
	}

	if newPassword == "" {
		return fmt.Errorf("new password is required")
	}

	if len(newPassword) < minPasswordLen {
		return fmt.Errorf("password must be at least %d characters", minPasswordLen)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcryptCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	creds.Hash = string(hash)
	return storage.WriteJSON(storage.AuthFile(), &creds)
}

// SetSessionCookie writes the session cookie to the response.
func SetSessionCookie(w http.ResponseWriter, token string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})
}

// ClearSessionCookie removes the session cookie.
func ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
}

// GetSessionToken extracts the session token from the request cookie.
func GetSessionToken(r *http.Request) string {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}

// CleanExpiredSessions removes all expired sessions. Called periodically.
func (m *Manager) CleanExpiredSessions() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for token, sess := range m.sessions {
		if now.After(sess.ExpiresAt) {
			delete(m.sessions, token)
		}
	}
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
