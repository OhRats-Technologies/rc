package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestMCPInputCancelAndProtocolErrors(t *testing.T) {
	_, ts := newMcpDevServer(t)

	res, payload := mcpCall(t, ts, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{
			"name": "process_run",
			"arguments": map[string]any{
				"deviceId": "dev_test", "argv": mcpHelperArgv(), "env": mcpHelperEnv("cat"),
				"waitSeconds": 0,
			},
		},
	}, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("process_run status = %d: %s", res.StatusCode, payload)
	}
	var run struct {
		Result struct {
			StructuredContent mcpProcessResult `json:"structuredContent"`
		} `json:"result"`
	}
	_ = json.Unmarshal(payload, &run)
	processID := run.Result.StructuredContent.ProcessID
	if processID == "" {
		t.Fatalf("missing process ID: %s", payload)
	}

	res, payload = mcpCall(t, ts, map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "tools/call",
		"params": map[string]any{
			"name": "process_input",
			"arguments": map[string]any{
				"deviceId": "dev_test", "processId": processID,
				"data": "hello\n", "eof": true,
			},
		},
	}, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("process_input status = %d: %s", res.StatusCode, payload)
	}

	var output mcpProcessResult
	var outputText strings.Builder
	cursor := uint64(0)
	for attempt := 0; attempt < 10 && output.Status != "exited"; attempt++ {
		res, payload = mcpCall(t, ts, map[string]any{
			"jsonrpc": "2.0", "id": 3, "method": "tools/call",
			"params": map[string]any{
				"name": "process_status",
				"arguments": map[string]any{
					"deviceId": "dev_test", "processId": processID,
					"cursor": cursor, "waitSeconds": 5,
				},
			},
		}, nil)
		var response struct {
			Result struct {
				StructuredContent mcpProcessResult `json:"structuredContent"`
			} `json:"result"`
		}
		_ = json.Unmarshal(payload, &response)
		output = response.Result.StructuredContent
		outputText.WriteString(processChunkText(output))
		cursor = output.NextCursor
	}
	if output.Status != "exited" || !strings.Contains(outputText.String(), "hello") {
		t.Fatalf("process output = %+v", output)
	}

	_, payload = mcpCall(t, ts, map[string]any{
		"jsonrpc": "2.0", "id": 4, "method": "tools/call",
		"params": map[string]any{
			"name": "process_run",
			"arguments": map[string]any{
				"deviceId": "dev_test", "argv": mcpHelperArgv(), "env": mcpHelperEnv("sleep"),
				"waitSeconds": 0,
			},
		},
	}, nil)
	var sleeping struct {
		Result struct {
			StructuredContent mcpProcessResult `json:"structuredContent"`
		} `json:"result"`
	}
	_ = json.Unmarshal(payload, &sleeping)
	res, payload = mcpCall(t, ts, map[string]any{
		"jsonrpc": "2.0", "id": 5, "method": "tools/call",
		"params": map[string]any{
			"name": "process_cancel",
			"arguments": map[string]any{
				"deviceId": "dev_test", "processId": sleeping.Result.StructuredContent.ProcessID,
				"signal": "KILL",
			},
		},
	}, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("process_cancel status = %d: %s", res.StatusCode, payload)
	}

	res, payload = mcpCall(t, ts, map[string]any{
		"jsonrpc": "2.0", "id": 6, "method": "resources/list",
	}, map[string]string{"MCP-Protocol-Version": "2025-06-18"})
	if res.StatusCode != http.StatusOK || !strings.Contains(string(payload), "-32601") {
		t.Fatalf("unknown method = %d: %s", res.StatusCode, payload)
	}

	res, payload = mcpCall(t, ts, map[string]any{
		"jsonrpc": "2.0", "id": 7, "method": "initialize",
		"params": map[string]any{"protocolVersion": "2025-06-18"},
	}, map[string]string{"MCP-Protocol-Version": "1900-01-01"})
	if res.StatusCode != http.StatusBadRequest || !strings.Contains(string(payload), "-32022") {
		t.Fatalf("unsupported protocol = %d: %s", res.StatusCode, payload)
	}
}

func processChunkText(result mcpProcessResult) string {
	var text strings.Builder
	for _, chunk := range result.Chunks {
		text.WriteString(chunk.Data)
	}
	return text.String()
}
