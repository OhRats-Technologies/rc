package server

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	authstore "github.com/OhRats-Technologies/rc/pkg/auth"
)

func (s *Server) handleListAPITokens(w http.ResponseWriter, req *http.Request) {
	user, _, err := s.currentSession(req)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	keys, err := s.auth.ListAPIKeys(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "API keys could not be loaded")
		return
	}
	rows := make([]map[string]any, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, map[string]any{
			"id": key.ID, "name": key.Name, "publicKey": key.PublicKey,
			"scopes": key.Scopes, "createdAt": msOrZero(key.CreatedAt),
			"expiresAt": msOrZero(key.ExpiresAt), "lastUsedAt": optionalMs(key.LastUsedAt),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"tokens": rows})
}

func (s *Server) handleCreateAPIToken(w http.ResponseWriter, req *http.Request) {
	user, _, err := s.currentSession(req)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	if err := s.consumeStepUp(req, user.ID); err != nil {
		writeError(w, http.StatusUnauthorized, "fresh passkey verification required")
		return
	}
	var input struct {
		Name      string   `json:"name"`
		Scopes    []string `json:"scopes"`
		PublicKey string   `json:"publicKey"`
		Lifetime  string   `json:"lifetime"`
	}
	if err := decodeJSON(req, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	publicKey := strings.TrimSpace(input.PublicKey)
	if decoded, err := base64.RawURLEncoding.DecodeString(publicKey); err != nil ||
		len(decoded) != 32 {
		writeError(w, http.StatusBadRequest, "invalid API signing key")
		return
	}
	count, err := s.auth.CountAPIKeys(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "API keys could not be loaded")
		return
	}
	if count >= authstore.MaxAPIKeysPerUser {
		writeError(w, http.StatusConflict, "API key limit reached")
		return
	}
	expires, err := authLifetime(input.Lifetime, "never", true)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	key := authstore.APIKey{
		ID: newAPIKeyID(), UserID: user.ID,
		Name:      authstore.NormalizeAPIKeyName(input.Name),
		PublicKey: publicKey, Scopes: authstore.NormalizeAPIKeyScopes(input.Scopes),
		CreatedAt: time.Now().UTC(), ExpiresAt: expires,
	}
	if err := s.auth.PutAPIKey(key); err != nil {
		writeError(w, http.StatusInternalServerError, "API key could not be stored")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id": key.ID, "name": key.Name, "publicKey": key.PublicKey,
		"scopes": key.Scopes, "createdAt": msOrZero(key.CreatedAt),
		"expiresAt": msOrZero(key.ExpiresAt),
	})
}

func (s *Server) handleDeleteAPIToken(w http.ResponseWriter, req *http.Request) {
	user, _, err := s.currentSession(req)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	if err := s.consumeStepUp(req, user.ID); err != nil {
		writeError(w, http.StatusUnauthorized, "fresh passkey verification required")
		return
	}
	removed, err := s.auth.DeleteAPIKey(user.ID, req.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "API key could not be removed")
		return
	}
	if !removed {
		writeError(w, http.StatusNotFound, "API key not found")
		return
	}
	workspaces, _ := s.auth.WorkspacesForOwner(user.ID)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "workspaceIds": workspaces})
}

func (s *Server) consumeStepUp(req *http.Request, userID string) error {
	cookie, err := req.Cookie(stepUpCookieName)
	if err != nil || cookie.Value == "" {
		return errors.New("step-up required")
	}
	session, err := s.auth.LookupSession(cookie.Value)
	if err != nil || session.UserID != userID {
		return errors.New("step-up required")
	}
	return s.auth.RevokeSession(cookie.Value)
}

func newAPIKeyID() string {
	value := make([]byte, 16)
	_, _ = rand.Read(value)
	return hex.EncodeToString(value)
}

func optionalMs(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UnixMilli()
}
