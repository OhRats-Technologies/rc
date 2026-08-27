package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

const defaultServer = "https://rc.ohrats.party"

func commandDefaults(stateDir, urlFlag string) (string, string, nodeConfig) {
	dir := resolveStateDir(stateDir)
	config, _ := loadConfig(dir)
	server := urlFlag
	if server == "" {
		server = env("RC_URL", config.Server)
	}
	if server == "" {
		server = defaultServer
	}
	return dir, server, config
}

func enrollCommand(args []string) error {
	flags := flag.NewFlagSet("rc enroll", flag.ContinueOnError)
	stateDir := flags.String("state-dir", "", "Node state directory")
	serverFlag := flags.String("url", "", "RC server URL")
	nameFlag := flags.String("name", "", "Device display name")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("usage: rc enroll [flags] TOKEN")
	}
	dir, server, config := commandDefaults(*stateDir, *serverFlag)
	if existing, err := loadState(dir); err == nil {
		remote, statusErr := fetchStatus(server, existing)
		switch {
		case statusErr == nil:
			return fmt.Errorf("this machine is already enrolled as %s (%s); remove or uninstall that enrollment before moving it", remote.Name, existing.DeviceID)
		case errors.Is(statusErr, errNodeRemoved):
			if lockHash(dir) != "" {
				return fmt.Errorf("locked enrollment %s is no longer recognized by RC; local state was preserved. Use an owner-authorized remove while the Node is online, or uninstall locally", existing.DeviceID)
			}
			if err := os.Remove(statePath(dir)); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			fmt.Printf("Cleared stale enrollment %s\n", existing.DeviceID)
		default:
			return fmt.Errorf("could not verify existing enrollment %s: %w", existing.DeviceID, statusErr)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	name := *nameFlag
	if name == "" {
		name = env("RC_NAME", config.Name)
	}
	if *serverFlag != "" {
		config.Server = server
	}
	if *nameFlag != "" {
		config.Name = name
	}
	if *serverFlag != "" || *nameFlag != "" {
		if err := saveConfig(dir, config); err != nil {
			return err
		}
	}
	value, err := enroll(server, flags.Arg(0), name)
	if err != nil {
		return err
	}
	if changed, err := ensureTransportIdentity(&value); err != nil {
		return err
	} else if changed {
		if err := saveState(dir, value); err != nil {
			return err
		}
	}
	if err := saveState(dir, value); err != nil {
		return err
	}
	fmt.Printf("Enrolled %s\n", value.DeviceID)
	return nil
}

func runNode(args []string) error {
	flags := flag.NewFlagSet("rc run", flag.ContinueOnError)
	stateDir := flags.String("state-dir", "", "Node state directory")
	serverFlag := flags.String("url", "", "RC server URL")
	if err := flags.Parse(args); err != nil {
		return err
	}
	dir, server, _ := commandDefaults(*stateDir, *serverFlag)
	value, err := loadState(dir)
	if errors.Is(err, os.ErrNotExist) {
		return errors.New("not enrolled; run rc enroll TOKEN")
	}
	if err != nil {
		return err
	}
	if changed, err := ensureTransportIdentity(&value); err != nil {
		return err
	} else if changed {
		if err := saveState(dir, value); err != nil {
			return err
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	manager := newProcessManager()
	defer manager.shutdown()
	fmt.Printf("Connecting to %s as %s\n", server, value.DeviceID)
	for {
		if err := connect(ctx, server, value, dir, manager); err != nil && ctx.Err() == nil {
			if errors.Is(err, errNodeRemoved) {
				if serviceErr := disarmService(); serviceErr != nil {
					fmt.Fprintf(os.Stderr, "warning: could not remove background service: %v\n", serviceErr)
				}
				fmt.Println("This device was removed from RC; local enrollment cleared.")
				fmt.Println("Enroll it again from RC to reconnect.")
				return nil
			}
			fmt.Fprintf(os.Stderr, "connection ended: %v\n", err)
		}
		if ctx.Err() != nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(3 * time.Second):
		}
	}
}

func updateCommand(args []string) error {
	flags := flag.NewFlagSet("rc update", flag.ContinueOnError)
	flags.String("state-dir", "", "Node state directory (deprecated for update)")
	flags.String("url", "", "RC server URL (deprecated for update)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if err := replaceExecutable(); err != nil {
		return err
	}
	if serviceInstalled() {
		dir := resolveStateDir("")
		if _, err := loadState(dir); errors.Is(err, os.ErrNotExist) {
			if err := removeService(); err != nil {
				return fmt.Errorf("updated, but could not remove stale RC Node service: %w", err)
			}
			fmt.Println("RC Node updated; removed stale background service because this machine is not enrolled")
			return nil
		} else if err != nil {
			return err
		}
		if err := restartService(); err != nil {
			return fmt.Errorf("updated, but could not restart RC Node: %w", err)
		}
		fmt.Println("RC Node updated and restarted")
		return nil
	}
	fmt.Println("RC Node updated")
	return nil
}

func uninstallCommand(args []string) error {
	flags := flag.NewFlagSet("rc uninstall", flag.ContinueOnError)
	stateDir := flags.String("state-dir", "", "Node state directory")
	serverFlag := flags.String("url", "", "RC server URL")
	if err := flags.Parse(args); err != nil {
		return err
	}
	dir, server, _ := commandDefaults(*stateDir, *serverFlag)
	_ = removeService()
	if value, err := loadState(dir); err == nil {
		if err := unregister(server, value); err != nil {
			fmt.Fprintf(os.Stderr, "warning: server unregister failed: %v\n", err)
		}
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	executable, _ := os.Executable()
	if filepath.Base(executable) == "rc" {
		_ = os.Remove(executable)
	}
	fmt.Println("RC Node uninstalled")
	return nil
}
