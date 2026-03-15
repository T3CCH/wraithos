// WraithOS Web Management Server
//
// Single-binary web management interface for WraithOS.
// Serves the embedded frontend and exposes a REST API for
// managing Docker containers, Samba mounts, network config, and system stats.
package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	wraithui "github.com/wraithos/wraith-ui"
	"github.com/wraithos/wraith-ui/internal/api"
	"github.com/wraithos/wraith-ui/internal/auth"
	"github.com/wraithos/wraith-ui/internal/docker"
	"github.com/wraithos/wraith-ui/internal/storage"
	"github.com/wraithos/wraith-ui/internal/system"
)

var version = "0.3.2"

func main() {
	port := flag.Int("port", 82, "HTTP listen port")
	configDir := flag.String("config-dir", "/wraith/config", "Path to config disk mount point (RAM)")
	configDiskDir := flag.String("config-disk-dir", "/wraith/config-disk", "Path to physical config disk for sync-back")
	cacheDir := flag.String("cache-dir", "/wraith/cache", "Path to cache disk mount point")
	tlsCert := flag.String("tls-cert", "", "Path to TLS certificate file (optional)")
	tlsKey := flag.String("tls-key", "", "Path to TLS key file (optional)")
	flag.Parse()

	// Apply path overrides before any other initialization
	storage.ConfigBase = *configDir
	storage.ConfigDiskDir = *configDiskDir
	storage.CacheDisk = *cacheDir

	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Printf("WraithOS Management Server v%s starting", version)

	// Initialize log collector (1000-entry ring buffer)
	logCollector := system.NewLogCollector(1000)
	logCollector.Info("system", "WraithOS v%s starting", version)

	// Initialize auth manager
	authMgr := auth.NewManager()
	authMgr.SecureCookies = *tlsCert != "" && *tlsKey != ""
	if authMgr.NeedsSetup() {
		logCollector.Info("auth", "first boot detected, setup required")
	}

	// Initialize Docker client
	dockerClient, err := docker.NewClient()
	if err != nil {
		log.Printf("WARNING: Docker client init failed: %v (Docker features will be unavailable)", err)
		logCollector.Warn("docker", "client init failed: %v", err)
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := dockerClient.Ping(ctx); err != nil {
			log.Printf("WARNING: Docker daemon unreachable: %v", err)
			logCollector.Warn("docker", "daemon unreachable: %v", err)
		} else {
			logCollector.Info("docker", "connected to Docker daemon")
		}
		cancel()
	}

	// Initialize compose manager
	composeMgr := docker.NewComposeManager()

	// Initialize Samba manager
	sambaMgr := system.NewSambaManager()

	// Start SSH if it was enabled before reboot
	system.StartSSHIfEnabled()
	logCollector.Info("system", "SSH auto-start check complete")

	// Prepare embedded static filesystem
	// The embed.FS has paths like "web/static/index.html", so we strip the prefix.
	staticFS, err := fs.Sub(wraithui.StaticFiles, "web/static")
	if err != nil {
		log.Fatalf("failed to create static filesystem: %v", err)
	}

	// Create API server
	srv := api.NewServer(
		authMgr,
		dockerClient,
		composeMgr,
		sambaMgr,
		logCollector,
		version,
		http.FS(staticFS),
	)

	addr := fmt.Sprintf(":%d", *port)
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      srv.Handler(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start session cleanup goroutine
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			authMgr.CleanExpiredSessions()
			authMgr.CleanLoginAttempts()
		}
	}()

	// Start server
	go func() {
		if *tlsCert != "" && *tlsKey != "" {
			httpServer.TLSConfig = &tls.Config{
				MinVersion: tls.VersionTLS12,
			}
			log.Printf("listening on https://0.0.0.0%s", addr)
			logCollector.Info("system", "HTTPS server listening on %s", addr)
			if err := httpServer.ListenAndServeTLS(*tlsCert, *tlsKey); err != http.ErrServerClosed {
				log.Fatalf("HTTPS server error: %v", err)
			}
		} else {
			log.Printf("listening on http://0.0.0.0%s", addr)
			logCollector.Info("system", "HTTP server listening on %s", addr)
			if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
				log.Fatalf("HTTP server error: %v", err)
			}
		}
	}()

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh

	log.Printf("received %v, shutting down", sig)
	logCollector.Info("system", "shutting down (signal: %v)", sig)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("shutdown error: %v", err)
	}

	if dockerClient != nil {
		dockerClient.Close()
	}

	log.Println("server stopped")
}
