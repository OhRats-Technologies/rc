package node

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/OhRats-Technologies/rc/pkg/crypto"
	"github.com/OhRats-Technologies/rc/pkg/protocol"
	"github.com/OhRats-Technologies/rc/pkg/transport"
	"github.com/pion/webrtc/v4"
)

type sessionAuthority struct {
	UserID     string
	Role       string
	PublicKey  string
	CanExecute bool
	CanManage  bool
}

type ControlReady struct {
	SessionID          string                       `json:"sessionId"`
	TransportPublicKey string                       `json:"transportPublicKey"`
	EphemeralPublicKey string                       `json:"ephemeralPublicKey"`
	Signature          string                       `json:"signature"`
	Attempts           []protocol.ControlIceAttempt `json:"attempts"`
}

func controlSessionAuthority(stateDir string, message protocol.ServerToNode) (*sessionAuthority, error) {
	if message.ControlGrant == "" {
		authority, err := VerifyApiControlAuthority(stateDir, message.ClientID)
		if err != nil {
			return nil, err
		}
		if authority.UserID != message.UserID {
			return nil, errors.New("control user mismatch")
		}
		return &sessionAuthority{
			UserID: authority.UserID, Role: authority.Role, PublicKey: authority.PublicKey,
			CanExecute: authority.CanExecute, CanManage: authority.CanManageDevices,
		}, nil
	}
	proof := &protocol.ControlProof{
		Grant: message.ControlGrant, CredentialID: message.CredentialID,
		Assertion: message.ControlAssertion,
	}
	authority, err := HostedControlAuthority(stateDir, proof, message.UserID)
	if err != nil {
		return nil, err
	}
	if authority.Grant.ClientID != message.ClientID {
		return nil, errors.New("control client mismatch")
	}
	return &sessionAuthority{
		UserID: authority.Grant.UserID, Role: authority.Role,
		PublicKey:  authority.Grant.SigningPublicKey,
		CanExecute: authority.Role != "viewer", CanManage: authority.Role == "owner",
	}, nil
}

func (m *ControlManager) OpenAuthorizedSession(
	challenge, clientID, clientPublicKey, signature string,
	authority *sessionAuthority, iceServers []protocol.IceServer,
) (*ControlReady, error) {
	if !m.consumeChallenge(challenge) {
		return nil, errors.New("control challenge expired")
	}
	if _, err := crypto.DecodeX25519(clientPublicKey); err != nil {
		return nil, errors.New("invalid control transport key")
	}
	payload := crypto.SessionPayload(challenge, m.state.DeviceID, clientID, clientPublicKey)
	if err := crypto.VerifyEd25519(authority.PublicKey, []byte(payload), signature); err != nil {
		return nil, errors.New("invalid control client signature")
	}
	ephemeralPrivate := randomControlValue(32)
	ephemeralPublic, err := crypto.X25519Public(ephemeralPrivate)
	if err != nil {
		return nil, err
	}
	key, err := crypto.DeriveNodeKey(
		m.state.TransportSecret, ephemeralPrivate, clientPublicKey,
		challenge, m.state.DeviceID, clientID,
	)
	if err != nil {
		return nil, err
	}
	transportPublicKey, err := m.state.TransportPublicKey()
	if err != nil {
		return nil, err
	}
	sessionID := "sess_" + randomControlValue(18)
	attempts := controlAttempts(iceServers)
	readyPayload := crypto.ReadyPayload(
		challenge, m.state.DeviceID, clientID, clientPublicKey,
		transportPublicKey, ephemeralPublic, sessionID,
		protocol.ControlAttemptsPayload(attempts),
	)
	nodeSignature, err := crypto.SignEd25519Seed(m.state.IdentitySeed, []byte(readyPayload))
	if err != nil {
		return nil, err
	}
	session := &ControlSession{
		SessionID: sessionID, Key: key,
		UserID: authority.UserID, Role: authority.Role,
		CanExecute: authority.CanExecute, CanManage: authority.CanManage,
	}
	m.mu.Lock()
	m.sessions[sessionID] = session
	m.mu.Unlock()
	return &ControlReady{
		SessionID: sessionID, TransportPublicKey: transportPublicKey,
		EphemeralPublicKey: ephemeralPublic, Signature: nodeSignature,
		Attempts: attempts,
	}, nil
}

func controlAttempts(iceServers []protocol.IceServer) []protocol.ControlIceAttempt {
	attempts := []protocol.ControlIceAttempt{
		{Mode: protocol.ControlIceModeHost, Route: protocol.ControlRouteClassDirectHost,
			GatherTimeoutMs: 5000, ConnectTimeoutMs: 10000, RetryDelayMs: 500},
		{Mode: protocol.ControlIceModeStun, Route: protocol.ControlRouteClassDirectStun,
			GatherTimeoutMs: 8000, ConnectTimeoutMs: 12000, RetryDelayMs: 750},
	}
	for _, server := range iceServers {
		for _, url := range server.URLs {
			if strings.HasPrefix(strings.ToLower(url), "turn:") ||
				strings.HasPrefix(strings.ToLower(url), "turns:") {
				attempts = append(attempts, protocol.ControlIceAttempt{
					Mode: protocol.ControlIceModeRelay, Route: protocol.ControlRouteClassTurnRelay,
					GatherTimeoutMs: 10000, ConnectTimeoutMs: 15000, RetryDelayMs: 1000,
				})
				return attempts
			}
		}
	}
	return attempts
}

func (m *ControlManager) AnswerWebRTC(sessionID, sdp string, iceServers []protocol.IceServer) (string, error) {
	if sdp == "" || len(sdp) > 131072 {
		return "", errors.New("invalid WebRTC offer")
	}
	m.mu.RLock()
	session, ok := m.sessions[sessionID]
	m.mu.RUnlock()
	if !ok {
		return "", errors.New("control session unavailable")
	}
	peer, err := transport.NewPeer(transportIceServers(iceServers))
	if err != nil {
		return "", errors.New("WebRTC unavailable")
	}
	answer, err := peer.AcceptOffer(sdp)
	if err != nil {
		peer.Close()
		return "", errors.New("WebRTC negotiation failed")
	}
	session.setPeer(peer)
	go m.readTransport(sessionID, peer)
	return answer, nil
}

func (m *ControlManager) readTransport(sessionID string, peer *transport.Peer) {
	for {
		message, err := peer.Receive(time.Second)
		if err != nil {
			if !m.hasPeer(sessionID, peer) {
				return
			}
			continue
		}
		var frame protocol.ControlTransportMessage
		if json.Unmarshal(message, &frame) != nil || frame.Type != protocol.TypeControlFrame ||
			frame.SessionID != sessionID || frame.Sequence == 0 {
			m.CloseSession(sessionID)
			return
		}
		if _, err := m.HandleDirectFrame(sessionID, frame.Sequence, frame.Ciphertext); err != nil {
			return
		}
	}
}

func (m *ControlManager) hasPeer(sessionID string, peer *transport.Peer) bool {
	m.mu.RLock()
	session, ok := m.sessions[sessionID]
	m.mu.RUnlock()
	return ok && session.currentPeer() == peer
}

func transportIceServers(values []protocol.IceServer) []webrtc.ICEServer {
	servers := make([]webrtc.ICEServer, 0, len(values))
	for _, value := range values {
		servers = append(servers, webrtc.ICEServer{
			URLs: value.URLs, Username: value.Username, Credential: value.Credential,
		})
	}
	return servers
}
