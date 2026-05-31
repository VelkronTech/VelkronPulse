package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFileConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"port":9090,"bind":"127.0.0.1","refresh":5}`), 0600); err != nil {
		t.Fatal(err)
	}

	fc, err := loadFileConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if fc.Port == nil || *fc.Port != 9090 {
		t.Fatalf("expected port 9090, got %+v", fc.Port)
	}
}

func TestMaskToken(t *testing.T) {
	masked := MaskToken("abcdef1234567890")
	if masked == "abcdef1234567890" {
		t.Fatal("expected masked token")
	}
}
