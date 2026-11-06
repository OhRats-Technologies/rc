//go:build windows

package update

// syncDir is a no-op on Windows. Directory sync is not reliably supported
// on Windows and conflicts with strict ACL protection.
func syncDir(path string) error {
	return nil
}
