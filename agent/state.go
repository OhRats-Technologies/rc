package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func defaultStateDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".relay"
	}
	return filepath.Join(home, ".config", "relay")
}

func statePath(dir string) string { return filepath.Join(dir, "device.json") }

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
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(statePath(dir), data, 0600)
}
