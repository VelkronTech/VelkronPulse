package store

import (
	"fmt"
	"time"
)

// EndpointUptimeStat summarizes probe history for a custom endpoint.
type EndpointUptimeStat struct {
	EndpointID    int64   `json:"endpoint_id"`
	TotalChecks   int     `json:"total_checks"`
	UpChecks      int     `json:"up_checks"`
	UptimePercent float64 `json:"uptime_percent"`
	LastChecked   string  `json:"last_checked,omitempty"`
}

func (s *Store) enableForeignKeys() error {
	_, err := s.db.Exec("PRAGMA foreign_keys=ON")
	return err
}

func (s *Store) migrateEndpointChecks() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS endpoint_checks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		endpoint_id INTEGER NOT NULL,
		status TEXT NOT NULL,
		response_time_ns INTEGER NOT NULL DEFAULT 0,
		checked_at TEXT NOT NULL DEFAULT (datetime('now')),
		FOREIGN KEY(endpoint_id) REFERENCES custom_endpoints(id) ON DELETE CASCADE
	)`)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_endpoint_checks ON endpoint_checks(endpoint_id, checked_at)`)
	return err
}

// RecordEndpointCheck stores the result of a health probe.
func (s *Store) RecordEndpointCheck(endpointID int64, status string, responseTime time.Duration) error {
	_, err := s.db.Exec(
		`INSERT INTO endpoint_checks (endpoint_id, status, response_time_ns) VALUES (?, ?, ?)`,
		endpointID, status, responseTime.Nanoseconds(),
	)
	if err != nil {
		return fmt.Errorf("failed to record endpoint check: %w", err)
	}
	return nil
}

// GetEndpointUptimeStats returns 24-hour uptime stats for all custom endpoints.
func (s *Store) GetEndpointUptimeStats() (map[int64]EndpointUptimeStat, error) {
	rows, err := s.db.Query(`
		SELECT endpoint_id,
		       COUNT(*) AS total,
		       SUM(CASE WHEN status = 'UP' THEN 1 ELSE 0 END) AS up_count,
		       MAX(checked_at) AS last_checked
		FROM endpoint_checks
		WHERE checked_at >= datetime('now', '-1 day')
		GROUP BY endpoint_id`)
	if err != nil {
		return nil, fmt.Errorf("failed to query endpoint uptime: %w", err)
	}
	defer rows.Close()

	stats := make(map[int64]EndpointUptimeStat)
	for rows.Next() {
		var stat EndpointUptimeStat
		if err := rows.Scan(&stat.EndpointID, &stat.TotalChecks, &stat.UpChecks, &stat.LastChecked); err != nil {
			return nil, fmt.Errorf("failed to scan endpoint uptime: %w", err)
		}
		if stat.TotalChecks > 0 {
			stat.UptimePercent = float64(stat.UpChecks) / float64(stat.TotalChecks) * 100
		}
		stats[stat.EndpointID] = stat
	}
	return stats, rows.Err()
}

// PruneOldEndpointChecks deletes probe records older than 7 days.
func (s *Store) PruneOldEndpointChecks() error {
	_, err := s.db.Exec(`DELETE FROM endpoint_checks WHERE checked_at < datetime('now', '-7 days')`)
	if err != nil {
		return fmt.Errorf("failed to prune endpoint checks: %w", err)
	}
	return nil
}
