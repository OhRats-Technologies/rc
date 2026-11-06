package platform

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestExecutableName(t *testing.T) {
	name := ExecutableName("rc")
	if runtime.GOOS == "windows" {
		if name != "rc.exe" {
			t.Fatalf("expected rc.exe, got %s", name)
		}
	} else {
		if name != "rc" {
			t.Fatalf("expected rc, got %s", name)
		}
	}
}

func TestDirectoryOverrides(t *testing.T) {
	temp := t.TempDir()
	customState := filepath.Join(temp, "custom-state")
	os.Setenv("RC_STATE_DIR", customState)
	defer os.Unsetenv("RC_STATE_DIR")

	dir, err := StateDir()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dir != customState {
		t.Fatalf("expected %s, got %s", customState, dir)
	}
}

func TestDefaultDirsUnix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping Unix test on Windows")
	}
	temp := t.TempDir()
	os.Setenv("HOME", temp)
	os.Unsetenv("RC_STATE_DIR")
	os.Unsetenv("RC_DATA_DIR")
	os.Unsetenv("RC_COMPONENT_DIR")

	state, err := StateDir()
	if err != nil {
		t.Fatalf("StateDir failed: %v", err)
	}
	if state != filepath.Join(temp, ".config", "rc") {
		t.Fatalf("unexpected state dir: %s", state)
	}

	data, err := DataDir()
	if err != nil {
		t.Fatalf("DataDir failed: %v", err)
	}
	if data != filepath.Join(temp, ".local", "share", "rc") {
		t.Fatalf("unexpected data dir: %s", data)
	}

	comp, err := ComponentDir()
	if err != nil {
		t.Fatalf("ComponentDir failed: %v", err)
	}
	if comp != filepath.Join(temp, ".local", "share", "rc", "components") {
		t.Fatalf("unexpected component dir: %s", comp)
	}
}
