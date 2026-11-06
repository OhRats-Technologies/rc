package server

import (
	"encoding/base64"
	"net/http"
	"time"

	authstore "github.com/OhRats-Technologies/rc/pkg/auth"
	"github.com/OhRats-Technologies/rc/pkg/auth/webauthn"
)

func (s *Server) handleSetupOptions(w http.ResponseWriter, req *http.Request) {
	if s.auth == nil {
		writeError(w, http.StatusServiceUnavailable, "account store is unavailable")
		return
	}
	if !s.requireSetupAuthorization(w, req) {
		return
	}
	count, err := s.auth.UserCount()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if count > 0 {
		writeError(w, http.StatusConflict, "setup already completed")
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
	userID := randomID(16)
	challenge, options, err := webauthn.CreationOptions(userID, *input.Name, rp, nil)
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
	if err := s.auth.PutCeremony(ceremonyID, "setup", userID, state, time.Now().Add(ceremonyTTL).UnixMilli()); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ceremonyId": ceremonyID, "options": options})
}

func (s *Server) handleSetupVerify(w http.ResponseWriter, req *http.Request) {
	if s.auth == nil {
		writeError(w, http.StatusServiceUnavailable, "account store is unavailable")
		return
	}
	if !s.requireSetupAuthorization(w, req) {
		return
	}
	count, err := s.auth.UserCount()
	if err != nil || count > 0 {
		writeError(w, http.StatusConflict, "setup already completed")
		return
	}
	var input verifyInput
	if err := decodeJSON(req, &input); err != nil || input.CeremonyID == nil || len(input.Response) == 0 {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	stateBytes, userID, ok, err := s.auth.TakeCeremony(*input.CeremonyID, "setup")
	if err != nil || !ok || userID == "" {
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
	user, err := s.auth.CreateUserWithID(userID, state.Name)
	if err != nil {
		writeError(w, http.StatusConflict, "setup already completed")
		return
	}
	if _, err := s.auth.AddPasskey(user.ID, "Passkey", registration.Credential.ID, registration.Credential.PublicKeyCOSE, authstore.AlgorithmES256); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.finishSession(w, req, user.ID, "30d", http.StatusCreated)
}
