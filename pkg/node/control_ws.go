package node

import (
	"errors"

	"github.com/OhRats-Technologies/rc/pkg/crypto"
	"github.com/OhRats-Technologies/rc/pkg/protocol"
)

type sessionAuthority struct {
	UserID     string
	Role       string
	PublicKey  string
	CanExecute bool
	CanManage  bool
}

type ControlReady struct {
	SessionID          string `json:"sessionId"`
	TransportPublicKey string `json:"transportPublicKey"`
	EphemeralPublicKey string `json:"ephemeralPublicKey"`
	Signature          string `json:"signature"`
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
	authority *sessionAuthority, emit func(protocol.NodeToServer) error,
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
	readyPayload := crypto.ReadyPayload(
		challenge, m.state.DeviceID, clientID, clientPublicKey,
		transportPublicKey, ephemeralPublic, sessionID,
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
	session.setEmitter(emit)
	m.mu.Lock()
	m.sessions[sessionID] = session
	m.mu.Unlock()
	return &ControlReady{
		SessionID: sessionID, TransportPublicKey: transportPublicKey,
		EphemeralPublicKey: ephemeralPublic, Signature: nodeSignature,
	}, nil
}
