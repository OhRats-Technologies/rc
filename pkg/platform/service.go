package platform

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const (
	linuxServiceUnit = "rc.service"
	macOSServiceName = "party.ohrats.rc"
	windowsTaskName  = "OhRats RC Node"
)

// ServiceStatus reports whether the per-user background service is installed
// and running. detail is a short human-readable state and never includes
// arguments or environment values.
func ServiceStatus(ctx context.Context) (installed bool, running bool, detail string) {
	switch runtime.GOOS {
	case "linux":
		path := linuxServicePath()
		if path == "" {
			return false, false, "home directory unavailable"
		}
		if _, err := os.Stat(path); err != nil {
			return false, false, "unit file missing"
		}
		if _, err := exec.LookPath("systemctl"); err != nil {
			return true, false, "systemctl unavailable"
		}
		output, err := exec.CommandContext(ctx, "systemctl", "--user", "is-active", linuxServiceUnit).Output()
		state := strings.TrimSpace(string(output))
		if err != nil {
			if state == "" {
				state = "inactive"
			}
			return true, false, state
		}
		return true, state == "active", state
	case "darwin":
		path := macOSServicePath()
		if path == "" {
			return false, false, "home directory unavailable"
		}
		if _, err := os.Stat(path); err != nil {
			return false, false, "launch agent missing"
		}
		if _, err := exec.LookPath("launchctl"); err != nil {
			return true, false, "launchctl unavailable"
		}
		target := "gui/" + strconv.Itoa(os.Getuid()) + "/" + macOSServiceName
		if err := exec.CommandContext(ctx, "launchctl", "print", target).Run(); err != nil {
			return true, false, "not loaded"
		}
		return true, true, "loaded"
	case "windows":
		if _, err := exec.LookPath("schtasks.exe"); err != nil {
			return false, false, "schtasks unavailable"
		}
		err := exec.CommandContext(ctx, "schtasks.exe", "/Query", "/TN", windowsTaskName).Run()
		return err == nil, false, "scheduled task queried"
	default:
		return false, false, "unsupported platform"
	}
}

// ServiceRestart restarts the background service when it is installed.
func ServiceRestart(ctx context.Context) error {
	switch runtime.GOOS {
	case "linux":
		if _, err := exec.LookPath("systemctl"); err != nil {
			return nil
		}
		return exec.CommandContext(ctx, "systemctl", "--user", "restart", linuxServiceUnit).Run()
	case "darwin":
		if _, err := exec.LookPath("launchctl"); err != nil {
			return nil
		}
		target := "gui/" + strconv.Itoa(os.Getuid()) + "/" + macOSServiceName
		return exec.CommandContext(ctx, "launchctl", "kickstart", "-k", target).Run()
	case "windows":
		if _, err := exec.LookPath("schtasks.exe"); err != nil {
			return nil
		}
		return exec.CommandContext(ctx, "schtasks.exe", "/End", "/TN", windowsTaskName).Run()
	default:
		return nil
	}
}

func linuxServicePath() string {
	home, err := platformUserHome()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "systemd", "user", linuxServiceUnit)
}

func macOSServicePath() string {
	home, err := platformUserHome()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Library", "LaunchAgents", macOSServiceName+".plist")
}
