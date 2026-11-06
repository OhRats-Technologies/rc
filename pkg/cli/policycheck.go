package cli

import (
	"fmt"
	"sort"

	"github.com/OhRats-Technologies/rc/pkg/kernel"
	"github.com/OhRats-Technologies/rc/pkg/platform"
)

func runPolicyCheck(componentDir string) error {
	dir := componentDir
	if dir == "" {
		var err error
		dir, err = platform.ComponentDir()
		if err != nil {
			return err
		}
	}
	runtimeHandle, err := kernel.NewRuntime(dir)
	if err != nil {
		return err
	}
	defer runtimeHandle.Close()
	if _, err := runtimeHandle.Reconcile(); err != nil {
		return err
	}
	if failed := runtimeHandle.FailedPaths(); len(failed) > 0 {
		paths := make([]string, 0, len(failed))
		for path := range failed {
			paths = append(paths, path)
		}
		sort.Strings(paths)
		for _, path := range paths {
			return fmt.Errorf("component %s failed to load: %s", path, failed[path])
		}
	}
	if runtimeHandle.Count() == 0 {
		return fmt.Errorf("no components found in %s", dir)
	}
	if _, err := verifyInstalledProfile(dir); err != nil {
		return err
	}
	fmt.Printf("policy-check: ok (%d components)\n", runtimeHandle.Count())
	return nil
}
