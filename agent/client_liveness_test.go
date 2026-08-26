package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func livenessFixture(t *testing.T, readMessages bool) (string, state, func()) {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v1/agent/challenge":
			_ = json.NewEncoder(writer).Encode(map[string]string{"challenge": "test-challenge"})
		case "/api/v1/agent/ws":
			connection, upgradeErr := upgrader.Upgrade(writer, request, nil)
			if upgradeErr != nil {
				return
			}
			defer connection.Close()
			if readMessages {
				for {
					if _, _, readErr := connection.ReadMessage(); readErr != nil {
						return
					}
				}
			}
			<-request.Context().Done()
		default:
			http.NotFound(writer, request)
		}
	}))
	value := state{DeviceID: "device", PrivateKey: base64.RawStdEncoding.EncodeToString(privateKey)}
	return server.URL, value, server.Close
}

func TestConnectEndsSilentHalfOpenConnection(t *testing.T) {
	server, value, closeServer := livenessFixture(t, false)
	defer closeServer()
	manager := newProcessManager()
	defer manager.shutdown()
	started := time.Now()
	err := connectWithLiveness(context.Background(), server, value, t.TempDir(), manager, 10*time.Millisecond, 50*time.Millisecond)
	if err == nil {
		t.Fatal("silent connection did not time out")
	}
	if time.Since(started) > time.Second {
		t.Fatalf("silent connection took too long to fail: %v", time.Since(started))
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("unexpected liveness error: %v", err)
	}
}

func TestConnectStaysAliveWhenPeerAnswersPings(t *testing.T) {
	server, value, closeServer := livenessFixture(t, true)
	defer closeServer()
	manager := newProcessManager()
	defer manager.shutdown()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := connectWithLiveness(ctx, server, value, t.TempDir(), manager, 10*time.Millisecond, 50*time.Millisecond); err != nil {
		t.Fatalf("healthy connection ended: %v", err)
	}
}
