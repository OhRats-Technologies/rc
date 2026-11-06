package server

import (
	"net/http"
	"net/url"
	"strings"
)

// handleOAuthAuthorize implements the authorization-code + PKCE grant. It
// requires an existing passkey session and redirects to login without one.
func (s *Server) handleOAuthAuthorize(w http.ResponseWriter, req *http.Request) {
	if s.auth == nil {
		writeOAuthError(w, http.StatusServiceUnavailable, "temporarily_unavailable", "account store is unavailable")
		return
	}
	query := req.URL.Query()
	clientID := strings.TrimSpace(query.Get("client_id"))
	redirectURI := query.Get("redirect_uri")
	state := query.Get("state")

	if clientID == "" || redirectURI == "" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "client_id and redirect_uri are required")
		return
	}
	if err := s.auth.ValidateRedirectURI(clientID, redirectURI); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "client or redirect URI is invalid")
		return
	}
	fail := func(code, description string) {
		s.redirectOAuthResult(w, req, redirectURI, url.Values{
			"error":             {code},
			"error_description": {description},
			"state":             {state},
		})
	}

	if strings.TrimSpace(query.Get("response_type")) != "code" {
		fail("unsupported_response_type", "only the code response type is supported")
		return
	}
	challenge := strings.TrimSpace(query.Get("code_challenge"))
	if strings.TrimSpace(query.Get("code_challenge_method")) != "S256" || challenge == "" {
		fail("invalid_request", "PKCE with the S256 code challenge method is required")
		return
	}
	resource := strings.TrimSpace(query.Get("resource"))
	if resource == "" {
		resource = s.mcpResource(req)
	}
	if resource != s.mcpResource(req) {
		fail("invalid_target", "the requested resource is not served by this authorization server")
		return
	}

	user, _, err := s.currentSession(req)
	if err != nil {
		next := req.URL.RequestURI()
		http.Redirect(w, req, "/login?next="+url.QueryEscape(next), http.StatusSeeOther)
		return
	}
	code, err := s.auth.IssueOAuthCode(clientID, user.ID, redirectURI, challenge, resource)
	if err != nil {
		fail("server_error", "the authorization code could not be issued")
		return
	}
	s.redirectOAuthResult(w, req, redirectURI, url.Values{
		"code":  {code},
		"state": {state},
	})
}

func (s *Server) redirectOAuthResult(w http.ResponseWriter, req *http.Request, redirectURI string, values url.Values) {
	target, err := url.Parse(redirectURI)
	if err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "redirect URI is invalid")
		return
	}
	query := target.Query()
	for key, entries := range values {
		for _, value := range entries {
			if value == "" {
				continue
			}
			query.Set(key, value)
		}
	}
	target.RawQuery = query.Encode()
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, req, target.String(), http.StatusSeeOther)
}
