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

const defaultServer = "https://relay.ohrats.party"

func commandDefaults(stateDir, urlFlag string) (string, string, nodeConfig) {
	dir := resolveStateDir(stateDir)
	config, _ := loadConfig(dir)
	server := urlFlag
	if server == "" {
		server = env("RELAY_URL", config.Server)
	}
	if server == "" {
		server = defaultServer
	}
	return dir, server, config
}

func enrollCommand(args []string) error {
	flags := flag.NewFlagSet("ohrats-relay enroll", flag.ContinueOnError)
	stateDir := flags.String("state-dir", "", "Node state directory")
	serverFlag := flags.String("url", "", "Relay server URL")
	nameFlag := flags.String("name", "", "Device display name")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("usage: ohrats-relay enroll [flags] TOKEN")
	}
	dir, server, config := commandDefaults(*stateDir, *serverFlag)
	if existing, err := loadState(dir); err == nil {
		return fmt.Errorf("already enrolled as %s; uninstall first to replace identity", existing.DeviceID)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	name := *nameFlag
	if name == "" {
		name = env("RELAY_NAME", config.Name)
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
	if err := saveState(dir, value); err != nil {
		return err
	}
	fmt.Printf("Enrolled %s\n", value.DeviceID)
	return nil
}

func runNode(args []string) error {
	flags := flag.NewFlagSet("ohrats-relay run", flag.ContinueOnError)
	stateDir := flags.String("state-dir", "", "Node state directory")
	serverFlag := flags.String("url", "", "Relay server URL")
	if err := flags.Parse(args); err != nil {
		return err
	}
	dir, server, _ := commandDefaults(*stateDir, *serverFlag)
	value, err := loadState(dir)
	if errors.Is(err, os.ErrNotExist) {
		return errors.New("not enrolled; run ohrats-relay enroll TOKEN")
	}
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	manager := newProcessManager()
	defer manager.shutdown()
	fmt.Printf("Connecting to %s as %s\n", server, value.DeviceID)
	for {
		if err := connect(ctx, server, value, dir, manager); err != nil && ctx.Err() == nil {
			if errors.Is(err, errNodeRemoved) {
				fmt.Println("Device removed from Relay")
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
	flags := flag.NewFlagSet("ohrats-relay update", flag.ContinueOnError)
	stateDir := flags.String("state-dir", "", "Node state directory")
	serverFlag := flags.String("url", "", "Relay server URL")
	if err := flags.Parse(args); err != nil {
		return err
	}
	_, server, _ := commandDefaults(*stateDir, *serverFlag)
	if err := replaceExecutable(server); err != nil {
		return err
	}
	fmt.Println("OhRats Relay Node updated")
	return nil
}

func uninstallCommand(args []string) error {
	flags := flag.NewFlagSet("ohrats-relay uninstall", flag.ContinueOnError)
	stateDir := flags.String("state-dir", "", "Node state directory")
	serverFlag := flags.String("url", "", "Relay server URL")
	if err := flags.Parse(args); err != nil {
		return err
	}
	dir, server, _ := commandDefaults(*stateDir, *serverFlag)
	if value, err := loadState(dir); err == nil {
		if err := unregister(server, value); err != nil {
			fmt.Fprintf(os.Stderr, "warning: server unregister failed: %v\n", err)
		}
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	executable, _ := os.Executable()
	if filepath.Base(executable) == "ohrats-relay" {
		_ = os.Remove(executable)
	}
	fmt.Println("OhRats Relay Node uninstalled")
	return nil
}
