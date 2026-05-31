// Package config handles CLI flag parsing and configuration for Velkron Pulse.
package config

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
)

// Version is set at build time via ldflags.
var Version = "1.1.0"

// Config holds all runtime configuration for Velkron Pulse.
type Config struct {
	// Port is the HTTP server port (default: 2024).
	Port int
	// BindAddress is the network interface to bind (default: 127.0.0.1).
	BindAddress string
	// DBPath is the directory for the SQLite database file (default: ~/.velkron-pulse/).
	DBPath string
	// RefreshInterval is the metrics collection interval in seconds (default: 2).
	RefreshInterval int
	// NoBrowser disables auto-opening the browser on startup.
	NoBrowser bool
	// ShowVersion prints the version and exits.
	ShowVersion bool
	// Token is the bearer token required for API and WebSocket access.
	Token string
}

// Parse reads CLI flags and returns a populated Config.
// It expands "~" in DBPath to the user's home directory and creates the
// directory if it does not exist. On headless systems (no DISPLAY on Linux,
// no TTY), it auto-disables browser opening unless --no-browser is explicitly set.
func Parse() (*Config, error) {
	cfg := &Config{
		Port:            2024,
		BindAddress:     "127.0.0.1",
		DBPath:          "~/.velkron-pulse/",
		RefreshInterval: 2,
	}

	dbDir, err := expandPath(cfg.DBPath)
	if err != nil {
		return nil, err
	}
	if fileCfg, err := loadFileConfig(dbDir); err != nil {
		return nil, err
	} else {
		fileCfg.apply(cfg)
	}

	var tokenFlag string
	flag.IntVar(&cfg.Port, "port", cfg.Port, "HTTP server port")
	flag.StringVar(&cfg.BindAddress, "bind", cfg.BindAddress, "Network address to bind (use 0.0.0.0 for all interfaces)")
	flag.StringVar(&cfg.DBPath, "db-path", cfg.DBPath, "Database directory path")
	flag.IntVar(&cfg.RefreshInterval, "refresh", cfg.RefreshInterval, "Metrics collection interval in seconds")
	flag.BoolVar(&cfg.NoBrowser, "no-browser", cfg.NoBrowser, "Disable auto-opening browser")
	flag.BoolVar(&cfg.ShowVersion, "version", false, "Show version and exit")
	flag.StringVar(&tokenFlag, "token", "", "API bearer token (auto-generated if empty; also reads VELKRON_PULSE_TOKEN)")
	flag.Parse()

	if tokenFlag != "" {
		cfg.Token = tokenFlag
	}

	cfg.DBPath, err = expandPath(cfg.DBPath)
	if err != nil {
		return nil, err
	}

	cfg.BindAddress = strings.TrimSpace(cfg.BindAddress)
	if cfg.BindAddress == "" {
		cfg.BindAddress = "127.0.0.1"
	}

	if !cfg.NoBrowser && isHeadless() {
		cfg.NoBrowser = true
	}

	if err := os.MkdirAll(cfg.DBPath, 0700); err != nil {
		return nil, fmt.Errorf("cannot create database directory %s: %w", cfg.DBPath, err)
	}
	if err := os.Chmod(cfg.DBPath, 0700); err != nil && runtime.GOOS != "windows" {
		return nil, fmt.Errorf("cannot set database directory permissions: %w", err)
	}

	if cfg.Token == "" {
		cfg.Token = strings.TrimSpace(os.Getenv("VELKRON_PULSE_TOKEN"))
	}
	if cfg.Token == "" {
		token, err := generateToken()
		if err != nil {
			return nil, fmt.Errorf("cannot generate API token: %w", err)
		}
		cfg.Token = token
	}

	return cfg, nil
}

func expandPath(path string) (string, error) {
	if len(path) > 0 && path[0] == '~' {
		usr, err := user.Current()
		if err != nil {
			return "", fmt.Errorf("cannot determine home directory: %w", err)
		}
		path = filepath.Join(usr.HomeDir, path[1:])
	}
	return filepath.Clean(path), nil
}

func generateToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// isHeadless returns true if the environment appears to have no graphical display.
func isHeadless() bool {
	if runtime.GOOS == "windows" {
		return os.Getenv("SESSIONNAME") == "" && os.Getenv("TERM") == ""
	}
	return os.Getenv("DISPLAY") == ""
}

// DBFilePath returns the full path to the SQLite database file.
func (c *Config) DBFilePath() string {
	return filepath.Join(c.DBPath, "config.db")
}

// ListenAddress returns the host:port string for the HTTP server.
func (c *Config) ListenAddress() string {
	return fmt.Sprintf("%s:%d", c.BindAddress, c.Port)
}

// ExposedToNetwork reports whether the server binds outside loopback.
func (c *Config) ExposedToNetwork() bool {
	switch strings.ToLower(c.BindAddress) {
	case "127.0.0.1", "localhost", "::1":
		return false
	default:
		return true
	}
}
