// Velkron Pulse — Lightweight infrastructure monitoring agent with embedded web dashboard.
//
// Usage:
//   velkron-pulse [--port 2024] [--db-path ~/.velkron-pulse/] [--refresh 2] [--no-browser]
package main

import (
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
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
	log.Println("Velkron Pulse starting...")

	// 1. Parse configuration
	cfg, err := config.Parse()
	if err != nil {
		log.Fatalf("Failed to parse config: %v", err)
	}

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
			openBrowser("http://localhost:" + itoa(cfg.Port))
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
				diskJSON, _ := store.MarshalDisks(diskInfoToMap(snapshot.Disks))
				netJSON, _ := store.MarshalNetworks(netInfoToMap(snapshot.Networks))
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

	// 9. Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		log.Printf("Received signal %v, shutting down...", sig)
		collector.Stop()
		scanner.Stop()
		dataStore.Close()
		os.Exit(0)
	}()

	// 10. Start server (blocks)
	if err := server.Start(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

// diskInfoToMap converts []metrics.DiskInfo to []map[string]interface{} for JSON serialization.
func diskInfoToMap(disks []metrics.DiskInfo) []map[string]interface{} {
	result := make([]map[string]interface{}, len(disks))
	for i, d := range disks {
		result[i] = map[string]interface{}{
			"mount_point": d.MountPoint,
			"total":       d.Total,
			"used":        d.Used,
			"free":        d.Free,
			"percent":     d.Percent,
		}
	}
	return result
}

// netInfoToMap converts []metrics.NetworkInfo to []map[string]interface{} for JSON serialization.
func netInfoToMap(networks []metrics.NetworkInfo) []map[string]interface{} {
	result := make([]map[string]interface{}, len(networks))
	for i, n := range networks {
		result[i] = map[string]interface{}{
			"name":       n.Name,
			"bytes_sent": n.BytesSent,
			"bytes_recv": n.BytesRecv,
		}
	}
	return result
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

// itoa is a simple int to string conversion (avoids importing strconv in main).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}
