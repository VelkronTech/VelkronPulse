// Velkron Pulse — Lightweight infrastructure monitoring agent with embedded web dashboard.
//
// Usage:
//
//	velkron-pulse [--port 2024] [--db-path ~/.velkron-pulse/] [--refresh 2] [--no-browser]
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/velkron/pulse/internal/config"
	"github.com/velkron/pulse/internal/metrics"
	"github.com/velkron/pulse/internal/services"
	"github.com/velkron/pulse/internal/store"
	"github.com/velkron/pulse/internal/web"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	// 1. Parse configuration
	cfg, err := config.Parse()
	if err != nil {
		log.Fatalf("Failed to parse config: %v", err)
	}

	// Handle --version
	if cfg.ShowVersion {
		fmt.Printf("Velkron Pulse v%s\n", config.Version)
		os.Exit(0)
	}

	log.Println("Velkron Pulse v" + config.Version + " starting...")
	log.Printf("Configuration: port=%d, db-path=%s, refresh=%ds, no-browser=%v",
		cfg.Port, cfg.DBPath, cfg.RefreshInterval, cfg.NoBrowser)

	// 2. Initialize SQLite store
	dataStore, err := store.New(cfg.DBFilePath())
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer dataStore.Close()
	log.Println("Database initialized at", cfg.DBFilePath())

	// 3. Start metrics collector
	collector := metrics.New(time.Duration(cfg.RefreshInterval) * time.Second)
	collector.Start()
	defer collector.Stop()
	log.Println("Metrics collector started (interval:", cfg.RefreshInterval, "s)")

	// 4. Start service scanner
	scanner := services.New(60*time.Second, func() ([]services.CustomEndpoint, error) {
		endpoints, err := dataStore.ListEndpoints()
		if err != nil {
			return nil, err
		}
		var custom []services.CustomEndpoint
		for _, ep := range endpoints {
			custom = append(custom, services.CustomEndpoint{
				ID:   ep.ID,
				Name: ep.Name,
				URL:  ep.URL,
				Type: ep.Type,
			})
		}
		return custom, nil
	})
	scanner.Start()
	defer scanner.Stop()
	log.Println("Service scanner started")

	// 5. Start WebSocket hub
	hub := web.NewHub()
	go hub.Run()
	log.Println("WebSocket hub started")

	// 6. Start HTTP server with embedded frontend
	embeddedFS := embeddedFileSystem()
	server := web.NewServer(cfg.Port, hub, collector, scanner, dataStore, embeddedFS)

	// 7. Auto-open browser (unless --no-browser)
	if !cfg.NoBrowser {
		go func() {
			time.Sleep(1 * time.Second)
			openBrowser("http://localhost:" + strconv.Itoa(cfg.Port))
		}()
	}

	// 8. Start periodic DB save (every 30 seconds) and broadcast loop
	go func() {
		saveTicker := time.NewTicker(30 * time.Second)
		broadcastTicker := time.NewTicker(time.Duration(cfg.RefreshInterval) * time.Second)
		defer saveTicker.Stop()
		defer broadcastTicker.Stop()

		for {
			select {
			case <-saveTicker.C:
				// Save metrics snapshot to DB
				snapshot := collector.GetLatest()
				diskJSON, _ := store.MarshalToJSON(snapshot.Disks)
				netJSON, _ := store.MarshalToJSON(snapshot.Networks)
				if err := dataStore.SaveMetrics(
					snapshot.CPU.Percent,
					snapshot.Memory.Total,
					snapshot.Memory.Used,
					snapshot.Memory.Percent,
					diskJSON,
					netJSON,
				); err != nil {
					log.Printf("Failed to save metrics: %v", err)
				}

				// Prune old metrics
				if err := dataStore.PruneOldMetrics(); err != nil {
					log.Printf("Failed to prune metrics: %v", err)
				}

			case <-broadcastTicker.C:
				// Broadcast current status to all WebSocket clients
				snapshot := collector.GetLatest()
				servicesList := scanner.GetServices()

				payload := map[string]interface{}{
					"metrics":  snapshot,
					"services": servicesList,
				}

				data, err := json.Marshal(payload)
				if err != nil {
					log.Printf("Failed to marshal broadcast payload: %v", err)
					continue
				}
				hub.Broadcast(data)
			}
		}
	}()

	// 9. Setup signal handling (must happen before server blocks)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// 10. Start server in background so we can handle signals
	go func() {
		if err := server.Start(); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// 11. Wait for shutdown signal
	sig := <-sigCh
	log.Printf("Received signal %v, shutting down gracefully...", sig)
	signal.Stop(sigCh)

	// Graceful HTTP shutdown with 10s timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("HTTP shutdown error: %v", err)
	}

	// Stop background components
	collector.Stop()
	scanner.Stop()
	dataStore.Close()
	log.Println("Velkron Pulse stopped.")
}

// openBrowser opens the default browser to the given URL.
func openBrowser(url string) {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	default: // linux and others
		cmd = "xdg-open"
		args = []string{url}
	}

	if err := exec.Command(cmd, args...).Start(); err != nil {
		log.Printf("Failed to open browser: %v", err)
	} else {
		log.Printf("Browser opened to %s", url)
	}
}
