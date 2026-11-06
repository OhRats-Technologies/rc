package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"time"

	"github.com/OhRats-Technologies/rc/pkg/kernel"
	"github.com/OhRats-Technologies/rc/pkg/node"
	"github.com/OhRats-Technologies/rc/pkg/platform"
	"github.com/OhRats-Technologies/rc/pkg/update"
)

type doctorReport struct {
	lines  []string
	errors int
}

func (r *doctorReport) ok(name, value string) {
	r.lines = append(r.lines, fmt.Sprintf("%-18s %s", name+":", value))
}

func (r *doctorReport) fail(name string, err error) {
	r.errors++
	r.lines = append(r.lines, fmt.Sprintf("%-18s ERROR %v", name+":", err))
}

func runDoctor(_ context.Context, args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	stateDir := fs.String("state-dir", "", "Override state directory")
	componentDir := fs.String("component-dir", "", "Override component directory")
	if err := fs.Parse(reorderArgs(fs, args)); err != nil {
		return err
	}
	report := &doctorReport{}
	report.ok("version", Version)
	report.ok("platform", runtime.GOOS+"/"+runtime.GOARCH)

	dir, err := node.ResolveStateDir(*stateDir)
	if err != nil {
		report.fail("state", err)
	} else {
		if err := platform.ValidatePrivatePath(dir, true); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				report.ok("state", "missing at "+dir+" (not enrolled)")
			} else {
				report.fail("state", err)
			}
		} else {
			report.ok("state", dir)
		}
		checkEnrollment(report, dir)
	}
	checkComponents(report, *componentDir)
	checkService(report)
	checkBinary(report)

	fmt.Fprintln(os.Stdout, "RC diagnostics")
	for _, line := range report.lines {
		fmt.Fprintln(os.Stdout, line)
	}
	if report.errors > 0 {
		return fmt.Errorf("%d diagnostic checks failed", report.errors)
	}
	return nil
}

func resolveDirectory(name, explicit string) (string, error) {
	if explicit != "" {
		return filepath.Clean(explicit), nil
	}
	switch name {
	case "state":
		return platform.StateDir()
	case "data":
		return platform.DataDir()
	default:
		return platform.ComponentDir()
	}
}

func checkEnrollment(report *doctorReport, dir string) {
	state, err := node.LoadState(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			report.ok("enrollment", "not enrolled")
		} else {
			report.fail("enrollment", err)
		}
		return
	}
	report.ok("device", state.DeviceID)
	if _, err := node.LoadLock(dir); err != nil {
		if errors.Is(err, node.ErrLockMissing) {
			report.ok("lock", "not initialized")
		} else {
			report.fail("lock", err)
		}
	} else {
		report.ok("lock", "ok")
	}
	config, err := node.LoadConfig(dir)
	if err != nil {
		report.fail("config", err)
		return
	}
	if config.Server != "" {
		report.ok("server", config.Server)
	}
	if config.Name != "" {
		report.ok("name", config.Name)
	}
}

func checkComponents(report *doctorReport, explicit string) {
	dir, err := resolveDirectory("components", explicit)
	if err != nil {
		report.fail("components", err)
		return
	}
	if err := platform.ValidatePrivatePath(dir, true); err != nil {
		report.fail("components", err)
		return
	}
	runtimeHandle, err := kernel.NewRuntime(dir)
	if err != nil {
		report.fail("components", err)
		return
	}
	defer runtimeHandle.Close()
	if _, err := runtimeHandle.Reconcile(); err != nil {
		report.fail("components", err)
		return
	}
	failed := runtimeHandle.FailedPaths()
	report.ok("components", fmt.Sprintf("%s (%d failed)", runtimeHandle.Summary(), len(failed)))
	paths := make([]string, 0, len(failed))
	for path := range failed {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		report.fail("component", fmt.Errorf("%s: %s", filepath.Base(path), failed[path]))
	}
	if count, err := verifyInstalledProfile(dir); err != nil {
		report.fail("profile.lock", err)
	} else if count > 0 {
		report.ok("profile", fmt.Sprintf("%d components verified", count))
	}
}

func verifyInstalledProfile(dir string) (int, error) {
	data, err := os.ReadFile(filepath.Join(dir, "profile.lock"))
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	_, digests, err := update.ParseProfileLock(string(data))
	if err != nil {
		return 0, err
	}
	for name, expected := range digests {
		wasm, err := os.ReadFile(filepath.Join(dir, name+".wasm"))
		if err != nil {
			return 0, fmt.Errorf("%s is missing", name)
		}
		if err := update.VerifyDigest(wasm, expected); err != nil {
			return 0, fmt.Errorf("%s: %w", name, err)
		}
	}
	return len(digests), nil
}

func checkService(report *doctorReport) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	installed, running, detail := platform.ServiceStatus(ctx)
	switch {
	case !installed:
		report.ok("service", "not installed")
	case running:
		report.ok("service", "running ("+detail+")")
	default:
		report.ok("service", "installed, not running ("+detail+")")
	}
}

func checkBinary(report *doctorReport) {
	binDir, err := platform.BinaryDir()
	if err != nil {
		report.fail("binary", err)
		return
	}
	path := filepath.Join(binDir, platform.ExecutableName("rc"))
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			report.ok("binary", "not installed at "+path)
			return
		}
		report.fail("binary", err)
		return
	}
	report.ok("binary", fmt.Sprintf("%s (%d bytes)", path, info.Size()))
}
