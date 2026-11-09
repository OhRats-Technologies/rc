package server

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	authstore "github.com/OhRats-Technologies/rc/pkg/auth"
	"github.com/OhRats-Technologies/rc/pkg/crypto"
	"github.com/OhRats-Technologies/rc/pkg/protocol"
)

func testAPIKeyMaterial(t *testing.T, id string, scopes []string, expires time.Time) (authstore.APIKey, ed25519.PrivateKey) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return authstore.APIKey{
		ID: id, Name: id, PublicKey: base64.RawURLEncoding.EncodeToString(public),
		Scopes: scopes, CreatedAt: time.Now().UTC(), ExpiresAt: expires,
	}, private
}

func signedAPIRequest(t *testing.T, method, target string, body []byte,
	keyID string, private ed25519.PrivateKey, nonce string,
) *http.Request {
	t.Helper()
	request, err := http.NewRequest(method, target, strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	timestamp := time.Now().Unix()
	signature := base64.RawURLEncoding.EncodeToString(ed25519.Sign(private,
		[]byte(crypto.ApiPayload(keyID, strconv.FormatInt(timestamp, 10), nonce, method,
			request.URL.RequestURI(), body))))
	request.Header.Set("X-RC-Key-ID", keyID)
	request.Header.Set("X-RC-Timestamp", strconv.FormatInt(timestamp, 10))
	request.Header.Set("X-RC-Nonce", nonce)
	request.Header.Set("X-RC-Signature", signature)
	return request
}

func TestAuthoritySnapshotProjectsActiveAPIKeys(t *testing.T) {
	srv, _ := newAuthServer(t)
	user, err := srv.auth.CreateUser("API Projection")
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.auth.EnsureWorkspaceMembership(defaultWorkspaceID, user.ID, "owner"); err != nil {
		t.Fatal(err)
	}
	activeB, _ := testAPIKeyMaterial(t, "key_b", []string{"execute", "manage-devices"}, time.Time{})
	activeB.UserID = user.ID
	activeA, _ := testAPIKeyMaterial(t, "key_a", []string{"execute"}, time.Time{})
	activeA.UserID = user.ID
	expired, _ := testAPIKeyMaterial(t, "key_expired", []string{"execute"}, time.Now().Add(-time.Minute))
	expired.UserID = user.ID
	for _, key := range []authstore.APIKey{activeB, activeA, expired} {
		if err := srv.auth.PutAPIKey(key); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := srv.authoritySnapshot(defaultWorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot protocol.AuthoritySnapshot
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
		t.Fatal(err)
	}
	if len(snapshot.ApiKeys) != 2 || snapshot.ApiKeys[0].ID != "key_a" ||
		snapshot.ApiKeys[1].ID != "key_b" || snapshot.ApiKeys[1].ExpiresAt != 0 ||
		len(snapshot.ApiKeys[1].Scopes) != 2 {
		t.Fatalf("api key projection = %+v", snapshot.ApiKeys)
	}
	for _, id := range []string{"key_a", "key_b"} {
		if _, err := srv.auth.DeleteAPIKey(user.ID, id); err != nil {
			t.Fatal(err)
		}
	}
	raw, err = srv.authoritySnapshot(defaultWorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	var afterRevoke protocol.AuthoritySnapshot
	if err := json.Unmarshal([]byte(raw), &afterRevoke); err != nil {
		t.Fatal(err)
	}
	if len(afterRevoke.ApiKeys) != 0 {
		t.Fatalf("revoked key remained in snapshot: %+v", afterRevoke.ApiKeys)
	}
}

func TestAPIKeyHTTPAuthenticationRejectsExpiredRevokedAndReplay(t *testing.T) {
	srv, ts := newAuthServer(t)
	user, err := srv.auth.CreateUser("API Auth")
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.auth.EnsureWorkspaceMembership(defaultWorkspaceID, user.ID, "owner"); err != nil {
		t.Fatal(err)
	}
	active, private := testAPIKeyMaterial(t, "key_active", []string{"read", "execute"}, time.Time{})
	active.UserID = user.ID
	if err := srv.auth.PutAPIKey(active); err != nil {
		t.Fatal(err)
	}
	target := ts.URL + "/api/v1/devices"
	request := signedAPIRequest(t, http.MethodGet, target, nil, active.ID, private, "nonce-active-0000001")
	res, payload := oauthDo(t, http.DefaultClient, request)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("active API key = %d: %s", res.StatusCode, payload)
	}
	meRequest := signedAPIRequest(t, http.MethodGet, ts.URL+"/api/v1/me", nil, active.ID, private, "nonce-me-00000001")
	res, payload = oauthDo(t, http.DefaultClient, meRequest)
	if res.StatusCode != http.StatusOK || !strings.Contains(string(payload), `"role":"owner"`) {
		t.Fatalf("API key /me = %d: %s", res.StatusCode, payload)
	}
	replay := signedAPIRequest(t, http.MethodGet, target, nil, active.ID, private, "nonce-active-0000001")
	res, payload = oauthDo(t, http.DefaultClient, replay)
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("replayed API key = %d: %s", res.StatusCode, payload)
	}

	expired, expiredPrivate := testAPIKeyMaterial(t, "key_expired", []string{"read"}, time.Now().Add(-time.Minute))
	expired.UserID = user.ID
	if err := srv.auth.PutAPIKey(expired); err != nil {
		t.Fatal(err)
	}
	request = signedAPIRequest(t, http.MethodGet, target, nil, expired.ID, expiredPrivate, "nonce-expired-000001")
	res, payload = oauthDo(t, http.DefaultClient, request)
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expired API key = %d: %s", res.StatusCode, payload)
	}

	manageOnly, managePrivate := testAPIKeyMaterial(t, "key_manage", []string{"manage-devices"}, time.Time{})
	manageOnly.UserID = user.ID
	if err := srv.auth.PutAPIKey(manageOnly); err != nil {
		t.Fatal(err)
	}
	request = signedAPIRequest(t, http.MethodGet, target, nil, manageOnly.ID, managePrivate, "nonce-scope-0000001")
	res, payload = oauthDo(t, http.DefaultClient, request)
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("scope-restricted API key = %d: %s", res.StatusCode, payload)
	}
	enroll := signedAPIRequest(t, http.MethodPost,
		ts.URL+"/api/v1/workspaces/ws_default/enrollments", []byte("{}"),
		manageOnly.ID, managePrivate, "nonce-enroll-0000001")
	enroll.Header.Set("Content-Type", "application/json")
	res, payload = oauthDo(t, http.DefaultClient, enroll)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("manage-devices API key enrollment = %d: %s", res.StatusCode, payload)
	}

	if _, err := srv.auth.DeleteAPIKey(user.ID, active.ID); err != nil {
		t.Fatal(err)
	}
	request = signedAPIRequest(t, http.MethodGet, target, nil, active.ID, private, "nonce-revoked-0000001")
	res, payload = oauthDo(t, http.DefaultClient, request)
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked API key = %d: %s", res.StatusCode, payload)
	}
}

func TestAPITokenRoutesRequireFreshPasskeyAndRevoke(t *testing.T) {
	srv, ts, cookie, user := newOAuthTestServerWithUser(t)
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{
		"name": "Automation", "scopes": []string{"read"},
		"publicKey": base64.RawURLEncoding.EncodeToString(public), "lifetime": "30d",
	})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/tokens", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	res, payload := oauthDo(t, http.DefaultClient, req)
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("token create without step-up = %d: %s", res.StatusCode, payload)
	}
	step, _, err := srv.auth.IssueSession(user.ID, "5m")
	if err != nil {
		t.Fatal(err)
	}
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/v1/tokens", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	req.AddCookie(&http.Cookie{Name: stepUpCookieName, Value: step})
	res, payload = oauthDo(t, http.DefaultClient, req)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("token create = %d: %s", res.StatusCode, payload)
	}
	var created struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(payload, &created)
	if created.ID == "" {
		t.Fatalf("created token = %s", payload)
	}
	step, _, err = srv.auth.IssueSession(user.ID, "5m")
	if err != nil {
		t.Fatal(err)
	}
	req, _ = http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/tokens/"+created.ID, nil)
	req.AddCookie(cookie)
	req.AddCookie(&http.Cookie{Name: stepUpCookieName, Value: step})
	res, payload = oauthDo(t, http.DefaultClient, req)
	if res.StatusCode != http.StatusOK || !strings.Contains(string(payload), "workspaceIds") {
		t.Fatalf("token delete = %d: %s", res.StatusCode, payload)
	}
	if _, err := srv.auth.APIKey(created.ID); err == nil {
		t.Fatal("revoked token remained stored")
	}
}

func TestAPIPageRendersKeyManagement(t *testing.T) {
	_, ts, cookie, _ := newOAuthTestServerWithUser(t)
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api", nil)
	req.AddCookie(cookie)
	res, payload := oauthDo(t, http.DefaultClient, req)
	if res.StatusCode != http.StatusOK || !strings.Contains(string(payload), "data-api-key-dialog") ||
		!strings.Contains(string(payload), `id="token-list"`) {
		t.Fatalf("API page = %d: %s", res.StatusCode, payload)
	}
}
