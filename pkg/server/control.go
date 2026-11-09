package server

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	authstore "github.com/OhRats-Technologies/rc/pkg/auth"
	"github.com/OhRats-Technologies/rc/pkg/auth/webauthn"
	"github.com/OhRats-Technologies/rc/pkg/crypto"
	"github.com/OhRats-Technologies/rc/pkg/protocol"
)

type controlAuthorizationMeta struct {
	ClientID         string `json:"clientId"`
	SigningPublicKey string `json:"signingPublicKey"`
	Grant            string `json:"grant"`
	Kind             string `json:"kind"`
}

func (s *Server) handleControlAuthorizeOptions(w http.ResponseWriter, req *http.Request) {
	user, _, err := s.currentSession(req)
	if err != nil || s.auth == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	var input struct {
		ClientID         string `json:"clientId"`
		SigningPublicKey string `json:"signingPublicKey"`
		Lifetime         string `json:"lifetime"`
		Kind             string `json:"kind"`
	}
	if err := decodeJSON(req, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	clientID := strings.TrimSpace(input.ClientID)
	signingKey := strings.TrimSpace(input.SigningPublicKey)
	kind := strings.TrimSpace(input.Kind)
	if kind == "" {
		kind = "browser"
	}
	if !validControlClientID(clientID, kind) || !validControlPublicKey(signingKey) {
		writeError(w, http.StatusBadRequest, "invalid control client")
		return
	}
	expires, err := authLifetime(input.Lifetime, "30d", true)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	now := time.Now().UTC()
	expiresAt := int64(0)
	if !expires.IsZero() {
		expiresAt = expires.UnixMilli()
	}
	grant := protocol.ControlGrant{
		V:                1,
		ClientID:         clientID,
		UserID:           user.ID,
		SigningPublicKey: signingKey,
		IssuedAt:         now.UnixMilli(),
		ExpiresAt:        expiresAt,
	}
	encodedGrant, _ := json.Marshal(grant)
	rp, err := s.relyingParty(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	passkeys, err := s.auth.PasskeysForUser(user.ID)
	if err != nil || len(passkeys) == 0 {
		writeError(w, http.StatusUnauthorized, "no passkey is registered")
		return
	}
	ids := make([][]byte, 0, len(passkeys))
	for _, passkey := range passkeys {
		ids = append(ids, passkey.CredentialID)
	}
	challengeValue := crypto.ControlGrantChallenge(string(encodedGrant))
	_, options, err := webauthn.RequestOptions(ids, rp)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "no passkey is registered")
		return
	}
	options["challenge"] = challengeValue
	meta, _ := json.Marshal(controlAuthorizationMeta{
		ClientID: clientID, SigningPublicKey: signingKey,
		Grant: string(encodedGrant), Kind: kind,
	})
	authorizationID := randomID(16)
	if err := s.auth.PutCeremony(authorizationID, "control-authorize", user.ID, meta,
		time.Now().Add(ceremonyTTL).UnixMilli()); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"authorizationId": authorizationID,
		"grant":           string(encodedGrant),
		"options":         options,
	})
}

func (s *Server) handleControlAuthorizeVerify(w http.ResponseWriter, req *http.Request) {
	user, _, err := s.currentSession(req)
	if err != nil || s.auth == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	var input struct {
		AuthorizationID *string         `json:"authorizationId"`
		CeremonyID      *string         `json:"ceremonyId"`
		Response        json.RawMessage `json:"response"`
	}
	if err := decodeJSON(req, &input); err != nil || len(input.Response) == 0 {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	authorizationID := input.CeremonyID
	if input.AuthorizationID != nil {
		authorizationID = input.AuthorizationID
	}
	if authorizationID == nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	state, userID, ok, err := s.auth.TakeCeremony(*authorizationID, "control-authorize")
	if err != nil || !ok || userID != user.ID {
		writeError(w, http.StatusUnauthorized, "control authorization expired")
		return
	}
	var meta controlAuthorizationMeta
	if err := json.Unmarshal(state, &meta); err != nil {
		writeError(w, http.StatusUnauthorized, "control authorization expired")
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
	rp, err := s.relyingParty(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	challenge, err := base64.RawURLEncoding.DecodeString(crypto.ControlGrantChallenge(meta.Grant))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "control authorization expired")
		return
	}
	if err := verifyWithAccountHandles(passkey.UserID, func(handle []byte) error {
		return webauthn.VerifyAuthentication(input.Response, challenge, rp, passkey.CredentialID, passkey.PublicKey, handle)
	}); err != nil {
		writeError(w, http.StatusUnauthorized, "passkey verification failed")
		return
	}
	var grant protocol.ControlGrant
	if err := json.Unmarshal([]byte(meta.Grant), &grant); err != nil {
		writeError(w, http.StatusUnauthorized, "control grant is invalid")
		return
	}
	expires := time.Time{}
	if grant.ExpiresAt != 0 {
		expires = time.UnixMilli(grant.ExpiresAt).UTC()
	}
	if err := s.auth.PutControlClient(authstore.ControlClient{
		ID: meta.ClientID, UserID: user.ID, Kind: meta.Kind,
		SigningPublicKey: meta.SigningPublicKey,
		CredentialID:     base64.RawURLEncoding.EncodeToString(credentialID),
		Grant:            meta.Grant,
		Assertion:        string(input.Response),
		CreatedAt:        time.Now().UTC(),
		ExpiresAt:        expires,
	}); err != nil {
		if errors.Is(err, authstore.ErrControlClientConflict) {
			writeError(w, http.StatusConflict, "control identity belongs to another account; reload and authorize again")
			return
		}
		writeError(w, http.StatusInternalServerError, "control authorization could not be stored")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "expiresAt": msOrZero(expires),
	})
}

func (s *Server) handleControlClientStatus(w http.ResponseWriter, req *http.Request) {
	user, _, err := s.currentSession(req)
	if err != nil || s.auth == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	client, err := s.auth.ControlClient(user.ID, req.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"authorized": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"authorized": true, "expiresAt": msOrZero(client.ExpiresAt),
	})
}

func (s *Server) controlProof(userID, clientID string) (*protocol.ControlProof, error) {
	client, err := s.auth.ControlClient(userID, clientID)
	if err != nil {
		return nil, err
	}
	return &protocol.ControlProof{
		Grant: client.Grant, CredentialID: client.CredentialID, Assertion: client.Assertion,
	}, nil
}

func (s *Server) freshControlProof(userID, clientID string, maxAge time.Duration) (*protocol.ControlProof, error) {
	client, err := s.auth.ControlClient(userID, clientID)
	if err != nil {
		return nil, err
	}
	if time.Since(client.CreatedAt) > maxAge {
		return nil, authstore.ErrControlClientNotFound
	}
	return &protocol.ControlProof{
		Grant: client.Grant, CredentialID: client.CredentialID, Assertion: client.Assertion,
	}, nil
}

func (s *Server) verifyControlSignature(userID, clientID, payload, signature string) bool {
	client, err := s.auth.ControlClient(userID, clientID)
	if err != nil {
		return false
	}
	return crypto.VerifyEd25519(client.SigningPublicKey, []byte(payload), signature) == nil
}

func validControlClientID(value, kind string) bool {
	if len(value) < 16 || len(value) > 100 || (kind != "browser" && kind != "cli") {
		return false
	}
	for index := 0; index < len(value); index++ {
		char := value[index]
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' ||
			char >= '0' && char <= '9' || char == '-' || char == '_' {
			continue
		}
		return false
	}
	return true
}

func validControlPublicKey(value string) bool {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(decoded) == 32
}

func msOrZero(value time.Time) int64 {
	if value.IsZero() {
		return 0
	}
	return value.UnixMilli()
}
