package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/OhRats-Technologies/rc/pkg/crypto"
)

const nodeAuthWindow = 60 * time.Second

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
	device.Online = online
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
	s.registry.markOnline(device.ID, true)
	s.events.Publish("device.online", map[string]any{"deviceId": device.ID, "name": device.Name})
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"name":     device.Name,
		"online":   true,
		"version":  device.Version,
		"platform": device.Platform,
		"arch":     device.Arch,
	})
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

// handleNodeICE issues short-lived TURN credentials to an authenticated Node.
func (s *Server) handleNodeICE(w http.ResponseWriter, req *http.Request) {
	if _, err := s.registry.verifyNode(req, nil); err != nil {
		writeError(w, nodeAuthStatus(err), err.Error())
		return
	}
	servers, err := s.turn.iceServers(req.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, "TURN unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"iceServers": servers})
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
