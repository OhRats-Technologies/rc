package server

import (
	"encoding/base64"
	"net/http"
	"time"

	"github.com/OhRats-Technologies/rc/pkg/auth/webauthn"
)

func (s *Server) handleStepUpOptions(w http.ResponseWriter, req *http.Request) {
	user, _, err := s.currentSession(req)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	rp, err := s.relyingParty(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	passkeys, err := s.auth.PasskeysForUser(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	ids := make([][]byte, 0, len(passkeys))
	for _, passkey := range passkeys {
		ids = append(ids, passkey.CredentialID)
	}
	challenge, options, err := webauthn.RequestOptions(ids, rp)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "no passkey is registered")
		return
	}
	state, err := stateFor(challenge, rp, "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	ceremonyID := randomID(16)
	if err := s.auth.PutCeremony(ceremonyID, "step-up", user.ID, state, time.Now().Add(ceremonyTTL).UnixMilli()); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ceremonyId": ceremonyID, "options": options})
}

func (s *Server) handleStepUpVerify(w http.ResponseWriter, req *http.Request) {
	user, _, err := s.currentSession(req)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	var input verifyInput
	if err := decodeJSON(req, &input); err != nil || input.CeremonyID == nil || len(input.Response) == 0 {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	credentialID, err := responseCredentialID(input.Response)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "passkey verification failed")
		return
	}
	passkey, err := s.auth.PasskeyByCredentialID(credentialID)
	if err != nil || passkey.UserID != user.ID {
		writeError(w, http.StatusUnauthorized, "passkey verification failed")
		return
	}
	stateBytes, userID, ok, err := s.auth.TakeCeremony(*input.CeremonyID, "step-up")
	if err != nil || !ok || userID != user.ID {
		writeError(w, http.StatusUnauthorized, "step-up expired")
		return
	}
	state, rp, err := s.loadState(stateBytes)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "step-up expired")
		return
	}
	challenge, err := base64.RawURLEncoding.DecodeString(state.Challenge)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "step-up expired")
		return
	}
	if err := verifyWithAccountHandles(passkey.UserID, func(handle []byte) error {
		return webauthn.VerifyAuthentication(input.Response, challenge, rp, passkey.CredentialID, passkey.PublicKey, handle)
	}); err != nil {
		writeError(w, http.StatusUnauthorized, "passkey verification failed")
		return
	}
	token, expires, err := s.auth.IssueSession(user.ID, "5m")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.setSessionCookie(w, stepUpCookieName, token, expires, rp.Secure)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "expiresAt": expires.UnixMilli()})
}
