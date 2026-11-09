package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/OhRats-Technologies/rc/pkg/protocol"
)

type mcpTestGrant struct {
	userID  string
	grantID string
	access  string
}

func issueMCPTestGrant(t *testing.T, srv *Server, ts *httptest.Server, name string, scopes []string) mcpTestGrant {
	t.Helper()
	user, err := srv.auth.CreateUser(name)
	if err != nil {
		t.Fatal(err)
	}
	control := seedTestControl(t, srv, user.ID, ts.URL)
	grantID := seedTestMcpGrant(t, srv, control, scopes)
	access, _, _, _, err := srv.auth.IssueOAuthTokens(
		"mcp-test-client", user.ID, "https://rc.example.test/mcp",
		strings.Join(scopes, " "), grantID,
	)
	if err != nil {
		t.Fatal(err)
	}
	return mcpTestGrant{userID: user.ID, grantID: grantID, access: access}
}

func addMCPTestDevice(srv *Server, id, workspaceID string) {
	srv.registry.mu.Lock()
	srv.registry.devices[id] = &RegisteredDevice{
		ID: id, WorkspaceID: workspaceID, Name: id, Hostname: "test-host",
		Platform: "linux", Arch: "amd64", Version: "0.20.6",
		Capabilities: []string{"mcp-account-v2", "execution-v2"},
	}
	srv.registry.mu.Unlock()
}

func mcpToolResult(t *testing.T, ts *httptest.Server, token, name string, args map[string]any) (string, bool, map[string]any) {
	t.Helper()
	res, payload := mcpCall(t, ts, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": name, "arguments": args},
	}, map[string]string{"Authorization": "Bearer " + token})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("%s status = %d: %s", name, res.StatusCode, payload)
	}
	var response struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			StructuredContent map[string]any `json:"structuredContent"`
			IsError           bool           `json:"isError"`
		} `json:"result"`
	}
	if err := json.Unmarshal(payload, &response); err != nil {
		t.Fatal(err)
	}
	text := ""
	if len(response.Result.Content) > 0 {
		text = response.Result.Content[0].Text
	}
	return text, response.Result.IsError, response.Result.StructuredContent
}

func mcpMachineIDs(t *testing.T, structured map[string]any) []string {
	t.Helper()
	raw, _ := structured["machines"].([]any)
	ids := make([]string, 0, len(raw))
	for _, item := range raw {
		machine, _ := item.(map[string]any)
		id, _ := machine["id"].(string)
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func containsID(ids []string, target string) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}

func TestMCPToolsEnforceCurrentWorkspaceMembership(t *testing.T) {
	srv, ts := newAuthServer(t)
	owner := issueMCPTestGrant(t, srv, ts, "Private Owner", []string{"mcp:observe", "mcp:terminal"})
	outsider := issueMCPTestGrant(t, srv, ts, "Workspace Outsider", []string{"mcp:observe", "mcp:terminal"})
	if err := srv.auth.EnsureWorkspace("ws_private", "Private"); err != nil {
		t.Fatal(err)
	}
	if err := srv.auth.EnsureWorkspaceMembership("ws_private", owner.userID, "owner"); err != nil {
		t.Fatal(err)
	}
	addMCPTestDevice(srv, "dev_private", "ws_private")

	_, _, machines := mcpToolResult(t, ts, owner.access, "machines_list", map[string]any{})
	if !containsID(mcpMachineIDs(t, machines), "dev_private") {
		t.Fatalf("owner machines = %+v", machines)
	}
	_, _, machines = mcpToolResult(t, ts, outsider.access, "machines_list", map[string]any{})
	if containsID(mcpMachineIDs(t, machines), "dev_private") {
		t.Fatalf("outsider can see private machine: %+v", machines)
	}

	run := map[string]any{"deviceId": "dev_private", "argv": []string{"true"}, "waitSeconds": 0}
	text, isError, _ := mcpToolResult(t, ts, outsider.access, "process_run", run)
	if !isError || !strings.Contains(text, "device not found") {
		t.Fatalf("outsider process_run = %q", text)
	}
	text, isError, _ = mcpToolResult(t, ts, owner.access, "process_run", run)
	if !isError || !strings.Contains(text, "Node is offline") {
		t.Fatalf("owner process_run before demotion = %q", text)
	}

	if err := srv.auth.SetWorkspaceMembership("ws_private", owner.userID, "viewer"); err != nil {
		t.Fatal(err)
	}
	text, isError, _ = mcpToolResult(t, ts, owner.access, "process_run", run)
	if !isError || !strings.Contains(text, "Owner access is required") {
		t.Fatalf("demoted owner process_run = %q", text)
	}
	text, isError, _ = mcpToolResult(t, ts, owner.access, "image_view",
		map[string]any{"deviceId": "dev_private", "path": "/tmp/image.png"})
	if !isError || !strings.Contains(text, "Owner access is required") {
		t.Fatalf("demoted owner image_view = %q", text)
	}
	text, _, _ = mcpToolResult(t, ts, owner.access, "process_status",
		map[string]any{"deviceId": "dev_private", "processId": "proc_missing"})
	if !strings.Contains(text, "Node is offline") {
		t.Fatalf("demoted observer process_status = %q", text)
	}
}

func TestMCPExpiredGrantIsRejected(t *testing.T) {
	srv, ts := newAuthServer(t)
	user, err := srv.auth.CreateUser("Expired Grant")
	if err != nil {
		t.Fatal(err)
	}
	control := seedTestControl(t, srv, user.ID, ts.URL)
	grantID := seedTestMcpGrantUntil(t, srv, control, []string{"mcp:observe"}, time.Now().Add(-time.Minute).UnixMilli())
	access, _, _, _, err := srv.auth.IssueOAuthTokens(
		"mcp-test-client", user.ID, "https://rc.example.test/mcp", "mcp:observe", grantID,
	)
	if err != nil {
		t.Fatal(err)
	}
	res, payload := mcpCall(t, ts, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "machines_list",
	}, map[string]string{"Authorization": "Bearer " + access})
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expired grant status = %d: %s", res.StatusCode, payload)
	}
}

func TestMcpGrantPayloadUsesV2Audience(t *testing.T) {
	audience := "account"
	payload := protocol.McpGrantPayload{
		V: 2, ID: "mcp_test", UserID: "user_test",
		ClientID: "client_test", Audience: &audience,
		Scopes: []string{"mcp:observe", "mcp:terminal"},
	}
	if !payload.ValidAudience() || !payload.AllowsDevice("any") {
		t.Fatal("v2 account grant should authorize current and future devices")
	}
}
