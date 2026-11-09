package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/OhRats-Technologies/rc/pkg/node"
)

func runShell(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("shell", flag.ContinueOnError)
	device := fs.String("device", "", "Target device ID (local device when omitted)")
	stateDir := fs.String("state-dir", "", "Override state directory")
	command := fs.String("command", "", "Run one command instead of an interactive shell")
	if err := fs.Parse(reorderArgs(fs, args)); err != nil {
		return err
	}
	dir, err := node.ResolveStateDir(*stateDir)
	if err != nil {
		return err
	}
	state, err := node.LoadState(dir)
	if err != nil {
		return node.ErrNotEnrolled
	}
	if *device != "" && *device != state.DeviceID {
		config, _ := node.LoadConfig(dir)
		server := config.Server
		if server == "" {
			server = os.Getenv("RC_URL")
		}
		if server == "" {
			server = "https://rc.ohrats.party"
		}
		return runRemoteShell(ctx, dir, server, *device, *command)
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		if runtime.GOOS == "windows" {
			shell = os.Getenv("COMSPEC")
			if shell == "" {
				shell = "cmd.exe"
			}
		} else {
			shell = "/bin/sh"
		}
	}
	var cmd *exec.Cmd
	if *command != "" {
		if runtime.GOOS == "windows" {
			cmd = exec.CommandContext(ctx, shell, "/c", *command)
		} else {
			cmd = exec.CommandContext(ctx, shell, "-c", *command)
		}
	} else {
		cmd = exec.CommandContext(ctx, shell)
	}
	cmd.Dir, _ = os.Getwd()
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("%s exited with status %d", filepath.Base(shell), exit.ExitCode())
		}
		return err
	}
	return nil
}
