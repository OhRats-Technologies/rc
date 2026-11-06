//go:build unix

package update

import (
	"fmt"
	"os"
)

func syncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("sync %s: %w", path, err)
	}
	defer dir.Close()
	return dir.Sync()
}
