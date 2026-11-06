//go:build windows

package platform

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

func platformAcquireRunLock(path string) (*RunLock, error) {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(
		pathPtr,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		0, // exclusive, no sharing
		nil,
		windows.OPEN_ALWAYS,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		if errors.Is(err, windows.ERROR_SHARING_VIOLATION) || errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return nil, ErrAlreadyRunning
		}
		return nil, err
	}
	file := os.NewFile(uintptr(handle), path)
	return &RunLock{file: file}, nil
}

func platformReleaseRunLock(file *os.File) error {
	return nil
}
