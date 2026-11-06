package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIDoctorReportsRuntimeState(t *testing.T) {
	root := repoRoot(t)
	binary := buildBinary(t, "./cmd/rc", "rc")
	components := componentArtifacts(t, "crypto-control")
	stateDir := t.TempDir()
	env := []string{
		"RC_STATE_DIR=" + stateDir,
		"RC_COMPONENT_DIR=" + components,
		"RC_INSTALL_BIN_DIR=" + t.TempDir(),
		"HOME=" + t.TempDir(),
	}

	if err := os.Chmod(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(components, 0o700); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, err := runCommand(t, root, env, binary, "doctor", "--state-dir", stateDir, "--component-dir", components)
	if err != nil {
		t.Fatalf("doctor: %v\n%s%s", err, stdout, stderr)
	}
	for _, want := range []string{"RC diagnostics", "version:", "components:"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, stdout)
		}
	}

	// A malformed component must make doctor fail.
	if err := os.WriteFile(filepath.Join(components, "broken.wasm"), []byte("not wasm"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, err = runCommand(t, root, env, binary, "doctor", "--state-dir", stateDir, "--component-dir", components)
	if err == nil {
		t.Fatalf("doctor accepted a malformed component:\n%s%s", stdout, stderr)
	}
	if !strings.Contains(stderr, "diagnostic checks failed") {
		t.Fatalf("unexpected doctor failure: %v\n%s%s", err, stdout, stderr)
	}
}
