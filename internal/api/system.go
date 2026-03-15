package api

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/wraithos/wraith-ui/internal/auth"
	"github.com/wraithos/wraith-ui/internal/storage"
	"github.com/wraithos/wraith-ui/internal/system"
)

const maxBackupSize = 100 << 20 // 100 MB upload limit

// systemInfoResponse matches what the frontend JS expects.
type systemInfoResponse struct {
	Version  string  `json:"version"`
	Uptime   float64 `json:"uptime"`
	Kernel   string  `json:"kernel"`
	Arch     string  `json:"arch"`
	Hostname string  `json:"hostname"`
}

// handleSystemInfo returns OS-level system information
// in the format the frontend expects.
func (s *Server) handleSystemInfo(w http.ResponseWriter, r *http.Request) {
	info, err := system.GetSystemInfo(s.Version)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Parse uptime to seconds
	uptimeSecs := parseUptimeFromProc()

	// Get architecture from uname
	arch := system.GetArch()

	writeOK(w, systemInfoResponse{
		Version:  info.Version,
		Uptime:   uptimeSecs,
		Kernel:   info.Kernel,
		Arch:     arch,
		Hostname: info.Hostname,
	})
}

// handleSystemLogs returns recent log entries from the ring buffer.
// The frontend expects {logs: "..."} as a text blob.
func (s *Server) handleSystemLogs(w http.ResponseWriter, r *http.Request) {
	countStr := r.URL.Query().Get("count")
	count := 100 // default
	if countStr != "" {
		if n, err := strconv.Atoi(countStr); err == nil && n > 0 {
			count = n
		}
	}

	entries := s.Logs.GetLastN(count)

	// Format entries as a text log for the frontend
	var sb strings.Builder
	for _, e := range entries {
		sb.WriteString(fmt.Sprintf("[%s] %s: %s\n",
			e.Timestamp.Format("15:04:05"),
			e.Source,
			e.Message,
		))
	}

	writeOK(w, map[string]string{"logs": sb.String()})
}

// parseUptimeFromProc reads /proc/uptime and returns seconds.
func parseUptimeFromProc() float64 {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0
	}
	secs, _ := strconv.ParseFloat(fields[0], 64)
	return secs
}

// handleSystemBackup creates a tar.gz archive of the config directory
// and streams it as a download.
func (s *Server) handleSystemBackup(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", "attachment; filename=wraithos-config-backup.tar.gz")

	gzWriter := gzip.NewWriter(w)
	defer gzWriter.Close()

	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()

	configDir := storage.ConfigBase
	composeDir := storage.ComposeDir()

	for _, dir := range []string{configDir, composeDir} {
		if err := addDirToTar(tarWriter, dir); err != nil {
			s.Logs.Error("backup", "failed to add %s to backup: %v", dir, err)
			// Can't change status code after streaming has started,
			// so we just log and continue with what we have
		}
	}

	s.Logs.Info("system", "config backup downloaded")
}

func addDirToTar(tw *tar.Writer, dir string) error {
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return fmt.Errorf("create tar header for %s: %w", path, err)
		}

		// Use relative path within the archive
		relPath, err := filepath.Rel(filepath.Dir(dir), path)
		if err != nil {
			return err
		}
		header.Name = relPath

		if err := tw.WriteHeader(header); err != nil {
			return fmt.Errorf("write tar header: %w", err)
		}

		if d.IsDir() {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read file %s: %w", path, err)
		}

		if _, err := tw.Write(data); err != nil {
			return fmt.Errorf("write file to tar: %w", err)
		}

		return nil
	})
}

// handleSystemRestore accepts an uploaded tar.gz backup and restores config
// files to the config directory. It validates the archive structure before
// extracting to prevent path traversal attacks.
func (s *Server) handleSystemRestore(w http.ResponseWriter, r *http.Request) {
	// Limit upload size
	r.Body = http.MaxBytesReader(w, r.Body, maxBackupSize)

	file, header, err := r.FormFile("backup")
	if err != nil {
		writeError(w, http.StatusBadRequest, "backup file required")
		return
	}
	defer file.Close()

	// Validate file extension
	if !strings.HasSuffix(header.Filename, ".tar.gz") && !strings.HasSuffix(header.Filename, ".tgz") {
		writeError(w, http.StatusBadRequest, "file must be a .tar.gz archive")
		return
	}

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid gzip archive")
		return
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)

	// The backup archive stores files with paths like "config/auth.json",
	// "config/compose/docker-compose.yml", etc. We strip the leading
	// directory component ("config/") and write into ConfigBase.
	configDirName := filepath.Base(storage.ConfigBase) // "config"
	restored := 0

	for {
		hdr, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, "corrupt tar archive")
			return
		}

		// Sanitize: the archive path must start with the config dir name
		clean := filepath.Clean(hdr.Name)
		if !strings.HasPrefix(clean, configDirName+"/") && clean != configDirName {
			continue // skip entries outside the config directory
		}

		// Strip the leading config dir name to get relative path
		rel := strings.TrimPrefix(clean, configDirName+"/")
		if rel == "" {
			continue // skip the directory entry itself
		}

		// Security: reject any path traversal attempts
		if strings.Contains(rel, "..") {
			continue
		}

		destPath := filepath.Join(storage.ConfigBase, rel)

		// Ensure the destination is still under ConfigBase
		if !strings.HasPrefix(destPath, storage.ConfigBase) {
			continue
		}

		if hdr.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(destPath, 0755); err != nil {
				s.Logs.Error("restore", "create dir %s: %v", destPath, err)
			}
			continue
		}

		// Only restore regular files
		if hdr.Typeflag != tar.TypeReg {
			continue
		}

		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			s.Logs.Error("restore", "create parent dir for %s: %v", destPath, err)
			continue
		}

		data, err := io.ReadAll(io.LimitReader(tarReader, maxBackupSize))
		if err != nil {
			s.Logs.Error("restore", "read %s from archive: %v", rel, err)
			continue
		}

		perm := os.FileMode(0644)
		if hdr.Mode != 0 {
			perm = os.FileMode(hdr.Mode) & 0755
		}

		if err := os.WriteFile(destPath, data, perm); err != nil {
			s.Logs.Error("restore", "write %s: %v", destPath, err)
			continue
		}

		restored++
	}

	if restored == 0 {
		writeError(w, http.StatusBadRequest, "no config files found in archive")
		return
	}

	// Sync restored files to physical config disk
	if err := storage.SyncAll(); err != nil {
		s.Logs.Error("restore", "sync to disk failed: %v", err)
	}

	s.Logs.Info("system", "config backup restored (%d files)", restored)
	writeOK(w, map[string]interface{}{
		"status":   "restored",
		"restored": restored,
	})
}

type passwordChangeRequest struct {
	// Accept both frontend format (current/password) and API format (current_password/new_password)
	Current         string `json:"current"`
	Password        string `json:"password"`
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (p *passwordChangeRequest) currentPw() string {
	if p.Current != "" {
		return p.Current
	}
	return p.CurrentPassword
}

func (p *passwordChangeRequest) newPw() string {
	if p.Password != "" {
		return p.Password
	}
	return p.NewPassword
}

// handlePasswordChange updates the admin password.
func (s *Server) handlePasswordChange(w http.ResponseWriter, r *http.Request) {
	var req passwordChangeRequest
	if err := decodeJSONLenient(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	cur, newPw := req.currentPw(), req.newPw()
	if cur == "" || newPw == "" {
		writeError(w, http.StatusBadRequest, "current password and new password are required")
		return
	}

	if err := s.Auth.ChangePassword(cur, newPw); err != nil {
		if strings.Contains(err.Error(), "incorrect") {
			writeError(w, http.StatusUnauthorized, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Sync the root password so SSH/console login uses the same credentials.
	// Non-fatal: log a warning but don't fail the password change if chpasswd fails.
	if err := auth.SyncRootPassword(newPw); err != nil {
		s.Logs.Warn("auth", "failed to sync root password: %v", err)
	} else {
		s.Logs.Info("auth", "root password synced with web console")
	}

	s.Logs.Info("auth", "password changed")
	writeOK(w, map[string]string{"status": "password changed"})
}

// handleSSHGet returns the current SSH service status.
func (s *Server) handleSSHGet(w http.ResponseWriter, r *http.Request) {
	status, err := system.GetSSHStatus()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, status)
}

// handleSSHSet enables or disables the SSH service.
func (s *Server) handleSSHSet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeJSONLenient(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Enabled {
		if err := system.EnableSSH(); err != nil {
			s.Logs.Error("ssh", "enable failed: %v", err)
			writeError(w, http.StatusInternalServerError, "failed to enable SSH: "+err.Error())
			return
		}
		s.Logs.Info("ssh", "SSH enabled via web UI")
	} else {
		if err := system.DisableSSH(); err != nil {
			s.Logs.Error("ssh", "disable failed: %v", err)
			writeError(w, http.StatusInternalServerError, "failed to disable SSH: "+err.Error())
			return
		}
		s.Logs.Info("ssh", "SSH disabled via web UI")
	}

	// Return updated status
	status, _ := system.GetSSHStatus()
	writeOK(w, status)
}
