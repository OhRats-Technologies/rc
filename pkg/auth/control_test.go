package auth

import (
	"errors"
	"testing"
	"time"
)

func TestControlClientCannotMoveBetweenAccounts(t *testing.T) {
	store := openStore(t)
	first, err := store.CreateUser("First")
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.CreateUser("Second")
	if err != nil {
		t.Fatal(err)
	}
	client := ControlClient{
		ID: "control_shared", UserID: first.ID, Kind: "browser",
		SigningPublicKey: "key", CreatedAt: time.Now().UTC(),
	}
	if err := store.PutControlClient(client); err != nil {
		t.Fatal(err)
	}
	conflict := client
	conflict.UserID = second.ID
	if err := store.PutControlClient(conflict); !errors.Is(err, ErrControlClientConflict) {
		t.Fatalf("cross-account update = %v", err)
	}
	stored, err := store.ControlClient(first.ID, client.ID)
	if err != nil || stored.UserID != first.ID {
		t.Fatalf("original control client = %+v, %v", stored, err)
	}
	updated := client
	updated.SigningPublicKey = "rotated"
	if err := store.PutControlClient(updated); err != nil {
		t.Fatalf("same-account refresh: %v", err)
	}
	stored, _ = store.ControlClient(first.ID, client.ID)
	if stored.SigningPublicKey != "rotated" {
		t.Fatalf("refreshed key = %q", stored.SigningPublicKey)
	}
}
