// Velkron Pulse — Lightweight infrastructure monitoring agent with embedded web dashboard.
//
// Usage:
//
//	velkron-pulse [--port 2024] [--bind 127.0.0.1] [--db-path ~/.velkron-pulse/] [--refresh 2] [--no-browser] [--token TOKEN] [--version]
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/velkron/pulse/internal/config"
	"github.com/velkron/pulse/internal/logging"
	"github.com/velkron/pulse/internal/metrics"
	"github.com/velkron/pulse/internal/services"
	"github.com/velkron/pulse/internal/store"
	"github.com/velkron/pulse/internal/web"
)

func main() {
	// 1. Parse configuration
	cfg, err := config.Parse()
	if err != nil {
		logging.Logger.Error("failed to parse config", "error", err)
		os.Exit(1)
	}

	if cfg.ShowVersion {
		fmt.Printf("Velkron Pulse v%s\n", config.Version)
		os.Exit(0)
	}

	logging.Logger.Info("starting",
		"version", config.Version,
		"bind", cfg.BindAddress,
		"port", cfg.Port,
		"refresh", cfg.RefreshInterval,
		"no_browser", cfg.NoBrowser,
		"token", config.MaskToken(cfg.Token),
	)
	if cfg.ExposedToNetwork() {
		logging.Logger.Warn("listening on a non-loopback address — use firewall rules and a strong token")
	}

	// 2. Initialize SQLite store
	dataStore, err := store.New(cfg.DBFilePath())
	if err != nil {
		logging.Logger.Error("database init failed", "error", err)
		os.Exit(1)
	}
	defer dataStore.Close()
	logging.Logger.Info("database initialized")

	// 3. Start metrics collector
	collector := metrics.New(time.Duration(cfg.RefreshInterval) * time.Second)
	collector.Start()
	defer collector.Stop()
	logging.Logger.Info("metrics collector started", "interval_sec", cfg.RefreshInterval)

	recorder := func(endpointID int64, status string, responseTime time.Duration) {
		if err := dataStore.RecordEndpointCheck(endpointID, status, responseTime); err != nil {
			logging.Logger.Warn("failed to record endpoint check", "endpoint_id", endpointID, "error", err)
		}
	}

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
	}, recorder)
	scanner.Start()
	defer scanner.Stop()
	logging.Logger.Info("service scanner started")

	// 5. Start WebSocket hub
	hub := web.NewHub()
	go hub.Run()

	// 6. Start HTTP server with embedded frontend
	embeddedFS := embeddedFileSystem()
	server := web.NewServer(cfg.BindAddress, cfg.Port, cfg.Token, hub, collector, scanner, dataStore, embeddedFS)

	if !cfg.NoBrowser {
		go func() {
			time.Sleep(1 * time.Second)
			openBrowser(fmt.Sprintf("http://127.0.0.1:%d", cfg.Port))
		}()
	}

	// 7. Periodic DB save and WebSocket broadcast
	go func() {
		saveTicker := time.NewTicker(30 * time.Second)
		broadcastTicker := time.NewTicker(time.Duration(cfg.RefreshInterval) * time.Second)
		defer saveTicker.Stop()
		defer broadcastTicker.Stop()

		for {
			select {
			case <-saveTicker.C:
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
					logging.Logger.Warn("failed to save metrics", "error", err)
				}
				if err := dataStore.PruneOldMetrics(); err != nil {
					logging.Logger.Warn("failed to prune metrics", "error", err)
				}
				if err := dataStore.PruneOldEndpointChecks(); err != nil {
					logging.Logger.Warn("failed to prune endpoint checks", "error", err)
				}

			case <-broadcastTicker.C:
				snapshot := collector.GetLatest()
				servicesList := enrichServicesUptime(scanner.GetServices(), dataStore)
				payload := map[string]interface{}{
					"metrics":  snapshot,
					"services": servicesList,
				}
				data, err := json.Marshal(payload)
				if err != nil {
					logging.Logger.Warn("failed to marshal broadcast payload", "error", err)
					continue
				}
				hub.Broadcast(data)
			}
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := server.Start(); err != nil {
			logging.Logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	sig := <-sigCh
	logging.Logger.Info("shutting down", "signal", sig.String())
	signal.Stop(sigCh)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		logging.Logger.Warn("http shutdown error", "error", err)
	}

	collector.Stop()
	scanner.Stop()
	dataStore.Close()
	logging.Logger.Info("stopped")
}

func enrichServicesUptime(list []services.ServiceStatus, dataStore *store.Store) []services.ServiceStatus {
	stats, err := dataStore.GetEndpointUptimeStats()
	if err != nil || len(stats) == 0 {
		return list
	}
	out := make([]services.ServiceStatus, len(list))
	copy(out, list)
	for i := range out {
		if !out[i].IsCustom || out[i].EndpointID == 0 {
			continue
		}
		if stat, ok := stats[out[i].EndpointID]; ok {
			out[i].UptimePercent = stat.UptimePercent
			out[i].ChecksTotal = stat.TotalChecks
		}
	}
	return out
}

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
	default:
		cmd = "xdg-open"
		args = []string{url}
	}

	if err := exec.Command(cmd, args...).Start(); err != nil {
		logging.Logger.Warn("failed to open browser", "error", err)
	} else {
		logging.Logger.Info("browser opened", "url", url)
	}
}
