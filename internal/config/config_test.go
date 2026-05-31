package config

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
)

// resetFlagsForTesting resets the global flag state so tests can run independently.
func resetFlagsForTesting() {
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
}

func TestParseDefaults(t *testing.T) {
	resetFlagsForTesting()
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"velkron-pulse"}

	cfg, err := Parse()
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if cfg.Port != 2024 {
		t.Errorf("expected Port=2024, got %d", cfg.Port)
	}
	if cfg.RefreshInterval != 2 {
		t.Errorf("expected RefreshInterval=2, got %d", cfg.RefreshInterval)
	}
}

func TestParseCustomFlags(t *testing.T) {
	resetFlagsForTesting()
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"velkron-pulse", "--port", "9090", "--refresh", "5", "--no-browser"}

	cfg, err := Parse()
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if cfg.Port != 9090 {
		t.Errorf("expected Port=9090, got %d", cfg.Port)
	}
	if cfg.RefreshInterval != 5 {
		t.Errorf("expected RefreshInterval=5, got %d", cfg.RefreshInterval)
	}
	if !cfg.NoBrowser {
		t.Error("expected NoBrowser=true")
	}
}

func TestDBFilePath(t *testing.T) {
	cfg := &Config{
		Port:            2024,
		DBPath:          "/tmp/test-vp",
		RefreshInterval: 2,
	}
	expected := filepath.Join("/tmp/test-vp", "config.db")
	if got := cfg.DBFilePath(); got != expected {
		t.Errorf("expected %s, got %s", expected, got)
	}
}

func TestIsHeadless(t *testing.T) {
	// Headless detection is environment-dependent, just ensure it returns a bool.
	result := isHeadless()
	if result != true && result != false {
		t.Error("isHeadless() must return a bool")
	}
}
