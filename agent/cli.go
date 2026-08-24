package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
)

func color(code, text string) string {
	info, err := os.Stdout.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 || os.Getenv("NO_COLOR") != "" { return text }
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

func helpRow(code, label, description string) {
	padding := 18 - len(label)
	if padding < 1 { padding = 1 }
	fmt.Printf("  %s%s%s\n", color(code, label), strings.Repeat(" ", padding), description)
}

func printHelp() {
	fmt.Printf("%s %s\n\n", color("1", "OhRats Relay Node"), version)
	fmt.Println("Outbound device node for Relay.")
	fmt.Printf("\n%s %s\n\n", color("1", "Usage:"), "ohrats-relay <command> [flags]")
	fmt.Println(color("1", "Commands:"))
	helpRow("35;1", "run", "Connect this node to Relay")
	helpRow("35;1", "enroll TOKEN", "Enroll this machine")
	helpRow("35;1", "status", "Show node and Relay status")
	fmt.Println()
	helpRow("34;1", "config", "Read or change node configuration")
	helpRow("34;1", "uninstall", "Unenroll and remove this node")
	fmt.Println()
	helpRow("33;1", "version", "Print version")
	helpRow("36;1", "help", "Print this help")
	fmt.Printf("\n%s\n  ohrats-relay <command> --help\n", color("1", "Command help:"))
}

func printConfigHelp() {
	fmt.Println("Usage: ohrats-relay config <command>")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  show                  Show effective configuration")
	fmt.Println("  path                  Print the config file path")
	fmt.Println("  set server URL        Set the Relay server")
	fmt.Println("  set name NAME         Set the default enrollment name")
	fmt.Println("  unset server|name     Reset a setting")
}

func commandHelp(command string) error {
	switch command {
	case "config": printConfigHelp(); return nil
	case "run": return runNode([]string{"--help"})
	case "enroll": return enrollCommand([]string{"--help"})
	case "status": return statusCommand([]string{"--help"})
	case "uninstall": return uninstallCommand([]string{"--help"})
	default: return fmt.Errorf("unknown command %q", command)
	}
}

func statusCommand(args []string) error {
	flags := flag.NewFlagSet("ohrats-relay status", flag.ContinueOnError)
	stateDir := flags.String("state-dir", "", "Node state directory")
	serverFlag := flags.String("url", "", "Relay server URL")
	if err := flags.Parse(args); err != nil { return err }
	dir, server, _ := commandDefaults(*stateDir, *serverFlag)
	value, err := loadState(dir)
	fmt.Printf("OhRats Relay Node %s\n", version)
	fmt.Printf("Config  %s\n", dir)
	fmt.Printf("Relay   %s\n", server)
	if errors.Is(err, os.ErrNotExist) { fmt.Println("State   not enrolled"); return nil }
	if err != nil { return err }
	fmt.Printf("Device  %s\n", value.DeviceID)
	remote, err := fetchStatus(server, value)
	if err != nil { fmt.Printf("Remote  unavailable (%v)\n", err); return nil }
	fmt.Printf("Name    %s\n", remote.Name)
	fmt.Printf("Online  %v\n", remote.Online)
	fmt.Printf("Agent   %s\n", remote.AgentVersion)
	return nil
}

func configCommand(args []string) error {
	stateDir := resolveStateDir("")
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h" || args[0] == "help") { printConfigHelp(); return nil }
	if len(args) == 0 || args[0] == "show" {
		config, _ := loadConfig(stateDir)
		server := config.Server; if server == "" { server = defaultServer }
		name := config.Name; if name == "" { name = "<hostname>" }
		fmt.Printf("server  %s\nname    %s\nfile    %s\n", server, name, configPath(stateDir))
		return nil
	}
	if args[0] == "path" { fmt.Println(configPath(stateDir)); return nil }
	if len(args) < 2 { return errors.New("usage: ohrats-relay config <show|path|set|unset>") }
	config, _ := loadConfig(stateDir)
	switch args[0] {
	case "set":
		if len(args) < 3 { return errors.New("usage: ohrats-relay config set <server|name> VALUE") }
		value := strings.TrimSpace(strings.Join(args[2:], " "))
		switch args[1] { case "server": config.Server = strings.TrimRight(value, "/"); case "name": config.Name = value; default: return fmt.Errorf("unknown config key %q", args[1]) }
	case "unset":
		switch args[1] { case "server": config.Server = ""; case "name": config.Name = ""; default: return fmt.Errorf("unknown config key %q", args[1]) }
	default:
		return fmt.Errorf("unknown config command %q", args[0])
	}
	if err := saveConfig(stateDir, config); err != nil { return err }
	if args[0] == "unset" { fmt.Printf("unset %s\n", args[1]) } else { fmt.Printf("%s  %s\n", args[1], strings.Join(args[2:], " ")) }
	return nil
}
