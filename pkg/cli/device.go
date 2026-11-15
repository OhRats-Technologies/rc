package cli

import (
	"context"
	"flag"
	"fmt"
)

func runDevice(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: rc device <rename> <device-id> <name>")
	}
	switch args[0] {
	case "rename":
		return runDeviceRename(ctx, args[1:])
	default:
		return fmt.Errorf("unknown device command %q", args[0])
	}
}

func runDeviceRename(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("device rename", flag.ContinueOnError)
	server := fs.String("url", "", "RC server URL")
	token := fs.String("token", "", "RC API key (or RC_API_TOKEN)")
	stateDir := fs.String("state-dir", "", "Override state directory")
	if err := fs.Parse(reorderArgs(fs, args)); err != nil {
		return err
	}
	if fs.NArg() < 2 {
		return fmt.Errorf("usage: rc device rename <device-id> <name>")
	}
	id, name := fs.Arg(0), fs.Arg(1)

	client, err := deviceAPIClient(*server, *token, *stateDir)
	if err != nil {
		return err
	}
	renamed, err := client.RenameDevice(ctx, id, name)
	if err != nil {
		return err
	}
	fmt.Printf("%s renamed to %s\n", id, renamed)
	return nil
}
