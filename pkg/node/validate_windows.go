//go:build windows

package node

// validatePrivateFile skips validation on Windows where the installer handles ACL protection.
func validatePrivateFile(path string) error {
	return nil
}
