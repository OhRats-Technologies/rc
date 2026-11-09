package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

const oauthTestRedirect = "https://client.example/callback"

func TestOAuthAuthorizationCodeFlow(t *testing.T) {
	srv, ts, cookie, user := newOAuthTestServerWithUser(t)
	clientID := registerTestClient(t, ts)
	verifier := strings.Repeat("k", 48)
	state := "opaque-state-value"
	target := authorizeTarget(t, ts, clientID, verifier, state)

	// Without a passkey session the authorization request defers to login.
	res, _ := oauthDo(t, noRedirectClient(), oauthRequest(t, http.MethodGet, target, "", nil, nil, ""))
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("authorize without session = %d", res.StatusCode)
	}
	location, err := url.Parse(res.Header.Get("Location"))
	if err != nil || location.Path != "/login" {
		t.Fatalf("login redirect = %q (%v)", res.Header.Get("Location"), err)
	}
	if next := location.Query().Get("next"); !strings.Contains(next, "/oauth/authorize") {
		t.Fatalf("login next = %q", next)
	}

	// With a session the authorization server renders explicit consent.
	res, consent := oauthDo(t, noRedirectClient(), oauthRequest(t, http.MethodGet, target, "", nil, cookie, ""))
	if res.StatusCode != http.StatusOK || !bytes.Contains(consent, []byte("Connect Test MCP Client")) ||
		!bytes.Contains(consent, []byte(`value="mcp:terminal"`)) {
		t.Fatalf("authorize status = %d: %s", res.StatusCode, consent)
	}
	requestID := consentRequestID(t, consent)
	control := seedTestControl(t, srv, user.ID, ts.URL)
	res, payload := prepareAndApprove(t, ts, cookie, control, requestID, []string{"mcp:observe"}, "30d")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("approve status = %d: %s", res.StatusCode, payload)
	}
	var approved struct {
		Redirect string `json:"redirect"`
	}
	if err := json.Unmarshal(payload, &approved); err != nil {
		t.Fatal(err)
	}
	callback, err := url.Parse(approved.Redirect)
	if err != nil {
		t.Fatal(err)
	}
	if got := callback.Scheme + "://" + callback.Host + callback.Path; got != oauthTestRedirect {
		t.Fatalf("callback = %q", got)
	}
	code := callback.Query().Get("code")
	if code == "" || callback.Query().Get("state") != state || callback.Query().Get("iss") != "https://rc.example.test" {
		t.Fatalf("callback query = %q", callback.RawQuery)
	}

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {oauthTestRedirect},
		"client_id":     {clientID},
		"code_verifier": {verifier},
	}
	res, payload = oauthDo(t, http.DefaultClient, oauthRequest(t,
		http.MethodPost, ts.URL+"/oauth/token", "application/x-www-form-urlencoded",
		[]byte(form.Encode()), nil, ""))
	if res.StatusCode != http.StatusOK {
		t.Fatalf("token status = %d: %s", res.StatusCode, payload)
	}
	var tokens struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
		RefreshToken string `json:"refresh_token"`
		Scope        string `json:"scope"`
	}
	if err := json.Unmarshal(payload, &tokens); err != nil {
		t.Fatalf("decode tokens: %v", err)
	}
	if tokens.AccessToken == "" || tokens.RefreshToken == "" || tokens.TokenType != "Bearer" ||
		tokens.ExpiresIn != 900 || tokens.Scope != "mcp:observe" {
		t.Fatalf("tokens = %+v", tokens)
	}

	initBody := mustJSON(t, map[string]any{
		"jsonrpc": "2.0", "id": 0, "method": "initialize",
		"params": map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}},
	})
	res, payload = oauthDo(t, http.DefaultClient, oauthRequest(t,
		http.MethodPost, ts.URL+"/mcp", "application/json", initBody, nil, tokens.AccessToken))
	if res.StatusCode != http.StatusOK || !bytes.Contains(payload, []byte(`"protocolVersion":"2025-06-18"`)) {
		t.Fatalf("mcp initialize = %d: %s", res.StatusCode, payload)
	}

	toolsBody := mustJSON(t, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"})
	res, payload = oauthDo(t, http.DefaultClient, oauthRequest(t,
		http.MethodPost, ts.URL+"/mcp", "application/json", toolsBody, nil, ""))
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("mcp without token = %d: %s", res.StatusCode, payload)
	}
	wantChallenge := `Bearer resource_metadata="https://rc.example.test/.well-known/oauth-protected-resource", scope="mcp:observe"`
	if got := res.Header.Get("WWW-Authenticate"); got != wantChallenge {
		t.Fatalf("WWW-Authenticate = %q, want %q", got, wantChallenge)
	}
	res, payload = oauthDo(t, http.DefaultClient, oauthRequest(t,
		http.MethodPost, ts.URL+"/mcp", "application/json", toolsBody, nil, tokens.AccessToken))
	if res.StatusCode != http.StatusOK || !bytes.Contains(payload, []byte(`"machines_list"`)) ||
		bytes.Contains(payload, []byte(`"process_run"`)) {
		t.Fatalf("observe tools/list = %d: %s", res.StatusCode, payload)
	}

	callBody := mustJSON(t, map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "tools/call",
		"params": map[string]any{"name": "machines_list", "arguments": map[string]any{}},
	})
	res, payload = oauthDo(t, http.DefaultClient, oauthRequest(t,
		http.MethodPost, ts.URL+"/mcp", "application/json", callBody, nil, tokens.AccessToken))
	if res.StatusCode != http.StatusOK || !bytes.Contains(payload, []byte(`"structuredContent"`)) {
		t.Fatalf("mcp tools/call = %d: %s", res.StatusCode, payload)
	}

	mcpBody := mustJSON(t, map[string]any{"jsonrpc": "2.0", "id": 3, "method": "machines_list"})
	res, payload = oauthDo(t, http.DefaultClient, oauthRequest(t,
		http.MethodPost, ts.URL+"/mcp", "application/json", mcpBody, nil, tokens.AccessToken))
	if res.StatusCode != http.StatusOK {
		t.Fatalf("mcp with token = %d: %s", res.StatusCode, payload)
	}

	runBody := mustJSON(t, map[string]any{
		"jsonrpc": "2.0", "id": 3, "method": "tools/call",
		"params": map[string]any{
			"name":      "process_run",
			"arguments": map[string]any{"deviceId": "dev_missing", "argv": []string{"true"}},
		},
	})
	res, payload = oauthDo(t, http.DefaultClient, oauthRequest(t,
		http.MethodPost, ts.URL+"/mcp", "application/json", runBody, nil, tokens.AccessToken))
	if res.StatusCode != http.StatusForbidden ||
		!strings.Contains(res.Header.Get("WWW-Authenticate"), `error="insufficient_scope"`) ||
		!strings.Contains(res.Header.Get("WWW-Authenticate"), `scope="mcp:terminal"`) {
		t.Fatalf("observe process_run = %d: %s", res.StatusCode, payload)
	}

	// Refresh rotation replaces both tokens and rejects the old refresh token.
	refreshForm := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {tokens.RefreshToken},
		"client_id":     {clientID},
	}
	res, payload = oauthDo(t, http.DefaultClient, oauthRequest(t,
		http.MethodPost, ts.URL+"/oauth/token", "application/x-www-form-urlencoded",
		[]byte(refreshForm.Encode()), nil, ""))
	if res.StatusCode != http.StatusOK {
		t.Fatalf("refresh status = %d: %s", res.StatusCode, payload)
	}
	var rotated struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		Scope        string `json:"scope"`
	}
	if err := json.Unmarshal(payload, &rotated); err != nil {
		t.Fatalf("decode rotation: %v", err)
	}
	if rotated.AccessToken == "" || rotated.RefreshToken == "" ||
		rotated.RefreshToken == tokens.RefreshToken || rotated.AccessToken == tokens.AccessToken ||
		rotated.ExpiresIn != 900 || rotated.Scope != "mcp:observe" {
		t.Fatalf("rotated = %+v", rotated)
	}
	res, payload = oauthDo(t, http.DefaultClient, oauthRequest(t,
		http.MethodPost, ts.URL+"/mcp", "application/json", mcpBody, nil, rotated.AccessToken))
	if res.StatusCode != http.StatusOK {
		t.Fatalf("mcp with rotated token = %d: %s", res.StatusCode, payload)
	}

	res, payload = oauthDo(t, http.DefaultClient, oauthRequest(t,
		http.MethodPost, ts.URL+"/oauth/token", "application/x-www-form-urlencoded",
		[]byte(refreshForm.Encode()), nil, ""))
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("refresh reuse status = %d: %s", res.StatusCode, payload)
	}
	var oauthErr struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(payload, &oauthErr); err != nil || oauthErr.Error != "invalid_grant" {
		t.Fatalf("refresh reuse error = %q (%v)", oauthErr.Error, err)
	}
	_, err = srv.auth.LookupOAuthAccess(rotated.AccessToken, "https://rc.example.test/mcp")
	if err != nil {
		t.Fatalf("rotated access lookup: %v", err)
	}

	revokeForm := url.Values{"token": {rotated.AccessToken}}
	res, payload = oauthDo(t, http.DefaultClient, oauthRequest(t,
		http.MethodPost, ts.URL+"/oauth/revoke", "application/x-www-form-urlencoded",
		[]byte(revokeForm.Encode()), nil, ""))
	if res.StatusCode != http.StatusOK {
		t.Fatalf("revoke status = %d: %s", res.StatusCode, payload)
	}
	res, payload = oauthDo(t, http.DefaultClient, oauthRequest(t,
		http.MethodPost, ts.URL+"/mcp", "application/json", mcpBody, nil, rotated.AccessToken))
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("mcp after revocation = %d: %s", res.StatusCode, payload)
	}
}
