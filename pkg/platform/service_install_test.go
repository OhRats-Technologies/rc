package platform

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInstallServiceRestartsExistingLinuxUnit(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("skipping Linux systemd service test")
	}
	temp := t.TempDir()
	binDir := filepath.Join(temp, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	logPath := filepath.Join(temp, "systemctl.log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"" + logPath + "\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "systemctl"), []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile systemctl failed: %v", err)
	}
	t.Setenv("HOME", temp)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	stateDir := filepath.Join(temp, ".config", "rc")
	if err := InstallService("/home/test/.local/bin/rc", []string{"run", "--state-dir", stateDir}, stateDir); err != nil {
		t.Fatalf("InstallService failed: %v", err)
	}

	unitBytes, err := os.ReadFile(filepath.Join(temp, ".config", "systemd", "user", linuxServiceUnit))
	if err != nil {
		t.Fatalf("ReadFile unit failed: %v", err)
	}
	if !strings.Contains(string(unitBytes), "ExecStart=/home/test/.local/bin/rc run --state-dir "+stateDir) {
		t.Fatalf("unexpected unit content:\n%s", string(unitBytes))
	}

	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile systemctl.log failed: %v", err)
	}
	got := strings.TrimSpace(string(logBytes))
	want := strings.Join([]string{
		"--user daemon-reload",
		"--user enable rc.service",
		"--user restart rc.service",
	}, "\n")
	if got != want {
		t.Fatalf("systemctl calls = %q, want %q", got, want)
	}
}
