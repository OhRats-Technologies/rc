package server

import (
	"encoding/base64"
	"net/http"
	"time"

	"github.com/OhRats-Technologies/rc/pkg/auth/webauthn"
)

func (s *Server) handleLoginOptions(w http.ResponseWriter, req *http.Request) {
	if s.auth == nil {
		writeError(w, http.StatusServiceUnavailable, "account store is unavailable")
		return
	}
	rp, err := s.relyingParty(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	passkeys, err := s.auth.AllPasskeys()
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
		writeError(w, http.StatusUnauthorized, "No passkey is registered yet. Complete RC setup first.")
		return
	}
	state, err := stateFor(challenge, rp, "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	ceremonyID := randomID(16)
	if err := s.auth.PutCeremony(ceremonyID, "login", "", state, time.Now().Add(ceremonyTTL).UnixMilli()); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ceremonyId": ceremonyID, "options": options})
}

func (s *Server) handleLoginVerify(w http.ResponseWriter, req *http.Request) {
	if s.auth == nil {
		writeError(w, http.StatusServiceUnavailable, "account store is unavailable")
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
	if err != nil {
		writeError(w, http.StatusUnauthorized, "passkey verification failed")
		return
	}
	stateBytes, _, ok, err := s.auth.TakeCeremony(*input.CeremonyID, "login")
	if err != nil || !ok {
		writeError(w, http.StatusUnauthorized, "authentication expired")
		return
	}
	state, rp, err := s.loadState(stateBytes)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "authentication expired")
		return
	}
	challenge, err := base64.RawURLEncoding.DecodeString(state.Challenge)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "authentication expired")
		return
	}
	if err := verifyWithAccountHandles(passkey.UserID, func(handle []byte) error {
		return webauthn.VerifyAuthentication(input.Response, challenge, rp, passkey.CredentialID, passkey.PublicKey, handle)
	}); err != nil {
		writeError(w, http.StatusUnauthorized, "passkey verification failed")
		return
	}
	_ = s.auth.UpdatePasskeyLastUsed(passkey.ID, time.Now())
	lifetime := "30d"
	if input.Lifetime != nil {
		lifetime = *input.Lifetime
	}
	s.finishSession(w, req, passkey.UserID, lifetime, http.StatusOK)
}
