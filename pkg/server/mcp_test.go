package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func mcpHelperArgv() []string {
	return []string{os.Args[0], "-test.run=TestMCPHelperProcess"}
}

func mcpHelperEnv(mode string) map[string]*string {
	enabled, selected := "1", mode
	return map[string]*string{"RC_MCP_HELPER": &enabled, "RC_MCP_HELPER_MODE": &selected}
}

func TestMCPHelperProcess(t *testing.T) {
	if os.Getenv("RC_MCP_HELPER") != "1" {
		return
	}
	switch os.Getenv("RC_MCP_HELPER_MODE") {
	case "echo":
		fmt.Fprint(os.Stdout, "mcp-wire-ok")
	case "cat":
		_, _ = io.Copy(os.Stdout, os.Stdin)
	case "sleep":
		time.Sleep(30 * time.Second)
	}
	os.Exit(0)
}

func newMcpDevServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	srv, ts := newDevServer(t)
	srv.registry.mu.Lock()
	srv.registry.devices["dev_test"] = &RegisteredDevice{
		ID: "dev_test", WorkspaceID: "ws_default", Name: "Test Node",
		Hostname: "test-host", Platform: "linux", Arch: "amd64",
		Version: "0.20.6", Online: true,
	}
	srv.registry.mu.Unlock()
	return srv, ts
}

func mcpCall(t *testing.T, ts *httptest.Server, body map[string]any, headers map[string]string) (*http.Response, []byte) {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/mcp", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	response, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		t.Fatal(err)
	}
	return res, response
}

func TestMCPInitializeAndToolsList(t *testing.T) {
	_, ts := newMcpDevServer(t)

	res, payload := mcpCall(t, ts, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-06-18",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "test", "version": "1"},
		},
	}, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("initialize status = %d: %s", res.StatusCode, payload)
	}
	var initialized struct {
		Result struct {
			ProtocolVersion string         `json:"protocolVersion"`
			Capabilities    map[string]any `json:"capabilities"`
			ServerInfo      struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			} `json:"serverInfo"`
		} `json:"result"`
	}
	if err := json.Unmarshal(payload, &initialized); err != nil {
		t.Fatal(err)
	}
	if initialized.Result.ProtocolVersion != "2025-06-18" ||
		initialized.Result.ServerInfo.Name != "RC" ||
		initialized.Result.Capabilities["tools"] == nil {
		t.Fatalf("initialize result = %+v", initialized.Result)
	}
	if res.Header.Get("x-rc-request-id") == "" {
		t.Fatal("missing request reference header")
	}

	_, discover := mcpCall(t, ts, map[string]any{
		"jsonrpc": "2.0", "id": 10, "method": "server/discover",
	}, map[string]string{"MCP-Protocol-Version": mcpProtocolVersion})
	if !strings.Contains(string(discover), `"supportedVersions"`) {
		t.Fatalf("server/discover = %s", discover)
	}

	res, _ = mcpCall(t, ts, map[string]any{
		"jsonrpc": "2.0", "method": "notifications/initialized",
	}, map[string]string{"MCP-Protocol-Version": "2025-06-18"})
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("initialized notification status = %d", res.StatusCode)
	}

	res, payload = mcpCall(t, ts, map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "tools/list", "params": map[string]any{},
	}, map[string]string{"MCP-Protocol-Version": mcpProtocolVersion})
	if res.StatusCode != http.StatusOK || !strings.Contains(string(payload), `"resultType":"complete"`) {
		t.Fatalf("tools/list status = %d: %s", res.StatusCode, payload)
	}
	var listed struct {
		Result struct {
			Tools []map[string]any `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(payload, &listed); err != nil {
		t.Fatal(err)
	}
	want := []string{"machines_list", "image_view", "process_run", "process_status", "process_input", "process_cancel"}
	if len(listed.Result.Tools) != len(want) {
		t.Fatalf("tools = %d, want %d", len(listed.Result.Tools), len(want))
	}
	for index, name := range want {
		if listed.Result.Tools[index]["name"] != name {
			t.Fatalf("tool[%d] = %v, want %s", index, listed.Result.Tools[index]["name"], name)
		}
		if listed.Result.Tools[index]["outputSchema"] == nil {
			t.Fatalf("%s is missing outputSchema", name)
		}
		encoded, _ := json.Marshal(listed.Result.Tools[index])
		if bytes.Contains(encoded, []byte(`"minLength"`)) || bytes.Contains(encoded, []byte(`"maxLength"`)) {
			t.Fatalf("%s has an arbitrary string length constraint", name)
		}
	}
}

func TestMCPMachinesAndProcessRoundTrip(t *testing.T) {
	_, ts := newMcpDevServer(t)

	res, payload := mcpCall(t, ts, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": "machines_list", "arguments": map[string]any{}},
	}, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("machines_list status = %d: %s", res.StatusCode, payload)
	}
	var machines struct {
		Result struct {
			StructuredContent struct {
				Machines []struct {
					ID     string `json:"id"`
					Name   string `json:"name"`
					Online bool   `json:"online"`
				} `json:"machines"`
			} `json:"structuredContent"`
		} `json:"result"`
	}
	if err := json.Unmarshal(payload, &machines); err != nil {
		t.Fatal(err)
	}
	if len(machines.Result.StructuredContent.Machines) != 1 ||
		machines.Result.StructuredContent.Machines[0].ID != "dev_test" {
		t.Fatalf("machines = %+v", machines.Result.StructuredContent.Machines)
	}

	res, payload = mcpCall(t, ts, map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "tools/call",
		"params": map[string]any{
			"name": "process_run",
			"arguments": map[string]any{
				"deviceId":    "dev_test",
				"argv":        mcpHelperArgv(),
				"env":         mcpHelperEnv("echo"),
				"waitSeconds": 5,
			},
		},
	}, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("process_run status = %d: %s", res.StatusCode, payload)
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
	if run.Result.IsError || run.Result.StructuredContent.ProcessID == "" ||
		run.Result.StructuredContent.Status != "exited" {
		t.Fatalf("process_run result = %+v", run.Result)
	}
	if len(run.Result.StructuredContent.Chunks) == 0 ||
		!strings.Contains(run.Result.StructuredContent.Chunks[0].Data, "mcp-wire-ok") {
		t.Fatalf("process_run chunks = %+v", run.Result.StructuredContent.Chunks)
	}

	res, payload = mcpCall(t, ts, map[string]any{
		"jsonrpc": "2.0", "id": 3, "method": "tools/call",
		"params": map[string]any{
			"name": "process_status",
			"arguments": map[string]any{
				"deviceId": "dev_test", "processId": run.Result.StructuredContent.ProcessID,
				"waitSeconds": 5,
			},
		},
	}, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("process_status status = %d: %s", res.StatusCode, payload)
	}
	var status struct {
		Result struct {
			StructuredContent mcpProcessResult `json:"structuredContent"`
		} `json:"result"`
	}
	if err := json.Unmarshal(payload, &status); err != nil {
		t.Fatal(err)
	}
	if status.Result.StructuredContent.Status != "exited" {
		t.Fatalf("process_status = %+v", status.Result.StructuredContent)
	}
}
