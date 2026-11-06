package integration

import (
	"bufio"
	"context"
	"crypto/ecdh"
	"crypto/ed25519"
	"encoding/base64"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func enrollPayload(token string) map[string]any {
	idPub, _, _ := ed25519.GenerateKey(nil)
	transport, _ := ecdh.X25519().GenerateKey(nil)
	return map[string]any{
		"token":              token,
		"name":               "itest-node",
		"hostname":           "itest-host",
		"platform":           "linux",
		"arch":               "amd64",
		"version":            "0.20.2",
		"identityPublicKey":  base64.RawURLEncoding.EncodeToString(idPub),
		"transportPublicKey": base64.RawURLEncoding.EncodeToString(transport.PublicKey().Bytes()),
		"capabilities":       []string{"shell"},
	}
}

func TestServerFailsClosedWithoutDevelopment(t *testing.T) {
	srv := startServer(t)

	status := httpJSON(t, http.MethodGet, srv.baseURL+"/healthz", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("healthz status = %d", status)
	}

	for _, target := range []string{
		"/api/v1/devices",
		"/api/v1/node/enroll",
		"/api/v1/events",
		"/mcp",
	} {
		method := http.MethodGet
		if strings.Contains(target, "enroll") || target == "/mcp" {
			method = http.MethodPost
		}
		req, err := http.NewRequest(method, srv.baseURL+target, nil)
		if err != nil {
			t.Fatal(err)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s: %v", target, err)
		}
		_ = res.Body.Close()
		want := http.StatusUnauthorized
		switch target {
		case "/api/v1/node/enroll":
			want = http.StatusBadRequest
		}
		if res.StatusCode != want {
			t.Fatalf("%s status = %d, want %d", target, res.StatusCode, want)
		}
		if got := res.Header.Get("Cache-Control"); got != "no-store" {
			t.Fatalf("%s Cache-Control = %q", target, got)
		}
	}
}

func TestServerDevelopmentEnrollment(t *testing.T) {
	srv := startServer(t, "RC_DEV_INSECURE=1", "RC_DEV_ENROLL_TOKEN=itest-token")

	var enrolled struct {
		DeviceID string `json:"deviceId"`
	}
	if status := httpJSON(t, http.MethodPost, srv.baseURL+"/api/v1/node/enroll", enrollPayload("itest-token"), &enrolled); status != http.StatusOK {
		t.Fatalf("enroll status = %d", status)
	}
	if enrolled.DeviceID == "" {
		t.Fatal("empty device id")
	}

	// One-time token enforcement.
	if status := httpJSON(t, http.MethodPost, srv.baseURL+"/api/v1/node/enroll", enrollPayload("itest-token"), nil); status != http.StatusForbidden {
		t.Fatalf("replayed token status = %d, want 403", status)
	}

	var devices struct {
		Devices []struct {
			ID string `json:"id"`
		} `json:"devices"`
	}
	if status := httpJSON(t, http.MethodGet, srv.baseURL+"/api/v1/devices", nil, &devices); status != http.StatusOK {
		t.Fatalf("list status = %d", status)
	}
	if len(devices.Devices) != 1 || devices.Devices[0].ID != enrolled.DeviceID {
		t.Fatalf("unexpected devices: %s", jsonString(t, devices))
	}

	var mcp struct {
		Result struct {
			Machines []struct {
				ID string `json:"id"`
			} `json:"machines"`
		} `json:"result"`
	}
	request := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "machines_list"}
	if status := httpJSON(t, http.MethodPost, srv.baseURL+"/mcp", request, &mcp); status != http.StatusOK {
		t.Fatalf("mcp status = %d", status)
	}
	if len(mcp.Result.Machines) != 1 || mcp.Result.Machines[0].ID != enrolled.DeviceID {
		t.Fatalf("unexpected machines: %s", jsonString(t, mcp))
	}

	if status := httpJSON(t, http.MethodDelete, srv.baseURL+"/api/v1/devices/"+enrolled.DeviceID, nil, nil); status != http.StatusNoContent {
		t.Fatalf("delete status = %d", status)
	}
	if status := httpJSON(t, http.MethodGet, srv.baseURL+"/api/v1/devices/"+enrolled.DeviceID, nil, nil); status != http.StatusNotFound {
		t.Fatalf("deleted device status = %d, want 404", status)
	}
}

func TestServerEventStream(t *testing.T) {
	srv := startServer(t, "RC_DEV_INSECURE=1")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.baseURL+"/api/v1/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("events request: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("events status = %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content type = %q", ct)
	}

	scanner := bufio.NewScanner(res.Body)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !scanner.Scan() {
			break
		}
		if strings.HasPrefix(scanner.Text(), ": ping") {
			return
		}
	}
	t.Fatal("event stream did not send an initial ping")
}

func TestServerAssetsAreImmutable(t *testing.T) {
	assets := t.TempDir()
	if err := os.MkdirAll(assets, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assets, "app.abc123.js"), []byte("console.log(1)"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := startServer(t, "RC_ASSETS_DIR="+assets)

	res, err := http.Get(srv.baseURL + "/assets/app.abc123.js")
	if err != nil {
		t.Fatalf("asset request: %v", err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("asset status = %d", res.StatusCode)
	}
	if got := res.Header.Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("asset Cache-Control = %q", got)
	}

	// Encoded traversal must not escape the asset root.
	res, err = http.Get(srv.baseURL + "/assets/%2e%2e/rc-server")
	if err != nil {
		t.Fatalf("traversal request: %v", err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNotFound && res.StatusCode != http.StatusMovedPermanently && res.StatusCode != http.StatusBadRequest {
		t.Fatalf("traversal status = %d", res.StatusCode)
	}
}
