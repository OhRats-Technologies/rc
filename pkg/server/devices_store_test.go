package server

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestDevicesSurviveServerRestart(t *testing.T) {
	dir := t.TempDir()
	first := New(Config{DataDir: dir, PublicURL: "https://rc.example.test"})
	if first.auth == nil {
		t.Fatal("account store should be open")
	}
	device := &RegisteredDevice{
		ID: "dev_restart", WorkspaceID: defaultWorkspaceID, Name: "box", Hostname: "box",
		Platform: "linux", Arch: "amd64", Version: "0.20.4", Capabilities: []string{"process"},
		IdentityPublicKey:  base64URL(32, 1),
		TransportPublicKey: base64URL(32, 2),
	}
	if err := first.registry.persistEnrollment(device); err != nil {
		t.Fatalf("persist enrollment: %v", err)
	}
	first.registry.mu.Lock()
	first.registry.devices[device.ID] = device
	first.registry.mu.Unlock()
	if _, err := first.registry.Rename(device.ID, "renamed"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	first.Close()

	second := New(Config{DataDir: dir, PublicURL: "https://rc.example.test"})
	defer second.Close()
	devices := second.registry.List()
	if len(devices) != 1 || devices[0].ID != device.ID || devices[0].Name != "renamed" ||
		devices[0].IdentityPublicKey != device.IdentityPublicKey || len(devices[0].Capabilities) != 1 {
		t.Fatalf("devices after restart = %+v", devices)
	}
}

func TestRemovedNodeGetsGone(t *testing.T) {
	srv := New(Config{DataDir: t.TempDir(), PublicURL: "https://rc.example.test"})
	defer srv.Close()
	device := &RegisteredDevice{
		ID: "dev_removed", WorkspaceID: defaultWorkspaceID, Name: "gone", Hostname: "gone",
		Platform: "linux", Arch: "amd64", Version: "0.20.4",
		IdentityPublicKey: base64URL(32, 3), TransportPublicKey: base64URL(32, 4),
	}
	if err := srv.registry.persistEnrollment(device); err != nil {
		t.Fatal(err)
	}
	srv.registry.mu.Lock()
	srv.registry.devices[device.ID] = device
	srv.registry.mu.Unlock()
	if err := srv.registry.remove(device.ID); err != nil {
		t.Fatalf("remove: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/node/self", nil)
	req.Header.Set("x-rc-device", device.ID)
	req.Header.Set("x-rc-timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	req.Header.Set("x-rc-nonce", "nonce-removed-node")
	req.Header.Set("x-rc-signature", "unused")
	_, err := srv.registry.verifyNode(req, nil)
	if nodeAuthStatus(err) != http.StatusGone {
		t.Fatalf("removed Node status = %d, err = %v", nodeAuthStatus(err), err)
	}

	unknown := httptest.NewRequest(http.MethodGet, "/api/v1/node/self", nil)
	unknown.Header.Set("x-rc-device", "dev_never_existed")
	unknown.Header.Set("x-rc-timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	unknown.Header.Set("x-rc-nonce", "nonce-unknown-node")
	unknown.Header.Set("x-rc-signature", "unused")
	_, err = srv.registry.verifyNode(unknown, nil)
	if nodeAuthStatus(err) != http.StatusUnauthorized {
		t.Fatalf("unknown Node must stay 401, got %d", nodeAuthStatus(err))
	}
}

func base64URL(size int, seed byte) string {
	raw := make([]byte, size)
	for i := range raw {
		raw[i] = seed + byte(i)
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}
