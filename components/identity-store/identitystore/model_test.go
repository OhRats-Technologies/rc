package identitystore

import (
	"encoding/json"
	"testing"
)

func TestStoredUserWireShape(t *testing.T) {
	encoded, err := encode(storedUser{Id: "user-1", DisplayName: "Ada", CreatedAtMs: 7})
	if err != nil {
		t.Fatal(err)
	}
	const expected = `{"id":"user-1","display_name":"Ada","created_at_ms":7}`
	if string(encoded) != expected {
		t.Fatalf("stored user = %s", encoded)
	}
}

func TestStoredPasskeyWireShape(t *testing.T) {
	stored := storedPasskey{
		Id:     "passkey-1",
		UserId: "user-1",
		Name:   "Laptop",
		Credential: storedCredential{
			Id:             byteArray{1, 2},
			Algorithm:      "es256",
			PublicKeyCose:  byteArray{3},
			SignCount:      4,
			BackupEligible: true,
		},
		CreatedAtMs: 7,
	}
	encoded, err := stored.encoded()
	if err != nil {
		t.Fatal(err)
	}
	const expected = `{"id":"passkey-1","user_id":"user-1","name":"Laptop",` +
		`"credential":{"id":[1,2],"algorithm":"es256","public_key_cose":[3],` +
		`"sign_count":4,"backup_eligible":true,"backup_state":false},` +
		`"created_at_ms":7,"last_used_at_ms":null}`
	if string(encoded) != expected {
		t.Fatalf("stored passkey = %s", encoded)
	}
	decoded, err := decode[storedPasskey](encoded)
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded.Credential.Id) != 2 || decoded.Credential.Id[1] != 2 || decoded.LastUsedAtMs != nil {
		t.Fatalf("stored passkey round trip = %+v", decoded)
	}
}

func TestStoredCeremonyWireShape(t *testing.T) {
	userId := "user-1"
	encoded, err := encode(storedCeremony{
		Id:          "ceremony-1",
		Kind:        "admin-step-up",
		UserId:      &userId,
		Metadata:    byteArray{1},
		State:       byteArray{2},
		ExpiresAtMs: 9,
	})
	if err != nil {
		t.Fatal(err)
	}
	const expected = `{"id":"ceremony-1","kind":"admin-step-up","user_id":"user-1",` +
		`"metadata":[1],"state":[2],"expires_at_ms":9}`
	if string(encoded) != expected {
		t.Fatalf("stored ceremony = %s", encoded)
	}
	var generic map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &generic); err != nil {
		t.Fatal(err)
	}
	if string(generic["metadata"]) != "[1]" {
		t.Fatalf("metadata wire shape = %s", generic["metadata"])
	}
}

func TestValidationBounds(t *testing.T) {
	if err := validID("", "user id"); err == nil || err.Error() != "invalid user id" {
		t.Fatalf("empty id error = %v", err)
	}
	if err := validID("user/1", "user id"); err == nil {
		t.Fatal("id with a path separator was accepted")
	}
	if err := validDisplayName("   "); err == nil || err.Error() != "invalid user display name" {
		t.Fatalf("blank display name error = %v", err)
	}
	if err := validKind("Admin"); err == nil || err.Error() != "invalid ceremony kind" {
		t.Fatalf("uppercase ceremony kind error = %v", err)
	}
	if err := validStoredCredential(nil, "es256", []byte{1}); err == nil ||
		err.Error() != "invalid WebAuthn credential id" {
		t.Fatalf("empty credential id error = %v", err)
	}
	if err := validStoredCredential([]byte{1}, "ES256", []byte{1}); err == nil ||
		err.Error() != "invalid WebAuthn algorithm" {
		t.Fatalf("uppercase algorithm error = %v", err)
	}
}
