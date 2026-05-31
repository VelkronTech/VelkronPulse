// Package store provides SQLite persistence for Velkron Pulse.
// It manages custom endpoints, settings, and historical metrics snapshots.
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// CustomEndpoint represents a user-defined endpoint to monitor.
type CustomEndpoint struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	URL       string `json:"url"`
	Type      string `json:"type"` // "http" or "tcp"
	CreatedAt string `json:"created_at"`
}

// Setting represents a key-value configuration setting.
type Setting struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// MetricsSnapshot represents a point-in-time metrics record stored in the DB.
type MetricsSnapshot struct {
	ID         int64     `json:"id"`
	Timestamp  time.Time `json:"timestamp"`
	CPUPercent float64   `json:"cpu_percent"`
	MemTotal   uint64    `json:"mem_total"`
	MemUsed    uint64    `json:"mem_used"`
	MemPercent float64   `json:"mem_percent"`
	DiskJSON   string    `json:"disk_json"`
	NetJSON    string    `json:"net_json"`
}

// Store wraps the SQLite database connection and provides CRUD operations.
type Store struct {
	db *sql.DB
}

// New opens (or creates) the SQLite database at dbPath, runs migrations,
// and returns a ready-to-use Store.
func New(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Enable WAL mode for better concurrent performance
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return nil, fmt.Errorf("failed to set WAL mode: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return s, nil
}

// migrate creates tables if they do not exist.
func (s *Store) migrate() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS custom_endpoints (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			url TEXT NOT NULL,
			type TEXT NOT NULL DEFAULT 'http',
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS metrics_snapshots (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp TEXT NOT NULL,
			cpu_percent REAL NOT NULL DEFAULT 0,
			mem_total INTEGER NOT NULL DEFAULT 0,
			mem_used INTEGER NOT NULL DEFAULT 0,
			mem_percent REAL NOT NULL DEFAULT 0,
			disk_json TEXT NOT NULL DEFAULT '[]',
			net_json TEXT NOT NULL DEFAULT '[]'
		)`,
		`CREATE INDEX IF NOT EXISTS idx_metrics_timestamp ON metrics_snapshots(timestamp)`,
	}

	for _, q := range queries {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// --- Custom Endpoints ---

// AddEndpoint inserts a new custom endpoint and returns its ID.
func (s *Store) AddEndpoint(name, url, endpointType string) (int64, error) {
	result, err := s.db.Exec(
		"INSERT INTO custom_endpoints (name, url, type) VALUES (?, ?, ?)",
		name, url, endpointType,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to add endpoint: %w", err)
	}
	return result.LastInsertId()
}

// DeleteEndpoint removes a custom endpoint by ID.
func (s *Store) DeleteEndpoint(id int64) error {
	_, err := s.db.Exec("DELETE FROM custom_endpoints WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete endpoint: %w", err)
	}
	return nil
}

// ListEndpoints returns all custom endpoints.
func (s *Store) ListEndpoints() ([]CustomEndpoint, error) {
	rows, err := s.db.Query("SELECT id, name, url, type, created_at FROM custom_endpoints ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("failed to list endpoints: %w", err)
	}
	defer rows.Close()

	var endpoints []CustomEndpoint
	for rows.Next() {
		var ep CustomEndpoint
		if err := rows.Scan(&ep.ID, &ep.Name, &ep.URL, &ep.Type, &ep.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan endpoint: %w", err)
		}
		endpoints = append(endpoints, ep)
	}
	return endpoints, rows.Err()
}

// --- Settings ---

// GetSetting returns a setting value by key. Returns empty string if not found.
func (s *Store) GetSetting(key string) (string, error) {
	var value string
	err := s.db.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to get setting %s: %w", key, err)
	}
	return value, nil
}

// SetSetting upserts a setting key-value pair.
func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(
		"INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
		key, value,
	)
	if err != nil {
		return fmt.Errorf("failed to set setting %s: %w", key, err)
	}
	return nil
}

// GetAllSettings returns all settings as a map.
func (s *Store) GetAllSettings() (map[string]string, error) {
	rows, err := s.db.Query("SELECT key, value FROM settings")
	if err != nil {
		return nil, fmt.Errorf("failed to list settings: %w", err)
	}
	defer rows.Close()

	settings := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("failed to scan setting: %w", err)
		}
		settings[k] = v
	}
	return settings, rows.Err()
}

// --- Metrics Snapshots ---

// SaveMetrics persists a metrics snapshot to the database.
func (s *Store) SaveMetrics(cpuPercent float64, memTotal, memUsed uint64, memPercent float64, diskJSON, netJSON string) error {
	_, err := s.db.Exec(
		`INSERT INTO metrics_snapshots (timestamp, cpu_percent, mem_total, mem_used, mem_percent, disk_json, net_json)
		 VALUES (datetime('now'), ?, ?, ?, ?, ?, ?)`,
		cpuPercent, memTotal, memUsed, memPercent, diskJSON, netJSON,
	)
	if err != nil {
		return fmt.Errorf("failed to save metrics: %w", err)
	}
	return nil
}

// GetMetricsHistory returns metrics snapshots within the given time range.
func (s *Store) GetMetricsHistory(from, to time.Time) ([]MetricsSnapshot, error) {
	rows, err := s.db.Query(
		`SELECT id, timestamp, cpu_percent, mem_total, mem_used, mem_percent, disk_json, net_json
		 FROM metrics_snapshots
		 WHERE timestamp >= ? AND timestamp <= ?
		 ORDER BY timestamp ASC`,
		from.Format("2006-01-02 15:04:05"),
		to.Format("2006-01-02 15:04:05"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query metrics history: %w", err)
	}
	defer rows.Close()

	var snapshots []MetricsSnapshot
	for rows.Next() {
		var ms MetricsSnapshot
		var ts string
		if err := rows.Scan(&ms.ID, &ts, &ms.CPUPercent, &ms.MemTotal, &ms.MemUsed, &ms.MemPercent, &ms.DiskJSON, &ms.NetJSON); err != nil {
			return nil, fmt.Errorf("failed to scan metrics: %w", err)
		}
		ms.Timestamp, _ = time.Parse("2006-01-02 15:04:05", ts)
		snapshots = append(snapshots, ms)
	}
	return snapshots, rows.Err()
}

// PruneOldMetrics deletes metrics snapshots older than 24 hours.
func (s *Store) PruneOldMetrics() error {
	_, err := s.db.Exec(
		"DELETE FROM metrics_snapshots WHERE timestamp < datetime('now', '-1 day')",
	)
	if err != nil {
		return fmt.Errorf("failed to prune old metrics: %w", err)
	}
	return nil
}

// MarshalToJSON serializes any value to a JSON string.
func MarshalToJSON(v interface{}) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "[]", err
	}
	return string(data), nil
}
