package integration

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	authstore "github.com/OhRats-Technologies/rc/pkg/auth"
	"github.com/OhRats-Technologies/rc/pkg/crypto"
)

func domainRequest(t *testing.T, method, url, body string, cookies ...*http.Cookie) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer res.Body.Close()
	var payload bytes.Buffer
	_, _ = payload.ReadFrom(res.Body)
	return res, payload.Bytes()
}

func seedDomainServer(t *testing.T) (string, *authstore.User, *authstore.User, string, string, string, string) {
	t.Helper()
	dataDir := t.TempDir()
	store, err := authstore.Open(filepath.Join(dataDir, "rc.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	owner, err := store.CreateUser("Domain Owner")
	if err != nil {
		t.Fatal(err)
	}
	member, err := store.CreateUser("Domain Member")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureWorkspace("ws_domain", "Domain"); err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureWorkspaceMembership("ws_domain", owner.ID, "owner"); err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureWorkspaceMembership("ws_domain", member.ID, "viewer"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddPasskey(owner.ID, "Domain Passkey", []byte("domain-credential"), []byte{0xa1, 0x01, 0x01}, authstore.AlgorithmES256); err != nil {
		t.Fatal(err)
	}
	passkeys, err := store.PasskeysForUser(owner.ID)
	if err != nil || len(passkeys) != 1 {
		t.Fatalf("passkey domain = %+v, %v", passkeys, err)
	}
	if found, err := store.PasskeyByCredentialID([]byte("domain-credential")); err != nil || found.UserID != owner.ID {
		t.Fatalf("passkey lookup = %+v, %v", found, err)
	}
	device := authstore.Device{
		ID: "dev_domain", WorkspaceID: "ws_domain", Name: "Domain Node",
		Hostname: "domain-host", Platform: "linux", Arch: "amd64", Version: "0.20.6",
		IdentityPublicKey: "domain-identity", TransportPublicKey: "domain-transport",
		CreatedAt: time.Now().UTC(),
	}
	if err := store.InsertDevice(device); err != nil {
		t.Fatal(err)
	}
	if err := store.RenameDevice(device.ID, "Renamed Node"); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordAuditEvent(authstore.AuditEvent{
		WorkspaceID: "ws_domain", UserID: owner.ID, Kind: "workspace.member.role",
		Detail: `{"memberId":"` + member.ID + `","name":"Domain Member","role":"operator"}`,
	}); err != nil {
		t.Fatal(err)
	}
	ownerToken, _, err := store.IssueSession(owner.ID, "1h")
	if err != nil {
		t.Fatal(err)
	}
	memberToken, _, err := store.IssueSession(member.ID, "1h")
	if err != nil {
		t.Fatal(err)
	}
	roleStep, _, err := store.IssueSession(owner.ID, "5m")
	if err != nil {
		t.Fatal(err)
	}
	removeStep, _, err := store.IssueSession(owner.ID, "5m")
	if err != nil {
		t.Fatal(err)
	}
	return dataDir, owner, member, ownerToken, memberToken, roleStep, removeStep
}

func TestServerDomainAuthorityStoresEventsAndSessions(t *testing.T) {
	dataDir, _, member, ownerToken, memberToken, roleStep, removeStep := seedDomainServer(t)
	srv := startServer(t, "RC_DATA_DIR="+dataDir, "PUBLIC_URL=https://rc.example.test")
	ownerCookie := &http.Cookie{Name: "rc_session", Value: ownerToken}
	memberCookie := &http.Cookie{Name: "rc_session", Value: memberToken}
	roleStepCookie := &http.Cookie{Name: "rc_stepup", Value: roleStep}
	removeStepCookie := &http.Cookie{Name: "rc_stepup", Value: removeStep}

	res, payload := domainRequest(t, http.MethodGet, srv.baseURL+"/api/v1/me", "", ownerCookie)
	if res.StatusCode != http.StatusOK || !bytes.Contains(payload, []byte("ws_domain")) {
		t.Fatalf("session /me = %d: %s", res.StatusCode, payload)
	}
	res, payload = domainRequest(t, http.MethodGet, srv.baseURL+"/api/v1/devices", "", ownerCookie)
	if res.StatusCode != http.StatusOK || !bytes.Contains(payload, []byte("Renamed Node")) {
		t.Fatalf("device store = %d: %s", res.StatusCode, payload)
	}
	res, payload = domainRequest(t, http.MethodGet, srv.baseURL+"/api/v1/workspaces", "", ownerCookie)
	if res.StatusCode != http.StatusOK || !bytes.Contains(payload, []byte("Domain")) {
		t.Fatalf("workspace store = %d: %s", res.StatusCode, payload)
	}
	res, payload = domainRequest(t, http.MethodGet, srv.baseURL+"/api/v1/workspaces/ws_domain/access", "", ownerCookie)
	if res.StatusCode != http.StatusOK || !bytes.Contains(payload, []byte(member.ID)) {
		t.Fatalf("workspace access = %d: %s", res.StatusCode, payload)
	}
	res, payload = domainRequest(t, http.MethodPatch,
		srv.baseURL+"/api/v1/workspaces/ws_domain/members/"+member.ID,
		`{"role":"operator"}`, ownerCookie, roleStepCookie)
	if res.StatusCode != http.StatusOK || !bytes.Contains(payload, []byte("operator")) {
		t.Fatalf("authority role change = %d: %s", res.StatusCode, payload)
	}
	res, payload = domainRequest(t, http.MethodGet, srv.baseURL+"/workspaces/ws_domain/access", "", ownerCookie)
	if res.StatusCode != http.StatusOK || !bytes.Contains(payload, []byte(`data-authority-workspace="ws_domain"`)) {
		t.Fatalf("workspace access page = %d: %s", res.StatusCode, payload)
	}
	res, payload = domainRequest(t, http.MethodGet, srv.baseURL+"/api/v1/workspaces/ws_domain/activity", "", memberCookie)
	if res.StatusCode != http.StatusOK || !bytes.Contains(payload, []byte("workspace.member.role")) {
		t.Fatalf("event domain = %d: %s", res.StatusCode, payload)
	}
	res, payload = domainRequest(t, http.MethodGet, srv.baseURL+"/workspaces/ws_domain/activity", "", memberCookie)
	if res.StatusCode != http.StatusOK || !bytes.Contains(payload, []byte(`data-activity-page="ws_domain"`)) {
		t.Fatalf("event page = %d: %s", res.StatusCode, payload)
	}
	res, payload = domainRequest(t, http.MethodDelete,
		srv.baseURL+"/api/v1/workspaces/ws_domain/members/"+member.ID, "", ownerCookie, removeStepCookie)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("authority removal = %d: %s", res.StatusCode, payload)
	}
	_ = memberCookie
}

func TestAPIKeyDomainAuthentication(t *testing.T) {
	dataDir := t.TempDir()
	store, err := authstore.Open(filepath.Join(dataDir, "rc.db"))
	if err != nil {
		t.Fatal(err)
	}
	user, err := store.CreateUser("API Domain")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureWorkspace("ws_api_domain", "API Domain"); err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureWorkspaceMembership("ws_api_domain", user.ID, "owner"); err != nil {
		t.Fatal(err)
	}
	activePublic, activePrivate, err := ed25519Generate(t)
	if err != nil {
		t.Fatal(err)
	}
	expiredPublic, expiredPrivate, err := ed25519Generate(t)
	if err != nil {
		t.Fatal(err)
	}
	revokedPublic, revokedPrivate, err := ed25519Generate(t)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []authstore.APIKey{
		{ID: "key_active", UserID: user.ID, Name: "Active", PublicKey: activePublic, Scopes: []string{"read"}, CreatedAt: time.Now().UTC()},
		{ID: "key_expired", UserID: user.ID, Name: "Expired", PublicKey: expiredPublic, Scopes: []string{"read"}, CreatedAt: time.Now().UTC(), ExpiresAt: time.Now().Add(-time.Minute)},
		{ID: "key_revoked", UserID: user.ID, Name: "Revoked", PublicKey: revokedPublic, Scopes: []string{"read"}, CreatedAt: time.Now().UTC()},
	} {
		if err := store.PutAPIKey(key); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.DeleteAPIKey(user.ID, "key_revoked"); err != nil {
		t.Fatal(err)
	}
	store.Close()
	srv := startServer(t, "RC_DATA_DIR="+dataDir)

	request := domainSignedRequest(t, http.MethodGet, srv.baseURL+"/api/v1/me", "key_active", activePrivate, "nonce-domain-000001")
	res, payload := domainDo(t, request)
	if res.StatusCode != http.StatusOK || !bytes.Contains(payload, []byte("API Domain")) {
		t.Fatalf("active API key = %d: %s", res.StatusCode, payload)
	}
	replay := domainSignedRequest(t, http.MethodGet, srv.baseURL+"/api/v1/me", "key_active", activePrivate, "nonce-domain-000001")
	res, payload = domainDo(t, replay)
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("replayed API key = %d: %s", res.StatusCode, payload)
	}
	res, _ = domainDo(t, domainSignedRequest(t, http.MethodGet, srv.baseURL+"/api/v1/me", "key_expired", expiredPrivate, "nonce-domain-000002"))
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expired API key = %d", res.StatusCode)
	}
	res, _ = domainDo(t, domainSignedRequest(t, http.MethodGet, srv.baseURL+"/api/v1/me", "key_revoked", revokedPrivate, "nonce-domain-000003"))
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked API key = %d", res.StatusCode)
	}
}

func ed25519Generate(t *testing.T) (string, ed25519.PrivateKey, error) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", nil, err
	}
	return base64.RawURLEncoding.EncodeToString(public), private, nil
}

func domainSignedRequest(t *testing.T, method, target, keyID string, private ed25519.PrivateKey, nonce string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, target, nil)
	if err != nil {
		t.Fatal(err)
	}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	payload := crypto.ApiPayload(keyID, timestamp, nonce, method, req.URL.RequestURI(), nil)
	signature := base64.RawURLEncoding.EncodeToString(ed25519.Sign(private, []byte(payload)))
	req.Header.Set("X-RC-Key-ID", keyID)
	req.Header.Set("X-RC-Timestamp", timestamp)
	req.Header.Set("X-RC-Nonce", nonce)
	req.Header.Set("X-RC-Signature", signature)
	return req
}

func domainDo(t *testing.T, req *http.Request) (*http.Response, []byte) {
	t.Helper()
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var payload bytes.Buffer
	_, _ = payload.ReadFrom(res.Body)
	return res, payload.Bytes()
}
