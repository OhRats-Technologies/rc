package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/OhRats-Technologies/rc/pkg/protocol"
	"golang.org/x/net/websocket"
)

const maxControlIDLength = 128

// maxControlCiphertext bounds one encrypted frame: a 1 MiB plaintext plus the
// AES-GCM tag and base64 expansion.
const maxControlCiphertext = 2 << 20

type controlReadyResponse struct {
	SessionID          string `json:"sessionId"`
	TransportPublicKey string `json:"transportPublicKey"`
	EphemeralPublicKey string `json:"ephemeralPublicKey"`
	Signature          string `json:"signature"`
}

func (s *Server) handleControlChallenge(w http.ResponseWriter, req *http.Request) {
	userID, _, ok := controlIdentity(req)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	var input struct {
		DeviceID string `json:"deviceId"`
	}
	if err := decodeJSON(req, &input); err != nil || !validControlID(input.DeviceID) {
		writeError(w, http.StatusBadRequest, "invalid control request")
		return
	}
	if !s.controlDeviceRole(w, userID, input.DeviceID) {
		return
	}
	ctx, cancel := contextWithTimeout(req, 15*time.Second)
	defer cancel()
	response, err := s.nodes.Request(ctx, input.DeviceID, protocol.ServerToNode{
		Type: protocol.TypeControlChallenge,
	}, 15*time.Second)
	if err != nil {
		writeError(w, http.StatusConflict, "device is offline")
		return
	}
	if response.Error != "" || response.Challenge == "" {
		writeError(w, http.StatusConflict, "control challenge was rejected")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"challenge": response.Challenge})
}

func (s *Server) handleControlOpen(w http.ResponseWriter, req *http.Request) {
	userID, apiKeyID, ok := controlIdentity(req)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	var input struct {
		DeviceID  string `json:"deviceId"`
		Challenge string `json:"challenge"`
		ClientID  string `json:"clientId"`
		PublicKey string `json:"publicKey"`
		Signature string `json:"signature"`
	}
	if err := decodeJSON(req, &input); err != nil ||
		!validControlID(input.DeviceID) || !validControlID(input.ClientID) ||
		input.Challenge == "" || len(input.Challenge) > 512 ||
		input.PublicKey == "" || len(input.PublicKey) > 512 ||
		input.Signature == "" || len(input.Signature) > 512 {
		writeError(w, http.StatusBadRequest, "invalid control request")
		return
	}
	if !s.controlDeviceRole(w, userID, input.DeviceID) {
		return
	}
	message := protocol.ServerToNode{
		Type: protocol.TypeControlOpen, UserID: userID, ClientID: input.ClientID,
		Challenge: input.Challenge, PublicKey: input.PublicKey, Signature: input.Signature,
	}
	if apiKeyID != "" {
		if input.ClientID != apiKeyID {
			writeError(w, http.StatusForbidden, "control client identity mismatch")
			return
		}
	} else {
		proof, err := s.controlProof(userID, input.ClientID)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "control client authorization expired")
			return
		}
		message.ControlGrant = proof.Grant
		message.CredentialID = proof.CredentialID
		message.ControlAssertion = proof.Assertion
	}
	ctx, cancel := contextWithTimeout(req, 20*time.Second)
	defer cancel()
	response, err := s.nodes.Request(ctx, input.DeviceID, message, 20*time.Second)
	if err != nil {
		writeError(w, http.StatusConflict, "device is offline")
		return
	}
	if response.Error != "" || response.SessionID == "" ||
		response.TransportPublicKey == "" || response.EphemeralPublicKey == "" ||
		response.Signature == "" {
		writeError(w, http.StatusConflict, "control request was rejected")
		return
	}
	s.control.insert(response.SessionID, controlSession{
		UserID: userID, ClientID: input.ClientID, DeviceID: input.DeviceID,
	})
	writeJSON(w, http.StatusOK, controlReadyResponse{
		SessionID: response.SessionID, TransportPublicKey: response.TransportPublicKey,
		EphemeralPublicKey: response.EphemeralPublicKey, Signature: response.Signature,
	})
}

// handleControlPipe upgrades an authenticated control session into the
// WebSocket that relays encrypted frames to and from its Node.
func (s *Server) handleControlPipe(w http.ResponseWriter, req *http.Request) {
	userID, apiKeyID, ok := controlIdentity(req)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	sessionID := req.PathValue("session")
	if !validControlID(sessionID) {
		writeError(w, http.StatusBadRequest, "invalid control session")
		return
	}
	session, ok := s.control.get(sessionID)
	if !ok || session.UserID != userID ||
		(apiKeyID != "" && session.ClientID != apiKeyID) {
		writeError(w, http.StatusNotFound, "control session unavailable")
		return
	}
	if apiKeyID == "" && !sameOrigin(req) {
		writeError(w, http.StatusForbidden, "control session origin mismatch")
		return
	}
	server := websocket.Server{
		Handshake: func(*websocket.Config, *http.Request) error { return nil },
		Handler: func(conn *websocket.Conn) {
			s.serveControlPipe(conn, sessionID, session.DeviceID)
		},
	}
	server.ServeHTTP(w, req)
}

func (s *Server) serveControlPipe(conn *websocket.Conn, sessionID, deviceID string) {
	pipe := newControlPipe(conn)
	if !s.control.attach(sessionID, pipe) {
		pipe.close()
		return
	}
	go pipe.run()
	for {
		var payload []byte
		if err := websocket.Message.Receive(conn, &payload); err != nil {
			break
		}
		var frame protocol.ControlTransportMessage
		if json.Unmarshal(payload, &frame) != nil ||
			frame.Type != protocol.TypeControlFrame || frame.SessionID != sessionID ||
			frame.Sequence == 0 || frame.Ciphertext == "" ||
			len(frame.Ciphertext) > maxControlCiphertext {
			break
		}
		if err := s.nodes.Send(deviceID, protocol.ServerToNode{
			Type: protocol.TypeControlFrame, SessionID: sessionID,
			Sequence: frame.Sequence, Ciphertext: frame.Ciphertext,
		}); err != nil {
			break
		}
	}
	s.control.detach(sessionID, pipe)
	_ = s.nodes.Send(deviceID, protocol.ServerToNode{
		Type: protocol.TypeControlClose, SessionID: sessionID,
	})
}

func (s *Server) handleControlClose(w http.ResponseWriter, req *http.Request) {
	userID, apiKeyID, ok := controlIdentity(req)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	sessionID := req.PathValue("session")
	session, ok := s.control.get(sessionID)
	if !ok || session.UserID != userID || (apiKeyID != "" && session.ClientID != apiKeyID) {
		writeError(w, http.StatusNotFound, "control session unavailable")
		return
	}
	s.control.remove(sessionID)
	ctx, cancel := contextWithTimeout(req, 10*time.Second)
	defer cancel()
	_, _ = s.nodes.Request(ctx, session.DeviceID, protocol.ServerToNode{
		Type: protocol.TypeControlClose, SessionID: sessionID,
	}, 10*time.Second)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) controlDeviceRole(w http.ResponseWriter, userID, deviceID string) bool {
	device, ok := s.registry.Get(deviceID)
	if !ok {
		writeError(w, http.StatusNotFound, "device not found")
		return false
	}
	role, err := s.auth.WorkspaceRole(userID, device.WorkspaceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "device not found")
		return false
	}
	if role != "owner" && role != "operator" {
		writeError(w, http.StatusForbidden, "operator required")
		return false
	}
	return true
}

func controlIdentity(req *http.Request) (string, string, bool) {
	if principal := apiPrincipalFrom(req); principal != nil {
		return principal.UserID, principal.KeyID, true
	}
	if user := sessionUserFrom(req); user != nil {
		return user.ID, "", true
	}
	return "", "", false
}

// sameOrigin rejects cookie-authenticated WebSocket upgrades from another
// site. Non-browser clients that authenticate with a signed API key do not
// send Origin and are handled by the caller.
func sameOrigin(req *http.Request) bool {
	origin := strings.TrimSpace(req.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return strings.EqualFold(parsed.Host, req.Host)
}

func validControlID(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && len(value) <= maxControlIDLength
}

func contextWithTimeout(req *http.Request, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(req.Context(), timeout)
}
