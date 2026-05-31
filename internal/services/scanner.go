// Package services handles auto-discovery of common services and health checks
// for custom HTTP/TCP endpoints. It runs in a background goroutine and refreshes
// service status every 60 seconds.
package services

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

// ServiceStatus represents the health status of a single service.
type ServiceStatus struct {
	Name         string        `json:"name"`
	Port         int           `json:"port"`
	URL          string        `json:"url,omitempty"`
	Status       string        `json:"status"` // "UP" or "DOWN"
	ResponseTime time.Duration `json:"response_time"`
	IsCustom        bool          `json:"is_custom"`
	EndpointID      int64         `json:"endpoint_id,omitempty"`
	UptimePercent   float64       `json:"uptime_percent,omitempty"`
	ChecksTotal     int           `json:"checks_total,omitempty"`
}

// CheckRecorder persists the outcome of a custom endpoint probe.
type CheckRecorder func(endpointID int64, status string, responseTime time.Duration)

// CustomEndpoint represents a user-defined endpoint to monitor.
type CustomEndpoint struct {
	ID   int64
	Name string
	URL  string
	Type string // "http" or "tcp"
}

// knownServices maps service names to their default ports.
var knownServices = map[string][]int{
	"nginx":         {80, 443},
	"PostgreSQL":    {5432},
	"Redis":         {6379},
	"MySQL":         {3306},
	"Docker":        {2375, 2376},
	"MongoDB":       {27017},
	"Elasticsearch": {9200},
	"Prometheus":    {9090},
	"Grafana":       {3000},
}

// Scanner periodically checks service health and stores the latest results.
type Scanner struct {
	mu         sync.RWMutex
	services   []ServiceStatus
	interval   time.Duration
	stopCh     chan struct{}
	endpointFn func() ([]CustomEndpoint, error)
	recorder   CheckRecorder
}

// New creates a new Scanner with the given check interval and a function to
// retrieve custom endpoints from the database.
func New(interval time.Duration, endpointFn func() ([]CustomEndpoint, error), recorder CheckRecorder) *Scanner {
	return &Scanner{
		interval:   interval,
		stopCh:     make(chan struct{}),
		endpointFn: endpointFn,
		recorder:   recorder,
	}
}

// Start begins scanning services in a background goroutine.
// It runs an immediate scan on start, then every interval.
func (s *Scanner) Start() {
	go func() {
		s.scan()
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				s.scan()
			case <-s.stopCh:
				return
			}
		}
	}()
}

// Stop signals the scanner goroutine to stop.
func (s *Scanner) Stop() {
	close(s.stopCh)
}

// GetServices returns the latest service statuses.
func (s *Scanner) GetServices() []ServiceStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]ServiceStatus, len(s.services))
	copy(result, s.services)
	return result
}

// Rescan immediately re-checks all services and custom endpoints.
func (s *Scanner) Rescan() {
	s.scan()
}

// scan performs a full scan of known services and custom endpoints.
func (s *Scanner) scan() {
	var results []ServiceStatus

	// Scan known services
	for name, ports := range knownServices {
		for _, port := range ports {
			status := checkTCP(fmt.Sprintf("127.0.0.1:%d", port))
			results = append(results, ServiceStatus{
				Name:         name,
				Port:         port,
				Status:       status.Status,
				ResponseTime: status.ResponseTime,
				IsCustom:     false,
			})
		}
	}

	// Scan custom endpoints from DB
	if s.endpointFn != nil {
		endpoints, err := s.endpointFn()
		if err == nil {
			for _, ep := range endpoints {
				if err := ValidateProbeTarget(ep.Type, ep.URL); err != nil {
					continue
				}
				var status checkResult
				switch ep.Type {
				case "tcp":
					status = checkTCP(ep.URL)
				default:
					status = checkHTTP(ep.URL)
				}
				results = append(results, ServiceStatus{
					Name:         ep.Name,
					Port:         0,
					URL:          ep.URL,
					Status:       status.Status,
					ResponseTime: status.ResponseTime,
					IsCustom:     true,
					EndpointID:   ep.ID,
				})
				if s.recorder != nil {
					s.recorder(ep.ID, status.Status, status.ResponseTime)
				}
			}
		}
	}

	s.mu.Lock()
	s.services = results
	s.mu.Unlock()
}

// checkResult holds the outcome of a single health check.
type checkResult struct {
	Status       string
	ResponseTime time.Duration
}

// checkTCP attempts a TCP connection to the given address with a 2-second timeout.
func checkTCP(addr string) checkResult {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	elapsed := time.Since(start)
	if err != nil {
		return checkResult{Status: "DOWN", ResponseTime: elapsed}
	}
	conn.Close()
	return checkResult{Status: "UP", ResponseTime: elapsed}
}

// checkHTTP performs an HTTP GET request to the given URL with a 5-second timeout.
func checkHTTP(url string) checkResult {
	client := &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return errors.New("redirects not allowed")
		},
	}
	start := time.Now()
	resp, err := client.Get(url)
	elapsed := time.Since(start)
	if err != nil {
		return checkResult{Status: "DOWN", ResponseTime: elapsed}
	}
	defer resp.Body.Close()
	// Drain body to ensure accurate timing
	io.Copy(io.Discard, resp.Body)
	return checkResult{Status: "UP", ResponseTime: elapsed}
}
