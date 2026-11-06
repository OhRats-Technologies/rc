package server

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/OhRats-Technologies/rc/pkg/node"
)

func TestMcpExecutionRoundTrip(t *testing.T) {
	srv, ts := newDevServer(t)

	// Real execution with NativeExecutionManager
	res := postJSON(t, ts.URL+"/mcp", map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "process_run",
		"params": map[string]any{
			"deviceId":       "dev_test",
			"program":        "echo",
			"args":           []string{"mcp-wire-ok"},
			"timeoutSeconds": 2,
		},
	})
	defer res.Body.Close()
	var runResp struct {
		Result struct {
			ProcessID  string              `json:"processId"`
			Status     string              `json:"status"`
			Chunks     []node.JournalChunk `json:"chunks"`
			NextCursor uint64              `json:"nextCursor"`
		} `json:"result"`
	}
	if err := json.NewDecoder(res.Body).Decode(&runResp); err != nil {
		t.Fatal(err)
	}
	if runResp.Result.ProcessID == "" {
		t.Fatal("expected processId in result")
	}

	// Status query
	statusRes := postJSON(t, ts.URL+"/mcp", map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "process_status",
		"params": map[string]any{
			"processId":   runResp.Result.ProcessID,
			"cursor":      0,
			"waitSeconds": 1,
		},
	})
	defer statusRes.Body.Close()
	var statusResp struct {
		Result struct {
			Status string              `json:"status"`
			Chunks []node.JournalChunk `json:"chunks"`
		} `json:"result"`
	}
	if err := json.NewDecoder(statusRes.Body).Decode(&statusResp); err != nil {
		t.Fatal(err)
	}
	if statusResp.Result.Status != "completed" {
		t.Fatalf("expected completed status, got %s", statusResp.Result.Status)
	}

	// Fail closed when execution manager is nil
	srv.mcp.exec = nil
	nilRes := postJSON(t, ts.URL+"/mcp", map[string]any{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "process_run",
		"params":  map[string]any{"deviceId": "dev_test", "command": "true"},
	})
	defer nilRes.Body.Close()
	var errResp struct {
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(nilRes.Body).Decode(&errResp); err != nil {
		t.Fatal(err)
	}
	if errResp.Error.Code != -32603 || !strings.Contains(errResp.Error.Message, "control plane") {
		t.Fatalf("expected fail-closed MCP execution, got %#v", errResp)
	}
}
