package platform

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// InstallService registers the per-user background Node service running the
// given executable and arguments. It does not enroll the machine.
func InstallService(executable string, args []string, stateDir string) error {
	switch runtime.GOOS {
	case "linux":
		path := linuxServicePath()
		if path == "" {
			return errors.New("home directory unavailable")
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return err
		}
		command := strings.Join(append([]string{executable}, args...), " ")
		unit := fmt.Sprintf(`[Unit]
Description=RC Node
After=network-online.target

[Service]
Type=simple
ExecStart=%s
Restart=on-failure
RestartSec=2

[Install]
WantedBy=default.target
`, command)
		if err := os.WriteFile(path, []byte(unit), 0o600); err != nil {
			return err
		}
		if _, err := exec.LookPath("systemctl"); err != nil {
			return nil
		}
		_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
		return exec.Command("systemctl", "--user", "enable", "--now", linuxServiceUnit).Run()
	case "darwin":
		path := macOSServicePath()
		if path == "" {
			return errors.New("home directory unavailable")
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return err
		}
		var arguments strings.Builder
		for _, value := range append([]string{executable}, args...) {
			fmt.Fprintf(&arguments, "<string>%s</string>", xmlEscape(value))
		}
		plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>%s</string>
<key>ProgramArguments</key><array>%s</array>
<key>RunAtLoad</key><true/>
<key>KeepAlive</key><true/>
</dict></plist>
`, macOSServiceName, arguments.String())
		if err := os.WriteFile(path, []byte(plist), 0o600); err != nil {
			return err
		}
		if _, err := exec.LookPath("launchctl"); err != nil {
			return nil
		}
		target := "gui/" + strconv.Itoa(os.Getuid())
		_ = exec.Command("launchctl", "bootout", target, path).Run()
		return exec.Command("launchctl", "bootstrap", target, path).Run()
	case "windows":
		if _, err := exec.LookPath("schtasks.exe"); err != nil {
			return errors.New("schtasks.exe is unavailable")
		}
		command := strings.Join(append([]string{executable}, args...), " ")
		_ = exec.Command("schtasks.exe", "/Delete", "/F", "/TN", windowsTaskName).Run()
		create := exec.Command("schtasks.exe", "/Create", "/F", "/SC", "ONLOGON", "/TN", windowsTaskName, "/TR", command)
		if err := create.Run(); err != nil {
			return err
		}
		return exec.Command("schtasks.exe", "/Run", "/TN", windowsTaskName).Run()
	default:
		return errors.New("background service is not supported on this platform")
	}
}

// StopService stops the installed background service if present.
func StopService(ctx context.Context) error {
	switch runtime.GOOS {
	case "linux":
		if _, err := exec.LookPath("systemctl"); err != nil {
			return nil
		}
		return exec.CommandContext(ctx, "systemctl", "--user", "stop", linuxServiceUnit).Run()
	case "darwin":
		if _, err := exec.LookPath("launchctl"); err != nil {
			return nil
		}
		target := "gui/" + strconv.Itoa(os.Getuid()) + "/" + macOSServiceName
		return exec.CommandContext(ctx, "launchctl", "bootout", target).Run()
	case "windows":
		if _, err := exec.LookPath("schtasks.exe"); err != nil {
			return nil
		}
		return exec.CommandContext(ctx, "schtasks.exe", "/End", "/TN", windowsTaskName).Run()
	default:
		return nil
	}
}

// UninstallService removes the background service registration.
func UninstallService(ctx context.Context) error {
	_ = StopService(ctx)
	switch runtime.GOOS {
	case "linux":
		if path := linuxServicePath(); path != "" {
			_ = os.Remove(path)
			if _, err := exec.LookPath("systemctl"); err == nil {
				_ = exec.CommandContext(ctx, "systemctl", "--user", "daemon-reload").Run()
			}
		}
		return nil
	case "darwin":
		if path := macOSServicePath(); path != "" {
			_ = os.Remove(path)
			if _, err := exec.LookPath("launchctl"); err == nil {
				target := "gui/" + strconv.Itoa(os.Getuid())
				_ = exec.CommandContext(ctx, "launchctl", "bootout", target, path).Run()
			}
		}
		return nil
	case "windows":
		if _, err := exec.LookPath("schtasks.exe"); err == nil {
			_ = exec.CommandContext(ctx, "schtasks.exe", "/Delete", "/F", "/TN", windowsTaskName).Run()
		}
		return nil
	default:
		return errors.New("background service is not supported on this platform")
	}
}

func xmlEscape(value string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;")
	return replacer.Replace(value)
}
