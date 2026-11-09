package server

import (
	"net/http"
	"strings"
	"testing"
	"time"

	authstore "github.com/OhRats-Technologies/rc/pkg/auth"
)

func TestSessionDeviceRoutesEnforceWorkspaceRoles(t *testing.T) {
	srv, ts := newAuthServer(t)
	if err := srv.auth.EnsureWorkspace("ws_private", "Private"); err != nil {
		t.Fatal(err)
	}
	owner, err := srv.auth.CreateUser("Device Owner")
	if err != nil {
		t.Fatal(err)
	}
	viewer, err := srv.auth.CreateUser("Device Viewer")
	if err != nil {
		t.Fatal(err)
	}
	operator, err := srv.auth.CreateUser("Device Operator")
	if err != nil {
		t.Fatal(err)
	}
	outsider, err := srv.auth.CreateUser("Device Outsider")
	if err != nil {
		t.Fatal(err)
	}
	for userID, role := range map[string]string{
		owner.ID: "owner", viewer.ID: "viewer", operator.ID: "operator",
	} {
		if err := srv.auth.EnsureWorkspaceMembership("ws_private", userID, role); err != nil {
			t.Fatal(err)
		}
	}
	device := authstore.Device{
		ID: "dev_private", WorkspaceID: "ws_private", Name: "Private Node",
		Hostname: "private", Platform: "linux", Arch: "amd64", Version: "0.20.6",
		IdentityPublicKey: "identity-private", TransportPublicKey: "transport-private",
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

	cookies := map[string]*http.Cookie{}
	for name, user := range map[string]*authstore.User{
		"owner": owner, "viewer": viewer, "operator": operator, "outsider": outsider,
	} {
		session, _, err := srv.auth.IssueSession(user.ID, "1h")
		if err != nil {
			t.Fatal(err)
		}
		cookies[name] = &http.Cookie{Name: sessionCookieName, Value: session}
	}
	do := func(method, target string, body string, cookie *http.Cookie) (int, string) {
		req, _ := http.NewRequest(method, ts.URL+target, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(cookie)
		res, payload := oauthDo(t, http.DefaultClient, req)
		return res.StatusCode, string(payload)
	}
	if status, _ := do(http.MethodGet, "/api/v1/devices", "", cookies["owner"]); status != http.StatusOK {
		t.Fatalf("owner device list = %d", status)
	}
	if status, payload := do(http.MethodGet, "/api/v1/devices", "", cookies["outsider"]); status != http.StatusOK ||
		strings.Contains(payload, device.ID) {
		t.Fatalf("outsider device list = %d: %s", status, payload)
	}
	if status, _ := do(http.MethodGet, "/api/v1/devices/"+device.ID, "", cookies["outsider"]); status != http.StatusNotFound {
		t.Fatalf("outsider device read = %d", status)
	}
	if status, _ := do(http.MethodGet, "/api/v1/devices/"+device.ID, "", cookies["viewer"]); status != http.StatusOK {
		t.Fatalf("viewer device read = %d", status)
	}
	if status, _ := do(http.MethodPatch, "/api/v1/devices/"+device.ID, `{"name":"Nope"}`, cookies["viewer"]); status != http.StatusForbidden {
		t.Fatalf("viewer rename = %d", status)
	}
	if status, _ := do(http.MethodPatch, "/api/v1/devices/"+device.ID, `{"name":"Renamed"}`, cookies["operator"]); status != http.StatusOK {
		t.Fatalf("operator rename = %d", status)
	}
	if status, _ := do(http.MethodDelete, "/api/v1/devices/"+device.ID, "", cookies["operator"]); status != http.StatusForbidden {
		t.Fatalf("operator delete = %d", status)
	}
	if status, _ := do(http.MethodDelete, "/api/v1/devices/"+device.ID, "", cookies["owner"]); status != http.StatusNoContent {
		t.Fatalf("owner delete = %d", status)
	}
}
