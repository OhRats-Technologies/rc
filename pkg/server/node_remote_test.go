package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OhRats-Technologies/rc/pkg/node"
	"github.com/OhRats-Technologies/rc/pkg/version"
)

func startMCPTestNode(t *testing.T, srv *Server, ts *httptest.Server) string {
	t.Helper()
	token := srv.registry.GenerateToken()
	state, err := node.Enroll(context.Background(), ts.URL, token, "MCP Test Node", version.Version)
	if err != nil {
		t.Fatalf("enroll test node: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	exec := node.NewNativeExecutionManager()
	go node.RunControl(ctx, ts.URL, t.TempDir(), state, exec, func(message string) {
		t.Logf("node: %s", message)
	})
	waitCtx, waitCancel := context.WithTimeout(ctx, 5*time.Second)
	defer waitCancel()
	if err := srv.nodes.WaitOnline(waitCtx, state.DeviceID); err != nil {
		cancel()
		t.Fatalf("wait for test node: %v", err)
	}
	t.Cleanup(func() {
		cancel()
		exec.Shutdown()
		for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
			if !srv.nodes.Online(state.DeviceID) {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	})
	return state.DeviceID
}

func seedRemoteMCP(t *testing.T, srv *Server, ts *httptest.Server) string {
	t.Helper()
	user, err := srv.auth.CreateUser("MCP Remote User")
	if err != nil {
		t.Fatal(err)
	}
	control := seedTestControl(t, srv, user.ID, ts.URL)
	grantID := seedTestMcpGrant(t, srv, control, []string{"mcp:observe", "mcp:terminal"})
	access, _, _, _, err := srv.auth.IssueOAuthTokens(
		"mcp-test-client", user.ID, "https://rc.example.test/mcp",
		"mcp:observe mcp:terminal", grantID,
	)
	if err != nil {
		t.Fatal(err)
	}
	return access
}

func TestMCPRemoteProcessAndImage(t *testing.T) {
	srv, ts := newAuthServer(t)
	token := seedRemoteMCP(t, srv, ts)
	deviceID := startMCPTestNode(t, srv, ts)
	waitTestNodeLock(t, srv, deviceID)

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
	res, payload := mcpCall(t, ts, runBody, map[string]string{"Authorization": "Bearer " + token})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("remote process_run = %d: %s", res.StatusCode, payload)
	}
	var run struct {
		Result struct {
			StructuredContent mcpProcessResult `json:"structuredContent"`
			IsError           bool             `json:"isError"`
		} `json:"result"`
	}
	if err := json.Unmarshal(payload, &run); err != nil {
		t.Fatal(err)
	}
	if run.Result.IsError || run.Result.StructuredContent.Status != "exited" ||
		!strings.Contains(processChunkText(run.Result.StructuredContent), "mcp-wire-ok") {
		t.Fatalf("remote process_run = %+v (%s)", run.Result, payload)
	}

	inputRun := map[string]any{
		"jsonrpc": "2.0", "id": 3, "method": "tools/call",
		"params": map[string]any{
			"name": "process_run",
			"arguments": map[string]any{
				"deviceId": deviceID, "argv": mcpHelperArgv(), "env": mcpHelperEnv("cat"),
				"waitSeconds": 0,
			},
		},
	}
	_, payload = mcpCall(t, ts, inputRun, map[string]string{"Authorization": "Bearer " + token})
	var catRun struct {
		Result struct {
			StructuredContent mcpProcessResult `json:"structuredContent"`
		} `json:"result"`
	}
	_ = json.Unmarshal(payload, &catRun)
	catID := catRun.Result.StructuredContent.ProcessID
	if catID == "" {
		t.Fatalf("remote cat process = %s", payload)
	}
	inputBody := map[string]any{
		"jsonrpc": "2.0", "id": 4, "method": "tools/call",
		"params": map[string]any{
			"name": "process_input",
			"arguments": map[string]any{
				"deviceId": deviceID, "processId": catID, "data": "hello\n", "eof": true,
			},
		},
	}
	_, payload = mcpCall(t, ts, inputBody, map[string]string{"Authorization": "Bearer " + token})
	if bytes.Contains(payload, []byte(`"isError":true`)) {
		t.Fatalf("remote process_input = %s", payload)
	}
	var outputText strings.Builder
	cursor := uint64(0)
	var output mcpProcessResult
	for attempt := 0; attempt < 10 && output.Status != "exited"; attempt++ {
		statusBody := map[string]any{
			"jsonrpc": "2.0", "id": 5, "method": "tools/call",
			"params": map[string]any{
				"name": "process_status",
				"arguments": map[string]any{
					"deviceId": deviceID, "processId": catID, "cursor": cursor, "waitSeconds": 5,
				},
			},
		}
		_, payload = mcpCall(t, ts, statusBody, map[string]string{"Authorization": "Bearer " + token})
		var status struct {
			Result struct {
				StructuredContent mcpProcessResult `json:"structuredContent"`
			} `json:"result"`
		}
		_ = json.Unmarshal(payload, &status)
		output = status.Result.StructuredContent
		outputText.WriteString(processChunkText(output))
		cursor = output.NextCursor
	}
	if output.Status != "exited" || !strings.Contains(outputText.String(), "hello") {
		t.Fatalf("remote process_status = %+v", output)
	}

	sleepRun := map[string]any{
		"jsonrpc": "2.0", "id": 6, "method": "tools/call",
		"params": map[string]any{
			"name": "process_run",
			"arguments": map[string]any{
				"deviceId": deviceID, "argv": mcpHelperArgv(), "env": mcpHelperEnv("sleep"),
				"waitSeconds": 0,
			},
		},
	}
	_, payload = mcpCall(t, ts, sleepRun, map[string]string{"Authorization": "Bearer " + token})
	var sleeping struct {
		Result struct {
			StructuredContent mcpProcessResult `json:"structuredContent"`
		} `json:"result"`
	}
	_ = json.Unmarshal(payload, &sleeping)
	cancelBody := map[string]any{
		"jsonrpc": "2.0", "id": 7, "method": "tools/call",
		"params": map[string]any{
			"name": "process_cancel",
			"arguments": map[string]any{
				"deviceId": deviceID, "processId": sleeping.Result.StructuredContent.ProcessID,
				"signal": "KILL",
			},
		},
	}
	_, payload = mcpCall(t, ts, cancelBody, map[string]string{"Authorization": "Bearer " + token})
	if bytes.Contains(payload, []byte(`"isError":true`)) {
		t.Fatalf("remote process_cancel = %s", payload)
	}

	path := filepath.Join(t.TempDir(), "dot.png")
	data, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Y9ZQmcAAAAASUVORK5CYII=")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	imageBody := map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "tools/call",
		"params": map[string]any{
			"name":      "image_view",
			"arguments": map[string]any{"deviceId": deviceID, "path": path},
		},
	}
	res, payload = mcpCall(t, ts, imageBody, map[string]string{"Authorization": "Bearer " + token})
	if res.StatusCode != http.StatusOK || bytes.Contains(payload, []byte(`"isError":true`)) ||
		!bytes.Contains(payload, []byte(`"mimeType":"image/png"`)) {
		t.Fatalf("remote image_view = %d: %s", res.StatusCode, payload)
	}
}
