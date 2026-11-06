//go:build windows

package cli

import (
	"errors"
	"os"
)

// validateStateDir skips ACL validation on Windows where the installer handles it.
func validateStateDir(report *doctorReport, dir string) {
	if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
		report.ok("state", "missing at "+dir+" (not enrolled)")
	} else if err != nil {
		report.fail("state", err)
	} else {
		report.ok("state", dir)
	}
}

// validateComponentDir skips ACL validation on Windows where the installer handles it.
func validateComponentDir(report *doctorReport, dir string) error {
	return nil
}
