//go:build !unix && !windows

package platform

import (
	"errors"
)

var ErrUnsafePermissions = errors.New("private path protection is unsupported on this platform")

// ProtectPrivatePath returns an error on unsupported platforms.
func ProtectPrivatePath(path string, directory bool) error {
	return ErrUnsafePermissions
}

// ValidatePrivatePath returns an error on unsupported platforms.
func ValidatePrivatePath(path string, directory bool) error {
	return ErrUnsafePermissions
}
