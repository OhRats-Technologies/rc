package main

import (
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
)

type webrtcControlTransport struct {
	peer      *webrtc.PeerConnection
	channel   *webrtc.DataChannel
	signaling *websocket.Conn
	writeMu   *sync.Mutex
	deviceID  string
	sessionID string
	incoming  chan wireMessage
	closed    chan struct{}
	closeOnce sync.Once
}

func decodeIceServers(value any) []iceServer {
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var servers []iceServer
	if json.Unmarshal(data, &servers) != nil {
		return nil
	}
	return servers
}

func openWebRTCClientTransport(conn *websocket.Conn, writeMu *sync.Mutex, deviceID, sessionID string, servers []iceServer) (controlTransport, error) {
	peer, err := webrtc.NewPeerConnection(webrtc.Configuration{ICEServers: pionIceServers(servers)})
	if err != nil {
		return nil, err
	}
	channel, err := peer.CreateDataChannel("rc-control", nil)
	if err != nil {
		_ = peer.Close()
		return nil, err
	}
	transport := &webrtcControlTransport{peer: peer, channel: channel, signaling: conn, writeMu: writeMu,
		deviceID: deviceID, sessionID: sessionID, incoming: make(chan wireMessage, 128), closed: make(chan struct{})}
	opened := make(chan struct{})
	var openOnce sync.Once
	channel.OnOpen(func() { openOnce.Do(func() { close(opened) }) })
	channel.OnClose(transport.markClosed)
	channel.OnMessage(func(data webrtc.DataChannelMessage) {
		var frame wireMessage
		if json.Unmarshal(data.Data, &frame) != nil || frame.Type != "control.frame" || frame.SessionID != sessionID {
			_ = channel.Close()
			return
		}
		select {
		case transport.incoming <- frame:
		default:
			_ = channel.Close()
		}
	})
	offer, err := peer.CreateOffer(nil)
	if err != nil {
		_ = peer.Close()
		return nil, err
	}
	gathered := webrtc.GatheringCompletePromise(peer)
	if err = peer.SetLocalDescription(offer); err != nil {
		_ = peer.Close()
		return nil, err
	}
	select {
	case <-gathered:
	case <-time.After(5 * time.Second):
		_ = peer.Close()
		return nil, errors.New("WebRTC ICE gathering timed out")
	}
	local := peer.LocalDescription()
	if local == nil || local.SDP == "" {
		_ = peer.Close()
		return nil, errors.New("WebRTC offer unavailable")
	}
	requestID := randomURLBytes(12)
	if err = writeJSON(conn, writeMu, map[string]any{"type": "control.webrtc", "requestId": requestID, "deviceId": deviceID,
		"sessionId": sessionID, "sdp": local.SDP}); err != nil {
		_ = peer.Close()
		return nil, err
	}
	answer, err := waitOuterResponse(conn, requestID)
	if err != nil {
		_ = peer.Close()
		return nil, err
	}
	sdp, _ := answer["sdp"].(string)
	if sdp == "" || peer.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: sdp}) != nil {
		_ = peer.Close()
		return nil, errors.New("WebRTC answer rejected")
	}
	select {
	case <-opened:
		go transport.drainSignaling()
		return transport, nil
	case <-time.After(7 * time.Second):
		_ = peer.Close()
		return nil, errors.New("WebRTC connection timed out")
	}
}

func (transport *webrtcControlTransport) sendFrame(_, sessionID string, sequence uint64, ciphertext string) error {
	if sessionID != transport.sessionID || transport.channel.ReadyState() != webrtc.DataChannelStateOpen {
		return errors.New("WebRTC control channel unavailable")
	}
	data, _ := json.Marshal(wireMessage{Type: "control.frame", SessionID: sessionID, Sequence: sequence, Ciphertext: ciphertext})
	return transport.channel.Send(data)
}

func (transport *webrtcControlTransport) readFrame(sessionID string) (uint64, string, error) {
	if sessionID != transport.sessionID {
		return 0, "", errors.New("invalid control session")
	}
	select {
	case frame := <-transport.incoming:
		return frame.Sequence, frame.Ciphertext, nil
	case <-transport.closed:
		return 0, "", errors.New("WebRTC control channel closed")
	}
}

func (transport *webrtcControlTransport) markClosed() {
	transport.closeOnce.Do(func() { close(transport.closed) })
}

func (transport *webrtcControlTransport) drainSignaling() {
	for {
		var ignored any
		if transport.signaling.ReadJSON(&ignored) != nil {
			transport.markClosed()
			return
		}
	}
}

func (transport *webrtcControlTransport) close() error {
	_ = writeJSON(transport.signaling, transport.writeMu, map[string]any{"type": "control.close", "deviceId": transport.deviceID, "sessionId": transport.sessionID})
	err := transport.peer.Close()
	_ = transport.signaling.Close()
	transport.markClosed()
	return err
}
