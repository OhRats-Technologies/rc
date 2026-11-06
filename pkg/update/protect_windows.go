//go:build windows

package update

// protectPrivateDir is a no-op on Windows where the installer handles ACL protection.
func protectPrivateDir(path string) error {
	return nil
}
