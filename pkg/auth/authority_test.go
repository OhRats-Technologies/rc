package auth

import (
	"path/filepath"
	"testing"
	"time"
)

func TestAuthorityDeliverySurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	delivery := AuthorityDelivery{
		DeviceID: "dev_restart", PreviousHash: "previous",
		Generation: 3, NextHash: "next", Message: `{"type":"lock_sync"}`,
		ExpiresAt: time.Now().Add(time.Hour).UTC(),
	}
	if err := store.PutAuthorityDelivery(delivery); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err := reopened.AuthorityDelivery("dev_restart", "previous", 3)
	if err != nil || got == nil {
		t.Fatalf("delivery after restart = %+v, %v", got, err)
	}
	if got.NextHash != "next" || got.Generation != 3 || got.Message != delivery.Message {
		t.Fatalf("delivery = %+v", got)
	}
	if err := reopened.DeleteAuthorityDeliveries("dev_restart", "next", 4); err != nil {
		t.Fatal(err)
	}
	pending, err := reopened.AuthorityDelivery("dev_restart", "previous", 3)
	if err != nil || pending != nil {
		t.Fatalf("pruned delivery = %+v, %v", pending, err)
	}
}
