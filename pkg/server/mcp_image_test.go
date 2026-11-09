package server

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestMCPImageView(t *testing.T) {
	_, ts := newMcpDevServer(t)
	path := filepath.Join(t.TempDir(), "dot.png")
	data, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Y9ZQmcAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	res, payload := mcpCall(t, ts, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{
			"name":      "image_view",
			"arguments": map[string]any{"deviceId": "dev_test", "path": path},
		},
	}, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("image_view status = %d: %s", res.StatusCode, payload)
	}
	var response struct {
		Result struct {
			Content []struct {
				Type     string `json:"type"`
				MimeType string `json:"mimeType"`
				Data     string `json:"data"`
			} `json:"content"`
			StructuredContent struct {
				DeviceID  string `json:"deviceId"`
				Path      string `json:"path"`
				MimeType  string `json:"mimeType"`
				SizeBytes int    `json:"sizeBytes"`
			} `json:"structuredContent"`
		} `json:"result"`
	}
	if err := json.Unmarshal(payload, &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Result.Content) != 2 ||
		response.Result.Content[1].Type != "image" ||
		response.Result.Content[1].MimeType != "image/png" ||
		response.Result.Content[1].Data == "" ||
		response.Result.StructuredContent.MimeType != "image/png" ||
		response.Result.StructuredContent.SizeBytes != len(data) {
		t.Fatalf("image_view response = %+v", response.Result)
	}
}
