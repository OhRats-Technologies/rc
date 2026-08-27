package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestRevokedNodeClearsLocalEnrollment(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	device := state{DeviceID: "revoked-device", PrivateKey: base64.RawStdEncoding.EncodeToString(privateKey)}
	dir := t.TempDir()
	if err := saveState(dir, device); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lockPath(dir), []byte(`{"generation":1}`), 0600); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/agent/challenge":
			w.Header().Set("content-type", "application/json")
			_, _ = w.Write([]byte(`{"challenge":"revoked-challenge"}`))
		case "/api/v1/agent/ws":
			http.Error(w, "device removed", http.StatusGone)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	err = connectWithLiveness(context.Background(), server.URL, device, dir, newProcessManager(), time.Second, time.Second)
	if err != errNodeRemoved {
		t.Fatalf("expected node removed, got %v", err)
	}
	if _, err := os.Stat(statePath(dir)); !os.IsNotExist(err) {
		t.Fatalf("device state was not cleared: %v", err)
	}
	if _, err := os.Stat(lockPath(dir)); !os.IsNotExist(err) {
		t.Fatalf("lock state was not cleared: %v", err)
	}
}
