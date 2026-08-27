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
	manager.sessions["session"] = &controlSession{aead: nodeAEAD,
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
	receivedText := make(chan bool, 4)
	channel.OnOpen(func() { close(opened) })
	channel.OnMessage(func(message webrtc.DataChannelMessage) {
		var frame wireMessage
		if json.Unmarshal(message.Data, &frame) == nil {
			receivedText <- message.IsString
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
	select {
	case isString := <-receivedText:
		if !isString {
			t.Fatal("Node sent WebRTC control frame as binary instead of text")
		}
	case <-time.After(time.Second):
		t.Fatal("client did not receive WebRTC frame type")
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

	if err := channel.Close(); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(time.Second)
	for {
		manager.mu.Lock()
		_, exists := manager.sessions["session"]
		manager.mu.Unlock()
		if !exists {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("closed DataChannel did not end control session")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if manager.sendFrame("session", wireMessage{Type: "control.result", RequestID: "after-close", Output: "ok"}) {
		t.Fatal("closed WebRTC session still accepted an outbound frame")
	}
	select {
	case metadata := <-outbound:
		if metadata.Type != "control.closed" || metadata.SessionID != "session" {
			t.Fatalf("closed WebRTC session leaked unexpected agent WebSocket payload: %+v", metadata)
		}
	default:
		t.Fatal("closed WebRTC session did not report control.closed metadata")
	}
	select {
	case leaked := <-outbound:
		t.Fatalf("closed WebRTC session leaked extra agent WebSocket payload: %+v", leaked)
	default:
	}

}
