package node

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/OhRats-Technologies/rc/pkg/crypto"
	"github.com/OhRats-Technologies/rc/pkg/protocol"
)

type ControlSession struct {
	SessionID   string
	Key         [32]byte
	InboundSeq  atomic.Uint64
	OutboundSeq atomic.Uint64
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

type ControlManager struct {
	mu         sync.RWMutex
	state      *NodeState
	stateDir   string
	processes  ExecutionManager
	sessions   map[string]*ControlSession
	challenges map[string]string
}

func NewControlManager(state *NodeState, stateDir string, processes ExecutionManager) *ControlManager {
	return &ControlManager{
		state:      state,
		stateDir:   stateDir,
		processes:  processes,
		sessions:   make(map[string]*ControlSession),
		challenges: make(map[string]string),
	}
}

func (m *ControlManager) CreateChallenge(requestID string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	ch := crypto.SessionPayload(fmt.Sprintf("ch-%d", timeNowMicros()), m.state.DeviceID, requestID, "")
	m.challenges[requestID] = ch
	return ch
}

func (m *ControlManager) OpenSession(requestID, sessionID, clientPublicKey string) (*ControlSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	ch, ok := m.challenges[requestID]
	if !ok {
		return nil, errors.New("unknown challenge request")
	}
	delete(m.challenges, requestID)

	key, err := crypto.DeriveNodeKey(
		m.state.TransportSecret,
		m.state.TransportSecret,
		clientPublicKey,
		ch,
		m.state.DeviceID,
		requestID,
	)
	if err != nil {
		return nil, err
	}

	session := &ControlSession{
		SessionID: sessionID,
		Key:       key,
	}
	m.sessions[sessionID] = session
	return session, nil
}

func (m *ControlManager) CloseSession(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, sessionID)
	if m.processes != nil {
		m.processes.DetachSecureSession(sessionID)
	}
}

func (m *ControlManager) HandleRawFrame(sessionID string, frameBytes []byte) ([]byte, error) {
	if protocol.IsBinaryFrame(frameBytes) {
		binFrame, err := protocol.DecodeBinaryFrame(frameBytes)
		if err != nil {
			return nil, err
		}
		return m.HandleDirectFrame(sessionID, binFrame.Sequence, string(binFrame.Payload))
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

	plaintext, err := session.DecryptFrame(sequence, ciphertext)
	if err != nil {
		return nil, err
	}

	var msg protocol.ControlMessage
	if err := json.Unmarshal(plaintext, &msg); err != nil {
		return nil, err
	}

	return m.dispatchControlMessage(session, &msg)
}

func (m *ControlManager) dispatchControlMessage(session *ControlSession, msg *protocol.ControlMessage) ([]byte, error) {
	switch msg.Type {
	case protocol.TypeProcessStdin:
		if m.processes != nil {
			_ = m.processes.Input(msg.ID, []byte(msg.Data))
		}
	case protocol.TypeProcessStdinClose:
		if m.processes != nil {
			_ = m.processes.CloseInput(msg.ID)
		}
	case protocol.TypeProcessResize:
		if m.processes != nil {
			_ = m.processes.Resize(msg.ID, msg.Cols, msg.Rows)
		}
	case protocol.TypeProcessSignal:
		if m.processes != nil {
			_ = m.processes.Signal(msg.ID, msg.Signal)
		}
	case protocol.TypeProcessAttach:
		if m.processes != nil {
			m.processes.AttachSecure(msg.ID, session.SessionID)
		}
	}
	return nil, nil
}

func timeNowMicros() int64 {
	return time.Now().UnixMicro()
}
