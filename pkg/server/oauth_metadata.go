package server

import (
	"encoding/json"
	"errors"
	"net/http"

	authstore "github.com/OhRats-Technologies/rc/pkg/auth"
)

func (s *Server) handleOAuthAuthorizationServerMetadata(w http.ResponseWriter, req *http.Request) {
	issuer := s.issuer(req)
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                                issuer,
		"authorization_endpoint":                issuer + "/oauth/authorize",
		"token_endpoint":                        issuer + "/oauth/token",
		"registration_endpoint":                 issuer + "/oauth/register",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"none"},
		"scopes_supported":                      []string{"mcp"},
	})
}

func (s *Server) handleOAuthProtectedResourceMetadata(w http.ResponseWriter, req *http.Request) {
	issuer := s.issuer(req)
	writeJSON(w, http.StatusOK, map[string]any{
		"resource":              s.mcpResource(req),
		"authorization_servers": []string{issuer},
		"bearer_methods_supported": []string{
			"header",
		},
	})
}

func (s *Server) handleOAuthRegister(w http.ResponseWriter, req *http.Request) {
	if s.auth == nil {
		writeOAuthError(w, http.StatusServiceUnavailable, "temporarily_unavailable", "account store is unavailable")
		return
	}
	req.Body = http.MaxBytesReader(w, req.Body, 64<<10)
	var input struct {
		ClientName   string   `json:"client_name"`
		RedirectURIs []string `json:"redirect_uris"`
	}
	if err := json.NewDecoder(req.Body).Decode(&input); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_client_metadata", "registration body is invalid")
		return
	}
	clientID, err := s.auth.RegisterOAuthClient(input.ClientName, input.RedirectURIs)
	if err != nil {
		switch {
		case errors.Is(err, authstore.ErrOAuthInvalidRedirect):
			writeOAuthError(w, http.StatusBadRequest, "invalid_redirect_uri",
				"redirect_uris must be absolute https or http loopback URIs without fragments")
		case errors.Is(err, authstore.ErrInvalidName):
			writeOAuthError(w, http.StatusBadRequest, "invalid_client_metadata", "client_name is invalid")
		default:
			writeOAuthError(w, http.StatusInternalServerError, "server_error", "registration failed")
		}
		return
	}
	client, err := s.auth.GetOAuthClient(clientID)
	if err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "registration failed")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"client_id":                  client.ID,
		"client_name":                client.Name,
		"redirect_uris":              client.RedirectURIs,
		"token_endpoint_auth_method": "none",
	})
}
