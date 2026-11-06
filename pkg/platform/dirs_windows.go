//go:build windows

package platform

import (
	"os"
	"path/filepath"
)

func platformUserHome() (string, error) {
	home := os.Getenv("USERPROFILE")
	if home == "" {
		return "", ErrUserDirectoryUnavailable
	}
	return filepath.Clean(home), nil
}

func platformExecutableName(stem string) string {
	return stem + ".exe"
}

func defaultRootFrom(value string) (string, error) {
	if value == "" {
		return "", ErrUserDirectoryUnavailable
	}
	return filepath.Join(filepath.Clean(value), "OhRats", "RC"), nil
}

func defaultRoot() (string, error) {
	return defaultRootFrom(os.Getenv("LOCALAPPDATA"))
}

func defaultStateDir() (string, error) {
	root, err := defaultRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "state"), nil
}

func defaultDataDir() (string, error) {
	root, err := defaultRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "data"), nil
}

func defaultBinaryDir() (string, error) {
	root, err := defaultRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "bin"), nil
}
