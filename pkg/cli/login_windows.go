//go:build windows

package cli

// validateStateDirectory skips ACL validation on Windows.
func validateStateDirectory(dir string) error {
	return nil
}
