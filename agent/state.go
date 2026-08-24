package main

import (
	"encoding/json"
	"errors"
	"io"
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
	if err != nil { return ".ohrats-relay" }
	return filepath.Join(home, ".config", "ohrats-relay")
}

func legacyStateDir() string {
	home, err := os.UserHomeDir()
	if err != nil { return ".relay" }
	return filepath.Join(home, ".config", "relay")
}

func resolveStateDir(value string) string {
	if strings.TrimSpace(value) != "" { return value }
	if configured := strings.TrimSpace(os.Getenv("RELAY_STATE_DIR")); configured != "" { return configured }
	return defaultStateDir()
}

func migrateLegacy(dir string) error {
	if filepath.Clean(dir) != filepath.Clean(defaultStateDir()) { return nil }
	newPath, oldPath := statePath(dir), statePath(legacyStateDir())
	if _, err := os.Stat(newPath); err == nil { return nil }
	if _, err := os.Stat(oldPath); errors.Is(err, os.ErrNotExist) { return nil } else if err != nil { return err }
	if err := os.MkdirAll(dir, 0700); err != nil { return err }
	if err := os.Rename(oldPath, newPath); err == nil { _ = os.Remove(legacyStateDir()); return nil }
	source, err := os.Open(oldPath); if err != nil { return err }; defer source.Close()
	target, err := os.OpenFile(newPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600); if err != nil { return err }
	if _, err = io.Copy(target, source); err != nil { target.Close(); return err }
	if err = target.Close(); err != nil { return err }
	_ = os.Remove(oldPath); _ = os.Remove(legacyStateDir())
	return nil
}

func statePath(dir string) string { return filepath.Join(dir, "device.json") }
func configPath(dir string) string { return filepath.Join(dir, "config.json") }

func loadState(dir string) (state, error) {
	var value state
	if err := migrateLegacy(dir); err != nil { return value, err }
	data, err := os.ReadFile(statePath(dir))
	if err != nil {
		return value, err
	}
	err = json.Unmarshal(data, &value)
	return value, err
}

func saveState(dir string, value state) error {
	if err := migrateLegacy(dir); err != nil { return err }
	if err := os.MkdirAll(dir, 0700); err != nil { return err }
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(statePath(dir), data, 0600)
}

func loadConfig(dir string) (nodeConfig, error) {
	var value nodeConfig
	if err := migrateLegacy(dir); err != nil { return value, err }
	data, err := os.ReadFile(configPath(dir))
	if errors.Is(err, os.ErrNotExist) { return value, nil }
	if err != nil { return value, err }
	err = json.Unmarshal(data, &value)
	return value, err
}

func saveConfig(dir string, value nodeConfig) error {
	if err := os.MkdirAll(dir, 0700); err != nil { return err }
	data, err := json.MarshalIndent(value, "", "  "); if err != nil { return err }
	return os.WriteFile(configPath(dir), data, 0600)
}
