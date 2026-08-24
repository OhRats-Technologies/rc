package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type nodeConfig struct {
	Server string `json:"server,omitempty"`
	Name   string `json:"name,omitempty"`
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func defaultStateDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".ohrats-relay"
	}
	return filepath.Join(home, ".config", "ohrats-relay")
}

func resolveStateDir(value string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	if configured := strings.TrimSpace(os.Getenv("RELAY_STATE_DIR")); configured != "" {
		return configured
	}
	return defaultStateDir()
}

func statePath(dir string) string  { return filepath.Join(dir, "device.json") }
func configPath(dir string) string { return filepath.Join(dir, "config.json") }

func loadState(dir string) (state, error) {
	var value state
	data, err := os.ReadFile(statePath(dir))
	if err != nil {
		return value, err
	}
	err = json.Unmarshal(data, &value)
	return value, err
}

func saveState(dir string, value state) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(statePath(dir), data, 0600)
}

func loadConfig(dir string) (nodeConfig, error) {
	var value nodeConfig
	data, err := os.ReadFile(configPath(dir))
	if errors.Is(err, os.ErrNotExist) {
		return value, nil
	}
	if err != nil {
		return value, err
	}
	err = json.Unmarshal(data, &value)
	return value, err
}

func saveConfig(dir string, value nodeConfig) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath(dir), data, 0600)
}
