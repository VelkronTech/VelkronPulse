package web

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/velkron/pulse/internal/config"
)

func (s *Server) handlePrometheusMetrics(w http.ResponseWriter, r *http.Request) {
	snapshot := s.collector.GetLatest()
	services := s.scanner.GetServices()

	var b strings.Builder
	writeGauge := func(name, help string, value float64) {
		fmt.Fprintf(&b, "# HELP %s %s\n# TYPE %s gauge\n%s %.4f\n", name, help, name, name, value)
	}

	fmt.Fprintf(&b, "# HELP pulse_info Pulse build information\n# TYPE pulse_info gauge\npulse_info{version=\"%s\"} 1\n", config.Version)
	writeGauge("pulse_cpu_percent", "CPU usage percent", snapshot.CPU.Percent)
	writeGauge("pulse_memory_percent", "Memory usage percent", snapshot.Memory.Percent)
	writeGauge("pulse_memory_used_bytes", "Memory used bytes", float64(snapshot.Memory.Used))
	writeGauge("pulse_memory_total_bytes", "Memory total bytes", float64(snapshot.Memory.Total))
	writeGauge("pulse_uptime_seconds", "System uptime seconds", float64(snapshot.Uptime))

	up := 0
	for _, svc := range services {
		if svc.Status == "UP" {
			up++
		}
	}
	writeGauge("pulse_services_up", "Services currently up", float64(up))
	writeGauge("pulse_services_total", "Services monitored", float64(len(services)))

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.Write([]byte(b.String()))
}
