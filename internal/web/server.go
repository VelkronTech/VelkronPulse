// Package web provides the HTTP server, REST API handlers, and static file serving
// for the Velkron Pulse dashboard.
package web

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"github.com/velkron/pulse/internal/config"
	"github.com/velkron/pulse/internal/metrics"
	"github.com/velkron/pulse/internal/services"
	"github.com/velkron/pulse/internal/store"
)

const maxMetricsHistoryWindow = 24 * time.Hour

var allowedSettings = map[string]func(string) error{
	"disk_threshold": validateThresholdSetting,
	"cpu_threshold":  validateThresholdSetting,
}

// Server wraps the HTTP server, router, and references to other components.
type Server struct {
	router     *mux.Router
	server     *http.Server
	hub        *Hub
	collector  *metrics.Collector
	scanner    *services.Scanner
	store      *store.Store
	embeddedFS fs.FS
	bindAddr   string
	port       int
	token      string
	limiter    *rateLimiter
}

// NewServer creates a new Server with all routes registered.
func NewServer(
	bindAddr string,
	port int,
	token string,
	hub *Hub,
	collector *metrics.Collector,
	scanner *services.Scanner,
	dataStore *store.Store,
	embeddedFS fs.FS,
) *Server {
	s := &Server{
		router:     mux.NewRouter(),
		hub:        hub,
		collector:  collector,
		scanner:    scanner,
		store:      dataStore,
		embeddedFS: embeddedFS,
		bindAddr:   bindAddr,
		port:       port,
		token:      token,
		limiter:    newRateLimiter(),
	}

	hub.SetAuth(token)
	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	s.router.Use(s.securityHeadersMiddleware)

	// Public health check (no sensitive data).
	s.router.HandleFunc("/api/health", s.handleHealth).Methods("GET")

	api := s.router.PathPrefix("/api").Subrouter()
	api.Use(s.rateLimitMiddleware)
	api.Use(s.authMiddleware)
	api.HandleFunc("/status", s.handleStatus).Methods("GET")
	api.HandleFunc("/info", s.handleInfo).Methods("GET")
	api.HandleFunc("/endpoints", s.handleListEndpoints).Methods("GET")
	api.HandleFunc("/endpoints", s.handleAddEndpoint).Methods("POST")
	api.HandleFunc("/endpoints/{id}", s.handleDeleteEndpoint).Methods("DELETE")
	api.HandleFunc("/settings", s.handleGetSettings).Methods("GET")
	api.HandleFunc("/settings", s.handleUpdateSetting).Methods("PUT")
	api.HandleFunc("/metrics/history", s.handleMetricsHistory).Methods("GET")
	api.HandleFunc("/export/json", s.handleExportJSON).Methods("GET")
	api.HandleFunc("/export/csv", s.handleExportCSV).Methods("GET")
	api.HandleFunc("/metrics/prometheus", s.handlePrometheusMetrics).Methods("GET")

	s.router.HandleFunc("/ws", s.hub.HandleWebSocket)

	s.router.HandleFunc("/", s.handleIndex).Methods("GET")
	s.router.HandleFunc("/index.html", s.handleIndex).Methods("GET")

	fileServer := http.FileServer(http.FS(s.embeddedFS))
	s.router.PathPrefix("/").Handler(fileServer)
}

// Start begins listening on the configured address.
func (s *Server) Start() error {
	addr := fmt.Sprintf("%s:%d", s.bindAddr, s.port)
	s.server = &http.Server{
		Addr:              addr,
		Handler:           s.router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	log.Printf("[server] listening on http://%s", addr)
	return s.server.ListenAndServe()
}

// Shutdown gracefully shuts down the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.server == nil {
		return nil
	}
	return s.server.Shutdown(ctx)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	data, err := fs.ReadFile(s.embeddedFS, "index.html")
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	tokenJSON, _ := json.Marshal(s.token)
	versionJSON, _ := json.Marshal(config.Version)
	injection := fmt.Sprintf("<script>window.__PULSE_TOKEN__=%s;window.__PULSE_VERSION__=%s;</script>\n", tokenJSON, versionJSON)
	body := strings.Replace(string(data), "</head>", injection+"</head>", 1)

	http.SetCookie(w, &http.Cookie{
		Name:     "pulse_token",
		Value:    s.token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(body))
}

// --- API Handlers ---

type statusResponse struct {
	Metrics  metrics.MetricsSnapshot  `json:"metrics"`
	Services []services.ServiceStatus `json:"services"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	servicesList := s.enrichServicesWithUptime(s.scanner.GetServices())
	resp := statusResponse{
		Metrics:  s.collector.GetLatest(),
		Services: servicesList,
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"version": config.Version,
		"bind":    s.bindAddr,
		"port":    s.port,
	})
}

func (s *Server) enrichServicesWithUptime(list []services.ServiceStatus) []services.ServiceStatus {
	stats, err := s.store.GetEndpointUptimeStats()
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

func (s *Server) handleListEndpoints(w http.ResponseWriter, r *http.Request) {
	endpoints, err := s.store.ListEndpoints()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list endpoints")
		return
	}
	if endpoints == nil {
		endpoints = []store.CustomEndpoint{}
	}
	writeJSON(w, http.StatusOK, endpoints)
}

type addEndpointRequest struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Type string `json:"type"`
}

func (s *Server) handleAddEndpoint(w http.ResponseWriter, r *http.Request) {
	limitRequestBody(w, r)

	var req addEndpointRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.URL = strings.TrimSpace(req.URL)
	if req.Type == "" {
		req.Type = "http"
	}

	if err := services.ValidateEndpointInput(req.Name, req.Type, req.URL); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	id, err := s.store.AddEndpoint(req.Name, req.URL, req.Type)
	if err != nil {
		if errors.Is(err, store.ErrEndpointLimit) {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("Maximum of %d custom endpoints reached", store.MaxCustomEndpoints))
			return
		}
		writeError(w, http.StatusInternalServerError, "Failed to add endpoint")
		return
	}

	s.scanner.Rescan()
	writeJSON(w, http.StatusCreated, map[string]int64{"id": id})
}

func (s *Server) handleDeleteEndpoint(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid endpoint ID")
		return
	}

	if err := s.store.DeleteEndpoint(id); err != nil {
		if errors.Is(err, store.ErrEndpointNotFound) {
			writeError(w, http.StatusNotFound, "Endpoint not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "Failed to delete endpoint")
		return
	}

	s.scanner.Rescan()
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.store.GetAllSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get settings")
		return
	}
	filtered := make(map[string]string, len(allowedSettings))
	for key := range allowedSettings {
		if value, ok := settings[key]; ok {
			filtered[key] = value
		}
	}
	writeJSON(w, http.StatusOK, filtered)
}

type updateSettingRequest struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func (s *Server) handleUpdateSetting(w http.ResponseWriter, r *http.Request) {
	limitRequestBody(w, r)

	var req updateSettingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	req.Key = strings.TrimSpace(req.Key)
	req.Value = strings.TrimSpace(req.Value)
	if req.Key == "" {
		writeError(w, http.StatusBadRequest, "Key is required")
		return
	}

	validate, ok := allowedSettings[req.Key]
	if !ok {
		writeError(w, http.StatusBadRequest, "Unknown setting key")
		return
	}
	if len(req.Value) > 16 {
		writeError(w, http.StatusBadRequest, "Value too long")
		return
	}
	if err := validate(req.Value); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.store.SetSetting(req.Key, req.Value); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update setting")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (s *Server) handleMetricsHistory(w http.ResponseWriter, r *http.Request) {
	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")

	to := time.Now()
	from := to.Add(-1 * time.Hour)

	if fromStr != "" {
		if t, err := time.Parse(time.RFC3339, fromStr); err == nil {
			from = t
		}
	}
	if toStr != "" {
		if t, err := time.Parse(time.RFC3339, toStr); err == nil {
			to = t
		}
	}

	if to.Before(from) {
		writeError(w, http.StatusBadRequest, "Invalid time range")
		return
	}
	if to.Sub(from) > maxMetricsHistoryWindow {
		from = to.Add(-maxMetricsHistoryWindow)
	}

	snapshots, err := s.store.GetMetricsHistory(from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get metrics history")
		return
	}
	if snapshots == nil {
		snapshots = []store.MetricsSnapshot{}
	}

	writeJSON(w, http.StatusOK, snapshots)
}

func (s *Server) handleExportJSON(w http.ResponseWriter, r *http.Request) {
	resp := statusResponse{
		Metrics:  s.collector.GetLatest(),
		Services: s.enrichServicesWithUptime(s.scanner.GetServices()),
	}

	data, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to marshal JSON")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=velkron-pulse-snapshot.json")
	w.Write(data)
}

func (s *Server) handleExportCSV(w http.ResponseWriter, r *http.Request) {
	metrics := s.collector.GetLatest()

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=velkron-pulse-snapshot.csv")

	writer := csv.NewWriter(w)
	defer writer.Flush()

	writer.Write([]string{"timestamp", "cpu_percent", "mem_total", "mem_used", "mem_percent", "uptime"})
	writer.Write([]string{
		metrics.Timestamp.Format(time.RFC3339),
		fmt.Sprintf("%.2f", metrics.CPU.Percent),
		fmt.Sprintf("%d", metrics.Memory.Total),
		fmt.Sprintf("%d", metrics.Memory.Used),
		fmt.Sprintf("%.2f", metrics.Memory.Percent),
		fmt.Sprintf("%d", metrics.Uptime),
	})

	for _, d := range metrics.Disks {
		writer.Write([]string{
			"disk:" + d.MountPoint,
			fmt.Sprintf("%.2f", d.Percent),
			fmt.Sprintf("%d", d.Total),
			fmt.Sprintf("%d", d.Used),
			fmt.Sprintf("%d", d.Free),
			"",
		})
	}

	for _, n := range metrics.Networks {
		writer.Write([]string{
			"net:" + n.Name,
			fmt.Sprintf("%d", n.BytesSent),
			fmt.Sprintf("%d", n.BytesRecv),
			"", "", "",
		})
	}
}

func validateThresholdSetting(value string) error {
	n, err := strconv.Atoi(value)
	if err != nil || n < 1 || n > 100 {
		return fmt.Errorf("threshold must be between 1 and 100")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": html.EscapeString(message)})
}
