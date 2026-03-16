package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/wraithos/wraith-ui/internal/storage"
)

// Maximum upload size: 10GB
const maxUploadSize = 10 << 30

// fileRoot represents a browsable root directory.
type fileRoot struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Type string `json:"type"` // "samba", "nfs", "volumes"
}

// fileEntry represents a file or directory in a listing.
type fileEntry struct {
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	Modified string `json:"modified"`
	IsDir    bool   `json:"isDir"`
	Mode     string `json:"mode"`
}

// getAllowedRoots returns all currently allowed root paths for file browsing.
// Scoped to /dockerapps (app data) and /remotemounts/* (network shares).
func (s *Server) getAllowedRoots() []fileRoot {
	var roots []fileRoot

	// Add /dockerapps as the app data root (always present)
	appsDir := storage.AppsDir()
	os.MkdirAll(appsDir, 0755)
	roots = append(roots, fileRoot{
		Name: "App Data",
		Path: appsDir,
		Type: "local",
	})

	// Add mounted Samba/NFS shares from /remotemounts
	mounts, err := s.Samba.ListMounts()
	if err == nil {
		for _, m := range mounts {
			if !m.Mounted {
				continue
			}
			name := filepath.Base(m.MountPoint)
			mountType := m.Type
			if mountType == "" {
				mountType = "cifs"
			}
			roots = append(roots, fileRoot{
				Name: "Remote: " + name,
				Path: m.MountPoint,
				Type: mountType,
			})
		}
	}

	return roots
}

// validateFilePath checks that the requested path is under one of the allowed roots.
// It returns the cleaned absolute path or an error.
func (s *Server) validateFilePath(requestedPath string) (string, error) {
	if requestedPath == "" {
		return "", fmt.Errorf("path is required")
	}

	// Clean the path to resolve any .. or . segments
	cleanPath := filepath.Clean(requestedPath)

	// Ensure the path is absolute
	if !filepath.IsAbs(cleanPath) {
		return "", fmt.Errorf("path must be absolute")
	}

	// Check against allowed roots
	roots := s.getAllowedRoots()
	for _, root := range roots {
		// The path must be the root itself or a child of the root
		if cleanPath == root.Path || strings.HasPrefix(cleanPath, root.Path+"/") {
			return cleanPath, nil
		}
	}

	return "", fmt.Errorf("access denied: path is not under any allowed root")
}

// validateFilePathNotRoot checks that the path is valid AND not a root path itself.
func (s *Server) validateFilePathNotRoot(requestedPath string) (string, error) {
	cleanPath, err := s.validateFilePath(requestedPath)
	if err != nil {
		return "", err
	}

	roots := s.getAllowedRoots()
	for _, root := range roots {
		if cleanPath == root.Path {
			return "", fmt.Errorf("cannot perform this operation on a root path")
		}
	}

	return cleanPath, nil
}

// handleFileRoots returns the list of browsable root paths.
func (s *Server) handleFileRoots(w http.ResponseWriter, r *http.Request) {
	roots := s.getAllowedRoots()
	if roots == nil {
		roots = []fileRoot{}
	}
	writeOK(w, map[string]interface{}{"roots": roots})
}

// handleFileList returns the contents of a directory.
func (s *Server) handleFileList(w http.ResponseWriter, r *http.Request) {
	dirPath := r.URL.Query().Get("path")
	sortBy := r.URL.Query().Get("sort")

	cleanPath, err := s.validateFilePath(dirPath)
	if err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	info, err := os.Stat(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, "directory not found")
		} else {
			writeError(w, http.StatusInternalServerError, "failed to access path")
		}
		return
	}
	if !info.IsDir() {
		writeError(w, http.StatusBadRequest, "path is not a directory")
		return
	}

	entries, err := os.ReadDir(cleanPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read directory")
		return
	}

	files := make([]fileEntry, 0, len(entries))
	for _, e := range entries {
		fi, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, fileEntry{
			Name:     e.Name(),
			Size:     fi.Size(),
			Modified: fi.ModTime().UTC().Format(time.RFC3339),
			IsDir:    e.IsDir(),
			Mode:     fi.Mode().String(),
		})
	}

	// Sort entries: directories first, then by the requested sort field
	switch sortBy {
	case "size":
		sort.Slice(files, func(i, j int) bool {
			if files[i].IsDir != files[j].IsDir {
				return files[i].IsDir
			}
			return files[i].Size < files[j].Size
		})
	case "modified":
		sort.Slice(files, func(i, j int) bool {
			if files[i].IsDir != files[j].IsDir {
				return files[i].IsDir
			}
			return files[i].Modified > files[j].Modified
		})
	default: // "name" or empty
		sort.Slice(files, func(i, j int) bool {
			if files[i].IsDir != files[j].IsDir {
				return files[i].IsDir
			}
			return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
		})
	}

	writeOK(w, map[string]interface{}{
		"path":  cleanPath,
		"files": files,
	})
}

// handleFileDownload streams a file for download.
func (s *Server) handleFileDownload(w http.ResponseWriter, r *http.Request) {
	filePath := r.URL.Query().Get("path")

	cleanPath, err := s.validateFilePath(filePath)
	if err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	info, err := os.Stat(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, "file not found")
		} else {
			writeError(w, http.StatusInternalServerError, "failed to access file")
		}
		return
	}
	if info.IsDir() {
		writeError(w, http.StatusBadRequest, "cannot download a directory")
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(cleanPath)))
	http.ServeFile(w, r, cleanPath)
}

// handleFileUpload handles multipart file upload to a target directory.
func (s *Server) handleFileUpload(w http.ResponseWriter, r *http.Request) {
	// Limit request body size but allow streaming
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)

	// Parse the multipart form — 32MB buffer, rest goes to temp files
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "failed to parse upload: "+err.Error())
		return
	}
	defer r.MultipartForm.RemoveAll()

	targetDir := r.FormValue("path")
	cleanPath, err := s.validateFilePath(targetDir)
	if err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	info, err := os.Stat(cleanPath)
	if err != nil || !info.IsDir() {
		writeError(w, http.StatusBadRequest, "target path is not a valid directory")
		return
	}

	fhs := r.MultipartForm.File["file"]
	if len(fhs) == 0 {
		writeError(w, http.StatusBadRequest, "no files provided")
		return
	}

	uploaded := 0
	for _, fh := range fhs {
		// Sanitize filename — strip any path components
		name := filepath.Base(fh.Filename)
		if name == "." || name == ".." || name == "/" {
			continue
		}

		src, err := fh.Open()
		if err != nil {
			log.Printf("files: failed to open uploaded file %s: %v", name, err)
			continue
		}

		destPath := filepath.Join(cleanPath, name)
		dst, err := os.Create(destPath)
		if err != nil {
			src.Close()
			log.Printf("files: failed to create destination file %s: %v", destPath, err)
			continue
		}

		if _, err := io.Copy(dst, src); err != nil {
			src.Close()
			dst.Close()
			log.Printf("files: failed to write file %s: %v", destPath, err)
			continue
		}

		src.Close()
		dst.Close()
		uploaded++
	}

	s.Logs.Info("files", "uploaded %d file(s) to %s", uploaded, cleanPath)
	writeOK(w, map[string]interface{}{
		"uploaded": uploaded,
		"path":     cleanPath,
	})
}

// handleFileMkdir creates a new directory.
func (s *Server) handleFileMkdir(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	cleanPath, err := s.validateFilePath(req.Path)
	if err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	if err := os.MkdirAll(cleanPath, 0755); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create directory: "+err.Error())
		return
	}

	s.Logs.Info("files", "created directory %s", cleanPath)
	writeOK(w, map[string]interface{}{"path": cleanPath})
}

// handleFileDelete removes a file or directory.
func (s *Server) handleFileDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Use validateFilePathNotRoot to prevent deleting root mount points
	cleanPath, err := s.validateFilePathNotRoot(req.Path)
	if err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	if _, err := os.Stat(cleanPath); os.IsNotExist(err) {
		writeError(w, http.StatusNotFound, "path not found")
		return
	}

	if err := os.RemoveAll(cleanPath); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete: "+err.Error())
		return
	}

	s.Logs.Info("files", "deleted %s", cleanPath)
	writeOK(w, map[string]interface{}{"deleted": cleanPath})
}

// handleFileMove moves or renames a file/directory.
func (s *Server) handleFileMove(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Source      string `json:"source"`
		Destination string `json:"destination"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	srcPath, err := s.validateFilePathNotRoot(req.Source)
	if err != nil {
		writeError(w, http.StatusForbidden, "source: "+err.Error())
		return
	}

	dstPath, err := s.validateFilePath(req.Destination)
	if err != nil {
		writeError(w, http.StatusForbidden, "destination: "+err.Error())
		return
	}

	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		writeError(w, http.StatusNotFound, "source path not found")
		return
	}

	// Try os.Rename first (works within same filesystem)
	if err := os.Rename(srcPath, dstPath); err != nil {
		// Fall back to copy + delete for cross-filesystem moves
		if err := copyPath(srcPath, dstPath); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to move: "+err.Error())
			return
		}
		if err := os.RemoveAll(srcPath); err != nil {
			log.Printf("files: warning - copied but failed to remove source %s: %v", srcPath, err)
		}
	}

	s.Logs.Info("files", "moved %s to %s", srcPath, dstPath)
	writeOK(w, map[string]interface{}{
		"source":      srcPath,
		"destination": dstPath,
	})
}

// handleFileCopy copies a file or directory.
func (s *Server) handleFileCopy(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Source      string `json:"source"`
		Destination string `json:"destination"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	srcPath, err := s.validateFilePath(req.Source)
	if err != nil {
		writeError(w, http.StatusForbidden, "source: "+err.Error())
		return
	}

	dstPath, err := s.validateFilePath(req.Destination)
	if err != nil {
		writeError(w, http.StatusForbidden, "destination: "+err.Error())
		return
	}

	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		writeError(w, http.StatusNotFound, "source path not found")
		return
	}

	if err := copyPath(srcPath, dstPath); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to copy: "+err.Error())
		return
	}

	s.Logs.Info("files", "copied %s to %s", srcPath, dstPath)
	writeOK(w, map[string]interface{}{
		"source":      srcPath,
		"destination": dstPath,
	})
}

// copyPath copies a file or directory recursively.
func copyPath(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}

	if info.IsDir() {
		return copyDir(src, dst)
	}
	return copyFile(src, dst, info.Mode())
}

// copyFile copies a single file.
func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

// copyDir copies a directory recursively.
func copyDir(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, e := range entries {
		srcPath := filepath.Join(src, e.Name())
		dstPath := filepath.Join(dst, e.Name())
		if err := copyPath(srcPath, dstPath); err != nil {
			return fmt.Errorf("copy %s: %w", e.Name(), err)
		}
	}

	return nil
}

// handleFileRename renames a file or directory (simpler than move — same directory).
func (s *Server) handleFileRename(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path    string `json:"path"`
		NewName string `json:"newName"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	srcPath, err := s.validateFilePathNotRoot(req.Path)
	if err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	// Validate new name doesn't contain path separators or dangerous chars
	newName := req.NewName
	if newName == "" || strings.ContainsAny(newName, "/\\") || newName == "." || newName == ".." {
		writeError(w, http.StatusBadRequest, "invalid new name")
		return
	}

	dstPath := filepath.Join(filepath.Dir(srcPath), newName)

	// Validate the destination is also under an allowed root
	if _, err := s.validateFilePath(dstPath); err != nil {
		writeError(w, http.StatusForbidden, "destination: "+err.Error())
		return
	}

	if err := os.Rename(srcPath, dstPath); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to rename: "+err.Error())
		return
	}

	s.Logs.Info("files", "renamed %s to %s", srcPath, dstPath)
	writeOK(w, map[string]interface{}{
		"oldPath": srcPath,
		"newPath": dstPath,
	})
}

// decodeJSONBody is a helper that decodes JSON from the request body.
// It's used by file endpoints that need the request body.
func decodeJSONBody(r *http.Request, dest interface{}) error {
	return json.NewDecoder(r.Body).Decode(dest)
}
