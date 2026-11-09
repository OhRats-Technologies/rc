package server

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	authstore "github.com/OhRats-Technologies/rc/pkg/auth"
)

func (s *Server) handleOAuthToken(w http.ResponseWriter, req *http.Request) {
	if s.auth == nil {
		writeOAuthError(w, http.StatusServiceUnavailable, "temporarily_unavailable", "account store is unavailable")
		return
	}
	w.Header().Set("Pragma", "no-cache")
	req.Body = http.MaxBytesReader(w, req.Body, 64<<10)
	if mediaType := requestMediaType(req); mediaType != "" && mediaType != "application/x-www-form-urlencoded" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "token requests must be form encoded")
		return
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "token request body is invalid")
		return
	}
	values, err := url.ParseQuery(string(body))
	if err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "token request body is invalid")
		return
	}
	req.PostForm = values
	clientID := strings.TrimSpace(values.Get("client_id"))
	if clientID == "" {
		writeOAuthError(w, http.StatusUnauthorized, "invalid_client", "client_id is required")
		return
	}
	switch values.Get("grant_type") {
	case "authorization_code":
		s.exchangeAuthorizationCode(w, req, clientID)
	case "refresh_token":
		s.exchangeRefreshToken(w, req, clientID)
	case "":
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "grant_type is required")
	default:
		writeOAuthError(w, http.StatusBadRequest, "unsupported_grant_type", "unsupported grant_type")
	}
}

func (s *Server) exchangeAuthorizationCode(w http.ResponseWriter, req *http.Request, clientID string) {
	code := req.PostForm.Get("code")
	redirectURI := req.PostForm.Get("redirect_uri")
	verifier := req.PostForm.Get("code_verifier")
	if code == "" || redirectURI == "" || verifier == "" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request",
			"code, redirect_uri, and code_verifier are required")
		return
	}
	userID, resource, scopes, grantID, err := s.auth.ConsumeOAuthCode(
		code, clientID, redirectURI, verifier, req.PostForm.Get("resource"),
	)
	if err != nil {
		if errors.Is(err, authstore.ErrOAuthResourceMismatch) {
			writeOAuthError(w, http.StatusBadRequest, "invalid_target", "resource does not match this authorization code")
			return
		}
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "authorization code is invalid or expired")
		return
	}
	if grantID == "" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "authorization code is not bound to an active MCP grant")
		return
	}
	if _, err := s.auth.McpGrant(userID, grantID); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "MCP grant is revoked or expired")
		return
	}
	s.writeOAuthTokens(w, clientID, userID, resource, scopes, grantID)
}

func (s *Server) exchangeRefreshToken(w http.ResponseWriter, req *http.Request, clientID string) {
	refresh := req.PostForm.Get("refresh_token")
	if refresh == "" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "refresh_token is required")
		return
	}
	access, newRefresh, _, _, scopes, err := s.auth.ExchangeOAuthRefresh(
		refresh, clientID, req.PostForm.Get("resource"),
	)
	if err != nil {
		switch {
		case errors.Is(err, authstore.ErrOAuthInvalidClient),
			errors.Is(err, authstore.ErrOAuthClientNotFound):
			writeOAuthError(w, http.StatusUnauthorized, "invalid_client", "client_id is invalid")
		case errors.Is(err, authstore.ErrOAuthResourceMismatch):
			writeOAuthError(w, http.StatusBadRequest, "invalid_target", "resource does not match this grant")
		default:
			writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "refresh token is invalid or expired")
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token":  access,
		"token_type":    "Bearer",
		"expires_in":    int(authstore.OAuthAccessTokenLifetime / time.Second),
		"refresh_token": newRefresh,
		"scope":         scopes,
	})
}

func (s *Server) writeOAuthTokens(w http.ResponseWriter, clientID, userID, resource, scopes, grantID string) {
	access, refresh, _, _, err := s.auth.IssueOAuthTokens(clientID, userID, resource, scopes, grantID)
	if err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "tokens could not be issued")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token":  access,
		"token_type":    "Bearer",
		"expires_in":    int(authstore.OAuthAccessTokenLifetime / time.Second),
		"refresh_token": refresh,
		"scope":         scopes,
	})
}

func requestMediaType(req *http.Request) string {
	contentType := strings.TrimSpace(req.Header.Get("Content-Type"))
	if contentType == "" {
		return ""
	}
	if index := strings.IndexByte(contentType, ';'); index >= 0 {
		contentType = contentType[:index]
	}
	return strings.ToLower(strings.TrimSpace(contentType))
}
