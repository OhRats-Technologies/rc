//go:build !windows

package cli

import (
	"errors"
	"os"

	"github.com/OhRats-Technologies/rc/pkg/platform"
)

func validateStateDir(report *doctorReport, dir string) {
	if err := platform.ValidatePrivatePath(dir, true); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			report.ok("state", "missing at "+dir+" (not enrolled)")
		} else {
			report.fail("state", err)
		}
	} else {
		report.ok("state", dir)
	}
}

func validateComponentDir(report *doctorReport, dir string) error {
	return platform.ValidatePrivatePath(dir, true)
}
