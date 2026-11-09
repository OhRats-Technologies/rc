package integration

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/OhRats-Technologies/rc/pkg/cli"
)

func readJSONVersion(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var pkg struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return pkg.Version
}

func changelogVersion(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "## [") {
			rest := strings.TrimPrefix(line, "## [")
			if idx := strings.IndexByte(rest, ']'); idx > 0 {
				return rest[:idx]
			}
		}
	}
	t.Fatal("CHANGELOG.md has no version heading")
	return ""
}

func TestCLIReportsOneVersion(t *testing.T) {
	root := repoRoot(t)
	want := readJSONVersion(t, filepath.Join(root, "package.json"))
	if got := changelogVersion(t, filepath.Join(root, "CHANGELOG.md")); got != want {
		t.Fatalf("CHANGELOG version %q != package.json %q", got, want)
	}
	if cli.Version != want {
		t.Fatalf("pkg/cli.Version %q != package.json %q", cli.Version, want)
	}

	binary := buildBinary(t, "./cmd/rc", "rc")
	stdout, stderr, err := runCommand(t, root, nil, binary, "version")
	if err != nil {
		t.Fatalf("rc version: %v\n%s", err, stderr)
	}
	if !strings.Contains(stdout, want) {
		t.Fatalf("rc version output %q does not contain %q", stdout, want)
	}
}

func TestCLIHelpAndUnknownCommand(t *testing.T) {
	root := repoRoot(t)
	binary := buildBinary(t, "./cmd/rc", "rc")

	stdout, stderr, err := runCommand(t, root, nil, binary, "help")
	if err != nil {
		t.Fatalf("rc help: %v\n%s", err, stderr)
	}
	for _, command := range []string{"status", "devices", "config", "enroll", "crypto-check", "run", "logs", "ps", "service", "login", "logout"} {
		if !strings.Contains(stdout, command) {
			t.Fatalf("help output missing %q:\n%s", command, stdout)
		}
	}
	for _, command := range []string{"doctor", "update", "upgrade", "repair", "shell"} {
		if !strings.Contains(stdout, command) {
			t.Fatalf("help output missing implemented command %q:\n%s", command, stdout)
		}
	}

}

func TestCLIConfigRoundTrip(t *testing.T) {
	root := repoRoot(t)
	binary := buildBinary(t, "./cmd/rc", "rc")
	stateDir := t.TempDir()
	env := []string{"RC_STATE_DIR=" + stateDir}

	if _, stderr, err := runCommand(t, root, env, binary, "config", "set", "server", "http://127.0.0.1:1"); err != nil {
		t.Fatalf("config set: %v\n%s", err, stderr)
	}
	if _, stderr, err := runCommand(t, root, env, binary, "config", "set", "name", "cli-itest"); err != nil {
		t.Fatalf("config set name: %v\n%s", err, stderr)
	}
	stdout, stderr, err := runCommand(t, root, env, binary, "config")
	if err != nil {
		t.Fatalf("config: %v\n%s", err, stderr)
	}
	if !strings.Contains(stdout, "http://127.0.0.1:1") || !strings.Contains(stdout, "cli-itest") {
		t.Fatalf("unexpected config output:\n%s", stdout)
	}

	_, stderr, err = runCommand(t, root, env, binary, "config", "set", "bogus", "value")
	if err == nil {
		t.Fatal("unknown config key must fail")
	}
	if !strings.Contains(stderr, "unknown config key") {
		t.Fatalf("unexpected stderr: %s", stderr)
	}
}

func TestCLICryptoCheckLoadsRealComponent(t *testing.T) {
	root := repoRoot(t)
	binary := buildBinary(t, "./cmd/rc", "rc")
	components := componentArtifacts(t, "crypto-control")

	stdout, stderr, err := runCommand(t, root, nil, binary, "crypto-check", "--component-dir", components)
	if err != nil {
		t.Fatalf("crypto-check: %v\n%s%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "control crypto: ok") {
		t.Fatalf("unexpected crypto-check output: %s", stdout)
	}
}

// TestCLIEnrollAndStatus drives cmd/rc against a live cmd/rc-server process.
func TestCLIEnrollAndStatus(t *testing.T) {
	root := repoRoot(t)
	srv := startServer(t, "RC_DEV_INSECURE=1", "RC_DEV_ENROLL_TOKEN=cli-itest-token")
	binary := buildBinary(t, "./cmd/rc", "rc")
	stateDir := t.TempDir()
	env := []string{"RC_STATE_DIR=" + stateDir}

	stdout, stderr, err := runCommand(t, root, env, binary, "enroll", "cli-itest-token", "--url", srv.baseURL, "--name", "cli-itest")
	if err != nil {
		t.Fatalf("enroll: %v\n%s%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "Successfully enrolled device") {
		t.Fatalf("unexpected enroll output: %s", stdout)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "device.json")); err != nil {
		t.Fatalf("device.json not written: %v", err)
	}

	stdout, stderr, err = runCommand(t, root, env, binary, "status", "--url", srv.baseURL)
	if err != nil {
		t.Fatalf("status: %v\n%s%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "Device ID:") {
		t.Fatalf("status output missing device id:\n%s", stdout)
	}

	// The enrolled device must be visible through the server API.
	var devices struct {
		Devices []struct {
			ID string `json:"id"`
		} `json:"devices"`
	}
	if status := httpJSON(t, http.MethodGet, srv.baseURL+"/api/v1/devices", nil, &devices); status != http.StatusOK {
		t.Fatalf("list devices status = %d", status)
	}
	if len(devices.Devices) != 1 {
		t.Fatalf("expected one enrolled device, got %s", jsonString(t, devices))
	}
}

func TestCLILocalShellRunsCommand(t *testing.T) {
	root := repoRoot(t)
	srv := startServer(t, "RC_DEV_INSECURE=1", "RC_DEV_ENROLL_TOKEN=shell-itest-token")
	binary := buildBinary(t, "./cmd/rc", "rc")
	stateDir := t.TempDir()
	env := []string{
		"RC_STATE_DIR=" + stateDir,
		"HOME=" + t.TempDir(),
	}
	if _, stderr, err := runCommand(t, root, env, binary, "enroll", "shell-itest-token", "--url", srv.baseURL); err != nil {
		t.Fatalf("enroll: %v\n%s", err, stderr)
	}
	command := "printf shell-ok"
	if runtime.GOOS == "windows" {
		command = "echo shell-ok"
	}
	stdout, stderr, err := runCommand(t, root, env, binary, "shell", "--command", command)
	if err != nil {
		t.Fatalf("shell: %v\n%s%s", err, stdout, stderr)
	}
	if strings.TrimSpace(stdout) != "shell-ok" {
		t.Fatalf("shell output = %q", stdout)
	}
}
