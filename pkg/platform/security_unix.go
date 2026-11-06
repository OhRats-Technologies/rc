//go:build unix

package platform

import (
	"errors"
	"io/fs"
	"os"
)

var ErrUnsafePermissions = errors.New("RC private path has unsafe permissions or type")

// ProtectPrivatePath sets 0700 for directories or 0600 for files.
func ProtectPrivatePath(path string, directory bool) error {
	var mode fs.FileMode = 0600
	if directory {
		mode = 0700
	}
	return os.Chmod(path, mode)
}

// ValidatePrivatePath ensures the path is not a symlink, matches the expected type, and has no group/other permissions.
func ValidatePrivatePath(path string, directory bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return ErrUnsafePermissions
	}
	if directory && !info.IsDir() {
		return ErrUnsafePermissions
	}
	if !directory && !info.Mode().IsRegular() {
		return ErrUnsafePermissions
	}
	if info.Mode().Perm()&0077 != 0 {
		return ErrUnsafePermissions
	}
	return nil
}
