package cli

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/OhRats-Technologies/rc/pkg/platform"
	"github.com/OhRats-Technologies/rc/pkg/update"
)

func runUpgrade(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("upgrade", flag.ContinueOnError)
	releaseAPI := fs.String("release-api", "", "Release API URL")
	reexec := fs.Bool("reexec", false, "Re-exec the upgraded binary")
	force := fs.Bool("force", false, "Reinstall even when the version matches")
	if err := fs.Parse(reorderArgs(fs, args)); err != nil {
		return err
	}
	result, err := update.Upgrade(ctx, update.Config{
		ReleaseAPI: *releaseAPI,
		Reexec:     *reexec,
		Force:      *force,
		Stdout:     os.Stdout,
	})
	if err != nil {
		return err
	}
	if result.Updated {
		restartService(ctx)
	}
	return nil
}

func runUpdate(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	releaseAPI := fs.String("release-api", "", "Release API URL")
	force := fs.Bool("force", false, "Replace locally managed core components")
	if err := fs.Parse(reorderArgs(fs, args)); err != nil {
		return err
	}
	result, err := update.RefreshComponents(ctx, update.Config{
		ReleaseAPI: *releaseAPI,
		Force:      *force,
		Stdout:     os.Stdout,
	})
	if err != nil {
		return err
	}
	if result.Updated {
		restartService(ctx)
	}
	return nil
}

func runRepair(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("repair", flag.ContinueOnError)
	releaseAPI := fs.String("release-api", "", "Release API URL")
	if err := fs.Parse(reorderArgs(fs, args)); err != nil {
		return err
	}
	if err := runPolicyCheck(""); err != nil {
		fmt.Fprintf(os.Stderr, "local runtime needs repair: %v\n", err)
	}
	if _, err := update.RefreshComponents(ctx, update.Config{
		ReleaseAPI: *releaseAPI,
		Force:      true,
		Stdout:     os.Stdout,
	}); err != nil {
		return fmt.Errorf("repair failed: %w", err)
	}
	if err := runPolicyCheck(""); err != nil {
		return fmt.Errorf("repaired runtime still fails validation: %w", err)
	}
	fmt.Println("RC runtime repaired")
	return nil
}

func restartService(ctx context.Context) {
	installed, running, _ := platform.ServiceStatus(ctx)
	if !installed || !running {
		return
	}
	if err := platform.ServiceRestart(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "warning: upgraded, but could not restart the RC service: %v\n", err)
		return
	}
	fmt.Println("RC service restarted")
}
