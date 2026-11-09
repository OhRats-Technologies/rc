package server

import (
	"io"
	"net/http"
	"net/url"
)

// handleOAuthRevoke implements RFC 7009 token revocation for public MCP
// clients. Revoking an access token also revokes its refresh token family.
func (s *Server) handleOAuthRevoke(w http.ResponseWriter, req *http.Request) {
	if s.auth == nil {
		writeOAuthError(w, http.StatusServiceUnavailable, "temporarily_unavailable", "account store is unavailable")
		return
	}
	req.Body = http.MaxBytesReader(w, req.Body, 64<<10)
	if mediaType := requestMediaType(req); mediaType != "" && mediaType != "application/x-www-form-urlencoded" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "revocation requests must be form encoded")
		return
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "revocation request is invalid")
		return
	}
	values, err := url.ParseQuery(string(body))
	if err != nil || values.Get("token") == "" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "token is required")
		return
	}
	if err := s.auth.RevokeOAuthToken(values.Get("token")); err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "token revocation failed")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
}
