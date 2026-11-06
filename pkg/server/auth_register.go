package server

import (
	"encoding/base64"
	"net/http"
	"strings"
	"time"

	authstore "github.com/OhRats-Technologies/rc/pkg/auth"
	"github.com/OhRats-Technologies/rc/pkg/auth/webauthn"
)

func (s *Server) handleRegisterOptions(w http.ResponseWriter, req *http.Request) {
	user, _, err := s.currentSession(req)
	if err != nil || s.auth == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	var input nameInput
	if err := decodeJSON(req, &input); err != nil || input.Name == nil {
		writeError(w, http.StatusBadRequest, "invalid request")
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
	challenge, options, err := webauthn.CreationOptions(user.ID, *input.Name, rp, ids)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	state, err := stateFor(challenge, rp, *input.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	ceremonyID := randomID(16)
	if err := s.auth.PutCeremony(ceremonyID, "register", user.ID, state, time.Now().Add(ceremonyTTL).UnixMilli()); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ceremonyId": ceremonyID, "options": options})
}

func (s *Server) handleRegisterVerify(w http.ResponseWriter, req *http.Request) {
	if s.auth == nil {
		writeError(w, http.StatusServiceUnavailable, "account store is unavailable")
		return
	}
	var input verifyInput
	if err := decodeJSON(req, &input); err != nil || input.CeremonyID == nil || len(input.Response) == 0 {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	sessionUser, _, sessionErr := s.currentSession(req)
	stateBytes, userID, ok, err := s.auth.TakeCeremony(*input.CeremonyID, "register")
	signup := false
	if sessionErr != nil || !ok {
		// A public-signup ceremony has no session and is consumed as "signup".
		stateBytes, userID, ok, err = s.auth.TakeCeremony(*input.CeremonyID, "signup")
		signup = true
	}
	if err != nil || !ok {
		writeError(w, http.StatusUnauthorized, "registration expired")
		return
	}
	if !signup && (sessionErr != nil || sessionUser.ID != userID) {
		writeError(w, http.StatusUnauthorized, "registration expired")
		return
	}
	state, rp, err := s.loadState(stateBytes)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "registration expired")
		return
	}
	challenge, err := base64.RawURLEncoding.DecodeString(state.Challenge)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "registration expired")
		return
	}
	registration, err := webauthn.VerifyRegistration(input.Response, challenge, rp)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "passkey verification failed")
		return
	}
	name := strings.TrimSpace(state.Name)
	if name == "" {
		name = "Passkey"
	}
	if signup {
		if !s.publicSignupConfigured() || s.userCount() == 0 {
			writeError(w, http.StatusNotFound, "public signup is unavailable")
			return
		}
		if _, err := s.auth.CreateUserWithID(userID, name); err != nil {
			writeError(w, http.StatusConflict, "account is already registered")
			return
		}
		if _, err := s.auth.AddPasskey(userID, name, registration.Credential.ID, registration.Credential.PublicKeyCOSE, authstore.AlgorithmES256); err != nil {
			writeError(w, http.StatusConflict, "passkey is already registered")
			return
		}
		s.finishSession(w, req, userID, "", http.StatusCreated)
		return
	}
	if _, err := s.auth.AddPasskey(userID, name, registration.Credential.ID, registration.Credential.PublicKeyCOSE, authstore.AlgorithmES256); err != nil {
		writeError(w, http.StatusConflict, "passkey is already registered")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
