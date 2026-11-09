package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestOAuthTerminalConsentUsesControlAuthority(t *testing.T) {
	srv, ts, cookie, user := newOAuthTestServerWithUser(t)
	clientID := registerTestClient(t, ts)
	verifier := strings.Repeat("t", 48)
	target := authorizeTarget(t, ts, clientID, verifier, "terminal-state")
	parsed, _ := url.Parse(target)
	query := parsed.Query()
	query.Set("scope", "mcp:observe mcp:terminal")
	parsed.RawQuery = query.Encode()

	res, body := oauthDo(t, noRedirectClient(), oauthRequest(t, http.MethodGet,
		parsed.String(), "", nil, cookie, ""))
	if res.StatusCode != http.StatusOK || !bytes.Contains(body, []byte(`value="mcp:terminal"`)) {
		t.Fatalf("terminal consent page = %d: %s", res.StatusCode, body)
	}
	requestID := consentRequestID(t, body)
	control := seedTestControl(t, srv, user.ID, ts.URL)
	res, payload := prepareAndApprove(t, ts, cookie, control, requestID,
		[]string{"mcp:observe", "mcp:terminal"}, "30d")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("terminal approve = %d: %s", res.StatusCode, payload)
	}
	var approved struct {
		Redirect     string   `json:"redirect"`
		GrantID      string   `json:"grantId"`
		RequiresSync bool     `json:"requiresSync"`
		WorkspaceIDs []string `json:"workspaceIds"`
	}
	if err := json.Unmarshal(payload, &approved); err != nil {
		t.Fatal(err)
	}
	if approved.Redirect == "" || approved.GrantID == "" || !approved.RequiresSync ||
		len(approved.WorkspaceIDs) == 0 {
		t.Fatalf("terminal approval = %+v", approved)
	}
	callback, err := url.Parse(approved.Redirect)
	if err != nil {
		t.Fatal(err)
	}
	code := callback.Query().Get("code")
	if code == "" {
		t.Fatalf("terminal callback = %q", approved.Redirect)
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
		t.Fatalf("terminal token = %d: %s", res.StatusCode, payload)
	}
	var tokens struct {
		AccessToken string `json:"access_token"`
		Scope       string `json:"scope"`
	}
	_ = json.Unmarshal(payload, &tokens)
	if tokens.Scope != "mcp:observe mcp:terminal" {
		t.Fatalf("terminal token scope = %q", tokens.Scope)
	}

	deviceID := startMCPTestNode(t, srv, ts)
	waitTestNodeLock(t, srv, deviceID)
	toolsBody := mustJSON(t, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"})
	res, payload = oauthDo(t, http.DefaultClient, oauthRequest(t,
		http.MethodPost, ts.URL+"/mcp", "application/json", toolsBody, nil, tokens.AccessToken))
	if res.StatusCode != http.StatusOK || !bytes.Contains(payload, []byte(`"process_run"`)) {
		t.Fatalf("terminal tools/list = %d: %s", res.StatusCode, payload)
	}
	runBody := mustJSON(t, map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "tools/call",
		"params": map[string]any{
			"name": "process_run",
			"arguments": map[string]any{
				"deviceId": deviceID, "argv": mcpHelperArgv(), "env": mcpHelperEnv("echo"),
				"waitSeconds": 5,
			},
		},
	})
	res, payload = oauthDo(t, http.DefaultClient, oauthRequest(t,
		http.MethodPost, ts.URL+"/mcp", "application/json", runBody, nil, tokens.AccessToken))
	if res.StatusCode != http.StatusOK || !bytes.Contains(payload, []byte("mcp-wire-ok")) {
		t.Fatalf("terminal process_run = %d: %s", res.StatusCode, payload)
	}
}

func TestOAuthConsentCancelRedirectsWithAccessDenied(t *testing.T) {
	_, ts, cookie := newOAuthTestServer(t)
	clientID := registerTestClient(t, ts)
	verifier := strings.Repeat("c", 48)
	target := authorizeTarget(t, ts, clientID, verifier, "cancel-state")
	res, body := oauthDo(t, noRedirectClient(), oauthRequest(t, http.MethodGet, target, "", nil, cookie, ""))
	if res.StatusCode != http.StatusOK {
		t.Fatalf("consent status = %d: %s", res.StatusCode, body)
	}
	requestID := consentRequestID(t, body)
	form := url.Values{"request_id": {requestID}}
	req := oauthRequest(t, http.MethodPost, ts.URL+"/oauth/authorize/cancel",
		"application/x-www-form-urlencoded", []byte(form.Encode()), cookie, "")
	res, body = oauthDo(t, noRedirectClient(), req)
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("cancel status = %d: %s", res.StatusCode, body)
	}
	callback, err := url.Parse(res.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	query := callback.Query()
	if query.Get("error") != "access_denied" || query.Get("state") != "cancel-state" ||
		query.Get("iss") != "https://rc.example.test" {
		t.Fatalf("cancel redirect = %q", res.Header.Get("Location"))
	}
}

func consentRequestID(t *testing.T, body []byte) string {
	t.Helper()
	const marker = `data-mcp-request="`
	index := bytes.Index(body, []byte(marker))
	if index < 0 {
		t.Fatalf("consent request marker missing: %s", body)
	}
	start := index + len(marker)
	end := bytes.IndexByte(body[start:], '"')
	if end < 0 {
		t.Fatalf("consent request marker is malformed: %s", body)
	}
	return string(body[start : start+end])
}

func prepareAndApprove(t *testing.T, ts *httptest.Server, cookie *http.Cookie,
	control *testControl, requestID string, scopes []string, lifetime string,
) (*http.Response, []byte) {
	t.Helper()
	prepareBody, _ := json.Marshal(map[string]any{
		"requestId": requestID, "audience": "account", "scopes": scopes, "lifetime": lifetime,
	})
	req := oauthRequest(t, http.MethodPost, ts.URL+"/oauth/authorize/prepare",
		"application/json", prepareBody, cookie, "")
	res, payload := oauthDo(t, noRedirectClient(), req)
	if res.StatusCode != http.StatusOK {
		return res, payload
	}
	var prepared struct {
		Grant            string `json:"grant"`
		SignaturePayload string `json:"signaturePayload"`
	}
	if err := json.Unmarshal(payload, &prepared); err != nil {
		t.Fatal(err)
	}
	approveBody, _ := json.Marshal(map[string]any{
		"requestId": requestID, "controlClientId": control.ID,
		"signature": control.sign(prepared.SignaturePayload),
	})
	return oauthDo(t, noRedirectClient(), oauthRequest(t, http.MethodPost,
		ts.URL+"/oauth/authorize/approve", "application/json", approveBody, cookie, ""))
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
