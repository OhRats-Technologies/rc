package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const serviceLabel = "party.ohrats.rc"

func serviceCommand(args []string) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Println("Usage: rc service <install|start|stop|status|uninstall>")
		return nil
	}
	dir := resolveStateDir("")
	switch args[0] {
	case "install":
		if _, err := loadState(dir); err != nil {
			return errors.New("enroll this machine before installing the RC service")
		}
		return installService(dir)
	case "start":
		if _, err := loadState(dir); err != nil {
			return errors.New("enroll this machine before starting the RC service")
		}
		return startService(dir)
	case "stop":
		return stopService()
	case "status":
		return statusService()
	case "uninstall":
		return removeService()
	default:
		return fmt.Errorf("unknown service command %q", args[0])
	}
}

func installService(stateDir string) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		return err
	}
	switch runtime.GOOS {
	case "darwin":
		return installLaunchAgent(executable, stateDir)
	case "linux":
		return installSystemdUser(executable, stateDir)
	default:
		return fmt.Errorf("background service is not supported on %s", runtime.GOOS)
	}
}

func startService(stateDir string) error {
	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		path := filepath.Join(home, "Library", "LaunchAgents", serviceLabel+".plist")
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			return installService(stateDir)
		}
		return command("launchctl", "kickstart", "-k", fmt.Sprintf("gui/%d/%s", os.Getuid(), serviceLabel))
	case "linux":
		return command("systemctl", "--user", "start", "rc.service")
	default:
		return fmt.Errorf("background service is not supported on %s", runtime.GOOS)
	}
}

func serviceInstalled() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	var path string
	switch runtime.GOOS {
	case "darwin":
		path = filepath.Join(home, "Library", "LaunchAgents", serviceLabel+".plist")
	case "linux":
		path = filepath.Join(home, ".config", "systemd", "user", "rc.service")
	default:
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

func restartService() error {
	switch runtime.GOOS {
	case "darwin":
		return command("launchctl", "kickstart", "-k", fmt.Sprintf("gui/%d/%s", os.Getuid(), serviceLabel))
	case "linux":
		return command("systemctl", "--user", "restart", "rc.service")
	default:
		return nil
	}
}

func stopService() error {
	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		path := filepath.Join(home, "Library", "LaunchAgents", serviceLabel+".plist")
		_ = exec.Command("launchctl", "bootout", fmt.Sprintf("gui/%d", os.Getuid()), path).Run()
		return nil
	case "linux":
		_ = exec.Command("systemctl", "--user", "stop", "rc.service").Run()
		return nil
	default:
		return nil
	}
}

func statusService() error {
	switch runtime.GOOS {
	case "darwin":
		return command("launchctl", "print", fmt.Sprintf("gui/%d/%s", os.Getuid(), serviceLabel))
	case "linux":
		return command("systemctl", "--user", "status", "--no-pager", "rc.service")
	default:
		return fmt.Errorf("background service is not supported on %s", runtime.GOOS)
	}
}

func removeService() error {
	_ = stopService()
	return disarmService()
}

func disarmService() error {
	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		return removeIfExists(filepath.Join(home, "Library", "LaunchAgents", serviceLabel+".plist"))
	case "linux":
		home, _ := os.UserHomeDir()
		path := filepath.Join(home, ".config", "systemd", "user", "rc.service")
		_ = exec.Command("systemctl", "--user", "disable", "rc.service").Run()
		if err := removeIfExists(path); err != nil {
			return err
		}
		_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
		return nil
	default:
		return nil
	}
}

func installLaunchAgent(executable, stateDir string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	path := filepath.Join(dir, serviceLabel+".plist")
	logPath := filepath.Join(stateDir, "node.log")
	escape := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;").Replace
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict><key>Label</key><string>%s</string><key>ProgramArguments</key><array><string>%s</string><string>run</string><string>--state-dir</string><string>%s</string></array><key>RunAtLoad</key><true/><key>KeepAlive</key><dict><key>SuccessfulExit</key><false/></dict><key>ThrottleInterval</key><integer>3</integer><key>StandardOutPath</key><string>%s</string><key>StandardErrorPath</key><string>%s</string></dict></plist>
`, serviceLabel, escape(executable), escape(stateDir), escape(logPath), escape(logPath))
	if err := os.WriteFile(path, []byte(plist), 0644); err != nil {
		return err
	}
	domain := fmt.Sprintf("gui/%d", os.Getuid())
	_ = exec.Command("launchctl", "bootout", domain, path).Run()
	if err := command("launchctl", "bootstrap", domain, path); err != nil {
		return err
	}
	return command("launchctl", "kickstart", "-k", domain+"/"+serviceLabel)
}

func installSystemdUser(executable, stateDir string) error {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return errors.New("systemd user services are unavailable; run `rc run` manually")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, ".config", "systemd", "user")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	unit := fmt.Sprintf("[Unit]\nDescription=RC Node\nAfter=network-online.target\n\n[Service]\nExecStart=%s run --state-dir %s\nRestart=on-failure\nRestartSec=3\n\n[Install]\nWantedBy=default.target\n", strconv.Quote(executable), strconv.Quote(stateDir))
	if err := os.WriteFile(filepath.Join(dir, "rc.service"), []byte(unit), 0644); err != nil {
		return err
	}
	if err := command("systemctl", "--user", "daemon-reload"); err != nil {
		return err
	}
	return command("systemctl", "--user", "enable", "--now", "rc.service")
}

func command(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
