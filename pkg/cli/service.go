package cli

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/OhRats-Technologies/rc/pkg/node"
	"github.com/OhRats-Technologies/rc/pkg/platform"
)

func runService(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: rc service status|install|start|stop|uninstall")
	}
	command := args[0]
	fs := flag.NewFlagSet("service", flag.ContinueOnError)
	stateDir := fs.String("state-dir", "", "Override state directory")
	if err := fs.Parse(reorderArgs(fs, args[1:])); err != nil {
		return err
	}
	dir, err := node.ResolveStateDir(*stateDir)
	if err != nil {
		return err
	}
	switch command {
	case "status":
		installed, running, detail := platform.ServiceStatus(ctx)
		fmt.Printf("installed=%t running=%t detail=%s\n", installed, running, detail)
		return nil
	case "install":
		if _, err := node.LoadState(dir); err != nil {
			return node.ErrNotEnrolled
		}
		executable, err := os.Executable()
		if err != nil {
			return err
		}
		if err := platform.InstallService(executable, []string{"run", "--state-dir", dir}, dir); err != nil {
			return err
		}
		fmt.Println("RC background service installed")
		return nil
	case "start":
		if _, err := node.LoadState(dir); err != nil {
			return node.ErrNotEnrolled
		}
		return platform.ServiceRestart(ctx)
	case "stop":
		return platform.StopService(ctx)
	case "uninstall":
		if err := platform.UninstallService(ctx); err != nil {
			return err
		}
		fmt.Println("RC background service removed")
		return nil
	default:
		return fmt.Errorf("unknown service command %q", command)
	}
}
