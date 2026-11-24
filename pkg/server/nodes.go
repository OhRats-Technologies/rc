package server

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/OhRats-Technologies/rc/pkg/crypto"
	"github.com/OhRats-Technologies/rc/pkg/node"
)

// DevicePresence is the single derived connectivity state of a Node.
type DevicePresence string

const (
	PresenceOffline   DevicePresence = "offline"
	PresenceHeartbeat DevicePresence = "heartbeat"
	PresenceConnected DevicePresence = "connected"
)

const (
	nodeAuthWindow   = 60 * time.Second
	nodeHeartbeatTTL = 90 * time.Second
)

func (r *DeviceRegistry) verifyNode(req *http.Request, body []byte) (*RegisteredDevice, error) {
	deviceID := req.Header.Get("x-rc-device")
	timestamp := req.Header.Get("x-rc-timestamp")
	nonce := req.Header.Get("x-rc-nonce")
	signature := req.Header.Get("x-rc-signature")
	if deviceID == "" || timestamp == "" || nonce == "" || signature == "" {
		return nil, errors.New("missing Node authentication headers")
	}
	if len(nonce) < 8 || len(nonce) > 128 {
		return nil, errors.New("invalid Node nonce")
	}
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return nil, errors.New("invalid Node timestamp")
	}
	when := time.Unix(seconds, 0)
	if delta := time.Since(when); delta > nodeAuthWindow || delta < -nodeAuthWindow {
		return nil, errors.New("stale Node timestamp")
	}

	r.mu.Lock()
	device, ok := r.devices[deviceID]
	if !ok {
		r.mu.Unlock()
		return nil, r.unknownNode(deviceID)
	}
	r.pruneNoncesLocked()
	key := deviceID + "\x00" + nonce
	if _, replayed := r.nonces[key]; replayed {
		r.mu.Unlock()
		return nil, errors.New("replayed Node request")
	}
	r.nonces[key] = time.Now()
	r.mu.Unlock()

	payload := crypto.NodeHttpPayload(deviceID, timestamp, nonce, req.Method, req.URL.RequestURI(), body)
	if err := crypto.VerifyNodeHttp(device.IdentityPublicKey, signature, payload); err != nil {
		return nil, errors.New("invalid Node signature")
	}
	return device, nil
}

// errNodeRevoked tells a Node that its device was removed. The Node clears its
// local enrollment on this response; generic unknown devices stay 401.
var errNodeRevoked = errors.New("Node was removed from RC")

func nodeAuthStatus(err error) int {
	if errors.Is(err, errNodeRevoked) {
		return http.StatusGone
	}
	return http.StatusUnauthorized
}

func (r *DeviceRegistry) unknownNode(deviceID string) error {
	if r.store != nil {
		if revoked, err := r.store.DeviceRevoked(deviceID); err == nil && revoked {
			return errNodeRevoked
		}
	}
	return errors.New("unknown Node")
}

func (r *DeviceRegistry) pruneNoncesLocked() {
	cutoff := time.Now().Add(-5 * time.Minute)
	for key, seen := range r.nonces {
		if seen.Before(cutoff) {
			delete(r.nonces, key)
		}
	}
}

func (r *DeviceRegistry) markOnline(deviceID string, online bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	device, ok := r.devices[deviceID]
	if !ok {
		return
	}
	now := time.Now().UTC()
	device.LastSeen = now
	if online {
		device.heartbeatUntil = now.Add(nodeHeartbeatTTL)
	} else {
		device.heartbeatUntil = time.Time{}
	}
}

func (r *DeviceRegistry) touch(deviceID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	device, ok := r.devices[deviceID]
	if !ok {
		return
	}
	device.LastSeen = time.Now().UTC()
}

func (s *Server) handleNodeSelf(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	device, err := s.registry.verifyNode(req, nil)
	if err != nil {
		http.Error(w, err.Error(), nodeAuthStatus(err))
		return
	}
	s.registry.touch(device.ID)
	payload := map[string]any{
		"name":     device.Name,
		"online":   s.nodes.Online(device.ID),
		"version":  device.Version,
		"platform": device.Platform,
		"arch":     device.Arch,
	}
	if access, err := s.workspaceAuthority(device.WorkspaceID); err == nil {
		payload["rcLock"] = map[string]any{
			"workspaceId": device.WorkspaceID,
			"generation":  device.LockGeneration,
			"synced":      device.LockHash == access.Hash,
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(payload)
}

func (r *DeviceRegistry) presenceLocked(device *RegisteredDevice) DevicePresence {
	if r.isConnected != nil && r.isConnected(device.ID) {
		return PresenceConnected
	}
	if !device.heartbeatUntil.IsZero() && time.Now().UTC().Before(device.heartbeatUntil) {
		return PresenceHeartbeat
	}
	return PresenceOffline
}

func (r *DeviceRegistry) reapStaleOnline() []*RegisteredDevice {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	var expired []*RegisteredDevice
	for _, device := range r.devices {
		if device.heartbeatUntil.IsZero() {
			continue
		}
		if r.isConnected != nil && r.isConnected(device.ID) {
			continue
		}
		if !now.Before(device.heartbeatUntil) {
			device.heartbeatUntil = time.Time{}
			copied := *device
			copied.Presence = PresenceOffline
			copied.Online = false
			expired = append(expired, &copied)
		}
	}
	return expired
}

func (s *Server) handleNodeHeartbeat(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, req.Body, 64<<10))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	device, err := s.registry.verifyNode(req, body)
	if err != nil {
		http.Error(w, err.Error(), nodeAuthStatus(err))
		return
	}
	s.registry.markOnline(device.ID, true)
	w.WriteHeader(http.StatusNoContent)
}

// handleNodeConnect upgrades an authenticated Node request into the hosted
// command channel used for MCP execution and image transfer.
func (s *Server) handleNodeConnect(w http.ResponseWriter, req *http.Request) {
	device, err := s.registry.verifyNode(req, nil)
	if err != nil {
		http.Error(w, err.Error(), nodeAuthStatus(err))
		return
	}
	s.nodes.Serve(device.ID, w, req)
}

func (s *Server) runPresenceReaper() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopReaper:
			return
		case <-ticker.C:
			for _, device := range s.registry.reapStaleOnline() {
				s.events.Publish("device.offline", map[string]any{"deviceId": device.ID, "name": device.Name})
			}
		}
	}
}

// markOffline records that a Node disconnected without an authenticated
// removal. It is used by the server when a transport closes.
func (s *Server) markOffline(deviceID string) {
	s.registry.mu.Lock()
	device, ok := s.registry.devices[deviceID]
	s.registry.mu.Unlock()
	if !ok {
		return
	}
	s.registry.markOnline(device.ID, false)
	s.events.Publish("device.offline", map[string]any{"deviceId": device.ID, "name": device.Name})
}

func validateEnrollRequest(req *node.EnrollRequest) error {
	if strings.TrimSpace(req.Token) == "" {
		return errors.New("enrollment token required")
	}
	if len(req.Name) > 128 {
		return errors.New("device name exceeds 128 bytes")
	}
	identity, err := base64.RawURLEncoding.DecodeString(req.IdentityPublicKey)
	if err != nil || len(identity) != ed25519.PublicKeySize {
		return errors.New("identityPublicKey must be a base64url Ed25519 public key")
	}
	transport, err := base64.RawURLEncoding.DecodeString(req.TransportPublicKey)
	if err != nil || len(transport) != 32 {
		return errors.New("transportPublicKey must be a base64url X25519 public key")
	}
	return nil
}
