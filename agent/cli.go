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
	if err != nil || info.Mode()&os.ModeCharDevice == 0 || os.Getenv("NO_COLOR") != "" {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

func helpRow(code, label, description string) {
	padding := 18 - len(label)
	if padding < 1 {
		padding = 1
	}
	fmt.Printf("  %s%s%s\n", color(code, label), strings.Repeat(" ", padding), description)
}

func printHelp() {
	fmt.Printf("%s %s\n\n", color("1", "OhRats RC Node"), version)
	fmt.Println("Outbound device node for OhRats RC.")
	fmt.Printf("\n%s %s\n\n", color("1", "Usage:"), "ohrats-rc <command> [flags]")
	fmt.Println(color("1", "Commands:"))
	helpRow("35;1", "login", "Sign in with a passkey")
	helpRow("35;1", "logout", "Sign out this CLI")
	helpRow("35;1", "run", "Connect this node to RC")
	helpRow("35;1", "enroll TOKEN", "Enroll this machine")
	helpRow("35;1", "status", "Show node and RC status")
	helpRow("35;1", "devices", "List devices in your account")
	helpRow("35;1", "shell DEVICE", "Open a remote terminal")
	helpRow("35;1", "run DEVICE -- CMD", "Run a remote command")
	helpRow("35;1", "actions", "List saved actions")
	helpRow("35;1", "action run", "Run a saved action")
	fmt.Println()
	helpRow("34;1", "service", "Manage the background RC Node")
	helpRow("34;1", "update", "Update this RC Node")
	helpRow("34;1", "device delete ID", "Remove a device from RC")
	helpRow("34;1", "config", "Read or change node configuration")
	helpRow("34;1", "uninstall", "Unenroll and remove this node")
	fmt.Println()
	helpRow("33;1", "version", "Print version")
	helpRow("36;1", "help", "Print this help")
	fmt.Printf("\n%s\n  ohrats-rc <command> --help\n", color("1", "Command help:"))
}

func accountFlags(name string, args []string) (*flag.FlagSet, *string, *string, error) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	dir := resolveStateDir("")
	config, _ := loadConfig(dir)
	account, _ := loadAccountSession(dir)
	defaultURL := strings.TrimSpace(os.Getenv("RC_URL"))
	if defaultURL == "" {
		defaultURL = account.Server
	}
	if defaultURL == "" {
		defaultURL = config.Server
	}
	if defaultURL == "" {
		defaultURL = defaultServer
	}
	server := flags.String("url", defaultURL, "RC server URL")
	token := flags.String("token", env("RC_API_TOKEN", ""), "RC proof-of-possession API key override")
	if err := flags.Parse(args); err != nil {
		return flags, server, token, err
	}
	if *token == "" {
		if account.Token != "" && strings.TrimRight(account.Server, "/") == strings.TrimRight(*server, "/") {
			*token = account.Token
		}
	}
	return flags, server, token, nil
}

func devicesCommand(args []string) error {
	flags, server, token, err := accountFlags("ohrats-rc devices", args)
	if err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("usage: ohrats-rc devices [--token TOKEN]")
	}
	devices, err := listAccountDevices(*server, *token)
	if err != nil {
		return err
	}
	if len(devices) == 0 {
		fmt.Println("No devices")
		return nil
	}
	for _, device := range devices {
		state := "offline"
		if device.Online {
			state = "online"
		}
		fmt.Printf("%s  %s  %s  %s  %s\n", device.ID, device.Name, device.Workspace, state, device.AgentVersion)
	}
	return nil
}

func deviceCommand(args []string) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Println("Usage: ohrats-rc device delete [--token TOKEN] ID")
		fmt.Println("       RC_API_TOKEN=... ohrats-rc device delete ID")
		return nil
	}
	if args[0] != "delete" {
		return fmt.Errorf("unknown device command %q", args[0])
	}
	flags, server, token, err := accountFlags("ohrats-rc device delete", args[1:])
	if err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("usage: ohrats-rc device delete [--token TOKEN] ID")
	}
	device, err := resolveAccountDevice(*server, *token, flags.Arg(0))
	if err != nil {
		return err
	}
	control, err := openRemoteControl(*server, *token, device)
	if err != nil {
		return fmt.Errorf("secure device removal: %w", err)
	}
	if err := control.request(wireMessage{Type: "node.remove"}); err != nil {
		control.close()
		return fmt.Errorf("secure device removal: %w", err)
	}
	control.close()
	if err := deleteAccountDevice(*server, *token, device.ID); err != nil {
		return err
	}
	fmt.Printf("Removed %s\n", device.ID)
	return nil
}

func printConfigHelp() {
	fmt.Println("Usage: ohrats-rc config <command>")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  show                  Show effective configuration")
	fmt.Println("  path                  Print the config file path")
	fmt.Println("  set server URL        Set the RC server")
	fmt.Println("  set name NAME         Set the default enrollment name")
	fmt.Println("  unset server|name     Reset a setting")
}

func commandHelp(command string) error {
	switch command {
	case "login":
		return loginCommand([]string{"--help"})
	case "logout":
		return logoutCommand([]string{"--help"})
	case "config":
		printConfigHelp()
		return nil
	case "run":
		return runNode([]string{"--help"})
	case "enroll":
		return enrollCommand([]string{"--help"})
	case "status":
		return statusCommand([]string{"--help"})
	case "devices":
		return devicesCommand([]string{"--help"})
	case "shell":
		return shellCommand([]string{"--help"})
	case "actions":
		return actionsCommand([]string{"--help"})
	case "action":
		fmt.Println("Usage: ohrats-rc action run ACTION --device DEVICE [--confirm] [--token TOKEN]")
		return nil
	case "device":
		return deviceCommand([]string{"--help"})
	case "service":
		return serviceCommand([]string{"--help"})
	case "update":
		return updateCommand([]string{"--help"})
	case "uninstall":
		return uninstallCommand([]string{"--help"})
	default:
		return fmt.Errorf("unknown command %q", command)
	}
}

func statusCommand(args []string) error {
	flags := flag.NewFlagSet("ohrats-rc status", flag.ContinueOnError)
	stateDir := flags.String("state-dir", "", "Node state directory")
	serverFlag := flags.String("url", "", "RC server URL")
	if err := flags.Parse(args); err != nil {
		return err
	}
	dir, server, _ := commandDefaults(*stateDir, *serverFlag)
	value, err := loadState(dir)
	fmt.Printf("OhRats RC Node %s\n", version)
	fmt.Printf("Config  %s\n", dir)
	fmt.Printf("RC   %s\n", server)
	if errors.Is(err, os.ErrNotExist) {
		fmt.Println("State   not enrolled")
		return nil
	}
	if err != nil {
		return err
	}
	fmt.Printf("Device  %s\n", value.DeviceID)
	remote, err := fetchStatus(server, value)
	if err != nil {
		fmt.Printf("Remote  unavailable (%v)\n", err)
		return nil
	}
	fmt.Printf("Name    %s\n", remote.Name)
	fmt.Printf("Online  %v\n", remote.Online)
	fmt.Printf("Agent   %s\n", remote.AgentVersion)
	return nil
}

func configCommand(args []string) error {
	stateDir := resolveStateDir("")
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h" || args[0] == "help") {
		printConfigHelp()
		return nil
	}
	if len(args) == 0 || args[0] == "show" {
		config, _ := loadConfig(stateDir)
		server := config.Server
		if server == "" {
			server = defaultServer
		}
		name := config.Name
		if name == "" {
			name = "<hostname>"
		}
		fmt.Printf("server  %s\nname    %s\nfile    %s\n", server, name, configPath(stateDir))
		return nil
	}
	if args[0] == "path" {
		fmt.Println(configPath(stateDir))
		return nil
	}
	if len(args) < 2 {
		return errors.New("usage: ohrats-rc config <show|path|set|unset>")
	}
	config, _ := loadConfig(stateDir)
	switch args[0] {
	case "set":
		if len(args) < 3 {
			return errors.New("usage: ohrats-rc config set <server|name> VALUE")
		}
		value := strings.TrimSpace(strings.Join(args[2:], " "))
		switch args[1] {
		case "server":
			config.Server = strings.TrimRight(value, "/")
		case "name":
			config.Name = value
		default:
			return fmt.Errorf("unknown config key %q", args[1])
		}
	case "unset":
		switch args[1] {
		case "server":
			config.Server = ""
		case "name":
			config.Name = ""
		default:
			return fmt.Errorf("unknown config key %q", args[1])
		}
	default:
		return fmt.Errorf("unknown config command %q", args[0])
	}
	if err := saveConfig(stateDir, config); err != nil {
		return err
	}
	if args[0] == "unset" {
		fmt.Printf("unset %s\n", args[1])
	} else {
		fmt.Printf("%s  %s\n", args[1], strings.Join(args[2:], " "))
	}
	return nil
}
