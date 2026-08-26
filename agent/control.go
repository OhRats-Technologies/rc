package main

import (
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"time"
)

type controlSession struct {
	aead             cipher.AEAD
	send             func(wireMessage) bool
	transportID      string
	closeTransport   func()
	clientID         string
	userID           string
	role             string
	canExecute       bool
	canManageDevices bool
	recvSeq          uint64
	sendSeq          uint64
}

type controlManager struct {
	mu            sync.Mutex
	device        state
	stateDir      string
	serverURL     string
	processes     *processManager
	send          func(wireMessage) error
	challenges    map[string]time.Time
	sessions      map[string]*controlSession
	pendingStarts map[string]pendingSecureStart
}

func newControlManager(device state, stateDir, serverURL string, processes *processManager, send func(wireMessage) error) *controlManager {
	manager := &controlManager{device: device, stateDir: stateDir, serverURL: serverURL, processes: processes,
		send: send, challenges: map[string]time.Time{}, sessions: map[string]*controlSession{}, pendingStarts: map[string]pendingSecureStart{}}
	processes.setSecureSender(manager.sendFrame)
	return manager
}

func (manager *controlManager) challenge(requestID string) {
	challenge := randomURLBytes(32)
	manager.mu.Lock()
	manager.challenges[challenge] = time.Now().Add(60 * time.Second)
	manager.mu.Unlock()
	_ = manager.send(wireMessage{Type: "control.challenge", RequestID: requestID, Challenge: challenge})
}

func (manager *controlManager) consumeChallenge(value string) bool {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	expires, ok := manager.challenges[value]
	delete(manager.challenges, value)
	return ok && expires.After(time.Now())
}

func (manager *controlManager) open(message wireMessage) {
	if !manager.consumeChallenge(message.Challenge) {
		manager.controlError(message.RequestID, "control challenge expired")
		return
	}
	lock, err := loadLock(manager.stateDir)
	if err != nil {
		manager.controlError(message.RequestID, "RC Lock is not initialized")
		return
	}
	var snapshot authoritySnapshot
	if json.Unmarshal([]byte(lock.Snapshot), &snapshot) != nil {
		manager.controlError(message.RequestID, "invalid RC Lock state")
		return
	}
	var userID, role, signingPublicKey string
	var canExecute, canManageDevices bool
	if message.Grant != "" {
		proof := controlProof{Grant: message.Grant, CredentialID: message.CredentialID, Assertion: message.Assertion}
		grant, grantRole, grantErr := verifyControlProof(snapshot, proof, lock.Origin, lock.RPID)
		if grantErr != nil || grant.ClientID != message.ClientID {
			manager.controlError(message.RequestID, "control grant rejected")
			return
		}
		userID, role, signingPublicKey = grant.UserID, grantRole, grant.SigningPublicKey
		canExecute, canManageDevices = role != "viewer", role == "owner"
	} else {
		apiKey, member := apiAuthority(snapshot, message.ClientID)
		if apiKey == nil || member == nil || member.Role == "viewer" {
			manager.controlError(message.RequestID, "API control key rejected")
			return
		}
		userID, role, signingPublicKey = apiKey.UserID, member.Role, apiKey.PublicKey
		canExecute = hasScope(apiKey.Scopes, "execute")
		canManageDevices = member.Role == "owner" && hasScope(apiKey.Scopes, "manage-devices")
		if !canExecute && !canManageDevices {
			manager.controlError(message.RequestID, "API key lacks control scope")
			return
		}
	}
	clientKey, err := base64.RawURLEncoding.DecodeString(signingPublicKey)
	signature, sigErr := base64.RawURLEncoding.DecodeString(message.Signature)
	if err != nil || sigErr != nil || len(clientKey) != ed25519.PublicKeySize ||
		!ed25519.Verify(ed25519.PublicKey(clientKey), []byte(sessionPayload(message.Challenge, manager.device.DeviceID, message.ClientID, message.PublicKey)), signature) {
		manager.controlError(message.RequestID, "invalid control client signature")
		return
	}
	ephemeralPrivate, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		manager.controlError(message.RequestID, "control ephemeral key generation failed")
		return
	}
	ephemeralPrivateEncoded := base64.RawURLEncoding.EncodeToString(ephemeralPrivate.Bytes())
	ephemeralPublicEncoded := base64.RawURLEncoding.EncodeToString(ephemeralPrivate.PublicKey().Bytes())
	aead, err := deriveNodeAEAD(manager.device.TransportPrivateKey, ephemeralPrivateEncoded, message.PublicKey,
		message.Challenge, manager.device.DeviceID, message.ClientID)
	if err != nil {
		manager.controlError(message.RequestID, "control key agreement failed")
		return
	}
	sessionID := randomURLBytes(18)
	manager.mu.Lock()
	manager.sessions[sessionID] = &controlSession{aead: aead, send: manager.relayFrame, transportID: "relay", clientID: message.ClientID, userID: userID, role: role,
		canExecute: canExecute, canManageDevices: canManageDevices}
	manager.mu.Unlock()
	privateBytes, _ := base64.RawStdEncoding.DecodeString(manager.device.PrivateKey)
	ready := readyPayload(message.Challenge, manager.device.DeviceID, message.ClientID, message.PublicKey,
		manager.device.TransportPublicKey, ephemeralPublicEncoded, sessionID)
	deviceSignature := ed25519.Sign(ed25519.PrivateKey(privateBytes), []byte(ready))
	_ = manager.send(wireMessage{Type: "control.ready", RequestID: message.RequestID, SessionID: sessionID,
		TransportPublicKey: manager.device.TransportPublicKey, EphemeralPublicKey: ephemeralPublicEncoded,
		Signature: base64.RawURLEncoding.EncodeToString(deviceSignature)})
}

func (manager *controlManager) controlError(requestID, message string) {
	_ = manager.send(wireMessage{Type: "control.error", RequestID: requestID, Output: message})
}

func (manager *controlManager) receiveFrame(message wireMessage) error {
	manager.mu.Lock()
	session := manager.sessions[message.SessionID]
	if session == nil || message.Sequence != session.recvSeq+1 {
		manager.mu.Unlock()
		return errors.New("invalid control frame sequence")
	}
	ciphertext, err := base64.RawURLEncoding.DecodeString(message.Ciphertext)
	if err != nil {
		manager.mu.Unlock()
		return err
	}
	plaintext, err := session.aead.Open(nil, frameNonce(1, message.Sequence), ciphertext, frameAAD(message.SessionID, message.Sequence, "c2n"))
	if err != nil {
		manager.mu.Unlock()
		return errors.New("control frame authentication failed")
	}
	session.recvSeq = message.Sequence
	userID, role, canExecute, canManageDevices := session.userID, session.role, session.canExecute, session.canManageDevices
	manager.mu.Unlock()
	var command wireMessage
	if json.Unmarshal(plaintext, &command) != nil {
		return errors.New("invalid encrypted control command")
	}
	if command.Type == "node.remove" {
		if !canManageDevices {
			return errors.New("owner required")
		}
		manager.sendFrame(message.SessionID, wireMessage{Type: "control.result", RequestID: command.RequestID, Output: "ok"})
		manager.processes.shutdown()
		_ = os.Remove(lockPath(manager.stateDir))
		_ = os.Remove(statePath(manager.stateDir))
		return errNodeRemoved
	}
	if command.Type == "node.update" {
		if !canManageDevices {
			return errors.New("owner required")
		}
		go manager.authorizedUpdate(message.SessionID, command.RequestID)
		return nil
	}
	if !canExecute {
		return errors.New("execute scope required")
	}
	if command.Type == "process.start" {
		manager.queueSecureStart(message.SessionID, userID, command)
		return nil
	}
	manager.processes.secureHandle(message.SessionID, userID, role, command)
	return nil
}

func (manager *controlManager) authorizedUpdate(sessionID, requestID string) {
	if err := replaceExecutable(manager.serverURL); err != nil {
		manager.sendFrame(sessionID, wireMessage{Type: "control.result", RequestID: requestID, Output: err.Error()})
		return
	}
	manager.sendFrame(sessionID, wireMessage{Type: "control.result", RequestID: requestID, Output: "ok"})
	manager.processes.shutdown()
	_ = manager.send(wireMessage{Type: "node.update.ready", AgentVersion: version})
	_ = syscallExecCurrent()
}

func (manager *controlManager) sendFrame(sessionID string, message wireMessage) bool {
	if message.Type == "process.started" {
		_ = manager.send(wireMessage{Type: "process.started", ID: message.ID})
	}
	if message.Type == "process.exit" {
		_ = manager.send(wireMessage{Type: "process.exit", ID: message.ID, ExitCode: message.ExitCode, Signal: message.Signal})
	}
	manager.mu.Lock()
	session := manager.sessions[sessionID]
	if session == nil {
		manager.mu.Unlock()
		return false
	}
	session.sendSeq++
	sequence := session.sendSeq
	plaintext, _ := json.Marshal(message)
	ciphertext := session.aead.Seal(nil, frameNonce(2, sequence), plaintext, frameAAD(sessionID, sequence, "n2c"))
	send := session.send
	manager.mu.Unlock()
	return send != nil && send(wireMessage{Type: "control.frame", SessionID: sessionID, Sequence: sequence,
		Ciphertext: base64.RawURLEncoding.EncodeToString(ciphertext)})
}

func (manager *controlManager) relayFrame(message wireMessage) bool {
	return manager.send(message) == nil
}

func (manager *controlManager) handle(message wireMessage) error {
	switch message.Type {
	case "lock.bootstrap":
		if err := bootstrapLock(manager.stateDir, message.Snapshot, manager.serverURL); err != nil {
			return err
		}
		return manager.sendLockState()
	case "lock.sync":
		if err := syncLock(manager.stateDir, message.Snapshot, message.PreviousHash, message.PreviousGeneration,
			controlProof{Grant: message.Grant, CredentialID: message.CredentialID, Assertion: message.Assertion}, message.Signature); err != nil {
			return err
		}
		manager.invalidateSessions()
		return manager.sendLockState()
	case "control.challenge":
		manager.challenge(message.RequestID)
	case "control.open":
		manager.open(message)
	case "control.webrtc":
		manager.openWebRTC(message)
	case "control.frame":
		return manager.receiveFrame(message)
	case "process.permit":
		manager.permitSecureStart(message)
	case "control.close":
		manager.closeSession(message.SessionID)
	}
	return nil
}

func (manager *controlManager) invalidateSessions() {
	manager.mu.Lock()
	ids := make([]string, 0, len(manager.sessions))
	for sessionID := range manager.sessions {
		ids = append(ids, sessionID)
	}
	manager.mu.Unlock()
	for _, sessionID := range ids {
		manager.sendFrame(sessionID, wireMessage{Type: "control.revoked"})
	}
	for _, sessionID := range ids {
		manager.closeSession(sessionID)
	}
}

func (manager *controlManager) sendLockState() error {
	state, _ := loadLock(manager.stateDir)
	return manager.send(wireMessage{Type: "lock.state", LockHash: lockHash(manager.stateDir), LockGeneration: state.Generation})
}
