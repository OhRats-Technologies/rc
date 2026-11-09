package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	authstore "github.com/OhRats-Technologies/rc/pkg/auth"
	"github.com/OhRats-Technologies/rc/pkg/protocol"
)

type preparedMCPGrant struct {
	Grant            string
	SignaturePayload string
}

func mcpSignaturePayload(grantJSON string) (string, error) {
	var grant protocol.McpGrantPayload
	if err := json.Unmarshal([]byte(grantJSON), &grant); err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(grantJSON))
	return grant.SignaturePayload(hex.EncodeToString(digest[:])), nil
}

func (s *Server) handleOAuthPrepare(w http.ResponseWriter, req *http.Request) {
	user, _, err := s.currentSession(req)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	var input struct {
		RequestID string   `json:"requestId"`
		Audience  string   `json:"audience"`
		Scopes    []string `json:"scopes"`
		Lifetime  string   `json:"lifetime"`
	}
	if err := decodeJSON(req, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	request, err := s.auth.GetOAuthRequest(input.RequestID, user.ID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "MCP authorization expired")
		return
	}
	if input.Audience != "account" {
		writeError(w, http.StatusBadRequest, "explicit account-wide MCP consent is required")
		return
	}
	scopes, err := normalizeSelectedScopes(input.Scopes, request.Scopes)
	if err != nil {
		writeError(w, http.StatusBadRequest, "scope was not requested by this MCP client")
		return
	}
	expires, err := authLifetime(input.Lifetime, "never", true)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	client, err := s.auth.GetOAuthClient(request.ClientID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "MCP client is invalid")
		return
	}
	audience := "account"
	now := time.Now().UTC()
	payload := protocol.McpGrantPayload{
		V: 2, ID: "mcp_" + randomID(18),
		UserID: user.ID, ClientID: request.ClientID, ClientName: client.Name,
		Audience: &audience, Scopes: scopes,
		IssuedAt: now.UnixMilli(),
	}
	if !expires.IsZero() {
		payload.ExpiresAt = expires.UnixMilli()
	}
	grantJSON, _ := json.Marshal(payload)
	signaturePayload, err := mcpSignaturePayload(string(grantJSON))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "grant preparation failed")
		return
	}
	if err := s.auth.SetOAuthRequestPreparedGrant(request.ID, user.ID, string(grantJSON)); err != nil {
		writeError(w, http.StatusInternalServerError, "grant preparation failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"grant": string(grantJSON), "signaturePayload": signaturePayload,
	})
}

func (s *Server) handleOAuthApproveGrant(w http.ResponseWriter, req *http.Request) {
	user, _, err := s.currentSession(req)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	var input struct {
		RequestID       string `json:"requestId"`
		ControlClientID string `json:"controlClientId"`
		Signature       string `json:"signature"`
	}
	if err := decodeJSON(req, &input); err != nil || input.RequestID == "" ||
		input.ControlClientID == "" || input.Signature == "" {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	request, err := s.auth.GetOAuthRequest(input.RequestID, user.ID)
	if err != nil || request.PreparedGrant == "" {
		writeError(w, http.StatusBadRequest, "MCP authorization expired")
		return
	}
	var payload protocol.McpGrantPayload
	if err := json.Unmarshal([]byte(request.PreparedGrant), &payload); err != nil ||
		!payload.ValidAudience() || payload.UserID != user.ID || payload.ID == "" {
		writeError(w, http.StatusBadRequest, "invalid MCP grant")
		return
	}
	for _, scope := range payload.Scopes {
		if !containsScope(request.Scopes, scope) {
			writeError(w, http.StatusBadRequest, "invalid MCP grant scope")
			return
		}
	}
	signaturePayload, err := mcpSignaturePayload(request.PreparedGrant)
	if err != nil || !s.verifyControlSignature(user.ID, input.ControlClientID, signaturePayload, input.Signature) {
		writeError(w, http.StatusForbidden, "invalid MCP grant signature")
		return
	}
	proof, err := s.freshControlProof(user.ID, input.ControlClientID, 5*time.Minute)
	if err != nil {
		writeError(w, http.StatusForbidden, "fresh passkey authorization required")
		return
	}
	client, err := s.auth.GetOAuthClient(request.ClientID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "MCP client is invalid")
		return
	}
	workspaces, _ := s.auth.WorkspacesForOwner(user.ID)
	if hasScope(payload.Scopes, authstore.OAuthScopeTerminal) && len(workspaces) == 0 {
		writeError(w, http.StatusForbidden, "Owner access is required for Terminal")
		return
	}
	expires := time.Time{}
	if payload.ExpiresAt != 0 {
		expires = time.UnixMilli(payload.ExpiresAt).UTC()
	}
	if err := s.auth.PutMcpGrant(authstore.McpGrantRecord{
		ID: payload.ID, UserID: user.ID, ClientID: request.ClientID, Name: client.Name,
		Grant: request.PreparedGrant, Signature: input.Signature,
		ControlClientID: input.ControlClientID, CredentialID: proof.CredentialID,
		ControlGrant: proof.Grant, ControlAssertion: proof.Assertion,
		CreatedAt: time.Now().UTC(), ExpiresAt: expires,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "MCP grant could not be stored")
		return
	}
	consumed, err := s.auth.TakeOAuthRequest(input.RequestID, user.ID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "MCP authorization expired")
		return
	}
	code, err := s.auth.IssueOAuthCode(consumed.ClientID, user.ID, consumed.RedirectURI,
		consumed.CodeChallenge, consumed.Resource, strings.Join(payload.Scopes, " "), payload.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "authorization code could not be issued")
		return
	}
	redirect, err := oauthRedirectURL(consumed.RedirectURI, url.Values{
		"code": {code}, "state": {consumed.State}, "iss": {s.issuer(req)},
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, "redirect URI is invalid")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"redirect": redirect, "grantId": payload.ID, "workspaceIds": workspaces,
		"requiresSync": hasScope(payload.Scopes, authstore.OAuthScopeTerminal),
	})
}

func (s *Server) handleOAuthSwitchAccount(w http.ResponseWriter, req *http.Request) {
	user, _, err := s.currentSession(req)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	var input struct {
		RequestID string `json:"requestId"`
	}
	if err := decodeJSON(req, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	request, err := s.auth.GetOAuthRequest(input.RequestID, user.ID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "MCP authorization expired")
		return
	}
	_ = s.auth.DeleteOAuthRequest(input.RequestID, user.ID)
	if session, err := req.Cookie(sessionCookieName); err == nil {
		_ = s.auth.RevokeSession(session.Value)
	}
	clearSessionCookie(w, sessionCookieName)
	next := "/oauth/authorize?" + url.Values{
		"client_id": {request.ClientID}, "redirect_uri": {request.RedirectURI},
		"state": {request.State}, "response_type": {"code"},
		"code_challenge": {request.CodeChallenge}, "code_challenge_method": {"S256"},
		"resource": {request.Resource}, "scope": {strings.Join(request.Scopes, " ")},
	}.Encode()
	writeJSON(w, http.StatusOK, map[string]string{
		"redirect": "/login?next=" + url.QueryEscape(next),
	})
}

func (s *Server) handleOAuthRevokeGrant(w http.ResponseWriter, req *http.Request) {
	user, _, err := s.currentSession(req)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	id := req.PathValue("id")
	ok, err := s.auth.RevokeMcpGrant(user.ID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "grant revocation failed")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "MCP grant not found")
		return
	}
	workspaces, _ := s.auth.WorkspacesForOwner(user.ID)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "workspaceIds": workspaces})
}
