package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) { return }
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		printHelp()
		return nil
	}
	switch args[0] {
	case "help", "--help", "-h":
		if len(args) > 1 && args[0] == "help" { return commandHelp(args[1]) }
		printHelp()
		return nil
	case "version", "--version", "-version":
		fmt.Printf("OhRats Relay Node %s\n", version)
		return nil
	case "run":
		return runNode(args[1:])
	case "enroll":
		return enrollCommand(args[1:])
	case "status":
		return statusCommand(args[1:])
	case "config":
		return configCommand(args[1:])
	case "uninstall":
		return uninstallCommand(args[1:])
	default:
		printHelp()
		return fmt.Errorf("unknown command %q", args[0])
	}
}
