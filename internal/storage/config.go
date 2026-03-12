package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ReadJSON reads a JSON file at path and unmarshals it into dest.
// Returns os.ErrNotExist if the file does not exist.
func ReadJSON(path string, dest interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dest)
}

// WriteJSON marshals src to JSON and writes it atomically to path.
// The file is written to a temp file first, then renamed for crash safety.
func WriteJSON(path string, src interface{}) error {
	data, err := json.MarshalIndent(src, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}

	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename temp file: %w", err)
	}

	return nil
}

// ReadFile reads raw file content from path.
func ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// WriteFile writes raw content to path atomically.
func WriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}

	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename temp file: %w", err)
	}

	return nil
}

// Exists returns true if the file at path exists.
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
