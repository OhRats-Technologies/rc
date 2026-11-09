package server

import (
	"net/http"
	"net/url"
	"strings"

	authstore "github.com/OhRats-Technologies/rc/pkg/auth"
)

const maxOAuthAuthorizeQuery = 8 << 10

// handleOAuthAuthorize validates an authorization-code + PKCE request and
// presents explicit account/scope consent. It never issues a code before the
// signed-in user approves the request.
func (s *Server) handleOAuthAuthorize(w http.ResponseWriter, req *http.Request) {
	if s.auth == nil {
		writeOAuthError(w, http.StatusServiceUnavailable, "temporarily_unavailable", "account store is unavailable")
		return
	}
	if len(req.URL.RawQuery) > maxOAuthAuthorizeQuery {
		http.Error(w, "authorization query is too large", http.StatusRequestURITooLong)
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
	client, err := s.auth.GetOAuthClient(clientID)
	if err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "client is invalid")
		return
	}
	if err := s.auth.ValidateRedirectURI(clientID, redirectURI); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "redirect URI is invalid")
		return
	}

	fail := func(code, description string) {
		s.redirectOAuthResult(w, req, redirectURI, url.Values{
			"error":             {code},
			"error_description": {description},
			"state":             {state},
			"iss":               {s.issuer(req)},
		})
	}
	if strings.TrimSpace(query.Get("response_type")) != "code" {
		fail("unsupported_response_type", "only the code response type is supported")
		return
	}
	challenge := strings.TrimSpace(query.Get("code_challenge"))
	if strings.TrimSpace(query.Get("code_challenge_method")) != "S256" ||
		len(challenge) < 43 || len(challenge) > 128 || !base64URLValue(challenge) {
		fail("invalid_request", "PKCE with a valid S256 code challenge is required")
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
	rawScope := query.Get("scope")
	if strings.TrimSpace(rawScope) == "" {
		// MCP clients commonly omit scope. Ask for both so the user can
		// explicitly opt into Terminal on the consent page; Terminal remains
		// unchecked and requires a fresh passkey before it is granted.
		rawScope = authstore.OAuthScopeObserve + " " + authstore.OAuthScopeTerminal
	}
	scopes, err := authstore.NormalizeOAuthScopes(rawScope)
	if err != nil {
		fail("invalid_scope", "one or more requested scopes are not supported")
		return
	}

	user, _, err := s.currentSession(req)
	if err != nil {
		next := req.URL.RequestURI()
		http.Redirect(w, req, "/login?next="+url.QueryEscape(next), http.StatusSeeOther)
		return
	}
	request, err := s.auth.CreateOAuthRequest(
		clientID, user.ID, redirectURI, state, challenge, resource, scopes,
	)
	if err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "authorization request is invalid")
		return
	}
	s.renderOAuthConsent(w, req, user, client.Name, request, "")
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

func base64URLValue(value string) bool {
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
