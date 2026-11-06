package cli

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/OhRats-Technologies/rc/pkg/diagnostics"
	"github.com/OhRats-Technologies/rc/pkg/node"
	"github.com/OhRats-Technologies/rc/pkg/platform"
)

func runLogs(_ context.Context, args []string) error {
	fs := flag.NewFlagSet("logs", flag.ContinueOnError)
	stateDir := fs.String("state-dir", "", "Override state directory")
	service := fs.String("service", "rc", "Service name")
	components := fs.Bool("components", false, "Show component/runtime diagnostics")
	follow := fs.Bool("follow", false, "Stream new entries")
	fs.BoolVar(follow, "f", false, "Stream new entries")
	if err := fs.Parse(reorderArgs(fs, args)); err != nil {
		return err
	}
	limit := 20
	if fs.NArg() > 0 {
		if _, err := fmt.Sscanf(fs.Arg(0), "%d", &limit); err != nil || limit < 1 || limit > 100 {
			return fmt.Errorf("log limit must be between 1 and 100")
		}
	}
	dir, err := node.ResolveStateDir(*stateDir)
	if err != nil {
		return err
	}
	if *components {
		return showComponentLogs(dir, limit, *follow)
	}
	_ = service
	cursor := ""
	for {
		entries, err := platform.Recent(dir, cursor, limit)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			fmt.Printf("%s  %s\n", entry.Timestamp, entry.Message)
			cursor = entry.Cursor
		}
		if !*follow {
			return nil
		}
		time.Sleep(time.Second)
	}
}

func showComponentLogs(dir string, limit int, follow bool) error {
	last := ""
	for {
		events, err := diagnostics.Recent(dir, limit)
		if err != nil {
			return err
		}
		for _, event := range events {
			key := event.Timestamp + event.Code
			if key == last {
				continue
			}
			fmt.Printf("%s %s %s %s: %s\n", event.Timestamp, event.Level, event.Source, event.Code, event.Message)
			last = key
		}
		if !follow {
			return nil
		}
		time.Sleep(time.Second)
	}
}

func runPs(_ context.Context, args []string) error {
	fs := flag.NewFlagSet("ps", flag.ContinueOnError)
	watch := fs.Bool("watch", false, "Refresh every two seconds")
	commands := fs.Bool("commands", false, "Include OS argument vectors")
	if err := fs.Parse(reorderArgs(fs, args)); err != nil {
		return err
	}
	for {
		processes, err := platform.Processes(*commands)
		if err != nil {
			return err
		}
		for _, process := range processes {
			if *commands {
				fmt.Printf("%d\t%s\t%v", process.PID, process.Name, process.Argv)
				if process.Truncated {
					fmt.Print(" [truncated]")
				}
				fmt.Println()
			} else {
				fmt.Printf("%d\t%s\n", process.PID, process.Name)
			}
		}
		if !*watch {
			return nil
		}
		time.Sleep(2 * time.Second)
	}
}
