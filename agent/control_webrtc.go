package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/pion/webrtc/v4"
)

func pionIceServers(values []iceServer) []webrtc.ICEServer {
	servers := make([]webrtc.ICEServer, 0, len(values))
	for _, value := range values {
		if len(value.URLs) == 0 {
			continue
		}
		servers = append(servers, webrtc.ICEServer{URLs: value.URLs, Username: value.Username, Credential: value.Credential})
	}
	return servers
}

func (manager *controlManager) openWebRTC(message wireMessage) { go manager.answerWebRTC(message) }

func (manager *controlManager) answerWebRTC(message wireMessage) {
	fmt.Printf("WebRTC offer received for control session %s\n", shortControlID(message.SessionID))
	if len(message.SDP) == 0 || len(message.SDP) > 131072 || len(message.IceServers) > 8 {
		manager.controlError(message.RequestID, "invalid WebRTC offer")
		return
	}
	manager.mu.Lock()
	session := manager.sessions[message.SessionID]
	manager.mu.Unlock()
	if session == nil {
		manager.controlError(message.RequestID, "control session unavailable")
		return
	}
	peer, err := webrtc.NewPeerConnection(webrtc.Configuration{ICEServers: pionIceServers(message.IceServers)})
	if err != nil {
		manager.controlError(message.RequestID, "WebRTC unavailable")
		return
	}
	transportID := randomURLBytes(12)
	manager.registerWebRTCPeer(message.SessionID, transportID, peer)
	peer.OnDataChannel(func(channel *webrtc.DataChannel) {
		if channel.Label() != "rc-control" {
			_ = channel.Close()
			return
		}
		channel.OnOpen(func() { manager.bindWebRTC(message.SessionID, transportID, channel) })
		channel.OnMessage(func(data webrtc.DataChannelMessage) {
			if len(data.Data) > maxControlFrameBytes {
				_ = channel.Close()
				return
			}
			var frame wireMessage
			if json.Unmarshal(data.Data, &frame) != nil || frame.Type != "control.frame" || frame.SessionID != message.SessionID {
				_ = channel.Close()
				return
			}
			if manager.receiveFrame(frame) != nil {
				_ = channel.Close()
			}
		})
		channel.OnClose(func() { manager.webRTCClosed(message.SessionID, transportID) })
	})
	if err = peer.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: message.SDP}); err != nil {
		manager.failWebRTC(message.SessionID, transportID, peer, message.RequestID)
		return
	}
	answer, err := peer.CreateAnswer(nil)
	if err != nil {
		manager.failWebRTC(message.SessionID, transportID, peer, message.RequestID)
		return
	}
	gathered := webrtc.GatheringCompletePromise(peer)
	if err = peer.SetLocalDescription(answer); err != nil {
		manager.failWebRTC(message.SessionID, transportID, peer, message.RequestID)
		return
	}
	select {
	case <-gathered:
	case <-time.After(5 * time.Second):
		manager.failWebRTC(message.SessionID, transportID, peer, message.RequestID)
		return
	}
	local := peer.LocalDescription()
	if local == nil || local.SDP == "" {
		manager.failWebRTC(message.SessionID, transportID, peer, message.RequestID)
		return
	}
	if manager.send(wireMessage{Type: "control.webrtc.ready", RequestID: message.RequestID, SessionID: message.SessionID, SDP: local.SDP}) != nil {
		manager.clearWebRTCPeer(message.SessionID, transportID)
		_ = peer.Close()
		return
	}
	time.AfterFunc(10*time.Second, func() {
		if peer.ConnectionState() != webrtc.PeerConnectionStateConnected {
			manager.clearWebRTCPeer(message.SessionID, transportID)
			_ = peer.Close()
		}
	})
}

func (manager *controlManager) registerWebRTCPeer(sessionID, transportID string, peer *webrtc.PeerConnection) {
	manager.mu.Lock()
	session := manager.sessions[sessionID]
	var previous func()
	if session != nil {
		previous = session.closeTransport
		session.send = nil
		session.transportID = transportID
		session.closeTransport = func() { _ = peer.Close() }
	}
	manager.mu.Unlock()
	if previous != nil {
		previous()
	}
}

func (manager *controlManager) bindWebRTC(sessionID, transportID string, channel *webrtc.DataChannel) {
	manager.mu.Lock()
	session := manager.sessions[sessionID]
	valid := session != nil && session.transportID == transportID
	if valid {
		session.send = func(message wireMessage) bool {
			data, err := json.Marshal(message)
			if err == nil && channel.BufferedAmount() <= 1<<20 && channel.SendText(string(data)) == nil {
				return true
			}
			_ = channel.Close()
			return false
		}
	}
	manager.mu.Unlock()
	if !valid {
		_ = channel.Close()
		return
	}
	fmt.Printf("WebRTC connected for control session %s\n", shortControlID(sessionID))
}

func (manager *controlManager) clearWebRTCPeer(sessionID, transportID string) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	session := manager.sessions[sessionID]
	if session == nil || session.transportID != transportID {
		return
	}
	session.send = nil
	session.transportID = ""
	session.closeTransport = nil
}

func (manager *controlManager) webRTCClosed(sessionID, transportID string) {
	manager.mu.Lock()
	session := manager.sessions[sessionID]
	established := session != nil && session.transportID == transportID && session.send != nil
	if session != nil && session.transportID == transportID && !established {
		session.transportID = ""
		session.closeTransport = nil
	}
	manager.mu.Unlock()
	if established {
		fmt.Printf("WebRTC closed for control session %s\n", shortControlID(sessionID))
		manager.closeSession(sessionID)
	}
}

func (manager *controlManager) failWebRTC(sessionID, transportID string, peer *webrtc.PeerConnection, requestID string) {
	manager.clearWebRTCPeer(sessionID, transportID)
	_ = peer.Close()
	manager.controlError(requestID, "WebRTC negotiation failed")
}

func shortControlID(value string) string {
	if len(value) <= 8 {
		return value
	}
	return value[:8]
}

func (manager *controlManager) closeSession(sessionID string) {
	manager.mu.Lock()
	session := manager.sessions[sessionID]
	if session != nil {
		delete(manager.sessions, sessionID)
	}
	manager.mu.Unlock()
	if session == nil {
		return
	}
	if session.closeTransport != nil {
		session.closeTransport()
	}
	manager.processes.detachSecureSession(sessionID)
	manager.discardPendingSession(sessionID)
	_ = manager.send(wireMessage{Type: "control.closed", SessionID: sessionID})
}
