//go:build !windows

package cli

import (
	"os"

	"github.com/OhRats-Technologies/rc/pkg/platform"
)

func validateStateDirectory(dir string) error {
	if err := platform.ValidatePrivatePath(dir, true); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
