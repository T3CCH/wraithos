package setup

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// zoneinfoBase is the root directory for timezone data.
const zoneinfoBase = "/usr/share/zoneinfo"

// commonTimezones is a curated list shown first in the UI.
// The full list is also available via ListTimezones.
var commonTimezones = []string{
	"UTC",
	"US/Eastern",
	"US/Central",
	"US/Mountain",
	"US/Pacific",
	"US/Alaska",
	"US/Hawaii",
	"Canada/Eastern",
	"Canada/Central",
	"Canada/Pacific",
	"Europe/London",
	"Europe/Paris",
	"Europe/Berlin",
	"Europe/Moscow",
	"Asia/Tokyo",
	"Asia/Shanghai",
	"Asia/Kolkata",
	"Asia/Dubai",
	"Australia/Sydney",
	"Australia/Melbourne",
	"Pacific/Auckland",
}

// skipDirs are zoneinfo subdirectories that are not actual timezones.
var skipDirs = map[string]bool{
	"posix":     true,
	"right":     true,
	"posixrules": true,
	"Etc":       true,
}

// skipFiles are zoneinfo entries that are not timezone names.
var skipFiles = map[string]bool{
	"posixrules":  true,
	"localtime":   true,
	"leap-seconds.list": true,
	"leapseconds": true,
	"tzdata.zi":   true,
	"zone.tab":    true,
	"zone1970.tab": true,
	"iso3166.tab": true,
	"+VERSION":    true,
	"SECURITY":    true,
	"README":      true,
}

// ListTimezones reads /usr/share/zoneinfo and returns available timezone
// names (e.g. "US/Eastern", "Europe/London"). Returns the common list
// first, followed by all others sorted alphabetically.
func ListTimezones() ([]string, error) {
	all := make(map[string]bool)

	err := filepath.WalkDir(zoneinfoBase, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}

		rel, _ := filepath.Rel(zoneinfoBase, path)
		if rel == "." {
			return nil
		}

		// Skip known non-timezone directories
		parts := strings.SplitN(rel, "/", 2)
		if skipDirs[parts[0]] {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip known non-timezone files
		if skipFiles[d.Name()] {
			return nil
		}

		// Skip directories (we want leaf timezone files)
		if d.IsDir() {
			return nil
		}

		// Verify the file looks like a timezone file (binary TZif format)
		f, ferr := os.Open(path)
		if ferr != nil {
			return nil
		}
		header := make([]byte, 4)
		n, _ := f.Read(header)
		f.Close()
		if n < 4 || string(header) != "TZif" {
			return nil
		}

		all[rel] = true
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk zoneinfo: %w", err)
	}

	// Build result: common timezones first (if they exist), then the rest
	seen := make(map[string]bool)
	var result []string

	for _, tz := range commonTimezones {
		if all[tz] {
			result = append(result, tz)
			seen[tz] = true
		}
	}

	// Add remaining timezones sorted
	var remaining []string
	for tz := range all {
		if !seen[tz] {
			remaining = append(remaining, tz)
		}
	}
	sort.Strings(remaining)
	result = append(result, remaining...)

	return result, nil
}

// GetTimezone returns the current system timezone by reading /etc/timezone
// or resolving /etc/localtime. Defaults to "UTC" if undetermined.
func GetTimezone() string {
	// Try /etc/timezone first
	data, err := os.ReadFile("/etc/timezone")
	if err == nil {
		tz := strings.TrimSpace(string(data))
		if tz != "" {
			return tz
		}
	}

	// Try resolving /etc/localtime symlink
	target, err := os.Readlink("/etc/localtime")
	if err == nil {
		if rel, err := filepath.Rel(zoneinfoBase, target); err == nil {
			return rel
		}
	}

	return "UTC"
}

// SetTimezone sets the system timezone by creating a symlink from
// /etc/localtime to /usr/share/zoneinfo/<tz> and writing /etc/timezone.
func SetTimezone(tz string) error {
	if tz == "" {
		return fmt.Errorf("timezone cannot be empty")
	}

	// Validate that the timezone file exists
	tzPath := filepath.Join(zoneinfoBase, tz)

	// Prevent directory traversal
	resolved, err := filepath.Abs(tzPath)
	if err != nil || !strings.HasPrefix(resolved, zoneinfoBase) {
		return fmt.Errorf("invalid timezone %q", tz)
	}

	info, err := os.Stat(tzPath)
	if err != nil || info.IsDir() {
		return fmt.Errorf("unknown timezone %q", tz)
	}

	// Validate TZif magic header (consistent with ListTimezones)
	f, err := os.Open(tzPath)
	if err != nil {
		return fmt.Errorf("unknown timezone %q", tz)
	}
	header := make([]byte, 4)
	n, _ := f.Read(header)
	f.Close()
	if n < 4 || string(header) != "TZif" {
		return fmt.Errorf("invalid timezone file %q: not a TZif file", tz)
	}

	// Remove existing /etc/localtime and create symlink
	os.Remove("/etc/localtime")
	if err := os.Symlink(tzPath, "/etc/localtime"); err != nil {
		return fmt.Errorf("create localtime symlink: %w", err)
	}

	// Write /etc/timezone
	if err := os.WriteFile("/etc/timezone", []byte(tz+"\n"), 0644); err != nil {
		return fmt.Errorf("write /etc/timezone: %w", err)
	}

	return nil
}
