package auth

import (
	"errors"
	"testing"
	"time"
)

func TestAPIKeyProjectionAndRevocation(t *testing.T) {
	store := openStore(t)
	user, err := store.CreateUser("API User")
	if err != nil {
		t.Fatal(err)
	}
	outsider, err := store.CreateUser("Outsider")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureWorkspace("ws_api", "API Workspace"); err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureWorkspaceMembership("ws_api", user.ID, "owner"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	active := APIKey{
		ID: "key_active", UserID: user.ID, Name: "Active",
		PublicKey: "active-public", Scopes: []string{"execute"}, CreatedAt: now,
	}
	expired := APIKey{
		ID: "key_expired", UserID: user.ID, Name: "Expired",
		PublicKey: "expired-public", Scopes: []string{"execute"}, CreatedAt: now,
		ExpiresAt: now.Add(-time.Minute),
	}
	nonMember := APIKey{
		ID: "key_outsider", UserID: outsider.ID, Name: "Outsider",
		PublicKey: "outsider-public", Scopes: []string{"execute"}, CreatedAt: now,
	}
	for _, key := range []APIKey{active, expired, nonMember} {
		if err := store.PutAPIKey(key); err != nil {
			t.Fatal(err)
		}
	}
	keys, err := store.ActiveAPIKeysForWorkspace("ws_api")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0].ID != "key_active" || !keys[0].Active(now) {
		t.Fatalf("workspace projection = %+v", keys)
	}
	if keys[0].Scopes[0] != "execute" {
		t.Fatalf("scopes = %v", keys[0].Scopes)
	}
	removed, err := store.DeleteAPIKey(user.ID, "key_active")
	if err != nil || !removed {
		t.Fatalf("delete = %v, %v", removed, err)
	}
	keys, err = store.ActiveAPIKeysForWorkspace("ws_api")
	if err != nil || len(keys) != 0 {
		t.Fatalf("projection after revoke = %+v, %v", keys, err)
	}
	if _, err := store.APIKey("key_active"); !errors.Is(err, ErrAPIKeyNotFound) {
		t.Fatalf("revoked lookup = %v", err)
	}
}

func TestRememberNonceRejectsReplay(t *testing.T) {
	store := openStore(t)
	expires := time.Now().Add(time.Minute)
	fresh, err := store.RememberNonce("client:key", "nonce-0123456789", expires)
	if err != nil || !fresh {
		t.Fatalf("first nonce = %v, %v", fresh, err)
	}
	fresh, err = store.RememberNonce("client:key", "nonce-0123456789", expires)
	if err != nil || fresh {
		t.Fatalf("replayed nonce = %v, %v", fresh, err)
	}
	fresh, err = store.RememberNonce("client:other", "nonce-0123456789", expires)
	if err != nil || !fresh {
		t.Fatalf("different principal nonce = %v, %v", fresh, err)
	}
}
