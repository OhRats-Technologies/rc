package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

const oauthTestRedirect = "https://client.example/callback"

func newOAuthTestServer(t *testing.T) (*Server, *httptest.Server, *http.Cookie) {
	t.Helper()
	srv, ts := newAuthServer(t)
	user, err := srv.auth.CreateUser("Alice")
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := srv.auth.IssueSession(user.ID, "1h")
	if err != nil {
		t.Fatal(err)
	}
	return srv, ts, &http.Cookie{Name: sessionCookieName, Value: token}
}

func noRedirectClient() *http.Client {
	return &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func oauthRequest(t *testing.T, method, target, contentType string, body []byte, cookie *http.Cookie, bearer string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, target, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	return req
}

func oauthDo(t *testing.T, client *http.Client, req *http.Request) (*http.Response, []byte) {
	t.Helper()
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer res.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return res, payload
}

func registerTestClient(t *testing.T, ts *httptest.Server) string {
	t.Helper()
	body := mustJSON(t, map[string]any{
		"client_name":   "Test MCP Client",
		"redirect_uris": []string{oauthTestRedirect},
	})
	res, payload := oauthDo(t, http.DefaultClient, oauthRequest(t,
		http.MethodPost, ts.URL+"/oauth/register", "application/json", body, nil, ""))
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("register status = %d: %s", res.StatusCode, payload)
	}
	var created struct {
		ClientID                string   `json:"client_id"`
		ClientName              string   `json:"client_name"`
		RedirectURIs            []string `json:"redirect_uris"`
		TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	}
	if err := json.Unmarshal(payload, &created); err != nil {
		t.Fatalf("decode registration: %v", err)
	}
	if created.ClientID == "" || created.ClientName != "Test MCP Client" ||
		created.TokenEndpointAuthMethod != "none" ||
		len(created.RedirectURIs) != 1 || created.RedirectURIs[0] != oauthTestRedirect {
		t.Fatalf("registration = %+v", created)
	}
	return created.ClientID
}

func pkcePair(verifier string) string {
	digest := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func authorizeTarget(t *testing.T, ts *httptest.Server, clientID, verifier, state string) string {
	t.Helper()
	target, err := url.Parse(ts.URL + "/oauth/authorize")
	if err != nil {
		t.Fatal(err)
	}
	query := target.Query()
	query.Set("response_type", "code")
	query.Set("client_id", clientID)
	query.Set("redirect_uri", oauthTestRedirect)
	query.Set("code_challenge", pkcePair(verifier))
	query.Set("code_challenge_method", "S256")
	query.Set("state", state)
	target.RawQuery = query.Encode()
	return target.String()
}

func TestOAuthAuthorizationCodeFlow(t *testing.T) {
	srv, ts, cookie := newOAuthTestServer(t)
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

	// With a session the authorization server returns a code to the client.
	res, _ = oauthDo(t, noRedirectClient(), oauthRequest(t, http.MethodGet, target, "", nil, cookie, ""))
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("authorize status = %d", res.StatusCode)
	}
	callback, err := url.Parse(res.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if got := callback.Scheme + "://" + callback.Host + callback.Path; got != oauthTestRedirect {
		t.Fatalf("callback = %q", got)
	}
	code := callback.Query().Get("code")
	if code == "" || callback.Query().Get("state") != state {
		t.Fatalf("callback query = %q", callback.RawQuery)
	}

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {oauthTestRedirect},
		"client_id":     {clientID},
		"code_verifier": {verifier},
	}
	res, payload := oauthDo(t, http.DefaultClient, oauthRequest(t,
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
		tokens.ExpiresIn != 900 || tokens.Scope != "mcp" {
		t.Fatalf("tokens = %+v", tokens)
	}

	mcpBody := mustJSON(t, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "machines_list"})
	res, payload = oauthDo(t, http.DefaultClient, oauthRequest(t,
		http.MethodPost, ts.URL+"/mcp", "application/json", mcpBody, nil, ""))
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("mcp without token = %d: %s", res.StatusCode, payload)
	}
	wantChallenge := `Bearer resource_metadata="https://rc.example.test/.well-known/oauth-protected-resource"`
	if got := res.Header.Get("WWW-Authenticate"); got != wantChallenge {
		t.Fatalf("WWW-Authenticate = %q, want %q", got, wantChallenge)
	}
	res, payload = oauthDo(t, http.DefaultClient, oauthRequest(t,
		http.MethodPost, ts.URL+"/mcp", "application/json", mcpBody, nil, tokens.AccessToken))
	if res.StatusCode != http.StatusOK {
		t.Fatalf("mcp with token = %d: %s", res.StatusCode, payload)
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
		rotated.ExpiresIn != 900 || rotated.Scope != "mcp" {
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
}

func TestDevelopmentBypassesOAuth(t *testing.T) {
	_, ts := newDevServer(t)
	body := mustJSON(t, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "machines_list"})
	res, payload := oauthDo(t, http.DefaultClient, oauthRequest(t,
		http.MethodPost, ts.URL+"/mcp", "application/json", body, nil, ""))
	if res.StatusCode != http.StatusOK {
		t.Fatalf("development mcp = %d: %s", res.StatusCode, payload)
	}
}
