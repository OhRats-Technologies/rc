package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	authstore "github.com/OhRats-Technologies/rc/pkg/auth"
	"github.com/OhRats-Technologies/rc/pkg/server/pages"
	"github.com/OhRats-Technologies/rc/pkg/version"
)

func (s *Server) renderOAuthConsent(
	w http.ResponseWriter,
	req *http.Request,
	user *authstore.User,
	clientName string,
	request *authstore.OAuthRequest,
	message string,
) {
	body := pages.MCPConsent(pages.MCPConsentOptions{
		PublicURL:  s.cfg.PublicURL,
		Version:    version.Version,
		RequestID:  request.ID,
		ClientName: clientName,
		UserName:   user.Name,
		Callback:   request.RedirectURI,
		Scopes:     request.Scopes,
		Error:      message,
	})
	s.writePage(w, http.StatusOK, body, "no-store")
}

type oauthConsentInput struct {
	RequestID string   `json:"requestId"`
	Scopes    []string `json:"scopes"`
	Lifetime  string   `json:"lifetime,omitempty"`
}

func (s *Server) handleOAuthCancel(w http.ResponseWriter, req *http.Request) {
	if s.auth == nil {
		writeOAuthError(w, http.StatusServiceUnavailable, "temporarily_unavailable", "account store is unavailable")
		return
	}
	user, _, err := s.currentSession(req)
	if err != nil {
		writeOAuthError(w, http.StatusUnauthorized, "login_required", "sign in to cancel MCP access")
		return
	}
	jsonRequest := requestMediaType(req) == "application/json"
	input, err := readOAuthConsentInput(req, jsonRequest)
	if err != nil || input.RequestID == "" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "consent request is invalid")
		return
	}
	request, err := s.auth.TakeOAuthRequest(input.RequestID, user.ID)
	if err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "authorization request is expired or invalid")
		return
	}
	values := url.Values{
		"error":             {"access_denied"},
		"error_description": {"The user declined this MCP authorization request."},
		"state":             {request.State},
		"iss":               {s.issuer(req)},
	}
	if jsonRequest {
		target, err := oauthRedirectURL(request.RedirectURI, values)
		if err != nil {
			writeOAuthError(w, http.StatusBadRequest, "invalid_request", "redirect URI is invalid")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"redirect": target})
		return
	}
	s.redirectOAuthResult(w, req, request.RedirectURI, values)
}

func readOAuthConsentInput(req *http.Request, jsonRequest bool) (oauthConsentInput, error) {
	if !jsonRequest {
		req.Body = io.NopCloser(io.LimitReader(req.Body, 64<<10))
		if err := req.ParseForm(); err != nil {
			return oauthConsentInput{}, err
		}
		return oauthConsentInput{
			RequestID: req.FormValue("request_id"),
			Scopes:    req.Form["scope"],
		}, nil
	}
	defer req.Body.Close()
	var input oauthConsentInput
	decoder := json.NewDecoder(io.LimitReader(req.Body, 64<<10))
	if err := decoder.Decode(&input); err != nil {
		return oauthConsentInput{}, err
	}
	return input, nil
}

func normalizeSelectedScopes(selected, requested []string) ([]string, error) {
	scopes, err := authstore.NormalizeOAuthScopes(strings.Join(selected, " "))
	if err != nil || len(selected) == 0 {
		return nil, authstore.ErrOAuthInvalidScope
	}
	for _, scope := range scopes {
		if !containsScope(requested, scope) {
			return nil, authstore.ErrOAuthInvalidScope
		}
	}
	return scopes, nil
}

func containsScope(scopes []string, scope string) bool {
	for _, candidate := range scopes {
		if candidate == scope {
			return true
		}
	}
	return false
}

func oauthRedirectURL(redirectURI string, values url.Values) (string, error) {
	target, err := url.Parse(redirectURI)
	if err != nil {
		return "", err
	}
	query := target.Query()
	for key, entries := range values {
		for _, value := range entries {
			if value != "" {
				query.Set(key, value)
			}
		}
	}
	target.RawQuery = query.Encode()
	return target.String(), nil
}
