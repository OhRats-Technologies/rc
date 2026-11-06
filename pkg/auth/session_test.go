package auth

import (
	"errors"
	"testing"
	"time"
)

func TestSessionIssueLookupRevoke(t *testing.T) {
	store := openStore(t)
	user, err := store.CreateUser("Alice")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	token, expiresAt, err := store.IssueSession(user.ID, "1h")
	if err != nil {
		t.Fatalf("issue session: %v", err)
	}
	if len(token) != 43 {
		t.Fatalf("token length = %d", len(token))
	}
	if remaining := time.Until(expiresAt); remaining < 55*time.Minute || remaining > 65*time.Minute {
		t.Fatalf("session lifetime = %v", remaining)
	}
	session, err := store.LookupSession(token)
	if err != nil {
		t.Fatalf("lookup session: %v", err)
	}
	if session.UserID != user.ID {
		t.Fatalf("session user = %q", session.UserID)
	}
	if session.ExpiresAt.UnixMilli() != expiresAt.UnixMilli() {
		t.Fatalf("session expiry = %v", session.ExpiresAt)
	}
	if _, err := store.LookupSession("not-a-token"); err != ErrInvalidSessionToken {
		t.Fatalf("invalid token error = %v", err)
	}
	if _, err := store.LookupSession(mustToken(t, 31)); err != ErrInvalidSessionToken {
		t.Fatalf("short token error = %v", err)
	}
	if _, err := store.LookupSession(mustToken(t, 32)); err != ErrSessionNotFound {
		t.Fatalf("unknown token error = %v", err)
	}

	if err := store.RevokeSession(token); err != nil {
		t.Fatalf("revoke session: %v", err)
	}
	if _, err := store.LookupSession(token); err != ErrSessionNotFound {
		t.Fatalf("revoked lookup error = %v", err)
	}
	if err := store.RevokeSession(token); err != nil {
		t.Fatalf("second revoke: %v", err)
	}
}

func TestSessionLifetimesAndFailures(t *testing.T) {
	store := openStore(t)
	user, err := store.CreateUser("Alice")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	cases := map[string]time.Duration{
		"":    DefaultSessionLifetime,
		"30d": 30 * 24 * time.Hour,
		"1d":  24 * time.Hour,
		"1h":  time.Hour,
		"90m": 90 * time.Minute,
	}
	for lifetime, expected := range cases {
		_, expiresAt, err := store.IssueSession(user.ID, lifetime)
		if err != nil {
			t.Fatalf("issue session %q: %v", lifetime, err)
		}
		remaining := time.Until(expiresAt)
		if remaining < expected-time.Minute || remaining > expected+time.Minute {
			t.Fatalf("lifetime %q = %v", lifetime, remaining)
		}
	}
	for _, lifetime := range []string{"0s", "-1h", "soon", "1w"} {
		if _, _, err := store.IssueSession(user.ID, lifetime); err != ErrInvalidLifetime {
			t.Fatalf("lifetime %q error = %v", lifetime, err)
		}
	}
	if _, _, err := store.IssueSession("missing", "1h"); err != ErrUserNotFound {
		t.Fatalf("missing user error = %v", err)
	}
	if _, _, err := store.IssueSession("", "1h"); err != ErrUserNotFound {
		t.Fatalf("empty user error = %v", err)
	}
}

func TestSessionAndCeremonyExpiry(t *testing.T) {
	store := openStore(t)
	user, err := store.CreateUser("Alice")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	token, _, err := store.IssueSession(user.ID, "1h")
	if err != nil {
		t.Fatalf("issue session: %v", err)
	}
	if err := store.PutCeremony("expiring", CeremonyLogin, "", []byte("state"), nowMs()+time.Hour.Milliseconds()); err != nil {
		t.Fatalf("put ceremony: %v", err)
	}

	removed, err := store.PruneExpired(time.Now().Add(2 * time.Hour))
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if removed != 2 {
		t.Fatalf("removed = %d", removed)
	}
	if _, err := store.LookupSession(token); err != ErrSessionNotFound {
		t.Fatalf("pruned session error = %v", err)
	}
	if _, _, ok, err := store.TakeCeremony("expiring", CeremonyLogin); err != nil || ok {
		t.Fatalf("pruned ceremony = %v, %v", ok, err)
	}

	expiredToken, err := randomToken(SessionTokenBytes)
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	if _, err := store.db.ExecContext(
		t.Context(),
		`INSERT INTO sessions (token_hash, user_id, expires_ms) VALUES (?, ?, ?);`,
		hashOf(t, expiredToken),
		user.ID,
		nowMs()-1000,
	); err != nil {
		t.Fatalf("insert expired session: %v", err)
	}
	if _, err := store.LookupSession(expiredToken); !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("expired lookup error = %v", err)
	}
	if _, err := store.LookupSession(expiredToken); err != ErrSessionNotFound {
		t.Fatalf("expired session should be removed, error = %v", err)
	}
}

func mustToken(t *testing.T, size int) string {
	t.Helper()
	token, err := randomToken(size)
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	return token
}

func hashOf(t *testing.T, token string) []byte {
	t.Helper()
	hash, err := tokenHash(token)
	if err != nil {
		t.Fatalf("token hash: %v", err)
	}
	return hash
}
