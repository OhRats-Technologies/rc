package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	authstore "github.com/OhRats-Technologies/rc/pkg/auth"
)

func TestAuthoritySyncDeliversMcpGrantToLockedNode(t *testing.T) {
	srv, ts := newAuthServer(t)
	user, err := srv.auth.CreateUser("Authority User")
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := srv.auth.IssueSession(user.ID, "1h")
	if err != nil {
		t.Fatal(err)
	}
	cookie := &http.Cookie{Name: sessionCookieName, Value: token}
	control := seedTestControl(t, srv, user.ID, ts.URL)
	deviceID := startMCPTestNode(t, srv, ts)
	waitTestNodeLock(t, srv, deviceID)

	grantID := seedTestMcpGrant(t, srv, control, []string{"mcp:observe", "mcp:terminal"})
	access, _, _, _, err := srv.auth.IssueOAuthTokens(
		"mcp-test-client", user.ID, "https://rc.example.test/mcp",
		"mcp:observe mcp:terminal", grantID,
	)
	if err != nil {
		t.Fatal(err)
	}

	runBeforeSync := map[string]any{
		"jsonrpc": "2.0", "id": 0, "method": "tools/call",
		"params": map[string]any{
			"name": "process_run",
			"arguments": map[string]any{
				"deviceId": deviceID, "argv": mcpHelperArgv(), "env": mcpHelperEnv("echo"),
				"waitSeconds": 0,
			},
		},
	}
	res, payload := mcpCall(t, ts, runBeforeSync, map[string]string{"Authorization": "Bearer " + access})
	if res.StatusCode != http.StatusOK || !bytes.Contains(payload, []byte("not synchronized")) {
		t.Fatalf("unsynchronized MCP grant = %d: %s", res.StatusCode, payload)
	}

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/workspaces/ws_default/authority", nil)
	req.AddCookie(cookie)
	res, payload = oauthDo(t, http.DefaultClient, req)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("authority status = %d: %s", res.StatusCode, payload)
	}
	var status struct {
		Hash    string `json:"hash"`
		Parents []struct {
			Hash       string `json:"hash"`
			Generation uint64 `json:"generation"`
		} `json:"parents"`
	}
	if err := json.Unmarshal(payload, &status); err != nil {
		t.Fatal(err)
	}
	if status.Hash == "" || len(status.Parents) == 0 {
		t.Fatalf("authority status = %s", payload)
	}
	parent := status.Parents[0]
	payloadSignature := control.sign("rc-authority-v3\n" +
		itoa(int(parent.Generation)) + "\n" + parent.Hash + "\n" + status.Hash)
	syncBody, _ := json.Marshal(map[string]any{
		"clientId": control.ID,
		"transitions": []map[string]any{{
			"fromHash": parent.Hash, "generation": parent.Generation,
			"signature": payloadSignature,
		}},
	})
	req, _ = http.NewRequest(http.MethodPost,
		ts.URL+"/api/v1/workspaces/ws_default/authority/sync", bytes.NewReader(syncBody))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	res, payload = oauthDo(t, http.DefaultClient, req)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("authority sync = %d: %s", res.StatusCode, payload)
	}

	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		device, ok := srv.registry.Get(deviceID)
		if ok && device.LockHash == status.Hash {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	device, _ := srv.registry.Get(deviceID)
	if device == nil || device.LockHash != status.Hash {
		t.Fatalf("Node lock = %+v, want %s", device, status.Hash)
	}

	runBody := map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{
			"name": "process_run",
			"arguments": map[string]any{
				"deviceId": deviceID, "argv": mcpHelperArgv(), "env": mcpHelperEnv("echo"),
				"waitSeconds": 5,
			},
		},
	}
	res, payload = mcpCall(t, ts, runBody, map[string]string{"Authorization": "Bearer " + access})
	if res.StatusCode != http.StatusOK || !bytes.Contains(payload, []byte("mcp-wire-ok")) {
		t.Fatalf("process_run after authority sync = %d: %s", res.StatusCode, payload)
	}
}

func TestMcpGrantRevocationInvalidatesTokens(t *testing.T) {
	srv, ts, cookie, user := newOAuthTestServerWithUser(t)
	control := seedTestControl(t, srv, user.ID, ts.URL)
	grantID := seedTestMcpGrant(t, srv, control, []string{"mcp:observe", "mcp:terminal"})
	access, _, _, _, err := srv.auth.IssueOAuthTokens(
		"mcp-test-client", user.ID, "https://rc.example.test/mcp",
		"mcp:observe mcp:terminal", grantID,
	)
	if err != nil {
		t.Fatal(err)
	}
	srv.mcp.bindProcess("proc_grant_revoke", &authstore.OAuthGrant{
		UserID: user.ID, ClientID: "mcp-test-client", GrantID: grantID,
	}, "dev_grant_revoke")
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/oauth/grants/"+grantID, nil)
	req.AddCookie(cookie)
	res, payload := oauthDo(t, http.DefaultClient, req)
	if res.StatusCode != http.StatusOK || !bytes.Contains(payload, []byte(`"workspaceIds"`)) {
		t.Fatalf("grant revoke = %d: %s", res.StatusCode, payload)
	}
	toolsBody := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"}
	res, _ = mcpCall(t, ts, toolsBody, map[string]string{"Authorization": "Bearer " + access})
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked grant token status = %d", res.StatusCode)
	}
	if _, ok := srv.mcp.processBinding("proc_grant_revoke", "dev_grant_revoke", &authstore.OAuthGrant{
		UserID: user.ID, ClientID: "mcp-test-client", GrantID: grantID,
	}); ok {
		t.Fatal("revoked grant MCP process binding remained")
	}
}

func TestEnrollmentRequiresWorkspaceOwner(t *testing.T) {
	srv, ts, ownerCookie, owner := newOAuthTestServerWithUser(t)
	if err := srv.auth.EnsureWorkspace("ws_private", "Private"); err != nil {
		t.Fatal(err)
	}
	if err := srv.auth.EnsureWorkspaceMembership("ws_private", owner.ID, "owner"); err != nil {
		t.Fatal(err)
	}
	outsider, err := srv.auth.CreateUser("Enrollment Outsider")
	if err != nil {
		t.Fatal(err)
	}
	session, _, err := srv.auth.IssueSession(outsider.ID, "1h")
	if err != nil {
		t.Fatal(err)
	}
	outsiderCookie := &http.Cookie{Name: sessionCookieName, Value: session}
	if err := srv.auth.SetWorkspaceMembership("ws_private", outsider.ID, "viewer"); err != nil {
		t.Fatal(err)
	}

	post := func(cookie *http.Cookie) (*http.Response, []byte) {
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/workspaces/ws_private/enrollments", nil)
		req.AddCookie(cookie)
		return oauthDo(t, http.DefaultClient, req)
	}
	res, payload := post(ownerCookie)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("owner enrollment = %d: %s", res.StatusCode, payload)
	}
	res, payload = post(outsiderCookie)
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("viewer enrollment = %d: %s", res.StatusCode, payload)
	}
	unknown, err := srv.auth.CreateUser("Enrollment Unknown")
	if err != nil {
		t.Fatal(err)
	}
	session, _, err = srv.auth.IssueSession(unknown.ID, "1h")
	if err != nil {
		t.Fatal(err)
	}
	res, payload = post(&http.Cookie{Name: sessionCookieName, Value: session})
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("non-member enrollment = %d: %s", res.StatusCode, payload)
	}
}

func TestMCPPageRenders(t *testing.T) {
	srv, ts, cookie, user := newOAuthTestServerWithUser(t)
	control := seedTestControl(t, srv, user.ID, ts.URL)
	_ = seedTestMcpGrant(t, srv, control, []string{"mcp:observe", "mcp:terminal"})
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/integrations/mcp", nil)
	req.AddCookie(cookie)
	res, payload := oauthDo(t, http.DefaultClient, req)
	if res.StatusCode != http.StatusOK || !bytes.Contains(payload, []byte("/mcp")) ||
		!bytes.Contains(payload, []byte("data-mcp-revoke")) {
		t.Fatalf("MCP page = %d: %s", res.StatusCode, payload)
	}
}

func TestAuthorityStatusKeepsSameHashAtDifferentGenerations(t *testing.T) {
	srv, ts, cookie, user := newOAuthTestServerWithUser(t)
	if err := srv.auth.EnsureWorkspace("ws_multi", "Multi"); err != nil {
		t.Fatal(err)
	}
	if err := srv.auth.EnsureWorkspaceMembership("ws_multi", user.ID, "owner"); err != nil {
		t.Fatal(err)
	}
	for _, device := range []*RegisteredDevice{
		{ID: "dev_old", WorkspaceID: "ws_multi", Name: "Old", Hostname: "old", Platform: "linux", Arch: "amd64",
			Version: "0.20.6", IdentityPublicKey: "old-identity", TransportPublicKey: "old-transport",
			LockHash: strings.Repeat("a", 64), LockGeneration: 2},
		{ID: "dev_new", WorkspaceID: "ws_multi", Name: "New", Hostname: "new", Platform: "linux", Arch: "amd64",
			Version: "0.20.6", IdentityPublicKey: "new-identity", TransportPublicKey: "new-transport",
			LockHash: strings.Repeat("a", 64), LockGeneration: 7},
	} {
		srv.registry.mu.Lock()
		srv.registry.devices[device.ID] = device
		srv.registry.mu.Unlock()
	}
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/workspaces/ws_multi/authority", nil)
	req.AddCookie(cookie)
	res, payload := oauthDo(t, http.DefaultClient, req)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("authority status = %d: %s", res.StatusCode, payload)
	}
	var status struct {
		Parents []struct {
			Hash       string `json:"hash"`
			Generation uint64 `json:"generation"`
		} `json:"parents"`
	}
	if err := json.Unmarshal(payload, &status); err != nil {
		t.Fatal(err)
	}
	generations := map[uint64]bool{}
	for _, parent := range status.Parents {
		if parent.Hash == strings.Repeat("a", 64) {
			generations[parent.Generation] = true
		}
	}
	if !generations[2] || !generations[7] || len(status.Parents) != 2 {
		t.Fatalf("parents = %+v", status.Parents)
	}
}
