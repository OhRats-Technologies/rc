package webauthn

import (
	"bytes"
	"testing"
)

func TestES256CoseKeyMatchesVerifiedRegistrationKey(t *testing.T) {
	value := loadFixture(t)
	registration, _ := verifyFixtureRegistration(t, value)
	parsed, err := fromCOSEKey(registration.Credential.PublicKeyCOSE)
	if err != nil {
		t.Fatalf("decode credential key: %v", err)
	}
	encoded, err := ES256CoseKey(parsed.x, parsed.y)
	if err != nil {
		t.Fatalf("encode credential key: %v", err)
	}
	if !bytes.Equal(encoded, registration.Credential.PublicKeyCOSE) {
		t.Fatal("ES256CoseKey must produce the verifier's canonical key bytes")
	}
	canonical, err := CanonicalCoseKey(registration.Credential.PublicKeyCOSE)
	if err != nil || !bytes.Equal(canonical, registration.Credential.PublicKeyCOSE) {
		t.Fatalf("canonical key = %x, %v", canonical, err)
	}
	if _, err := ES256CoseKey(parsed.x, make([]byte, 32)); err == nil {
		t.Fatal("off-curve point must be rejected")
	}
}
