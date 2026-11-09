package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"
)

const oauthTestResource = "https://rc.example.test/mcp"

func seedOAuthClient(t *testing.T, store *Store) (string, string) {
	t.Helper()
	redirect := "https://client.example/callback"
	clientID, err := store.RegisterOAuthClient("Test Client", []string{redirect})
	if err != nil {
		t.Fatalf("register client: %v", err)
	}
	return clientID, redirect
}

func pkceChallenge(verifier string) string {
	digest := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func TestOAuthClientRegistration(t *testing.T) {
	store := openStore(t)

	valid := []string{
		"https://client.example/callback",
		"http://localhost:8080/callback",
		"http://127.0.0.1/callback",
	}
	clientID, err := store.RegisterOAuthClient("Client", valid)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	client, err := store.GetOAuthClient(clientID)
	if err != nil {
		t.Fatalf("get client: %v", err)
	}
	if client.Name != "Client" || len(client.RedirectURIs) != len(valid) {
		t.Fatalf("client = %+v", client)
	}
	for _, uri := range valid {
		if err := store.ValidateRedirectURI(clientID, uri); err != nil {
			t.Fatalf("validate %q: %v", uri, err)
		}
	}
	if err := store.ValidateRedirectURI(clientID, "https://client.example/other"); err != ErrOAuthInvalidRedirect {
		t.Fatalf("unregistered redirect error = %v", err)
	}
	if err := store.ValidateRedirectURI(clientID, "https://client.example/callback/"); err != ErrOAuthInvalidRedirect {
		t.Fatalf("prefix redirect error = %v", err)
	}
	if _, err := store.GetOAuthClient("missing"); err != ErrOAuthClientNotFound {
		t.Fatalf("missing client error = %v", err)
	}

	invalid := [][]string{
		nil,
		{"not-a-uri"},
		{"http://client.example/callback"},
		{"ftp://client.example/callback"},
		{"https://client.example/callback#fragment"},
		{"https://user:pass@client.example/callback"},
		{" https://client.example/callback"},
		{
			"https://a.example", "https://b.example", "https://c.example", "https://d.example",
			"https://e.example", "https://f.example", "https://g.example", "https://h.example",
			"https://i.example", "https://j.example", "https://k.example",
		},
	}
	for _, uris := range invalid {
		if _, err := store.RegisterOAuthClient("Bad", uris); !errors.Is(err, ErrOAuthInvalidRedirect) {
			t.Fatalf("register %v error = %v", uris, err)
		}
	}
}

func TestOAuthCodeSingleUseAndPKCE(t *testing.T) {
	store := openStore(t)
	clientID, redirect := seedOAuthClient(t, store)
	user, err := store.CreateUser("Alice")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	verifier := strings.Repeat("v", 48)
	challenge := pkceChallenge(verifier)

	code, err := store.IssueOAuthCode(clientID, user.ID, redirect, challenge, oauthTestResource, OAuthScopeObserve, "grant_test")
	if err != nil {
		t.Fatalf("issue code: %v", err)
	}
	gotUser, gotResource, gotScopes, gotGrant, err := store.ConsumeOAuthCode(code, clientID, redirect, verifier, oauthTestResource)
	if err != nil {
		t.Fatalf("consume code: %v", err)
	}
	if gotUser != user.ID || gotResource != oauthTestResource || gotScopes != OAuthScopeObserve || gotGrant != "grant_test" {
		t.Fatalf("consume returned %q, %q, %q, %q", gotUser, gotResource, gotScopes, gotGrant)
	}
	if _, _, _, _, err := store.ConsumeOAuthCode(code, clientID, redirect, verifier, ""); !errors.Is(err, ErrOAuthInvalidCode) {
		t.Fatalf("reused code error = %v", err)
	}

	code, err = store.IssueOAuthCode(clientID, user.ID, redirect, challenge, oauthTestResource, OAuthScopeObserve, "grant_test")
	if err != nil {
		t.Fatalf("issue code: %v", err)
	}
	if _, _, _, _, err := store.ConsumeOAuthCode(code, clientID, redirect, "wrong-verifier", ""); !errors.Is(err, ErrOAuthInvalidVerifier) {
		t.Fatalf("wrong verifier error = %v", err)
	}
	if _, _, _, _, err := store.ConsumeOAuthCode(code, clientID, redirect, verifier, ""); !errors.Is(err, ErrOAuthInvalidCode) {
		t.Fatalf("code survived failed PKCE: %v", err)
	}

	code, err = store.IssueOAuthCode(clientID, user.ID, redirect, challenge, oauthTestResource, OAuthScopeObserve, "grant_test")
	if err != nil {
		t.Fatalf("issue code: %v", err)
	}
	if _, _, _, _, err := store.ConsumeOAuthCode(code, clientID, "https://client.example/other", verifier, ""); !errors.Is(err, ErrOAuthInvalidGrant) {
		t.Fatalf("redirect mismatch error = %v", err)
	}

	code, err = store.IssueOAuthCode(clientID, user.ID, redirect, challenge, oauthTestResource, OAuthScopeObserve, "grant_test")
	if err != nil {
		t.Fatalf("issue code: %v", err)
	}
	_, err = store.db.ExecContext(context.Background(),
		`UPDATE oauth_codes SET expires_ms = ? WHERE code_hash IN (SELECT code_hash FROM oauth_codes);`,
		msFromTime(time.Now().Add(-time.Minute)))
	if err != nil {
		t.Fatalf("expire code: %v", err)
	}
	if _, _, _, _, err := store.ConsumeOAuthCode(code, clientID, redirect, verifier, ""); !errors.Is(err, ErrOAuthInvalidCode) {
		t.Fatalf("expired code error = %v", err)
	}
}

func TestOAuthTokenLookupAndResourceBinding(t *testing.T) {
	store := openStore(t)
	clientID, _ := seedOAuthClient(t, store)
	user, _ := store.CreateUser("Alice")

	access, refresh, accessExpires, refreshExpires, err := store.IssueOAuthTokens(
		clientID, user.ID, oauthTestResource, "mcp", "grant_test",
	)
	if err != nil {
		t.Fatalf("issue tokens: %v", err)
	}
	if access == "" || refresh == "" || access == refresh {
		t.Fatalf("tokens = %q, %q", access, refresh)
	}
	if until := time.Until(accessExpires); until < 14*time.Minute || until > 15*time.Minute {
		t.Fatalf("access lifetime = %v", until)
	}
	if until := time.Until(refreshExpires); until < 29*24*time.Hour || until > 30*24*time.Hour {
		t.Fatalf("refresh lifetime = %v", until)
	}

	grant, err := store.LookupOAuthAccess(access, oauthTestResource)
	if err != nil {
		t.Fatalf("lookup access: %v", err)
	}
	if grant.UserID != user.ID || grant.ClientID != clientID || grant.Scopes != "mcp" {
		t.Fatalf("grant = %+v", grant)
	}
	if _, err := store.LookupOAuthAccess(access, "https://other.example/mcp"); !errors.Is(err, ErrOAuthResourceMismatch) {
		t.Fatalf("resource mismatch error = %v", err)
	}
	if _, err := store.LookupOAuthAccess("unknown", oauthTestResource); !errors.Is(err, ErrOAuthTokenNotFound) {
		t.Fatalf("unknown token error = %v", err)
	}

	if _, err := store.db.ExecContext(context.Background(),
		`UPDATE oauth_tokens SET access_expires_ms = ?;`,
		msFromTime(time.Now().Add(-time.Minute))); err != nil {
		t.Fatalf("expire access: %v", err)
	}
	if _, err := store.LookupOAuthAccess(access, oauthTestResource); !errors.Is(err, ErrOAuthTokenExpired) {
		t.Fatalf("expired access error = %v", err)
	}

	access, _, _, _, err = store.IssueOAuthTokens(clientID, user.ID, oauthTestResource, "mcp", "grant_test")
	if err != nil {
		t.Fatalf("issue tokens: %v", err)
	}
	if _, err := store.db.ExecContext(context.Background(),
		`UPDATE oauth_tokens SET revoked_ms = ?;`, nowMs()); err != nil {
		t.Fatalf("revoke tokens: %v", err)
	}
	if _, err := store.LookupOAuthAccess(access, oauthTestResource); !errors.Is(err, ErrOAuthTokenRevoked) {
		t.Fatalf("revoked access error = %v", err)
	}
}

func TestOAuthRefreshRotation(t *testing.T) {
	store := openStore(t)
	clientID, _ := seedOAuthClient(t, store)
	user, _ := store.CreateUser("Alice")

	access, refresh, _, _, err := store.IssueOAuthTokens(clientID, user.ID, oauthTestResource, "mcp", "grant_test")
	if err != nil {
		t.Fatalf("issue tokens: %v", err)
	}
	newAccess, newRefresh, _, _, _, err := store.ExchangeOAuthRefresh(refresh, clientID, oauthTestResource)
	if err != nil {
		t.Fatalf("exchange refresh: %v", err)
	}
	if newAccess == access || newRefresh == refresh {
		t.Fatal("refresh exchange did not rotate tokens")
	}
	if _, err := store.LookupOAuthAccess(newAccess, oauthTestResource); err != nil {
		t.Fatalf("rotated access lookup: %v", err)
	}
	if _, err := store.LookupOAuthAccess(access, oauthTestResource); !errors.Is(err, ErrOAuthTokenNotFound) {
		t.Fatalf("stale access error = %v", err)
	}
	if _, _, _, _, _, err := store.ExchangeOAuthRefresh(refresh, clientID, oauthTestResource); !errors.Is(err, ErrOAuthInvalidGrant) {
		t.Fatalf("refresh reuse error = %v", err)
	}
	if _, _, _, _, _, err := store.ExchangeOAuthRefresh(newRefresh, "other-client", oauthTestResource); !errors.Is(err, ErrOAuthInvalidClient) {
		t.Fatalf("refresh client mismatch error = %v", err)
	}

	_, refresh2, _, _, err := store.IssueOAuthTokens(clientID, user.ID, oauthTestResource, "mcp", "grant_test")
	if err != nil {
		t.Fatalf("issue tokens: %v", err)
	}
	refresh2Hash, _ := oauthTokenHash(refresh2)
	if _, err := store.db.ExecContext(context.Background(),
		`UPDATE oauth_tokens SET refresh_expires_ms = ? WHERE refresh_hash = ?;`,
		msFromTime(time.Now().Add(-time.Minute)), refresh2Hash); err != nil {
		t.Fatalf("expire refresh: %v", err)
	}
	if _, _, _, _, _, err := store.ExchangeOAuthRefresh(refresh2, clientID, oauthTestResource); !errors.Is(err, ErrOAuthTokenExpired) {
		t.Fatalf("expired refresh error = %v", err)
	}

	_, refresh3, _, _, err := store.IssueOAuthTokens(clientID, user.ID, oauthTestResource, "mcp", "grant_test")
	if err != nil {
		t.Fatalf("issue tokens: %v", err)
	}
	refresh3Hash, _ := oauthTokenHash(refresh3)
	if _, err := store.db.ExecContext(context.Background(),
		`UPDATE oauth_tokens SET revoked_ms = ? WHERE refresh_hash = ?;`, nowMs(), refresh3Hash); err != nil {
		t.Fatalf("revoke tokens: %v", err)
	}
	if _, _, _, _, _, err := store.ExchangeOAuthRefresh(refresh3, clientID, oauthTestResource); !errors.Is(err, ErrOAuthTokenRevoked) {
		t.Fatalf("revoked refresh error = %v", err)
	}
}
