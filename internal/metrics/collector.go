// Package metrics collects system metrics (CPU, memory, disk, network, uptime)
// using the gopsutil library. It runs in a goroutine and exposes the latest
// snapshot via a mutex-protected getter.
package metrics

import (
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
)

// CPUInfo holds CPU usage percentages.
type CPUInfo struct {
	Percent float64 `json:"percent"`
}

// MemoryInfo holds memory usage statistics.
type MemoryInfo struct {
	Total     uint64  `json:"total"`
	Used      uint64  `json:"used"`
	Available uint64  `json:"available"`
	Percent   float64 `json:"percent"`
}

// DiskInfo holds disk usage for a single mount point.
type DiskInfo struct {
	MountPoint string  `json:"mount_point"`
	Total      uint64  `json:"total"`
	Used       uint64  `json:"used"`
	Free       uint64  `json:"free"`
	Percent    float64 `json:"percent"`
}

// NetworkInfo holds network I/O counters for a single interface.
type NetworkInfo struct {
	Name      string `json:"name"`
	BytesSent uint64 `json:"bytes_sent"`
	BytesRecv uint64 `json:"bytes_recv"`
}

// MetricsSnapshot is a point-in-time snapshot of all system metrics.
type MetricsSnapshot struct {
	Timestamp time.Time     `json:"timestamp"`
	CPU       CPUInfo       `json:"cpu"`
	Memory    MemoryInfo    `json:"memory"`
	Disks     []DiskInfo    `json:"disks"`
	Networks  []NetworkInfo `json:"networks"`
	Uptime    uint64        `json:"uptime"`
}

// Collector periodically gathers system metrics and stores the latest snapshot.
type Collector struct {
	mu       sync.RWMutex
	latest   MetricsSnapshot
	interval time.Duration
	stopCh   chan struct{}
}

// New creates a new Collector with the given collection interval.
func New(interval time.Duration) *Collector {
	return &Collector{
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// Start begins collecting metrics in a background goroutine.
// It collects immediately on start, then every interval.
func (c *Collector) Start() {
	go func() {
		c.collect()
		ticker := time.NewTicker(c.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				c.collect()
			case <-c.stopCh:
				return
			}
		}
	}()
}

// Stop signals the collector goroutine to stop.
func (c *Collector) Stop() {
	close(c.stopCh)
}

// GetLatest returns the most recent metrics snapshot.
func (c *Collector) GetLatest() MetricsSnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.latest
}

// collect gathers all system metrics and updates the latest snapshot.
func (c *Collector) collect() {
	snapshot := MetricsSnapshot{
		Timestamp: time.Now(),
	}

	// CPU
	cpuPercent, err := cpu.Percent(0, false)
	if err == nil && len(cpuPercent) > 0 {
		snapshot.CPU.Percent = cpuPercent[0]
	}

	// Memory
	vmem, err := mem.VirtualMemory()
	if err == nil {
		snapshot.Memory = MemoryInfo{
			Total:     vmem.Total,
			Used:      vmem.Used,
			Available: vmem.Available,
			Percent:   vmem.UsedPercent,
		}
	}

	// Disk
	partitions, err := disk.Partitions(false)
	if err == nil {
		for _, p := range partitions {
			usage, err := disk.Usage(p.Mountpoint)
			if err != nil {
				continue
			}
			snapshot.Disks = append(snapshot.Disks, DiskInfo{
				MountPoint: p.Mountpoint,
				Total:      usage.Total,
				Used:       usage.Used,
				Free:       usage.Free,
				Percent:    usage.UsedPercent,
			})
		}
	}

	// Network
	netIO, err := net.IOCounters(true)
	if err == nil {
		for _, n := range netIO {
			snapshot.Networks = append(snapshot.Networks, NetworkInfo{
				Name:      n.Name,
				BytesSent: n.BytesSent,
				BytesRecv: n.BytesRecv,
			})
		}
	}

	// Uptime
	uptime, err := host.Uptime()
	if err == nil {
		snapshot.Uptime = uptime
	}

	c.mu.Lock()
	c.latest = snapshot
	c.mu.Unlock()
}
