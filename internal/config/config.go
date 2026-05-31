// Package config handles CLI flag parsing and configuration for Velkron Pulse.
package config

import (
	"flag"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
)

// Config holds all runtime configuration for Velkron Pulse.
type Config struct {
	// Port is the HTTP server port (default: 2024).
	Port int
	// DBPath is the directory for the SQLite database file (default: ~/.velkron-pulse/).
	DBPath string
	// RefreshInterval is the metrics collection interval in seconds (default: 2).
	RefreshInterval int
	// NoBrowser disables auto-opening the browser on startup.
	NoBrowser bool
}

// Parse reads CLI flags and returns a populated Config.
// It expands "~" in DBPath to the user's home directory and creates the
// directory if it does not exist.
func Parse() (*Config, error) {
	cfg := &Config{}

	flag.IntVar(&cfg.Port, "port", 2024, "HTTP server port")
	flag.StringVar(&cfg.DBPath, "db-path", "~/.velkron-pulse/", "Database directory path")
	flag.IntVar(&cfg.RefreshInterval, "refresh", 2, "Metrics collection interval in seconds")
	flag.BoolVar(&cfg.NoBrowser, "no-browser", false, "Disable auto-opening browser")
	flag.Parse()

	// Expand ~ to home directory
	if len(cfg.DBPath) > 0 && cfg.DBPath[0] == '~' {
		usr, err := user.Current()
		if err != nil {
			return nil, fmt.Errorf("cannot determine home directory: %w", err)
		}
		cfg.DBPath = filepath.Join(usr.HomeDir, cfg.DBPath[1:])
	}

	// Ensure DB directory exists
	if err := os.MkdirAll(cfg.DBPath, 0755); err != nil {
		return nil, fmt.Errorf("cannot create database directory %s: %w", cfg.DBPath, err)
	}

	return cfg, nil
}

// DBFilePath returns the full path to the SQLite database file.
func (c *Config) DBFilePath() string {
	return filepath.Join(c.DBPath, "config.db")
}
