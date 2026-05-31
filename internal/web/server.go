// Package web provides the HTTP server, REST API handlers, and static file serving
// for the Velkron Pulse dashboard.
package web

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"

	"github.com/velkron/pulse/internal/metrics"
	"github.com/velkron/pulse/internal/services"
	"github.com/velkron/pulse/internal/store"
)

// Server wraps the HTTP server, router, and references to other components.
type Server struct {
	router   *mux.Router
	server   *http.Server
	hub      *Hub
	collector *metrics.Collector
	scanner  *services.Scanner
	store    *store.Store
	port     int
}

// NewServer creates a new Server with all routes registered.
func NewServer(
	port int,
	hub *Hub,
	collector *metrics.Collector,
	scanner *services.Scanner,
	dataStore *store.Store,
	embeddedFS fs.FS,
) *Server {
	s := &Server{
		router:    mux.NewRouter(),
		hub:       hub,
		collector: collector,
		scanner:   scanner,
		store:     dataStore,
		port:      port,
	}

	s.registerRoutes(embeddedFS)
	return s
}

// registerRoutes sets up all HTTP routes.
func (s *Server) registerRoutes(embeddedFS fs.FS) {
	// API routes
	api := s.router.PathPrefix("/api").Subrouter()
	api.HandleFunc("/status", s.handleStatus).Methods("GET")
	api.HandleFunc("/endpoints", s.handleListEndpoints).Methods("GET")
	api.HandleFunc("/endpoints", s.handleAddEndpoint).Methods("POST")
	api.HandleFunc("/endpoints/{id}", s.handleDeleteEndpoint).Methods("DELETE")
	api.HandleFunc("/settings", s.handleGetSettings).Methods("GET")
	api.HandleFunc("/settings", s.handleUpdateSetting).Methods("PUT")
	api.HandleFunc("/metrics/history", s.handleMetricsHistory).Methods("GET")
	api.HandleFunc("/export/json", s.handleExportJSON).Methods("GET")
	api.HandleFunc("/export/csv", s.handleExportCSV).Methods("GET")

	// WebSocket
	s.router.HandleFunc("/ws", s.hub.HandleWebSocket)

	// Static file server for embedded frontend
	fileServer := http.FileServer(http.FS(embeddedFS))
	s.router.PathPrefix("/").Handler(fileServer)
}

// Start begins listening on the configured port.
func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.port)
	s.server = &http.Server{
		Addr:    addr,
		Handler: s.router,
	}
	log.Printf("[server] listening on http://localhost%s", addr)
	return s.server.ListenAndServe()
}

// --- API Handlers ---

// statusResponse is the JSON structure returned by /api/status.
type statusResponse struct {
	Metrics  metrics.MetricsSnapshot   `json:"metrics"`
	Services []services.ServiceStatus  `json:"services"`
}

// handleStatus returns the current metrics and services snapshot.
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	resp := statusResponse{
		Metrics:  s.collector.GetLatest(),
		Services: s.scanner.GetServices(),
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleListEndpoints returns all custom endpoints from the database.
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

// addEndpointRequest is the JSON body for adding a custom endpoint.
type addEndpointRequest struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Type string `json:"type"`
}

// handleAddEndpoint adds a new custom endpoint.
func (s *Server) handleAddEndpoint(w http.ResponseWriter, r *http.Request) {
	var req addEndpointRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	if req.Name == "" || req.URL == "" {
		writeError(w, http.StatusBadRequest, "Name and URL are required")
		return
	}

	if req.Type == "" {
		req.Type = "http"
	}
	if req.Type != "http" && req.Type != "tcp" {
		writeError(w, http.StatusBadRequest, "Type must be 'http' or 'tcp'")
		return
	}

	id, err := s.store.AddEndpoint(req.Name, req.URL, req.Type)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to add endpoint")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]int64{"id": id})
}

// handleDeleteEndpoint removes a custom endpoint by ID.
func (s *Server) handleDeleteEndpoint(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid endpoint ID")
		return
	}

	if err := s.store.DeleteEndpoint(id); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to delete endpoint")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// handleGetSettings returns all settings.
func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.store.GetAllSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get settings")
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

// updateSettingRequest is the JSON body for updating a setting.
type updateSettingRequest struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// handleUpdateSetting updates a single setting.
func (s *Server) handleUpdateSetting(w http.ResponseWriter, r *http.Request) {
	var req updateSettingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	if req.Key == "" {
		writeError(w, http.StatusBadRequest, "Key is required")
		return
	}

	if err := s.store.SetSetting(req.Key, req.Value); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update setting")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// handleMetricsHistory returns historical metrics within a time range.
func (s *Server) handleMetricsHistory(w http.ResponseWriter, r *http.Request) {
	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")

	// Default: last hour
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

// handleExportJSON returns the current status as a downloadable JSON file.
func (s *Server) handleExportJSON(w http.ResponseWriter, r *http.Request) {
	resp := statusResponse{
		Metrics:  s.collector.GetLatest(),
		Services: s.scanner.GetServices(),
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

// handleExportCSV returns the current metrics as a downloadable CSV file.
func (s *Server) handleExportCSV(w http.ResponseWriter, r *http.Request) {
	metrics := s.collector.GetLatest()

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=velkron-pulse-snapshot.csv")

	writer := csv.NewWriter(w)
	defer writer.Flush()

	// Header
	writer.Write([]string{"timestamp", "cpu_percent", "mem_total", "mem_used", "mem_percent", "uptime"})

	// Row
	writer.Write([]string{
		metrics.Timestamp.Format(time.RFC3339),
		fmt.Sprintf("%.2f", metrics.CPU.Percent),
		fmt.Sprintf("%d", metrics.Memory.Total),
		fmt.Sprintf("%d", metrics.Memory.Used),
		fmt.Sprintf("%.2f", metrics.Memory.Percent),
		fmt.Sprintf("%d", metrics.Uptime),
	})

	// Disk rows
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

	// Network rows
	for _, n := range metrics.Networks {
		writer.Write([]string{
			"net:" + n.Name,
			fmt.Sprintf("%d", n.BytesSent),
			fmt.Sprintf("%d", n.BytesRecv),
			"", "", "",
		})
	}
}

// --- Helpers ---

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": html.EscapeString(message)})
}
