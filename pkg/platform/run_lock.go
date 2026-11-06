package platform

import (
	"errors"
	"os"
	"path/filepath"
)

// ErrAlreadyRunning indicates another process currently holds the node lock.
var ErrAlreadyRunning = errors.New("RC Node is already running for this enrollment; stop the service or exit the other `rc run` first")

// RunLock represents an active, exclusive run lock on a state directory.
type RunLock struct {
	file *os.File
}

// NodeLockPath returns the path to node.lock inside the given directory.
func NodeLockPath(dir string) string {
	return filepath.Join(dir, "node.lock")
}

// AcquireRunLock opens and acquires an exclusive non-blocking lock on node.lock.
func AcquireRunLock(dir string) (*RunLock, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	path := NodeLockPath(dir)
	return platformAcquireRunLock(path)
}

// Close releases the run lock and closes the underlying file handle.
func (l *RunLock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	err := platformReleaseRunLock(l.file)
	closeErr := l.file.Close()
	l.file = nil
	if err != nil {
		return err
	}
	return closeErr
}
