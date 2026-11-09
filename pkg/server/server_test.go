package server

import (
	"bytes"
	"crypto/ecdh"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func newDevServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	srv := New(Config{Addr: ":0", Development: true})
	ts := httptest.NewServer(srv.httpSrv.Handler)
	t.Cleanup(ts.Close)
	return srv, ts
}

func validEnrollPayload(token, name string) map[string]any {
	idPub, _, _ := ed25519.GenerateKey(nil)
	transport, _ := ecdh.X25519().GenerateKey(nil)
	return map[string]any{
		"token":              token,
		"name":               name,
		"hostname":           "host-1",
		"platform":           "linux",
		"arch":               "amd64",
		"version":            "0.20.2",
		"identityPublicKey":  base64.RawURLEncoding.EncodeToString(idPub),
		"transportPublicKey": base64.RawURLEncoding.EncodeToString(transport.PublicKey().Bytes()),
		"capabilities":       []string{"shell"},
	}
}

func postJSON(t *testing.T, url string, value any) *http.Response {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	res, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post %s failed: %v", url, err)
	}
	return res
}

func TestDevServerEnrollListAndMcp(t *testing.T) {
	srv, ts := newDevServer(t)
	token := srv.registry.GenerateToken()

	res := postJSON(t, ts.URL+"/api/v1/node/enroll", validEnrollPayload(token, "ServerTestNode"))
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("enroll status %d: %s", res.StatusCode, body)
	}
	var enrollResp struct {
		DeviceID string `json:"deviceId"`
	}
	_ = json.NewDecoder(res.Body).Decode(&enrollResp)
	res.Body.Close()
	if enrollResp.DeviceID == "" {
		t.Fatal("empty device ID from enroll")
	}

	// The enrollment token is one-time: replaying it must be rejected.
	res = postJSON(t, ts.URL+"/api/v1/node/enroll", validEnrollPayload(token, "Replay"))
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("replayed token status = %d, want 403", res.StatusCode)
	}
	res.Body.Close()

	res, err := http.Get(ts.URL + "/api/v1/devices")
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("list devices failed: %v, status: %v", err, res.StatusCode)
	}
	var devResp struct {
		Devices []struct {
			ID string `json:"id"`
		} `json:"devices"`
	}
	_ = json.NewDecoder(res.Body).Decode(&devResp)
	res.Body.Close()
	if len(devResp.Devices) != 1 || devResp.Devices[0].ID != enrollResp.DeviceID {
		t.Fatalf("unexpected devices list: %+v", devResp)
	}

	res = postJSON(t, ts.URL+"/mcp", map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "machines_list",
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("mcp machines_list status = %d", res.StatusCode)
	}
	var mcpResp struct {
		Result struct {
			Machines []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"machines"`
		} `json:"result"`
	}
	if err := json.NewDecoder(res.Body).Decode(&mcpResp); err != nil {
		t.Fatalf("decode MCP response: %v", err)
	}
	res.Body.Close()
	if len(mcpResp.Result.Machines) != 1 || mcpResp.Result.Machines[0].Name != "ServerTestNode" {
		t.Fatalf("unexpected MCP machines: %+v", mcpResp.Result.Machines)
	}

	res, err = http.Get(ts.URL + "/api/v1/devices/" + enrollResp.DeviceID)
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("get device failed: %v, status: %v", err, res.StatusCode)
	}
	res.Body.Close()

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/devices/"+enrollResp.DeviceID, nil)
	res, err = http.DefaultClient.Do(req)
	if err != nil || res.StatusCode != http.StatusNoContent {
		t.Fatalf("delete device failed: %v, status: %v", err, res.StatusCode)
	}
	res.Body.Close()
}

func TestProtectedRoutesFailClosed(t *testing.T) {
	srv := New(Config{Addr: ":0"})
	ts := httptest.NewServer(srv.httpSrv.Handler)
	defer ts.Close()

	cases := []struct {
		name   string
		method string
		path   string
	}{
		{"enroll", http.MethodPost, "/api/v1/node/enroll"},
		{"devices", http.MethodGet, "/api/v1/devices"},
		{"device", http.MethodGet, "/api/v1/devices/dev_x"},
		{"events", http.MethodGet, "/api/v1/events"},
		{"mcp", http.MethodPost, "/mcp"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest(tc.method, ts.URL+tc.path, bytes.NewReader([]byte(`{}`)))
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer res.Body.Close()
			want := http.StatusUnauthorized
			switch tc.name {
			case "enroll":
				want = http.StatusBadRequest
			case "mcp":
				want = http.StatusServiceUnavailable
			}
			if res.StatusCode != want {
				t.Fatalf("status = %d, want %d", res.StatusCode, want)
			}
			if got := res.Header.Get("Cache-Control"); got != "no-store" {
				t.Fatalf("Cache-Control = %q, want no-store", got)
			}
		})
	}
}

func TestAssetsRejectTraversal(t *testing.T) {
	root := t.TempDir()
	assets := filepath.Join(root, "assets")
	if err := os.MkdirAll(assets, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assets, "app.js"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := &Server{cfg: Config{AssetsDir: assets}}
	for _, target := range []string{"/assets/app.js", "/assets/../secret.txt", "/assets/%2e%2e/secret.txt", "/assets//etc/passwd"} {
		req := httptest.NewRequest(http.MethodGet, "http://localhost"+target, nil)
		req.URL.Path = target // bypass ServeMux path cleaning to test the handler
		rec := httptest.NewRecorder()
		srv.handleAssets(rec, req)
		if target == "/assets/app.js" {
			if rec.Code != http.StatusOK {
				t.Fatalf("%s status = %d", target, rec.Code)
			}
			continue
		}
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d, want 404", target, rec.Code)
		}
	}
}

func TestHealthAndIndex(t *testing.T) {
	srv := New(Config{Addr: ":0"})
	ts := httptest.NewServer(srv.httpSrv.Handler)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/healthz")
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("healthz failed: %v", err)
	}
	res.Body.Close()

	res, err = http.Get(ts.URL + "/")
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("index failed: %v", err)
	}
	res.Body.Close()

	res, err = http.Get(ts.URL + "/missing")
	if err != nil || res.StatusCode != http.StatusNotFound {
		t.Fatalf("missing route status = %v, want 404", res.StatusCode)
	}
	res.Body.Close()
}

func TestInstallerRoutesServePublicScripts(t *testing.T) {
	assets := t.TempDir()
	if err := os.WriteFile(filepath.Join(assets, "install.sh"), []byte("#!/bin/sh\necho bootstrapped\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assets, "install.ps1"), []byte("Write-Output bootstrapped\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := New(Config{Addr: ":0", DataDir: t.TempDir(), AssetsDir: assets})
	ts := httptest.NewServer(srv.httpSrv.Handler)
	defer ts.Close()
	for name, want := range map[string]string{
		"/install.sh":  "bootstrapped",
		"/install.ps1": "Write-Output bootstrapped",
	} {
		res, err := http.Get(ts.URL + name)
		if err != nil || res.StatusCode != http.StatusOK {
			t.Fatalf("%s = %v, %v", name, res, err)
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if !bytes.Contains(body, []byte(want)) {
			t.Fatalf("%s body = %q", name, body)
		}
		if res.Header.Get("Cache-Control") != "public, max-age=0, must-revalidate" {
			t.Fatalf("%s Cache-Control = %q", name, res.Header.Get("Cache-Control"))
		}
	}
}
