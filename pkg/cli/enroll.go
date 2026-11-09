package cli

import (
	"context"
	"flag"
	"fmt"
	"strings"

	"github.com/OhRats-Technologies/rc/pkg/node"
)

// splitEnrollToken extracts the enrollment token before flag parsing so a
// base64url token that begins with "-" is never mistaken for a flag.
func splitEnrollToken(args []string) (string, []string) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			if i+1 < len(args) {
				return args[i+1], append(append([]string{}, args[:i]...), args[i+2:]...)
			}
			return "", args
		}
		if !strings.HasPrefix(arg, "-") {
			return arg, append(append([]string{}, args[:i]...), args[i+1:]...)
		}
		name := strings.TrimLeft(arg, "-")
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			continue
		}
		switch name {
		case "url", "name", "state-dir":
			i++
		case "h", "help":
		default:
			// Unknown flag-like arguments are the token. Enrollment tokens are
			// generated from the URL-safe base64 alphabet and may start with '-'.
			return arg, append(append([]string{}, args[:i]...), args[i+1:]...)
		}
	}
	return "", args
}

func runEnroll(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("enroll", flag.ContinueOnError)
	server := fs.String("url", "https://rc.ohrats.party", "RC server URL")
	name := fs.String("name", "", "Device display name")
	stateDir := fs.String("state-dir", "", "Override state directory")
	token, args := splitEnrollToken(args)
	if err := fs.Parse(reorderArgs(fs, args)); err != nil {
		return err
	}
	if token == "" {
		token = fs.Arg(0)
	}
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
