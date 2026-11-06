package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/OhRats-Technologies/rc/pkg/kernel"
	"github.com/OhRats-Technologies/rc/pkg/node"
	"github.com/OhRats-Technologies/rc/pkg/platform"
	"github.com/OhRats-Technologies/rc/pkg/version"
)

var Version = version.Version

func Run(ctx context.Context, args []string) error {
	compDir := ""
	var cleanArgs []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--component-dir" && i+1 < len(args) {
			compDir = args[i+1]
			i++
		} else {
			cleanArgs = append(cleanArgs, args[i])
		}
	}

	if len(cleanArgs) == 0 {
		PrintUsage()
		return nil
	}

	switch cleanArgs[0] {
	case "version", "--version", "-v":
		fmt.Printf("RC %s\n", Version)
		return nil

	case "crypto-check":
		return runCryptoCheck(compDir)

	case "status":
		return runStatus(ctx, cleanArgs[1:])

	case "config":
		return runConfig(cleanArgs[1:])

	case "enroll":
		return runEnroll(ctx, cleanArgs[1:])

	case "run":
		return runRun(ctx, cleanArgs[1:])

	case "logs":
		return runLogs(ctx, cleanArgs[1:])

	case "ps":
		return runPs(ctx, cleanArgs[1:])

	case "devices":
		return runDevices(ctx, cleanArgs[1:])

	case "login":
		return runLogin(cleanArgs[1:])

	case "logout":
		return runLogout(cleanArgs[1:])

	case "service":
		return runService(ctx, cleanArgs[1:])

	case "doctor":
		return runDoctor(ctx, cleanArgs[1:])

	case "upgrade":
		return runUpgrade(ctx, cleanArgs[1:])

	case "update":
		return runUpdate(ctx, cleanArgs[1:])

	case "repair":
		return runRepair(ctx, cleanArgs[1:])

	case "policy-check":
		return runPolicyCheck(compDir)

	case "commands":
		return runCommands(compDir)

	case "shell":
		return runShell(ctx, cleanArgs[1:])

	case "help", "--help", "-h":
		PrintUsage()
		return nil

	default:
		dir := compDir
		if dir == "" {
			dir, _ = platform.ComponentDir()
		}
		if dir != "" {
			engine, err := kernel.NewEngine()
			if err == nil {
				found, code, message, invokeErr := engine.InvokeCommand(dir, cleanArgs[0], cleanArgs[1:])
				engine.Close()
				if found {
					if invokeErr != nil {
						return invokeErr
					}
					if code != 0 {
						return fmt.Errorf("component command exited with status %d", code)
					}
					if message != "" {
						return fmt.Errorf("%s", message)
					}
					return nil
				}
			}
		}
		return fmt.Errorf("unknown command: %s; run 'rc help' for usage", cleanArgs[0])
	}
}

// reorderArgs makes Go's flag package accept flags after positional arguments by
// moving flags (and their values) ahead of positional arguments.
func reorderArgs(fs *flag.FlagSet, args []string) []string {
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if len(arg) < 2 || arg[0] != '-' {
			positional = append(positional, arg)
			continue
		}
		flags = append(flags, arg)
		name := strings.TrimLeft(arg, "-")
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			name = name[:eq]
		}
		defined := fs.Lookup(name)
		if defined == nil || strings.Contains(arg, "=") {
			continue
		}
		if boolFlag, ok := defined.Value.(interface{ IsBoolFlag() bool }); ok && boolFlag.IsBoolFlag() {
			continue
		}
		if i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}
	return append(flags, positional...)
}

func runCryptoCheck(compDir string) error {
	if compDir == "" {
		var err error
		compDir, err = platform.ComponentDir()
		if err != nil {
			return err
		}
	}
	rt, err := kernel.NewRuntime(compDir)
	if err != nil {
		return err
	}
	defer rt.Close()
	_, _ = rt.Reconcile()
	if _, ok := rt.Entry("crypto-control"); !ok {
		return fmt.Errorf("control crypto provider missing")
	}
	fmt.Println("control crypto: ok")
	return nil
}

func runStatus(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	stateDir := fs.String("state-dir", "", "Override state directory")
	server := fs.String("url", "", "RC server URL")
	if err := fs.Parse(reorderArgs(fs, args)); err != nil {
		return err
	}

	dir, err := node.ResolveStateDir(*stateDir)
	if err != nil {
		return err
	}

	st, err := node.LoadState(dir)
	if err != nil {
		fmt.Printf("Node is not enrolled (state directory: %s)\n", dir)
		return nil
	}

	cfg, _ := node.LoadConfig(dir)
	srv := *server
	if srv == "" {
		srv = cfg.Server
	}
	if srv == "" {
		srv = os.Getenv("RC_URL")
	}
	if srv == "" {
		srv = "https://rc.ohrats.party"
	}

	fmt.Printf("Device ID: %s\n", st.DeviceID)
	fmt.Printf("Server:    %s\n", srv)
	if cfg.Name != "" {
		fmt.Printf("Name:      %s\n", cfg.Name)
	}

	remoteStatus, err := node.FetchStatus(ctx, srv, st)
	if err != nil {
		fmt.Printf("Remote:    offline or unreachable (%v)\n", err)
	} else {
		fmt.Printf("Remote:    online=%v, version=%s\n", remoteStatus.Online, remoteStatus.Version)
	}
	return nil
}

func runConfig(args []string) error {
	dir, err := platform.StateDir()
	if err != nil {
		return err
	}
	cfg, err := node.LoadConfig(dir)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		fmt.Printf("Server: %s\nName:   %s\n", cfg.Server, cfg.Name)
		return nil
	}
	if len(args) != 3 || args[0] != "set" {
		return fmt.Errorf("usage: rc config [set server|name VALUE]")
	}
	switch args[1] {
	case "server":
		cfg.Server = args[2]
	case "name":
		cfg.Name = args[2]
	default:
		return fmt.Errorf("unknown config key: %s", args[1])
	}
	return node.SaveConfig(dir, cfg)
}

func runEnroll(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("enroll", flag.ContinueOnError)
	server := fs.String("url", "https://rc.ohrats.party", "RC server URL")
	name := fs.String("name", "", "Device display name")
	stateDir := fs.String("state-dir", "", "Override state directory")
	if err := fs.Parse(reorderArgs(fs, args)); err != nil {
		return err
	}

	token := fs.Arg(0)
	if token == "" {
		return fmt.Errorf("enrollment token required")
	}

	dir, err := node.ResolveStateDir(*stateDir)
	if err != nil {
		return err
	}

	st, err := node.Enroll(ctx, *server, token, *name, Version)
	if err != nil {
		return fmt.Errorf("enrollment failed: %w", err)
	}

	if err := node.SaveState(dir, st); err != nil {
		return fmt.Errorf("failed to save state: %w", err)
	}

	cfg := &node.NodeConfig{
		Server: *server,
		Name:   *name,
	}
	_ = node.SaveConfig(dir, cfg)

	fmt.Printf("Successfully enrolled device %s (state saved to %s)\n", st.DeviceID, dir)
	return nil
}
