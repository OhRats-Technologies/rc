package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fakeLaunchctl(t *testing.T, loaded bool) (string, string) {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "launchctl.log")
	statePath := filepath.Join(dir, "loaded")
	if loaded {
		if err := os.WriteFile(statePath, []byte("loaded"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$RC_TEST_LAUNCHCTL_LOG"
case "$1" in
  print) test -f "$RC_TEST_LAUNCHCTL_STATE" ;;
  bootstrap) touch "$RC_TEST_LAUNCHCTL_STATE" ;;
  kickstart) test -f "$RC_TEST_LAUNCHCTL_STATE" ;;
  *) exit 1 ;;
esac
`
	path := filepath.Join(dir, "launchctl")
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("RC_TEST_LAUNCHCTL_LOG", logPath)
	t.Setenv("RC_TEST_LAUNCHCTL_STATE", statePath)
	return logPath, statePath
}

func TestStartLaunchAgentBootstrapsUnloadedService(t *testing.T) {
	logPath, statePath := fakeLaunchctl(t, false)
	if err := startLaunchAgent("/tmp/party.ohrats.rc.plist"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(statePath); err != nil {
		t.Fatal("launch agent was not bootstrapped")
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 || !strings.HasPrefix(lines[0], "print ") || !strings.HasPrefix(lines[1], "bootstrap ") || !strings.HasPrefix(lines[2], "kickstart ") {
		t.Fatalf("unexpected launchctl sequence: %q", lines)
	}
}

func TestStartLaunchAgentDoesNotRebootstrapLoadedService(t *testing.T) {
	logPath, _ := fakeLaunchctl(t, true)
	if err := startLaunchAgent("/tmp/party.ohrats.rc.plist"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "print ") || !strings.HasPrefix(lines[1], "kickstart ") {
		t.Fatalf("unexpected launchctl sequence: %q", lines)
	}
}
