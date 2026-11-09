package server

import (
	"sync"

	"github.com/OhRats-Technologies/rc/pkg/protocol"
)

type controlSession struct {
	UserID     string
	ClientID   string
	DeviceID   string
	IceServers []protocol.IceServer
}

type controlHub struct {
	mu       sync.Mutex
	nodes    *NodeHub
	turn     *turnProvider
	sessions map[string]controlSession
}

func newControlHub(nodes *NodeHub, turn *turnProvider) *controlHub {
	return &controlHub{
		nodes: nodes, turn: turn, sessions: make(map[string]controlSession),
	}
}

func (h *controlHub) insert(sessionID string, session controlSession) {
	h.mu.Lock()
	h.sessions[sessionID] = session
	h.mu.Unlock()
}

func (h *controlHub) get(sessionID string) (controlSession, bool) {
	h.mu.Lock()
	session, ok := h.sessions[sessionID]
	h.mu.Unlock()
	return session, ok
}

func (h *controlHub) remove(sessionID string) {
	h.mu.Lock()
	delete(h.sessions, sessionID)
	h.mu.Unlock()
}

func (h *controlHub) releaseDevice(deviceID string) {
	h.mu.Lock()
	for sessionID, session := range h.sessions {
		if session.DeviceID == deviceID {
			delete(h.sessions, sessionID)
		}
	}
	h.mu.Unlock()
}
