package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "version", "--version", "-version":
			fmt.Printf("OhRats Relay Node %s\n", version)
			return nil
		case "uninstall":
			return uninstall(args[1:])
		case "run":
			args = args[1:]
		case "enroll":
			if len(args) < 2 {
				return errors.New("usage: ohrats-relay enroll TOKEN")
			}
			args = append([]string{"--enroll", args[1]}, args[2:]...)
		}
	}
	return runNode(args)
}

func runNode(args []string) error {
	flags := flag.NewFlagSet("ohrats-relay", flag.ContinueOnError)
	serverURL := flags.String("url", env("RELAY_URL", "https://relay.ohrats.party"), "Relay server URL")
	enrollToken := flags.String("enroll", os.Getenv("RELAY_ENROLL_TOKEN"), "Fleet enrollment token")
	stateDir := flags.String("state-dir", env("RELAY_STATE_DIR", defaultStateDir()), "Node state directory")
	name := flags.String("name", "", "Device display name")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if err := os.MkdirAll(*stateDir, 0700); err != nil {
		return err
	}

	value, err := loadState(*stateDir)
	if errors.Is(err, os.ErrNotExist) {
		if strings.TrimSpace(*enrollToken) == "" {
			return errors.New("device is not enrolled; run ohrats-relay enroll TOKEN")
		}
		value, err = enroll(*serverURL, *enrollToken, *name)
		if err != nil {
			return fmt.Errorf("enroll: %w", err)
		}
		if err := saveState(*stateDir, value); err != nil {
			return fmt.Errorf("save state: %w", err)
		}
		log.Printf("enrolled device %s", value.DeviceID)
	} else if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	for {
		if err := connect(ctx, *serverURL, value); err != nil && ctx.Err() == nil {
			log.Printf("connection ended: %v", err)
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

func uninstall(args []string) error {
	flags := flag.NewFlagSet("ohrats-relay uninstall", flag.ContinueOnError)
	serverURL := flags.String("url", env("RELAY_URL", "https://relay.ohrats.party"), "Relay server URL")
	stateDir := flags.String("state-dir", env("RELAY_STATE_DIR", defaultStateDir()), "Node state directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if value, err := loadState(*stateDir); err == nil {
		if err := unregister(*serverURL, value); err != nil {
			log.Printf("warning: server unregister failed: %v", err)
		}
	}
	if err := os.RemoveAll(*stateDir); err != nil {
		return err
	}
	executable, _ := os.Executable()
	if filepath.Base(executable) == "ohrats-relay" {
		_ = os.Remove(executable)
	}
	if executable != "" {
		_ = os.Remove(filepath.Join(filepath.Dir(executable), "relay-agent"))
	}
	fmt.Println("OhRats Relay Node uninstalled")
	return nil
}
