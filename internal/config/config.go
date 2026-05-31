// Package config handles CLI flag parsing and configuration for Velkron Pulse.
package config

import (
	"flag"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
)

// Version is set at build time via ldflags.
var Version = "1.0.1"

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
	// ShowVersion prints the version and exits.
	ShowVersion bool
}

// Parse reads CLI flags and returns a populated Config.
// It expands "~" in DBPath to the user's home directory and creates the
// directory if it does not exist. On headless systems (no DISPLAY on Linux,
// no TTY), it auto-disables browser opening unless --no-browser is explicitly set.
func Parse() (*Config, error) {
	cfg := &Config{}

	flag.IntVar(&cfg.Port, "port", 2024, "HTTP server port")
	flag.StringVar(&cfg.DBPath, "db-path", "~/.velkron-pulse/", "Database directory path")
	flag.IntVar(&cfg.RefreshInterval, "refresh", 2, "Metrics collection interval in seconds")
	flag.BoolVar(&cfg.NoBrowser, "no-browser", false, "Disable auto-opening browser")
	flag.BoolVar(&cfg.ShowVersion, "version", false, "Show version and exit")
	flag.Parse()

	// Auto-detect headless environment: skip browser if no graphical display.
	// User can still force browser with --no-browser=false on headless systems.
	if !cfg.NoBrowser && isHeadless() {
		cfg.NoBrowser = true
	}

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

// isHeadless returns true if the environment appears to have no graphical display.
func isHeadless() bool {
	if runtime.GOOS == "windows" {
		// On Windows, check if running in a console session
		return os.Getenv("SESSIONNAME") == "" && os.Getenv("TERM") == ""
	}
	// Linux/macOS: check DISPLAY environment variable
	return os.Getenv("DISPLAY") == ""
}

// DBFilePath returns the full path to the SQLite database file.
func (c *Config) DBFilePath() string {
	return filepath.Join(c.DBPath, "config.db")
}
