package server

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestOAuthMetadata(t *testing.T) {
	_, ts, _ := newOAuthTestServer(t)

	res, payload := oauthDo(t, http.DefaultClient, oauthRequest(t,
		http.MethodGet, ts.URL+"/.well-known/oauth-authorization-server", "", nil, nil, ""))
	if res.StatusCode != http.StatusOK {
		t.Fatalf("metadata status = %d", res.StatusCode)
	}
	if got := res.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("metadata content type = %q", got)
	}
	var metadata struct {
		Issuer                        string   `json:"issuer"`
		AuthorizationEndpoint         string   `json:"authorization_endpoint"`
		TokenEndpoint                 string   `json:"token_endpoint"`
		RevocationEndpoint            string   `json:"revocation_endpoint"`
		RegistrationEndpoint          string   `json:"registration_endpoint"`
		ResponseTypesSupported        []string `json:"response_types_supported"`
		GrantTypesSupported           []string `json:"grant_types_supported"`
		CodeChallengeMethodsSupported []string `json:"code_challenge_methods_supported"`
		TokenEndpointAuthMethods      []string `json:"token_endpoint_auth_methods_supported"`
		ScopesSupported               []string `json:"scopes_supported"`
		IssSupported                  bool     `json:"authorization_response_iss_parameter_supported"`
	}
	if err := json.Unmarshal(payload, &metadata); err != nil {
		t.Fatalf("decode metadata: %v", err)
	}
	if metadata.Issuer != "https://rc.example.test" ||
		metadata.AuthorizationEndpoint != metadata.Issuer+"/oauth/authorize" ||
		metadata.TokenEndpoint != metadata.Issuer+"/oauth/token" ||
		metadata.RevocationEndpoint != metadata.Issuer+"/oauth/revoke" ||
		metadata.RegistrationEndpoint != metadata.Issuer+"/oauth/register" {
		t.Fatalf("metadata endpoints = %+v", metadata)
	}
	if len(metadata.ResponseTypesSupported) != 1 || metadata.ResponseTypesSupported[0] != "code" ||
		len(metadata.GrantTypesSupported) != 2 ||
		metadata.GrantTypesSupported[0] != "authorization_code" ||
		metadata.GrantTypesSupported[1] != "refresh_token" ||
		len(metadata.CodeChallengeMethodsSupported) != 1 ||
		metadata.CodeChallengeMethodsSupported[0] != "S256" ||
		len(metadata.TokenEndpointAuthMethods) != 1 ||
		metadata.TokenEndpointAuthMethods[0] != "none" ||
		len(metadata.ScopesSupported) != 2 ||
		metadata.ScopesSupported[0] != "mcp:observe" ||
		metadata.ScopesSupported[1] != "mcp:terminal" || !metadata.IssSupported {
		t.Fatalf("metadata capabilities = %+v", metadata)
	}

	res, payload = oauthDo(t, http.DefaultClient, oauthRequest(t,
		http.MethodGet, ts.URL+"/.well-known/oauth-protected-resource", "", nil, nil, ""))
	if res.StatusCode != http.StatusOK {
		t.Fatalf("protected resource metadata status = %d", res.StatusCode)
	}
	var resource struct {
		Resource               string   `json:"resource"`
		AuthorizationServers   []string `json:"authorization_servers"`
		BearerMethodsSupported []string `json:"bearer_methods_supported"`
		ScopesSupported        []string `json:"scopes_supported"`
	}
	if err := json.Unmarshal(payload, &resource); err != nil {
		t.Fatalf("decode protected resource: %v", err)
	}
	res, payload = oauthDo(t, http.DefaultClient, oauthRequest(t,
		http.MethodGet, ts.URL+"/.well-known/oauth-protected-resource/mcp", "", nil, nil, ""))
	if res.StatusCode != http.StatusOK || !strings.Contains(string(payload), `"resource":"https://rc.example.test/mcp"`) {
		t.Fatalf("resource-specific metadata = %d: %s", res.StatusCode, payload)
	}
	if resource.Resource != "https://rc.example.test/mcp" ||
		len(resource.AuthorizationServers) != 1 ||
		resource.AuthorizationServers[0] != "https://rc.example.test" ||
		len(resource.BearerMethodsSupported) != 1 ||
		resource.BearerMethodsSupported[0] != "header" ||
		len(resource.ScopesSupported) != 2 ||
		resource.ScopesSupported[0] != "mcp:observe" ||
		resource.ScopesSupported[1] != "mcp:terminal" {
		t.Fatalf("protected resource = %+v", resource)
	}
}

func TestOAuthRegistrationValidation(t *testing.T) {
	_, ts, _ := newOAuthTestServer(t)

	cases := []struct {
		name string
		body []byte
	}{
		{"empty redirects", []byte(`{"client_name":"Bad","redirect_uris":[]}`)},
		{"insecure redirect", []byte(`{"client_name":"Bad","redirect_uris":["http://example.com/callback"]}`)},
		{"fragment redirect", []byte(`{"client_name":"Bad","redirect_uris":["https://example.com/callback#fragment"]}`)},
		{"malformed json", []byte(`{"client_name":`)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, payload := oauthDo(t, http.DefaultClient, oauthRequest(t,
				http.MethodPost, ts.URL+"/oauth/register", "application/json", tc.body, nil, ""))
			if res.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d: %s", res.StatusCode, payload)
			}
			var oauthErr struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(payload, &oauthErr); err != nil || oauthErr.Error == "" {
				t.Fatalf("error body = %s (%v)", payload, err)
			}
		})
	}
}

func TestOAuthAuthorizationRejectsUnknownClient(t *testing.T) {
	_, ts, cookie := newOAuthTestServer(t)
	target := authorizeTarget(t, ts, "unknown-client", strings.Repeat("k", 48), "")
	res, payload := oauthDo(t, noRedirectClient(), oauthRequest(t,
		http.MethodGet, target, "", nil, cookie, ""))
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown client status = %d: %s", res.StatusCode, payload)
	}

	clientID := registerTestClient(t, ts)
	target = authorizeTarget(t, ts, clientID, strings.Repeat("k", 48), "state")
	parsed, _ := url.Parse(target)
	query := parsed.Query()
	query.Set("response_type", "token")
	parsed.RawQuery = query.Encode()
	res, _ = oauthDo(t, noRedirectClient(), oauthRequest(t,
		http.MethodGet, parsed.String(), "", nil, cookie, ""))
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("unsupported response type status = %d", res.StatusCode)
	}
	callback, err := url.Parse(res.Header.Get("Location"))
	if err != nil || callback.Query().Get("error") != "unsupported_response_type" {
		t.Fatalf("error redirect = %q (%v)", res.Header.Get("Location"), err)
	}
	if callback.Query().Get("state") != "state" {
		t.Fatalf("state not preserved: %q", callback.RawQuery)
	}
}

func TestOAuthTokenRejectsInvalidClient(t *testing.T) {
	_, ts, _ := newOAuthTestServer(t)
	form := url.Values{"grant_type": {"authorization_code"}, "code": {"x"}}
	res, payload := oauthDo(t, http.DefaultClient, oauthRequest(t,
		http.MethodPost, ts.URL+"/oauth/token", "application/x-www-form-urlencoded",
		[]byte(form.Encode()), nil, ""))
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("missing client status = %d: %s", res.StatusCode, payload)
	}
	var oauthErr struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(payload, &oauthErr); err != nil || oauthErr.Error != "invalid_client" {
		t.Fatalf("missing client error = %s", payload)
	}
}
