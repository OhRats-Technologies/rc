package crypto

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type rs256Vector struct {
	Assertion       json.RawMessage `json:"assertion"`
	Challenge       string          `json:"challenge"`
	CredentialID    string          `json:"credentialId"`
	Origin          string          `json:"origin"`
	RpID            string          `json:"rpId"`
	StoredPublicKey string          `json:"storedPublicKey"`
}

func TestVerifiesRS256AssertionFixture(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "fixtures", "webauthn-rs256.json")
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture failed: %v", err)
	}

	var v rs256Vector
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("unmarshal fixture failed: %v", err)
	}

	assertionJSON := string(v.Assertion)
	err = VerifyWebauthnAssertion(
		assertionJSON,
		v.CredentialID,
		v.StoredPublicKey,
		v.Challenge,
		v.Origin,
		v.RpID,
	)
	if err != nil {
		t.Fatalf("VerifyWebauthnAssertion failed: %v", err)
	}

	// Wrong challenge must fail verification
	err = VerifyWebauthnAssertion(
		assertionJSON,
		v.CredentialID,
		v.StoredPublicKey,
		"wrong-challenge",
		v.Origin,
		v.RpID,
	)
	if err == nil {
		t.Fatal("expected failure on wrong challenge")
	}
}
