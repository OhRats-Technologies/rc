//go:build !windows

package platform

import (
	"os"
	"path/filepath"
)

func platformUserHome() (string, error) {
	home := os.Getenv("HOME")
	if home == "" {
		return "", ErrUserDirectoryUnavailable
	}
	return filepath.Clean(home), nil
}

func platformExecutableName(stem string) string {
	return stem
}

func defaultStateDir() (string, error) {
	home, err := platformUserHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "rc"), nil
}

func defaultDataDir() (string, error) {
	home, err := platformUserHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "rc"), nil
}

func defaultBinaryDir() (string, error) {
	home, err := platformUserHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "bin"), nil
}
