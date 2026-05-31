package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type fileConfig struct {
	Port            *int    `json:"port"`
	BindAddress     *string `json:"bind"`
	DBPath          *string `json:"db_path"`
	RefreshInterval *int    `json:"refresh"`
	NoBrowser       *bool   `json:"no_browser"`
	Token           *string `json:"token"`
}

// loadFileConfig reads optional JSON config from dbDir/config.json.
func loadFileConfig(dbDir string) (*fileConfig, error) {
	path := filepath.Join(dbDir, "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var fc fileConfig
	if err := json.Unmarshal(data, &fc); err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}
	return &fc, nil
}

func (fc *fileConfig) apply(cfg *Config) {
	if fc == nil {
		return
	}
	if fc.Port != nil {
		cfg.Port = *fc.Port
	}
	if fc.BindAddress != nil {
		cfg.BindAddress = *fc.BindAddress
	}
	if fc.DBPath != nil {
		cfg.DBPath = *fc.DBPath
	}
	if fc.RefreshInterval != nil {
		cfg.RefreshInterval = *fc.RefreshInterval
	}
	if fc.NoBrowser != nil {
		cfg.NoBrowser = *fc.NoBrowser
	}
	if fc.Token != nil && *fc.Token != "" {
		cfg.Token = *fc.Token
	}
}

// MaskToken returns a redacted token safe for logs.
func MaskToken(token string) string {
	if len(token) <= 8 {
		return "****"
	}
	return token[:4] + "…" + token[len(token)-4:]
}
