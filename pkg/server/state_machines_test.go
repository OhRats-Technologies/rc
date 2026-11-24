package server

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	authstore "github.com/OhRats-Technologies/rc/pkg/auth"
)

func TestProcessHubMonotonicTransitionsAndReconcile(t *testing.T) {
	hub := newProcessHub()
	hub.add(browserProcess{ID: "p_active", DeviceID: "dev_a", Status: ProcessStarting})
	hub.add(browserProcess{ID: "p_ghost", DeviceID: "dev_a", Status: ProcessRunning})
	hub.add(browserProcess{ID: "p_done", DeviceID: "dev_a", Status: ProcessRunning})
	hub.add(browserProcess{ID: "p_other", DeviceID: "dev_b", Status: ProcessRunning})

	if !hub.update("p_active", ProcessRunning, nil, "", "") {
		t.Fatal("starting -> running should succeed")
	}
	var zero int32
	if !hub.update("p_done", ProcessExited, &zero, "", "") {
		t.Fatal("running -> exited should succeed")
	}
	if hub.update("p_done", ProcessRunning, nil, "", "") {
		t.Fatal("terminal exited process must not revert to running")
	}

	lost := hub.reconcile("dev_a", []string{"p_active"})
	if len(lost) != 1 || lost[0].ID != "p_ghost" || lost[0].Status != ProcessLost {
		t.Fatalf("reconcile lost = %+v, want [p_ghost]", lost)
	}
	if hub.update("p_ghost", ProcessRunning, nil, "", "") {
		t.Fatal("terminal lost process must not revert to running")
	}
	if p, _ := hub.get("p_active"); p.Status != ProcessRunning {
		t.Fatalf("active process status = %s, want running", p.Status)
	}
	if p, _ := hub.get("p_other"); p.Status != ProcessRunning {
		t.Fatalf("other device process status = %s, want running", p.Status)
	}
}

func TestLockSyncStateClassification(t *testing.T) {
	current := strings.Repeat("a", 64)
	older := strings.Repeat("b", 64)
	if got := classifyLockState("", current); got != LockUnbootstrapped {
		t.Fatalf("empty hash = %s, want unbootstrapped", got)
	}
	if got := classifyLockState(current, current); got != LockSynced {
		t.Fatalf("matching hash = %s, want synced", got)
	}
	if got := classifyLockState(older, current); got != LockStale {
		t.Fatalf("older hash = %s, want stale", got)
	}
}

func TestDevicePageCLISnippetsUseSupportedCommands(t *testing.T) {
	srv, ts := newAuthServer(t)
	owner := workspaceTestUser(t, srv, "CLI Owner")
	if err := srv.auth.EnsureWorkspace("ws_cli", "CLI Workspace"); err != nil {
		t.Fatal(err)
	}
	if err := srv.auth.EnsureWorkspaceMembership("ws_cli", owner.ID, "owner"); err != nil {
		t.Fatal(err)
	}
	device := authstore.Device{
		ID: "dev_cli", WorkspaceID: "ws_cli", Name: "fernix",
		Hostname: "ferpc", Platform: "linux", Arch: "amd64", Version: "0.20.12",
		IdentityPublicKey: "id-cli", TransportPublicKey: "tr-cli", CreatedAt: time.Now().UTC(),
	}
	if err := srv.auth.InsertDevice(device); err != nil {
		t.Fatal(err)
	}
	srv.registry.mu.Lock()
	srv.registry.devices[device.ID] = registeredFromStore(device)
	srv.registry.mu.Unlock()

	cookie := workspaceTestCookie(t, srv, owner.ID)
	res, payload := workspaceRequest(t, ts.URL, http.MethodGet, "/devices/dev_cli", "", cookie)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("device page = %d", res.StatusCode)
	}
	page := string(payload)
	if strings.Contains(page, "rc ssh ") || strings.Contains(page, "rc run ") {
		t.Fatalf("device page still renders unsupported CLI snippet: %s", page)
	}
	pattern := regexp.MustCompile(`data-copy-value="rc ([a-z-]+)`)
	matches := pattern.FindAllStringSubmatch(page, -1)
	if len(matches) == 0 {
		t.Fatal("expected at least one rc CLI copy snippet on device page")
	}
	allowed := map[string]bool{"shell": true, "devices": true, "device": true, "status": true, "doctor": true, "login": true, "enroll": true}
	for _, match := range matches {
		if !allowed[match[1]] {
			t.Fatalf("device page rendered unsupported rc subcommand %q", match[1])
		}
	}
}
