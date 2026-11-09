package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	authstore "github.com/OhRats-Technologies/rc/pkg/auth"
)

func TestAccountPageExposesDeleteAction(t *testing.T) {
	srv, ts, cookie, user := newOAuthTestServerWithUser(t)
	_ = srv
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/account", nil)
	req.AddCookie(cookie)
	res, payload := oauthDo(t, http.DefaultClient, req)
	page := string(payload)
	if res.StatusCode != http.StatusOK || !strings.Contains(page, `data-delete-endpoint="/account/delete"`) ||
		!strings.Contains(page, `data-delete-kind="account"`) ||
		!strings.Contains(page, `src="/assets/sidebar.js`) {
		t.Fatalf("account page = %d: %s", res.StatusCode, page)
	}
	if !strings.Contains(page, escape(user.Name)) {
		t.Fatalf("account page missing user name")
	}
}

func TestAccountDeleteRequiresFreshPasskeyAndRemovesOwnedWorkspace(t *testing.T) {
	srv, ts := newAuthServer(t)
	owner, err := srv.auth.CreateUser("Account Owner")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := srv.auth.CreateUser("Survivor"); err != nil {
		t.Fatal(err)
	}
	if err := srv.auth.EnsureWorkspace("ws_owned", "Owned"); err != nil {
		t.Fatal(err)
	}
	if err := srv.auth.EnsureWorkspaceMembership("ws_owned", owner.ID, "owner"); err != nil {
		t.Fatal(err)
	}
	device := authstore.Device{
		ID: "dev_account_delete", WorkspaceID: "ws_owned", Name: "Account Node",
		Hostname: "host", Platform: "linux", Arch: "amd64", Version: "0.20.6",
		IdentityPublicKey: "account-delete-identity", TransportPublicKey: "account-delete-transport",
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
	ownerCookie := workspaceTestCookie(t, srv, owner.ID)

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/account/delete", nil)
	req.AddCookie(ownerCookie)
	res, payload := oauthDo(t, http.DefaultClient, req)
	if res.StatusCode != http.StatusUnauthorized || !strings.Contains(string(payload), "fresh passkey") {
		t.Fatalf("account delete without step-up = %d: %s", res.StatusCode, payload)
	}
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/account/delete", nil)
	req.AddCookie(ownerCookie)
	req.AddCookie(workspaceStepUpCookie(t, srv, owner.ID))
	res, payload = oauthDo(t, http.DefaultClient, req)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("account delete = %d: %s", res.StatusCode, payload)
	}
	var result struct {
		OK            bool `json:"ok"`
		SetupRequired bool `json:"setupRequired"`
	}
	if err := json.Unmarshal(payload, &result); err != nil || !result.OK || result.SetupRequired {
		t.Fatalf("account delete result = %s", payload)
	}
	cleared := strings.Join(res.Header.Values("Set-Cookie"), "\n")
	if !strings.Contains(cleared, sessionCookieName) {
		t.Fatalf("account delete did not clear the session cookie: %q", cleared)
	}
	if _, err := srv.auth.GetUser(owner.ID); err == nil {
		t.Fatal("deleted account still exists")
	}
	if _, err := srv.auth.WorkspaceByID("ws_owned"); err == nil {
		t.Fatal("owned workspace still exists")
	}
	if revoked, err := srv.auth.DeviceRevoked(device.ID); err != nil || !revoked {
		t.Fatalf("revoked device = %v, %v", revoked, err)
	}
	if _, ok := srv.registry.Get(device.ID); ok {
		t.Fatal("revoked device remained in registry")
	}
}

func TestAccountDeleteLastHumanReturnsToSetup(t *testing.T) {
	srv, ts := newAuthServer(t)
	user, err := srv.auth.CreateUser("Last Human")
	if err != nil {
		t.Fatal(err)
	}
	cookie := workspaceTestCookie(t, srv, user.ID)
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/account", nil)
	req.AddCookie(cookie)
	req.AddCookie(workspaceStepUpCookie(t, srv, user.ID))
	res, payload := oauthDo(t, http.DefaultClient, req)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("last account delete = %d: %s", res.StatusCode, payload)
	}
	var result struct {
		SetupRequired bool `json:"setupRequired"`
	}
	if err := json.Unmarshal(payload, &result); err != nil || !result.SetupRequired {
		t.Fatalf("last account result = %s", payload)
	}
	if count, err := srv.auth.UserCount(); err != nil || count != 0 {
		t.Fatalf("active users = %d, %v", count, err)
	}
	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/", nil)
	res, payload = oauthDo(t, http.DefaultClient, req)
	if res.StatusCode != http.StatusOK || !strings.Contains(string(payload), `id="setup-form"`) {
		t.Fatalf("post-delete home = %d: %s", res.StatusCode, payload)
	}
}
