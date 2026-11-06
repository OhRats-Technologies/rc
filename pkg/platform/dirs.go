package platform

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// ErrUserDirectoryUnavailable is returned when user directory cannot be resolved.
var ErrUserDirectoryUnavailable = errors.New("the platform user directory is unavailable")

// StateDir returns the RC state directory.
func StateDir() (string, error) {
	if dir := overridden("RC_STATE_DIR"); dir != "" {
		return dir, nil
	}
	return defaultStateDir()
}

// DataDir returns the RC data directory.
func DataDir() (string, error) {
	if dir := overridden("RC_DATA_DIR"); dir != "" {
		return dir, nil
	}
	return defaultDataDir()
}

// ComponentDir returns the directory where managed components are placed.
func ComponentDir() (string, error) {
	if dir := overridden("RC_COMPONENT_DIR"); dir != "" {
		return dir, nil
	}
	data, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(data, "components"), nil
}

// WasmtimeCacheDir returns the cache directory for wasmtime artifacts.
func WasmtimeCacheDir() (string, error) {
	if dir := overridden("RC_WASMTIME_CACHE_DIR"); dir != "" {
		return dir, nil
	}
	data, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(data, "cache", "wasmtime"), nil
}

// BinaryDir returns the directory containing RC executable binaries.
func BinaryDir() (string, error) {
	if dir := overridden("RC_INSTALL_BIN_DIR"); dir != "" {
		return dir, nil
	}
	return defaultBinaryDir()
}

// RuntimeVersionsDir returns the directory containing installed runtime generations.
func RuntimeVersionsDir() (string, error) {
	data, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(data, "runtime", "versions"), nil
}

// RuntimeActivationFile returns the path to the active runtime pointer file.
func RuntimeActivationFile() (string, error) {
	data, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(data, "runtime", "active"), nil
}

// RuntimePreviousActivationFile returns the path to the previous runtime pointer file.
func RuntimePreviousActivationFile() (string, error) {
	data, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(data, "runtime", "previous"), nil
}

// ActiveRuntimeDir reads the active runtime generation path if set.
func ActiveRuntimeDir() string {
	file, err := RuntimeActivationFile()
	if err != nil {
		return ""
	}
	content, err := os.ReadFile(file)
	if err != nil {
		return ""
	}
	value := strings.TrimSpace(string(content))
	return value
}

// UserHome returns the home directory for the current user.
func UserHome() (string, error) {
	return platformUserHome()
}

// ExecutableName returns the platform-specific executable name with extension.
func ExecutableName(stem string) string {
	return platformExecutableName(stem)
}

func overridden(name string) string {
	val := os.Getenv(name)
	if strings.TrimSpace(val) == "" {
		return ""
	}
	return filepath.Clean(val)
}
