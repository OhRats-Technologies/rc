package integration

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

func TestNodeRunHeartbeatAndLogs(t *testing.T) {
	root := repoRoot(t)
	srv := startServer(t, "RC_DEV_INSECURE=1", "RC_DEV_ENROLL_TOKEN=run-itest-token")
	binary := buildBinary(t, "./cmd/rc", "rc")
	stateDir := t.TempDir()
	env := []string{
		"RC_STATE_DIR=" + stateDir,
		"RC_COMPONENT_DIR=" + t.TempDir(),
		"HOME=" + t.TempDir(),
	}

	stdout, stderr, err := runCommand(t, root, env, binary, "enroll", "run-itest-token", "--url", srv.baseURL, "--name", "run-itest")
	if err != nil {
		t.Fatalf("enroll: %v\n%s%s", err, stdout, stderr)
	}

	stdout, stderr, err = runCommand(t, root, env, binary, "run", "--once", "--interval", "1s")
	if err != nil {
		t.Fatalf("run --once: %v\n%s%s", err, stdout, stderr)
	}

	var devices struct {
		Devices []struct {
			ID     string `json:"id"`
			Online bool   `json:"online"`
		} `json:"devices"`
	}
	if status := httpJSON(t, http.MethodGet, srv.baseURL+"/api/v1/devices", nil, &devices); status != http.StatusOK {
		t.Fatalf("devices status = %d", status)
	}
	if len(devices.Devices) != 1 || !devices.Devices[0].Online {
		t.Fatalf("heartbeat did not mark the device online: %s", jsonString(t, devices))
	}

	stdout, stderr, err = runCommand(t, root, env, binary, "logs")
	if err != nil {
		t.Fatalf("logs: %v\n%s%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "heartbeat ok") {
		t.Fatalf("logs output missing heartbeat: %s", stdout)
	}

	stdout, stderr, err = runCommand(t, root, env, binary, "logs", "--components")
	if err != nil {
		t.Fatalf("logs --components: %v\n%s%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "node.lifecycle") {
		t.Fatalf("component diagnostics output missing lifecycle event: %s", stdout)
	}

	stdout, stderr, err = runCommand(t, root, env, binary, "devices", "--url", srv.baseURL)
	if err != nil {
		t.Fatalf("devices: %v\n%s%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "run-itest") {
		t.Fatalf("devices output missing enrolled name: %s", stdout)
	}

	if _, err := filepath.Abs(stateDir); err != nil {
		t.Fatal(err)
	}
}
