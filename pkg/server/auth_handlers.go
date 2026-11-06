package server

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/OhRats-Technologies/rc/pkg/auth/webauthn"
)

type ceremonyState struct {
	Challenge string `json:"challenge"`
	RPID      string `json:"rpId"`
	Origin    string `json:"origin"`
	Name      string `json:"name,omitempty"`
}

type nameInput struct {
	Name *string `json:"name"`
}

type verifyInput struct {
	CeremonyID *string         `json:"ceremonyId"`
	Response   json.RawMessage `json:"response"`
	Lifetime   *string         `json:"lifetime"`
}

func stateFor(challenge []byte, rp webauthn.RelyingParty, name string) ([]byte, error) {
	return json.Marshal(ceremonyState{
		Challenge: base64.RawURLEncoding.EncodeToString(challenge),
		RPID:      rp.ID,
		Origin:    rp.Origin,
		Name:      name,
	})
}

func (s *Server) loadState(raw []byte) (ceremonyState, webauthn.RelyingParty, error) {
	var state ceremonyState
	if err := json.Unmarshal(raw, &state); err != nil {
		return state, webauthn.RelyingParty{}, err
	}
	challenge, err := base64.RawURLEncoding.DecodeString(state.Challenge)
	if err != nil || len(challenge) < 16 {
		return state, webauthn.RelyingParty{}, errors.New("invalid ceremony challenge")
	}
	rp, err := webauthn.NewRelyingParty(state.RPID, state.Origin)
	if err != nil {
		return state, webauthn.RelyingParty{}, err
	}
	return state, rp, nil
}

func (s *Server) handleSignupOptions(w http.ResponseWriter, req *http.Request) {
	if s.auth == nil || !s.publicSignupConfigured() || s.userCount() == 0 {
		writeError(w, http.StatusNotFound, "public signup is unavailable")
		return
	}
	var input struct {
		Name      string `json:"name"`
		Turnstile string `json:"turnstile"`
	}
	if err := decodeJSON(req, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	name := cleanDisplayName(input.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	if err := s.verifyTurnstile(req.Context(), input.Turnstile); err != nil {
		status := http.StatusUnauthorized
		message := err.Error()
		if strings.Contains(message, "unavailable") {
			status = http.StatusBadGateway
		} else if strings.Contains(message, "complete the human") {
			status = http.StatusBadRequest
		}
		writeError(w, status, message)
		return
	}
	rp, err := s.relyingParty(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	userID := randomID(16)
	challenge, options, err := webauthn.CreationOptions(userID, name, rp, nil)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	state, err := stateFor(challenge, rp, name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	ceremonyID := randomID(16)
	if err := s.auth.PutCeremony(ceremonyID, "signup", userID, state, time.Now().Add(ceremonyTTL).UnixMilli()); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ceremonyId": ceremonyID, "options": options})
}

func cleanDisplayName(value string) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > 120 {
		value = string(runes[:120])
	}
	return strings.TrimSpace(value)
}

func (s *Server) finishSession(w http.ResponseWriter, req *http.Request, userID, lifetime string, status int) {
	rp, err := s.relyingParty(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	token, expires, err := s.auth.IssueSession(userID, lifetime)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.setSessionCookie(w, sessionCookieName, token, expires, rp.Secure)
	writeJSON(w, status, map[string]any{"ok": true, "expiresAt": expires.UnixMilli()})
}

func responseCredentialID(response json.RawMessage) ([]byte, error) {
	var parsed struct {
		RawID string `json:"rawId"`
		ID    string `json:"id"`
	}
	if err := json.Unmarshal(response, &parsed); err != nil {
		return nil, err
	}
	value := parsed.RawID
	if value == "" {
		value = parsed.ID
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(decoded) == 0 {
		return nil, errors.New("invalid credential id")
	}
	return decoded, nil
}
