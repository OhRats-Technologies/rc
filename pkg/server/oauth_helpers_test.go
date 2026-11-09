package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	authstore "github.com/OhRats-Technologies/rc/pkg/auth"
)

func newOAuthTestServer(t *testing.T) (*Server, *httptest.Server, *http.Cookie) {
	t.Helper()
	srv, ts, cookie, _ := newOAuthTestServerWithUser(t)
	return srv, ts, cookie
}

func newOAuthTestServerWithUser(t *testing.T) (*Server, *httptest.Server, *http.Cookie, *authstore.User) {
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
	return srv, ts, &http.Cookie{Name: sessionCookieName, Value: token}, user
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
