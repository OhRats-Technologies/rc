package server

import (
	"net/http"
	"strings"
	"testing"
	"time"

	authstore "github.com/OhRats-Technologies/rc/pkg/auth"
)

func TestAuthorityWarningUntilEveryNodeAcceptsAccess(t *testing.T) {
	srv, ts := newAuthServer(t)
	owner := workspaceTestUser(t, srv, "Sync Owner")
	if err := srv.auth.EnsureWorkspace("ws_sync", "Sync Workspace"); err != nil {
		t.Fatal(err)
	}
	if err := srv.auth.EnsureWorkspaceMembership("ws_sync", owner.ID, "owner"); err != nil {
		t.Fatal(err)
	}
	device := authstore.Device{
		ID: "dev_sync", WorkspaceID: "ws_sync", Name: "Sync Node",
		Hostname: "sync", Platform: "linux", Arch: "amd64", Version: "0.20.10",
		IdentityPublicKey: "identity-sync", TransportPublicKey: "transport-sync",
		CreatedAt: time.Now().UTC(),
	}
	if err := srv.auth.InsertDevice(device); err != nil {
		t.Fatal(err)
	}
	srv.registry.mu.Lock()
	srv.registry.devices[device.ID] = &RegisteredDevice{
		ID: device.ID, WorkspaceID: device.WorkspaceID, Name: device.Name,
		Hostname: device.Hostname, Platform: device.Platform, Arch: device.Arch,
		Version: device.Version, IdentityPublicKey: device.IdentityPublicKey,
		TransportPublicKey: device.TransportPublicKey,
	}
	srv.registry.mu.Unlock()
	cookie := workspaceTestCookie(t, srv, owner.ID)

	// A Node that never acknowledged a lock is pending: the warning must appear on every surface that
	// an Owner uses to act on it.
	res, payload := workspaceRequest(t, ts.URL, http.MethodGet, "/workspaces/ws_sync/access", "", cookie)
	page := string(payload)
	if res.StatusCode != http.StatusOK || !strings.Contains(page, `data-authority-alert="ws_sync"`) ||
		!strings.Contains(page, "Sync Node (offline)") || !strings.Contains(page, "SYNC DEVICE ACCESS") {
		t.Fatalf("pending access page = %d: %s", res.StatusCode, page)
	}
	res, payload = workspaceRequest(t, ts.URL, http.MethodGet, "/devices/dev_sync", "", cookie)
	if res.StatusCode != http.StatusOK || !strings.Contains(string(payload), "RC Lock out of sync") ||
		!strings.Contains(string(payload), `data-authority-sync="ws_sync"`) ||
		!strings.Contains(string(payload), `data-access-workspace="ws_sync"`) {
		t.Fatalf("pending device page = %d: %s", res.StatusCode, payload)
	}
	res, payload = workspaceRequest(t, ts.URL, http.MethodGet, "/devices", "", cookie)
	if res.StatusCode != http.StatusOK || !strings.Contains(string(payload), `workspace-sync-warning`) ||
		!strings.Contains(string(payload), "Sync device access") {
		t.Fatalf("pending sidebar = %d: %s", res.StatusCode, payload)
	}

	// Once the Node acknowledges the current snapshot, every warning clears.
	status, err := srv.workspaceAuthority("ws_sync")
	if err != nil {
		t.Fatal(err)
	}
	srv.registry.mu.Lock()
	srv.registry.devices["dev_sync"].LockHash = status.Hash
	srv.registry.mu.Unlock()

	res, payload = workspaceRequest(t, ts.URL, http.MethodGet, "/workspaces/ws_sync/access", "", cookie)
	if strings.Contains(string(payload), `data-authority-alert=`) || !strings.Contains(string(payload), "the current workspace access") {
		t.Fatalf("settled access page = %d: %s", res.StatusCode, payload)
	}
	res, payload = workspaceRequest(t, ts.URL, http.MethodGet, "/devices/dev_sync", "", cookie)
	if strings.Contains(string(payload), "RC Lock out of sync") {
		t.Fatalf("settled device page still warns: %s", payload)
	}
	res, payload = workspaceRequest(t, ts.URL, http.MethodGet, "/devices", "", cookie)
	if strings.Contains(string(payload), `workspace-sync-warning`) {
		t.Fatalf("settled sidebar still warns: %s", payload)
	}
}
