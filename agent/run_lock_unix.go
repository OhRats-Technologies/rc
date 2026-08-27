//go:build darwin || linux

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func acquireRunLock(stateDir string) (*os.File, error) {
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		return nil, err
	}
	path := filepath.Join(stateDir, "node.lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("RC Node is already running for this enrollment; stop it with `rc service stop` or exit the other `rc run` first")
		}
		return nil, err
	}
	return file, nil
}
