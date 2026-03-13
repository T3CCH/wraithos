package api

import (
	"log"
	"net/http"
	"os/exec"
	"time"

	"github.com/wraithos/wraith-ui/internal/setup"
)

// --- Setup status ---

type diskStatusResponse struct {
	ConfigDisk     setup.MountStatus  `json:"configDisk"`
	CacheDisk      setup.MountStatus  `json:"cacheDisk"`
	NeedsDiskSetup bool               `json:"needsDiskSetup"`
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
		AvailableDisks: disks,
	})
}

// --- Disk setup ---

type diskSetupRequest struct {
	ConfigDisk   string `json:"configDisk"`
	CacheDisk    string `json:"cacheDisk"`
	ConfirmFormat bool  `json:"confirmFormat"`
}

type diskSetupResponse struct {
	Status             string                  `json:"status"`
	ConfigDisk         *setup.DiskSetupResult  `json:"configDisk,omitempty"`
	CacheDisk          *setup.DiskSetupResult  `json:"cacheDisk,omitempty"`
	RebootRecommended  bool                    `json:"rebootRecommended"`
	HotRemountSuccess  bool                    `json:"hotRemountSuccess"`
	MigratedFiles      []string                `json:"migratedFiles"`
}

// handleSetupDisks formats and mounts disks (POST /api/setup/disks).
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

	if req.ConfigDisk == "" && req.CacheDisk == "" {
		writeError(w, http.StatusBadRequest, "at least one disk must be specified")
		return
	}

	// Validate that config and cache are not the same device
	if req.ConfigDisk != "" && req.CacheDisk != "" && req.ConfigDisk == req.CacheDisk {
		writeError(w, http.StatusBadRequest, "config and cache must be different disks")
		return
	}

	// Prevent concurrent disk setup requests
	if !setup.AcquireSetup() {
		writeError(w, http.StatusConflict, "disk setup is already in progress")
		return
	}
	defer setup.ReleaseSetup()

	resp := diskSetupResponse{
		MigratedFiles: []string{},
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

	resp.Status = "complete"
	resp.HotRemountSuccess = true
	resp.RebootRecommended = true

	writeOK(w, resp)
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

// handleReboot triggers a system reboot with a 3-second delay
// (POST /api/system/reboot).
func (s *Server) handleReboot(w http.ResponseWriter, r *http.Request) {
	s.Logs.Info("system", "reboot requested via API")

	// Send response before rebooting
	writeOK(w, map[string]interface{}{
		"status": "rebooting",
		"delay":  3,
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
