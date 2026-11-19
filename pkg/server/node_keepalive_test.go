package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OhRats-Technologies/rc/pkg/protocol"
	"golang.org/x/net/websocket"
)

func dialNodeHub(t *testing.T, hub *NodeHub, deviceID string) (*websocket.Conn, *httptest.Server) {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		hub.Serve(deviceID, w, req)
	}))
	conn, err := websocket.Dial("ws"+strings.TrimPrefix(ts.URL, "http"), "", "http://localhost/")
	if err != nil {
		ts.Close()
		t.Fatal(err)
	}
	return conn, ts
}

func sendNodeMessage(t *testing.T, conn *websocket.Conn, message protocol.NodeToServer) {
	t.Helper()
	encoded, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	frames, err := protocol.Encode(string(encoded))
	if err != nil {
		t.Fatal(err)
	}
	for _, frame := range frames {
		if err := websocket.Message.Send(conn, frame); err != nil {
			t.Fatal(err)
		}
	}
}

// readServerFrames drains frames the server sends and reports the first read error.
func readServerFrames(conn *websocket.Conn, received *atomic.Int64) <-chan error {
	errs := make(chan error, 1)
	go func() {
		for {
			var payload []byte
			if err := websocket.Message.Receive(conn, &payload); err != nil {
				errs <- err
				return
			}
			received.Add(1)
		}
	}()
	return errs
}

func TestKeepaliveNodeStaysConnectedWhileIdleAndIsDroppedWhenSilent(t *testing.T) {
	hub := newNodeHub()
	hub.keepaliveInterval = 10 * time.Millisecond
	hub.idleTimeout = 200 * time.Millisecond
	conn, ts := dialNodeHub(t, hub, "dev_keepalive")
	defer ts.Close()
	defer conn.Close()

	sendNodeMessage(t, conn, protocol.NodeToServer{Type: "hello", Hello: &protocol.NodeHello{
		Capabilities: []string{protocol.CapabilityKeepalive},
	}})
	var serverFrames atomic.Int64
	errs := readServerFrames(conn, &serverFrames)

	// Node keepalives keep an otherwise idle channel open for several idle timeouts.
	stop := time.After(3 * hub.idleTimeout)
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	active := true
	for active {
		select {
		case <-stop:
			active = false
		case <-tick.C:
			sendNodeMessage(t, conn, protocol.NodeToServer{Type: protocol.TypeKeepalive})
		case err := <-errs:
			t.Fatalf("keepalive Node was disconnected while sending keepalives: %v", err)
		}
	}
	if serverFrames.Load() == 0 {
		t.Fatal("server sent no keepalive frames to the idle Node")
	}

	// Silence must end the channel: the idle deadline closes it within a few timeouts.
	select {
	case <-errs:
	case <-time.After(5 * hub.idleTimeout):
		t.Fatal("silent keepalive Node was not disconnected by the idle timeout")
	}
	if hub.Online("dev_keepalive") {
		t.Fatal("silent Node still reported online")
	}
}

func TestNodeWithoutKeepaliveCapabilityIsNotIdleDisconnected(t *testing.T) {
	hub := newNodeHub()
	hub.keepaliveInterval = 10 * time.Millisecond
	hub.idleTimeout = 100 * time.Millisecond
	conn, ts := dialNodeHub(t, hub, "dev_legacy")
	defer ts.Close()
	defer conn.Close()

	// A Node from before keepalives never announces the capability, so the server must not impose
	// the idle deadline on it.
	sendNodeMessage(t, conn, protocol.NodeToServer{Type: "hello", Hello: &protocol.NodeHello{
		Capabilities: []string{"shell"},
	}})
	var serverFrames atomic.Int64
	errs := readServerFrames(conn, &serverFrames)
	select {
	case err := <-errs:
		t.Fatalf("legacy Node disconnected while idle: %v", err)
	case <-time.After(4 * hub.idleTimeout):
	}
}
