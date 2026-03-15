package api

import (
	"log"
	"net/http"
	"os/exec"
	"time"

	"github.com/wraithos/wraith-ui/internal/setup"
	"github.com/wraithos/wraith-ui/internal/storage"
)

// --- Setup status ---

type diskStatusResponse struct {
	ConfigDisk     setup.MountStatus   `json:"configDisk"`
	CacheDisk      setup.MountStatus   `json:"cacheDisk"`
	NeedsDiskSetup bool                `json:"needsDiskSetup"`
	SingleDiskMode bool                `json:"singleDiskMode"`
	AvailableDisks []setup.BlockDevice `json:"availableDisks"`
}

// handleSetupStatus returns the current disk setup state (GET /api/setup/status).
func (s *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	config, cache := setup.GetDiskStatus()

	disks, err := setup.DetectDisks()
	if err != nil {
		s.Logs.Warn("setup", "failed to detect disks: %v", err)
		disks = []setup.BlockDevice{}
	}

	writeOK(w, diskStatusResponse{
		ConfigDisk:     config,
		CacheDisk:      cache,
		NeedsDiskSetup: !config.Persistent || !cache.Persistent,
		SingleDiskMode: setup.IsSingleDiskMode(),
		AvailableDisks: disks,
	})
}

// --- Disk setup ---

type diskSetupRequest struct {
	Mode         string `json:"mode"`         // "dual" (default) or "single"
	ConfigDisk   string `json:"configDisk"`
	CacheDisk    string `json:"cacheDisk"`
	SingleDisk   string `json:"singleDisk"`   // device for single-disk mode
	ConfirmFormat bool  `json:"confirmFormat"`
}

type diskSetupResponse struct {
	Status             string                  `json:"status"`
	Mode               string                  `json:"mode"`
	ConfigDisk         *setup.DiskSetupResult  `json:"configDisk,omitempty"`
	CacheDisk          *setup.DiskSetupResult  `json:"cacheDisk,omitempty"`
	SingleDisk         *setup.DiskSetupResult  `json:"singleDisk,omitempty"`
	RebootRecommended  bool                    `json:"rebootRecommended"`
	HotRemountSuccess  bool                    `json:"hotRemountSuccess"`
	MigratedFiles      []string                `json:"migratedFiles"`
}

// handleSetupDisks formats and mounts disks (POST /api/setup/disks).
// Supports two modes via the "mode" field:
//   - "dual" (default): separate config and cache disks (original behavior)
//   - "single": one disk serves both config and cache via subdirectories
func (s *Server) handleSetupDisks(w http.ResponseWriter, r *http.Request) {
	var req diskSetupRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if !req.ConfirmFormat {
		writeError(w, http.StatusBadRequest, "confirmFormat must be true to proceed with disk setup")
		return
	}

	// Default to dual mode for backwards compatibility
	mode := req.Mode
	if mode == "" {
		mode = "dual"
	}

	if mode != "dual" && mode != "single" {
		writeError(w, http.StatusBadRequest, "mode must be \"dual\" or \"single\"")
		return
	}

	// Prevent concurrent disk setup requests
	if !setup.AcquireSetup() {
		writeError(w, http.StatusConflict, "disk setup is already in progress")
		return
	}
	defer setup.ReleaseSetup()

	resp := diskSetupResponse{
		Mode:          mode,
		MigratedFiles: []string{},
	}

	if mode == "single" {
		// Single-disk mode: one device for both config and cache
		if req.SingleDisk == "" {
			writeError(w, http.StatusBadRequest, "singleDisk must be specified for single disk mode")
			return
		}

		result, migrated, err := setup.SetupSingleDisk(req.SingleDisk)
		if err != nil {
			s.Logs.Error("setup", "single disk setup failed: %v", err)
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		resp.SingleDisk = result
		resp.MigratedFiles = append(resp.MigratedFiles, migrated...)
		s.Logs.Info("setup", "single disk %s: %s", req.SingleDisk, result.Action)
	} else {
		// Dual-disk mode: separate config and cache (original behavior)
		if req.ConfigDisk == "" && req.CacheDisk == "" {
			writeError(w, http.StatusBadRequest, "at least one disk must be specified")
			return
		}

		// Validate that config and cache are not the same device
		if req.ConfigDisk != "" && req.CacheDisk != "" && req.ConfigDisk == req.CacheDisk {
			writeError(w, http.StatusBadRequest, "config and cache must be different disks")
			return
		}

		// Set up config disk first
		if req.ConfigDisk != "" {
			result, migrated, err := setup.SetupDisk(req.ConfigDisk, setup.ConfigLabel)
			if err != nil {
				s.Logs.Error("setup", "config disk setup failed: %v", err)
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			resp.ConfigDisk = result
			resp.MigratedFiles = append(resp.MigratedFiles, migrated...)
			s.Logs.Info("setup", "config disk %s: %s", req.ConfigDisk, result.Action)
		}

		// Set up cache disk
		if req.CacheDisk != "" {
			result, migrated, err := setup.SetupDisk(req.CacheDisk, setup.CacheLabel)
			if err != nil {
				s.Logs.Error("setup", "cache disk setup failed: %v", err)
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			resp.CacheDisk = result
			resp.MigratedFiles = append(resp.MigratedFiles, migrated...)
			s.Logs.Info("setup", "cache disk %s: %s", req.CacheDisk, result.Action)
		}
	}

	resp.Status = "complete"
	resp.HotRemountSuccess = true
	resp.RebootRecommended = true

	writeOK(w, resp)
}

// --- Disk wipe ---

type diskWipeRequest struct {
	DiskType    string `json:"diskType"`    // "config", "cache", or "single"
	ConfirmWipe bool   `json:"confirmWipe"` // must be true
}

// handleDiskWipe reformats a wraith disk, erasing all data (POST /api/setup/wipe).
func (s *Server) handleDiskWipe(w http.ResponseWriter, r *http.Request) {
	var req diskWipeRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if !req.ConfirmWipe {
		writeError(w, http.StatusBadRequest, "confirmWipe must be true to proceed")
		return
	}

	if req.DiskType != "config" && req.DiskType != "cache" && req.DiskType != "single" {
		writeError(w, http.StatusBadRequest, "diskType must be \"config\", \"cache\", or \"single\"")
		return
	}

	// Prevent concurrent disk operations
	if !setup.AcquireSetup() {
		writeError(w, http.StatusConflict, "a disk operation is already in progress")
		return
	}
	defer setup.ReleaseSetup()

	s.Logs.Info("setup", "wiping %s disk", req.DiskType)

	if err := setup.WipeDisk(req.DiskType); err != nil {
		s.Logs.Error("setup", "wipe %s disk failed: %v", req.DiskType, err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.Logs.Info("setup", "%s disk wiped successfully", req.DiskType)
	writeOK(w, map[string]interface{}{
		"status":            "wiped",
		"diskType":          req.DiskType,
		"rebootRecommended": true,
	})
}

// --- Rescan ---

type rescanResponse struct {
	AvailableDisks []setup.BlockDevice `json:"availableDisks"`
}

// handleSetupRescan re-scans for block devices (POST /api/setup/rescan).
func (s *Server) handleSetupRescan(w http.ResponseWriter, r *http.Request) {
	disks, err := setup.DetectDisks()
	if err != nil {
		s.Logs.Warn("setup", "failed to rescan disks: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to scan for disks")
		return
	}
	if disks == nil {
		disks = []setup.BlockDevice{}
	}

	writeOK(w, rescanResponse{AvailableDisks: disks})
}

// --- Timezone ---

type timezoneGetResponse struct {
	Timezone  string   `json:"timezone"`
	Available []string `json:"available"`
}

type timezoneSetRequest struct {
	Timezone string `json:"timezone"`
}

// handleTimezoneGet returns the current timezone and available list
// (GET /api/system/timezone).
func (s *Server) handleTimezoneGet(w http.ResponseWriter, r *http.Request) {
	current := setup.GetTimezone()

	available, err := setup.ListTimezones()
	if err != nil {
		s.Logs.Warn("setup", "failed to list timezones: %v", err)
		available = []string{"UTC"}
	}

	writeOK(w, timezoneGetResponse{
		Timezone:  current,
		Available: available,
	})
}

// handleTimezoneSet sets the system timezone (PUT /api/system/timezone).
func (s *Server) handleTimezoneSet(w http.ResponseWriter, r *http.Request) {
	var req timezoneSetRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Timezone == "" {
		writeError(w, http.StatusBadRequest, "timezone is required")
		return
	}

	if err := setup.SetTimezone(req.Timezone); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.Logs.Info("system", "timezone set to %s", req.Timezone)
	writeOK(w, map[string]string{"status": "applied", "timezone": req.Timezone})
}

// --- Reboot ---

// handleReboot syncs config to the persistent disk then triggers a system
// reboot with a 3-second delay (POST /api/system/reboot).
func (s *Server) handleReboot(w http.ResponseWriter, r *http.Request) {
	s.Logs.Info("system", "reboot requested via API -- syncing config to disk")

	// Sync all config files to the physical config disk before rebooting.
	// This ensures nothing in the RAM config is lost on reboot.
	if err := storage.SyncAll(); err != nil {
		s.Logs.Error("system", "pre-reboot config sync failed: %v", err)
		writeError(w, http.StatusInternalServerError,
			"failed to save config before reboot: "+err.Error())
		return
	}
	s.Logs.Info("system", "config synced to disk, proceeding with reboot")

	// Send response before rebooting
	writeOK(w, map[string]interface{}{
		"status":     "rebooting",
		"delay":      3,
		"configSaved": true,
	})

	// Schedule reboot after a short delay so the HTTP response can be sent
	go func() {
		time.Sleep(3 * time.Second)
		log.Printf("executing system reboot")
		cmd := exec.Command("reboot")
		if output, err := cmd.CombinedOutput(); err != nil {
			log.Printf("reboot failed: %s: %v", string(output), err)
		}
	}()
}

// --- Disk expand ---

type expandableDisksResponse struct {
	Disks []setup.DiskExpandInfo `json:"disks"`
}

// handleExpandableDisks returns disks that can be expanded
// (GET /api/system/disks/expandable).
func (s *Server) handleExpandableDisks(w http.ResponseWriter, r *http.Request) {
	disks := setup.GetExpandableDisks()
	if disks == nil {
		disks = []setup.DiskExpandInfo{}
	}
	writeOK(w, expandableDisksResponse{Disks: disks})
}

type expandDiskRequest struct {
	Device string `json:"device"`
}

// handleExpandDisk runs resize2fs on a wraith-managed disk
// (POST /api/system/disks/expand).
func (s *Server) handleExpandDisk(w http.ResponseWriter, r *http.Request) {
	var req expandDiskRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Device == "" {
		writeError(w, http.StatusBadRequest, "device is required")
		return
	}

	// Prevent concurrent disk operations
	if !setup.AcquireSetup() {
		writeError(w, http.StatusConflict, "a disk operation is already in progress")
		return
	}
	defer setup.ReleaseSetup()

	s.Logs.Info("system", "expanding filesystem on %s", req.Device)

	if err := setup.ExpandDisk(req.Device); err != nil {
		s.Logs.Error("system", "expand %s failed: %v", req.Device, err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.Logs.Info("system", "filesystem on %s expanded successfully", req.Device)
	writeOK(w, map[string]string{
		"status": "expanded",
		"device": req.Device,
	})
}
