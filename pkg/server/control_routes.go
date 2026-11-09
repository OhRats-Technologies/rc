package server

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/OhRats-Technologies/rc/pkg/protocol"
)

const maxControlIDLength = 128

type controlReadyResponse struct {
	SessionID          string                       `json:"sessionId"`
	TransportPublicKey string                       `json:"transportPublicKey"`
	EphemeralPublicKey string                       `json:"ephemeralPublicKey"`
	Signature          string                       `json:"signature"`
	IceServers         []protocol.IceServer         `json:"iceServers"`
	Attempts           []protocol.ControlIceAttempt `json:"attempts"`
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
	iceServers, err := s.turn.iceServers(req.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, "TURN unavailable")
		return
	}
	message.IceServers = iceServers
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
		IceServers: iceServers,
	})
	writeJSON(w, http.StatusOK, controlReadyResponse{
		SessionID: response.SessionID, TransportPublicKey: response.TransportPublicKey,
		EphemeralPublicKey: response.EphemeralPublicKey, Signature: response.Signature,
		IceServers: iceServers, Attempts: response.Attempts,
	})
}

func (s *Server) handleControlWebRTC(w http.ResponseWriter, req *http.Request) {
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
	var input struct {
		DeviceID string                  `json:"deviceId"`
		SDP      string                  `json:"sdp"`
		Mode     protocol.ControlIceMode `json:"mode"`
	}
	if err := decodeJSON(req, &input); err != nil || !validControlID(input.DeviceID) ||
		input.SDP == "" || len(input.SDP) > 131072 {
		writeError(w, http.StatusBadRequest, "invalid WebRTC offer")
		return
	}
	session, ok := s.control.get(sessionID)
	if !ok || session.UserID != userID || session.DeviceID != input.DeviceID ||
		(apiKeyID != "" && session.ClientID != apiKeyID) {
		writeError(w, http.StatusNotFound, "control session unavailable")
		return
	}
	ctx, cancel := contextWithTimeout(req, 25*time.Second)
	defer cancel()
	response, err := s.nodes.Request(ctx, input.DeviceID, protocol.ServerToNode{
		Type: protocol.TypeControlWebRTC, SessionID: sessionID, SDP: input.SDP,
		Mode: input.Mode, IceServers: session.IceServers,
	}, 25*time.Second)
	if err != nil || response.Error != "" || response.SDP == "" {
		writeError(w, http.StatusBadGateway, "WebRTC negotiation failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"sdp": response.SDP})
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

func validControlID(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && len(value) <= maxControlIDLength
}

func contextWithTimeout(req *http.Request, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(req.Context(), timeout)
}
