package node

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/OhRats-Technologies/rc/pkg/crypto"
	"github.com/OhRats-Technologies/rc/pkg/protocol"
	"github.com/OhRats-Technologies/rc/pkg/transport"
)

type challengeEntry struct {
	value   string
	expires time.Time
}

type ControlSession struct {
	SessionID   string
	Key         [32]byte
	InboundSeq  atomic.Uint64
	OutboundSeq atomic.Uint64
	UserID      string
	Role        string
	CanExecute  bool
	CanManage   bool

	seqMu       sync.Mutex
	transportMu sync.Mutex
	peer        *transport.Peer
}

func (s *ControlSession) DecryptFrame(sequence uint64, ciphertext string) ([]byte, error) {
	return crypto.DecryptFrame(&s.Key, 1, sequence, s.SessionID, "c2n", ciphertext)
}

func (s *ControlSession) EncryptFrame(plaintext []byte) (string, uint64, error) {
	seq := s.OutboundSeq.Add(1)
	ct, err := crypto.EncryptFrame(&s.Key, 2, seq, s.SessionID, "n2c", plaintext)
	if err != nil {
		return "", 0, err
	}
	return ct, seq, nil
}

func (s *ControlSession) acceptSequence(sequence uint64) bool {
	s.seqMu.Lock()
	defer s.seqMu.Unlock()
	if sequence != s.InboundSeq.Load()+1 {
		return false
	}
	s.InboundSeq.Store(sequence)
	return true
}

func (s *ControlSession) setPeer(peer *transport.Peer) {
	s.transportMu.Lock()
	s.peer = peer
	s.transportMu.Unlock()
}

func (s *ControlSession) currentPeer() *transport.Peer {
	s.transportMu.Lock()
	defer s.transportMu.Unlock()
	return s.peer
}

type ControlManager struct {
	mu              sync.RWMutex
	state           *NodeState
	stateDir        string
	processes       ExecutionManager
	sessions        map[string]*ControlSession
	challenges      map[string]challengeEntry
	challengeValues map[string]time.Time
	onProcessState  func(protocol.NodeToServer)
}

func NewControlManager(state *NodeState, stateDir string, processes ExecutionManager) *ControlManager {
	return &ControlManager{
		state:           state,
		stateDir:        stateDir,
		processes:       processes,
		sessions:        make(map[string]*ControlSession),
		challenges:      make(map[string]challengeEntry),
		challengeValues: make(map[string]time.Time),
	}
}

func (m *ControlManager) SetProcessStateHandler(handler func(protocol.NodeToServer)) {
	m.mu.Lock()
	m.onProcessState = handler
	m.mu.Unlock()
}

func (m *ControlManager) publishProcessState(message protocol.NodeToServer) {
	m.mu.RLock()
	handler := m.onProcessState
	m.mu.RUnlock()
	if handler != nil {
		handler(message)
	}
}

func (m *ControlManager) CreateChallenge(requestID string) string {
	value := randomControlValue(32)
	expires := time.Now().Add(time.Minute)
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, entry := range m.challenges {
		if entry.expires.Before(time.Now()) {
			delete(m.challenges, key)
			delete(m.challengeValues, entry.value)
		}
	}
	m.challenges[requestID] = challengeEntry{value: value, expires: expires}
	m.challengeValues[value] = expires
	return value
}

func (m *ControlManager) consumeChallenge(challenge string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	expires, ok := m.challengeValues[challenge]
	if !ok || expires.Before(time.Now()) {
		delete(m.challengeValues, challenge)
		return false
	}
	delete(m.challengeValues, challenge)
	return true
}

// OpenSession is the legacy test helper for the static-key handshake. Live
// browser/CLI sessions use OpenAuthorizedSession.
func (m *ControlManager) OpenSession(requestID, sessionID, clientPublicKey string) (*ControlSession, error) {
	m.mu.Lock()
	entry, ok := m.challenges[requestID]
	if ok {
		delete(m.challenges, requestID)
		delete(m.challengeValues, entry.value)
	}
	m.mu.Unlock()
	if !ok {
		return nil, errors.New("unknown challenge request")
	}
	key, err := crypto.DeriveNodeKey(
		m.state.TransportSecret, m.state.TransportSecret, clientPublicKey,
		entry.value, m.state.DeviceID, requestID,
	)
	if err != nil {
		return nil, err
	}
	session := &ControlSession{SessionID: sessionID, Key: key, CanExecute: true, CanManage: true}
	m.mu.Lock()
	m.sessions[sessionID] = session
	m.mu.Unlock()
	return session, nil
}

func (m *ControlManager) CloseSession(sessionID string) {
	m.mu.Lock()
	session, ok := m.sessions[sessionID]
	if ok {
		delete(m.sessions, sessionID)
	}
	m.mu.Unlock()
	if !ok {
		return
	}
	if peer := session.currentPeer(); peer != nil {
		peer.Close()
	}
	if m.processes != nil {
		m.processes.DetachSecureSession(sessionID)
	}
}

// CloseAllSessions invalidates every live encrypted control channel after the
// Node accepts a new RC Lock generation.
func (m *ControlManager) CloseAllSessions() []string {
	m.mu.Lock()
	sessions := m.sessions
	m.sessions = make(map[string]*ControlSession)
	m.mu.Unlock()
	closed := make([]string, 0, len(sessions))
	for sessionID, session := range sessions {
		closed = append(closed, sessionID)
		if peer := session.currentPeer(); peer != nil {
			peer.Close()
		}
		if m.processes != nil {
			m.processes.DetachSecureSession(sessionID)
		}
	}
	return closed
}

func (m *ControlManager) HandleRawFrame(sessionID string, frameBytes []byte) ([]byte, error) {
	if protocol.IsBinaryFrame(frameBytes) {
		frame, err := protocol.DecodeBinaryFrame(frameBytes)
		if err != nil {
			return nil, err
		}
		return m.HandleDirectFrame(sessionID, frame.Sequence, string(frame.Payload))
	}
	var frame struct {
		Sequence   uint64 `json:"seq"`
		Ciphertext string `json:"ct"`
	}
	if err := json.Unmarshal(frameBytes, &frame); err != nil {
		return nil, err
	}
	return m.HandleDirectFrame(sessionID, frame.Sequence, frame.Ciphertext)
}

func (m *ControlManager) HandleDirectFrame(sessionID string, sequence uint64, ciphertext string) ([]byte, error) {
	m.mu.RLock()
	session, ok := m.sessions[sessionID]
	m.mu.RUnlock()
	if !ok {
		return nil, errors.New("unknown control session")
	}
	if !session.acceptSequence(sequence) {
		m.CloseSession(sessionID)
		return nil, errors.New("invalid control frame sequence")
	}
	plaintext, err := session.DecryptFrame(sequence, ciphertext)
	if err != nil {
		m.CloseSession(sessionID)
		return nil, err
	}
	var msg protocol.ControlMessage
	if err := json.Unmarshal(plaintext, &msg); err != nil {
		m.CloseSession(sessionID)
		return nil, err
	}
	return m.dispatchControlMessage(session, &msg)
}

func (m *ControlManager) sendFrame(sessionID string, message protocol.ControlMessage) bool {
	m.mu.RLock()
	session, ok := m.sessions[sessionID]
	m.mu.RUnlock()
	if !ok || session == nil {
		return false
	}
	plaintext, err := json.Marshal(message)
	if err != nil || len(plaintext) > protocol.NodeControlMessageLimit {
		return false
	}
	ciphertext, sequence, err := session.EncryptFrame(plaintext)
	if err != nil {
		return false
	}
	peer := session.currentPeer()
	if peer == nil {
		return false
	}
	frame, err := json.Marshal(protocol.ControlTransportMessage{
		Type: protocol.TypeControlFrame, SessionID: sessionID,
		Sequence: sequence, Ciphertext: ciphertext,
	})
	if err != nil {
		return false
	}
	return peer.Send(frame) == nil
}

func randomControlValue(size int) string {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return ""
	}
	return b64URL.EncodeToString(value)
}

func timeNowMicros() int64 {
	return time.Now().UnixMicro()
}
