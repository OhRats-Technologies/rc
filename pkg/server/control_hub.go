package server

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/OhRats-Technologies/rc/pkg/protocol"
	"golang.org/x/net/websocket"
)

// controlPipeQueue bounds frames waiting for one client WebSocket. A full
// queue fails the session closed: encrypted control frames are ordered and a
// silent drop would desynchronize the sequence.
const controlPipeQueue = 32

type controlSession struct {
	UserID   string
	ClientID string
	DeviceID string
}

// controlPipe relays ciphertext for one browser/CLI control session. The
// server never holds session keys; it only forwards frames between the client
// WebSocket and the authenticated Node WebSocket.
type controlPipe struct {
	conn *websocket.Conn
	out  chan protocol.ControlTransportMessage
	done chan struct{}
	once sync.Once
}

func newControlPipe(conn *websocket.Conn) *controlPipe {
	return &controlPipe{
		conn: conn,
		out:  make(chan protocol.ControlTransportMessage, controlPipeQueue),
		done: make(chan struct{}),
	}
}

func (p *controlPipe) run() {
	for {
		select {
		case <-p.done:
			return
		case frame := <-p.out:
			encoded, err := json.Marshal(frame)
			if err != nil {
				p.close()
				return
			}
			_ = p.conn.SetWriteDeadline(time.Now().Add(20 * time.Second))
			// Text frames: the browser client requires string event.data, and
			// a []byte payload would be delivered as a binary Blob.
			if err := websocket.Message.Send(p.conn, string(encoded)); err != nil {
				p.close()
				return
			}
		}
	}
}

func (p *controlPipe) enqueue(frame protocol.ControlTransportMessage) bool {
	select {
	case <-p.done:
		return false
	default:
	}
	select {
	case p.out <- frame:
		return true
	default:
		return false
	}
}

func (p *controlPipe) close() {
	p.once.Do(func() {
		close(p.done)
		_ = p.conn.Close()
	})
}

type controlHub struct {
	mu       sync.Mutex
	sessions map[string]controlSession
	pipes    map[string]*controlPipe
}

func newControlHub() *controlHub {
	return &controlHub{
		sessions: make(map[string]controlSession),
		pipes:    make(map[string]*controlPipe),
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
	pipe := h.pipes[sessionID]
	delete(h.pipes, sessionID)
	h.mu.Unlock()
	if pipe != nil {
		pipe.close()
	}
}

func (h *controlHub) releaseDevice(deviceID string) {
	h.mu.Lock()
	var pipes []*controlPipe
	for sessionID, session := range h.sessions {
		if session.DeviceID != deviceID {
			continue
		}
		delete(h.sessions, sessionID)
		if pipe := h.pipes[sessionID]; pipe != nil {
			pipes = append(pipes, pipe)
			delete(h.pipes, sessionID)
		}
	}
	h.mu.Unlock()
	for _, pipe := range pipes {
		pipe.close()
	}
}

// attach binds one client pipe to a live session. A newer pipe replaces an
// older one so a reconnecting client cannot leave a stale writer behind.
func (h *controlHub) attach(sessionID string, pipe *controlPipe) bool {
	h.mu.Lock()
	if _, ok := h.sessions[sessionID]; !ok {
		h.mu.Unlock()
		return false
	}
	old := h.pipes[sessionID]
	h.pipes[sessionID] = pipe
	h.mu.Unlock()
	if old != nil {
		old.close()
	}
	return true
}

func (h *controlHub) detach(sessionID string, pipe *controlPipe) {
	h.mu.Lock()
	if current := h.pipes[sessionID]; current == pipe {
		delete(h.pipes, sessionID)
	}
	h.mu.Unlock()
	pipe.close()
}

// wire connects Node control frames to their client pipes.
func (h *controlHub) wire(nodes *NodeHub) {
	nodes.onControlFrame = func(deviceID string, message protocol.NodeToServer) {
		h.deliver(deviceID, message.SessionID, protocol.ControlTransportMessage{
			Type: protocol.TypeControlFrame, SessionID: message.SessionID,
			Sequence: message.Sequence, Ciphertext: message.Ciphertext,
		})
	}
}

// deliver forwards one Node frame to the client. It reports false when the
// session belongs to another device, no live pipe exists, or the pipe backlog
// overflowed.
func (h *controlHub) deliver(deviceID, sessionID string, frame protocol.ControlTransportMessage) bool {
	h.mu.Lock()
	session, ok := h.sessions[sessionID]
	pipe := h.pipes[sessionID]
	h.mu.Unlock()
	if !ok || session.DeviceID != deviceID || pipe == nil {
		return false
	}
	if pipe.enqueue(frame) {
		return true
	}
	h.remove(sessionID)
	return false
}
