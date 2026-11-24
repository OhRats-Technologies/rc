package server

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/OhRats-Technologies/rc/pkg/protocol"
	"golang.org/x/net/websocket"
)

type nodeResponse struct {
	message protocol.NodeToServer
	err     error
}

type pendingNodeRequest struct {
	deviceID string
	channel  chan nodeResponse
}

type nodePeer struct {
	hub      *NodeHub
	deviceID string
	conn     *websocket.Conn
	sendMu   sync.Mutex
	// idleEnforced is set when the Node announces keepalive support. Only the read loop touches it.
	idleEnforced bool
}

// NodeHub owns authenticated Node command channels. It carries MCP execution
// requests and responses; it never owns process output or execution state.
type NodeHub struct {
	mu              sync.RWMutex
	peers           map[string]*nodePeer
	pending         map[string]pendingNodeRequest
	images          map[string]*pendingNodeImage
	onPresence      func(string, bool)
	onHello         func(string, *protocol.NodeHello)
	onLock          func(string, string, uint64)
	onControlFrame  func(string, protocol.NodeToServer)
	onControlClosed func(string, string)
	onProcessState  func(string, protocol.NodeToServer)
	// keepaliveInterval and idleTimeout default to the protocol values; tests shorten them.
	keepaliveInterval time.Duration
	idleTimeout       time.Duration
}

// timing returns the keepalive interval and idle timeout, falling back to the protocol values.
func (h *NodeHub) timing() (time.Duration, time.Duration) {
	interval, idle := h.keepaliveInterval, h.idleTimeout
	if interval <= 0 {
		interval = protocol.KeepaliveInterval
	}
	if idle <= 0 {
		idle = protocol.IdleTimeout
	}
	return interval, idle
}

func newNodeHub() *NodeHub {
	return &NodeHub{
		peers:   make(map[string]*nodePeer),
		pending: make(map[string]pendingNodeRequest),
		images:  make(map[string]*pendingNodeImage),
	}
}

func (h *NodeHub) Serve(deviceID string, w http.ResponseWriter, req *http.Request) {
	server := websocket.Server{
		Handshake: func(*websocket.Config, *http.Request) error { return nil },
		Handler: func(conn *websocket.Conn) {
			h.servePeer(deviceID, conn)
		},
	}
	server.ServeHTTP(w, req)
}

func (h *NodeHub) servePeer(deviceID string, conn *websocket.Conn) {
	peer := &nodePeer{hub: h, deviceID: deviceID, conn: conn}
	_, idleTimeout := h.timing()
	h.mu.Lock()
	old := h.peers[deviceID]
	h.peers[deviceID] = peer
	h.mu.Unlock()
	if old != nil {
		_ = old.conn.Close()
	}
	started := time.Now()
	log.Printf("node %s control channel connected", deviceID)
	stop := make(chan struct{})
	go peer.keepalive(stop)
	if h.onPresence != nil {
		h.onPresence(deviceID, true)
	}
	decoder := &protocol.Decoder{}
	var closeErr error
	for {
		if peer.idleEnforced {
			_ = conn.SetReadDeadline(time.Now().Add(idleTimeout))
		}
		var payload []byte
		if err := websocket.Message.Receive(conn, &payload); err != nil {
			closeErr = err
			break
		}
		plain, done, err := decoder.Accept(payload)
		if err != nil || !done {
			continue
		}
		var message protocol.NodeToServer
		if json.Unmarshal(plain, &message) != nil {
			continue
		}
		if message.Type == "hello" && message.Hello != nil && hasCapability(message.Hello.Capabilities, protocol.CapabilityKeepalive) {
			peer.idleEnforced = true
		}
		h.dispatch(deviceID, message)
	}
	close(stop)
	log.Printf("node %s control channel closed after %s: %v", deviceID, time.Since(started).Round(time.Second), closeErr)
	h.removeIf(deviceID, peer)
}

// keepalive keeps an idle control channel open through proxies until the read loop stops.
func (p *nodePeer) keepalive(stop <-chan struct{}) {
	interval, _ := p.hub.timing()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			if err := p.send(protocol.ServerToNode{Type: protocol.TypeKeepalive}); err != nil {
				_ = p.conn.Close()
				return
			}
		}
	}
}

func (h *NodeHub) removeIf(deviceID string, peer *nodePeer) {
	h.mu.Lock()
	current, ok := h.peers[deviceID]
	if ok && current == peer {
		delete(h.peers, deviceID)
	}
	h.mu.Unlock()
	if !ok || current != peer {
		return
	}
	h.failDevice(deviceID)
	if h.onPresence != nil {
		h.onPresence(deviceID, false)
	}
}

func (h *NodeHub) failDevice(deviceID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for requestID, pending := range h.pending {
		if pending.deviceID == deviceID {
			select {
			case pending.channel <- nodeResponse{err: errors.New("Node disconnected")}:
			default:
			}
			delete(h.pending, requestID)
		}
	}
	for requestID, pending := range h.images {
		if pending.deviceID == deviceID {
			select {
			case pending.channel <- nodeImageResult{err: errors.New("Node disconnected")}:
			default:
			}
			delete(h.images, requestID)
		}
	}
}

// Disconnect closes one live Node transport after its enrollment is revoked.
func (h *NodeHub) Disconnect(deviceID string) {
	h.mu.RLock()
	peer := h.peers[deviceID]
	h.mu.RUnlock()
	if peer != nil {
		_ = peer.conn.Close()
	}
}

func (h *NodeHub) Online(deviceID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.peers[deviceID] != nil
}

func (h *NodeHub) send(deviceID string, message protocol.ServerToNode) error {
	h.mu.RLock()
	peer := h.peers[deviceID]
	h.mu.RUnlock()
	if peer == nil {
		return errors.New("Node is offline")
	}
	return peer.send(message)
}

// Send delivers one server command to a connected Node.
func (h *NodeHub) Send(deviceID string, message protocol.ServerToNode) error {
	return h.send(deviceID, message)
}

func (p *nodePeer) send(message protocol.ServerToNode) error {
	encoded, err := json.Marshal(message)
	if err != nil {
		return err
	}
	frames, err := protocol.Encode(string(encoded))
	if err != nil {
		return err
	}
	p.sendMu.Lock()
	defer p.sendMu.Unlock()
	_ = p.conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
	for _, frame := range frames {
		if err := websocket.Message.Send(p.conn, frame); err != nil {
			return err
		}
	}
	return nil
}

func (h *NodeHub) Request(ctx context.Context, deviceID string, message protocol.ServerToNode, timeout time.Duration) (protocol.NodeToServer, error) {
	requestID := randomID(18)
	message.RequestID = requestID
	channel := make(chan nodeResponse, 1)
	h.mu.Lock()
	h.pending[requestID] = pendingNodeRequest{deviceID: deviceID, channel: channel}
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.pending, requestID)
		h.mu.Unlock()
	}()
	if err := h.send(deviceID, message); err != nil {
		return protocol.NodeToServer{}, err
	}
	select {
	case response := <-channel:
		return response.message, response.err
	case <-ctx.Done():
		return protocol.NodeToServer{}, ctx.Err()
	case <-time.After(timeout):
		return protocol.NodeToServer{}, errors.New("Node command timed out")
	}
}

func (h *NodeHub) WaitOnline(ctx context.Context, deviceID string) error {
	for {
		if h.Online(deviceID) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
}
