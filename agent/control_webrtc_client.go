package main

import (
	"encoding/json"
	"errors"
	"github.com/pion/webrtc/v4"
	"net/http"
	"net/url"
	"sync"
	"time"
)

type webrtcControlTransport struct {
	peer      *webrtc.PeerConnection
	channel   *webrtc.DataChannel
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

func openWebRTCClientTransport(server, token, deviceID, sessionID string, servers []iceServer) (controlTransport, error) {
	peer, err := webrtc.NewPeerConnection(webrtc.Configuration{ICEServers: pionIceServers(servers)})
	if err != nil {
		return nil, err
	}
	channel, err := peer.CreateDataChannel("rc-control", nil)
	if err != nil {
		_ = peer.Close()
		return nil, err
	}
	transport := &webrtcControlTransport{peer: peer, channel: channel, sessionID: sessionID,
		incoming: make(chan wireMessage, 128), closed: make(chan struct{})}
	opened := make(chan struct{})
	var openOnce sync.Once
	channel.OnOpen(func() { openOnce.Do(func() { close(opened) }) })
	channel.OnClose(transport.markClosed)
	channel.OnMessage(func(data webrtc.DataChannelMessage) {
		if len(data.Data) > maxControlFrameBytes {
			_ = channel.Close()
			return
		}
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
	var answer struct {
		SDP string `json:"sdp"`
	}
	if err = controlJSON(server, token, http.MethodPost, "/api/v1/control/"+url.PathEscape(sessionID)+"/webrtc",
		map[string]any{"deviceId": deviceID, "sdp": local.SDP}, &answer); err != nil {
		_ = peer.Close()
		return nil, err
	}
	sdp := answer.SDP
	if sdp == "" || peer.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: sdp}) != nil {
		_ = peer.Close()
		return nil, errors.New("WebRTC answer rejected")
	}
	select {
	case <-opened:
		return transport, nil
	case <-time.After(7 * time.Second):
		_ = peer.Close()
		return nil, errors.New("WebRTC connection timed out")
	}
}

func (transport *webrtcControlTransport) sendFrame(_, sessionID string, sequence uint64, ciphertext string) error {
	if sessionID != transport.sessionID {
		return errors.New("invalid control session")
	}
	if transport.channel.ReadyState() != webrtc.DataChannelStateOpen || transport.channel.BufferedAmount() > 1<<20 {
		return errors.New("WebRTC control unavailable")
	}
	data, _ := json.Marshal(wireMessage{Type: "control.frame", SessionID: sessionID, Sequence: sequence, Ciphertext: ciphertext})
	if err := transport.channel.SendText(string(data)); err != nil {
		transport.markClosed()
		_ = transport.peer.Close()
		return errors.New("WebRTC control unavailable")
	}
	return nil
}

func (transport *webrtcControlTransport) readFrame(sessionID string) (uint64, string, error) {
	if sessionID != transport.sessionID {
		return 0, "", errors.New("invalid control session")
	}
	select {
	case frame := <-transport.incoming:
		return frame.Sequence, frame.Ciphertext, nil
	case <-transport.closed:
		return 0, "", errors.New("WebRTC control closed")
	}
}

func (transport *webrtcControlTransport) markClosed() {
	transport.closeOnce.Do(func() { close(transport.closed) })
}

func (transport *webrtcControlTransport) close() error {
	err := transport.peer.Close()
	transport.markClosed()
	return err
}
