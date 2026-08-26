package main

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
)

func TestWebRTCControlFramesBypassRelay(t *testing.T) {
	staticPrivate, staticPublic := encodedX25519(t)
	ephemeralPrivate, ephemeralPublic := encodedX25519(t)
	clientPrivate, clientPublic := encodedX25519(t)
	nodeAEAD, err := deriveNodeAEAD(staticPrivate, ephemeralPrivate, clientPublic, "challenge", "device", "client")
	if err != nil {
		t.Fatal(err)
	}
	clientAEAD, err := deriveClientAEAD(clientPrivate, staticPublic, ephemeralPublic, "challenge", "device", "client")
	if err != nil {
		t.Fatal(err)
	}

	outbound := make(chan wireMessage, 8)
	processes := newProcessManager()
	defer processes.shutdown()
	manager := &controlManager{processes: processes, send: func(message wireMessage) error { outbound <- message; return nil },
		sessions: map[string]*controlSession{}, challenges: map[string]time.Time{}, pendingStarts: map[string]pendingSecureStart{}}
	manager.sessions["session"] = &controlSession{aead: nodeAEAD, send: manager.relayFrame, transportID: "relay",
		clientID: "client", userID: "user", role: "owner", canExecute: true}
	processes.setSecureSender(manager.sendFrame)

	peer, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	channel, err := peer.CreateDataChannel("rc-control", nil)
	if err != nil {
		t.Fatal(err)
	}
	opened := make(chan struct{})
	received := make(chan wireMessage, 4)
	channel.OnOpen(func() { close(opened) })
	channel.OnMessage(func(message webrtc.DataChannelMessage) {
		var frame wireMessage
		if json.Unmarshal(message.Data, &frame) == nil {
			received <- frame
		}
	})
	offer, err := peer.CreateOffer(nil)
	if err != nil {
		t.Fatal(err)
	}
	gathered := webrtc.GatheringCompletePromise(peer)
	if err = peer.SetLocalDescription(offer); err != nil {
		t.Fatal(err)
	}
	<-gathered
	manager.answerWebRTC(wireMessage{Type: "control.webrtc", RequestID: "offer", SessionID: "session", SDP: peer.LocalDescription().SDP})
	var answer wireMessage
	select {
	case answer = <-outbound:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for WebRTC answer")
	}
	if answer.Type != "control.webrtc.ready" {
		t.Fatalf("unexpected answer: %+v", answer)
	}
	if err = peer.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: answer.SDP}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-opened:
	case <-time.After(3 * time.Second):
		t.Fatal("data channel did not open")
	}
	time.Sleep(25 * time.Millisecond)

	command := wireMessage{Type: "process.resize", ID: "missing", Cols: 80, Rows: 24}
	plain, _ := json.Marshal(command)
	ciphertext := clientAEAD.Seal(nil, frameNonce(1, 1), plain, frameAAD("session", 1, "c2n"))
	frame, _ := json.Marshal(wireMessage{Type: "control.frame", SessionID: "session", Sequence: 1,
		Ciphertext: base64.RawURLEncoding.EncodeToString(ciphertext)})
	if err = channel.Send(frame); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		manager.mu.Lock()
		sequence := manager.sessions["session"].recvSeq
		manager.mu.Unlock()
		if sequence == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Node did not receive direct control frame")
		}
		time.Sleep(5 * time.Millisecond)
	}

	if !manager.sendFrame("session", wireMessage{Type: "control.result", RequestID: "reply", Output: "ok"}) {
		t.Fatal("Node did not send direct control frame")
	}
	select {
	case relayed := <-outbound:
		t.Fatalf("direct frame leaked to relay: %+v", relayed)
	default:
	}
	var response wireMessage
	select {
	case response = <-received:
	case <-time.After(time.Second):
		t.Fatal("client did not receive direct control frame")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(response.Ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	openedReply, err := clientAEAD.Open(nil, frameNonce(2, response.Sequence), decoded, frameAAD("session", response.Sequence, "n2c"))
	if err != nil {
		t.Fatal(err)
	}
	var reply wireMessage
	if json.Unmarshal(openedReply, &reply) != nil || reply.Type != "control.result" || reply.Output != "ok" {
		t.Fatalf("unexpected direct reply: %s", openedReply)
	}

	plain, _ = json.Marshal(wireMessage{Type: "process.resize", ID: "missing", Cols: 100, Rows: 30})
	ciphertext = clientAEAD.Seal(nil, frameNonce(1, 2), plain, frameAAD("session", 2, "c2n"))
	if err = manager.handle(wireMessage{Type: "control.frame", SessionID: "session", Sequence: 2,
		Ciphertext: base64.RawURLEncoding.EncodeToString(ciphertext)}); err != nil {
		t.Fatal(err)
	}
	if !manager.sendFrame("session", wireMessage{Type: "control.result", RequestID: "fallback", Output: "ok"}) {
		t.Fatal("Node did not fall back to relay")
	}
	select {
	case relayed := <-outbound:
		if relayed.Type != "control.frame" || relayed.SessionID != "session" || relayed.Sequence != 2 {
			t.Fatalf("unexpected fallback frame: %+v", relayed)
		}
		decoded, decodeErr := base64.RawURLEncoding.DecodeString(relayed.Ciphertext)
		if decodeErr != nil {
			t.Fatal(decodeErr)
		}
		openedFallback, openErr := clientAEAD.Open(nil, frameNonce(2, relayed.Sequence), decoded,
			frameAAD("session", relayed.Sequence, "n2c"))
		if openErr != nil {
			t.Fatal(openErr)
		}
		var fallback wireMessage
		if json.Unmarshal(openedFallback, &fallback) != nil || fallback.RequestID != "fallback" || fallback.Output != "ok" {
			t.Fatalf("unexpected fallback reply: %s", openedFallback)
		}
	case <-time.After(time.Second):
		t.Fatal("fallback reply did not use relay")
	}
}
