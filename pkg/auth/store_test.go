package auth

import (
	"bytes"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func openStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
	return store
}

func TestUserAndPasskeyLifecycle(t *testing.T) {
	store := openStore(t)

	count, err := store.UserCount()
	if err != nil {
		t.Fatalf("user count: %v", err)
	}
	if count != 0 {
		t.Fatalf("initial user count = %d", count)
	}
	if _, err := store.GetUser("missing"); err != ErrUserNotFound {
		t.Fatalf("missing user error = %v", err)
	}
	if _, err := store.CreateUser("  "); err != ErrInvalidName {
		t.Fatalf("empty name error = %v", err)
	}

	user, err := store.CreateUser(" Alice ")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if user.Name != "Alice" || user.ID == "" || user.CreatedAt.IsZero() {
		t.Fatalf("user = %+v", user)
	}
	count, err = store.UserCount()
	if err != nil || count != 1 {
		t.Fatalf("user count = %d, %v", count, err)
	}
	found, err := store.GetUser(user.ID)
	if err != nil || found.Name != "Alice" {
		t.Fatalf("get user = %+v, %v", found, err)
	}

	credentialID := []byte("credential-one")
	publicKey := []byte("public-key-one")
	passkey, err := store.AddPasskey(user.ID, "Laptop", credentialID, publicKey, AlgorithmES256)
	if err != nil {
		t.Fatalf("add passkey: %v", err)
	}
	if passkey.ID == "" || passkey.UserID != user.ID || passkey.Algorithm != AlgorithmES256 {
		t.Fatalf("passkey = %+v", passkey)
	}
	if passkey.LastUsedAt != nil {
		t.Fatalf("last used = %v", passkey.LastUsedAt)
	}

	credentialID[0] ^= 0xff
	publicKey[0] ^= 0xff
	stored, err := store.PasskeyByCredentialID([]byte("credential-one"))
	if err != nil {
		t.Fatalf("passkey by credential: %v", err)
	}
	if !bytes.Equal(stored.PublicKey, []byte("public-key-one")) ||
		!bytes.Equal(stored.CredentialID, []byte("credential-one")) {
		t.Fatalf("stored passkey = %+v", stored)
	}

	if _, err := store.AddPasskey(user.ID, "Clone", []byte("credential-one"), []byte("other"), AlgorithmES256); !errors.Is(err, ErrCredentialRegistered) {
		t.Fatalf("duplicate credential error = %v", err)
	}
	if _, err := store.AddPasskey("missing", "Laptop", []byte("other"), []byte("other"), AlgorithmES256); err != ErrUserNotFound {
		t.Fatalf("missing user passkey error = %v", err)
	}
	if _, err := store.AddPasskey(user.ID, "Empty", nil, []byte("other"), AlgorithmES256); err != ErrInvalidCredential {
		t.Fatalf("empty credential error = %v", err)
	}
	if _, err := store.PasskeyByCredentialID([]byte("unknown")); err != ErrPasskeyNotFound {
		t.Fatalf("unknown credential error = %v", err)
	}

	usedAt := time.Now().UTC().Add(-time.Minute)
	if err := store.UpdatePasskeyLastUsed(passkey.ID, usedAt); err != nil {
		t.Fatalf("update last used: %v", err)
	}
	if err := store.UpdatePasskeyLastUsed("missing", usedAt); err != ErrPasskeyNotFound {
		t.Fatalf("missing last used error = %v", err)
	}
	passkeys, err := store.PasskeysForUser(user.ID)
	if err != nil {
		t.Fatalf("passkeys for user: %v", err)
	}
	if len(passkeys) != 1 || passkeys[0].LastUsedAt == nil {
		t.Fatalf("passkeys = %+v", passkeys)
	}
	if passkeys[0].LastUsedAt.UnixMilli() != usedAt.UnixMilli() {
		t.Fatalf("last used = %v", passkeys[0].LastUsedAt)
	}
}

func TestCeremonySingleUseAndExpiry(t *testing.T) {
	store := openStore(t)
	now := nowMs()

	if err := store.PutCeremony("ceremony", CeremonyRegistration, "user", []byte(`{"challenge":"a"}`), now+60000); err != nil {
		t.Fatalf("put ceremony: %v", err)
	}
	state, userID, ok, err := store.TakeCeremony("ceremony", CeremonyRegistration)
	if err != nil || !ok || userID != "user" || string(state) != `{"challenge":"a"}` {
		t.Fatalf("take ceremony = %q, %q, %v, %v", state, userID, ok, err)
	}
	if _, _, ok, err := store.TakeCeremony("ceremony", CeremonyRegistration); err != nil || ok {
		t.Fatalf("second take = %v, %v", ok, err)
	}

	if err := store.PutCeremony("expired", CeremonyLogin, "", []byte("state"), now-1); err != nil {
		t.Fatalf("put expired ceremony: %v", err)
	}
	if _, _, ok, err := store.TakeCeremony("expired", CeremonyLogin); err != nil || ok {
		t.Fatalf("expired take = %v, %v", ok, err)
	}

	if err := store.PutCeremony("kind", CeremonyLogin, "", []byte("state"), now+60000); err != nil {
		t.Fatalf("put ceremony: %v", err)
	}
	if _, _, ok, err := store.TakeCeremony("kind", CeremonyRegistration); err != nil || ok {
		t.Fatalf("kind mismatch take = %v, %v", ok, err)
	}
	if err := store.PutCeremony("", CeremonyLogin, "", []byte("state"), now+60000); err != ErrInvalidCeremony {
		t.Fatalf("invalid ceremony error = %v", err)
	}
	if err := store.PutCeremony("empty", CeremonyLogin, "", nil, now+60000); err != ErrInvalidCeremony {
		t.Fatalf("empty state error = %v", err)
	}
}
