//go:build windows

package update

// syncDir is a no-op on Windows where directory sync conflicts with ACL protection.
func syncDir(path string) error {
	return nil
}
