package update

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"

	"github.com/OhRats-Technologies/rc/pkg/platform"
)

// ensurePrivateDir creates a directory and validates it is not a symlink.
func ensurePrivateDir(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("%s is not a private directory", path)
	}
	// Only validate permissions after directory creation; strict ACL
	// enforcement happens in the installer.
	if runtime.GOOS != "windows" {
		return platform.ProtectPrivatePath(path, true)
	}
	return nil
}

func writeFileAtomic(path string, data []byte, mode fs.FileMode) error {
	dir := filepath.Dir(path)
	temporary, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	return syncDir(dir)
}

func syncDir(path string) error {
	// Directory sync on Windows with strict ACLs causes Access Denied errors.
	// It's not critical for Windows filesystems and is primarily a Unix
	// crash-safety mechanism.
	if runtime.GOOS == "windows" {
		return nil
	}
	dir, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("sync %s: %w", path, err)
	}
	defer dir.Close()
	return dir.Sync()
}

func copyExecutable(source, destination string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return writeFileAtomic(destination, data, 0o755)
}
