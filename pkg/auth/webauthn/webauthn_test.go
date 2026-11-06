package webauthn

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"testing"
)

type fixtureCeremony struct {
	Challenge         string `json:"challenge"`
	CredentialID      string `json:"credentialId"`
	ClientDataJSON    string `json:"clientDataJSON"`
	AttestationObject string `json:"attestationObject"`
	AuthenticatorData string `json:"authenticatorData"`
	Signature         string `json:"signature"`
	UserHandle        string `json:"userHandle"`
}

type fixture struct {
	Algorithm          string          `json:"algorithm"`
	RPID               string          `json:"rpId"`
	Origin             string          `json:"origin"`
	ExpectedUserHandle string          `json:"expectedUserHandle"`
	Registration       fixtureCeremony `json:"registration"`
	Authentication     fixtureCeremony `json:"authentication"`
}

func loadFixture(t *testing.T) fixture {
	t.Helper()
	raw, err := os.ReadFile("../../../fixtures/webauthn-es256.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var value fixture
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return value
}

func decodeFixture(t *testing.T, value string) []byte {
	t.Helper()
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		t.Fatalf("decode fixture field: %v", err)
	}
	return decoded
}

func fixtureRelyingParty(t *testing.T, value fixture) RelyingParty {
	t.Helper()
	relyingParty, err := NewRelyingParty(value.RPID, value.Origin)
	if err != nil {
		t.Fatalf("relying party: %v", err)
	}
	return relyingParty
}

func registrationJSON(t *testing.T, value fixture) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"id":    value.Registration.CredentialID,
		"rawId": value.Registration.CredentialID,
		"type":  "public-key",
		"response": map[string]any{
			"clientDataJSON":    value.Registration.ClientDataJSON,
			"attestationObject": value.Registration.AttestationObject,
		},
	})
	if err != nil {
		t.Fatalf("marshal registration response: %v", err)
	}
	return raw
}

func authenticationJSON(t *testing.T, value fixture, signature string) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"id":    value.Authentication.CredentialID,
		"rawId": value.Authentication.CredentialID,
		"type":  "public-key",
		"response": map[string]any{
			"clientDataJSON":    value.Authentication.ClientDataJSON,
			"authenticatorData": value.Authentication.AuthenticatorData,
			"signature":         signature,
			"userHandle":        value.Authentication.UserHandle,
		},
	})
	if err != nil {
		t.Fatalf("marshal authentication response: %v", err)
	}
	return raw
}

func verifyFixtureRegistration(t *testing.T, value fixture) (Registration, RelyingParty) {
	t.Helper()
	relyingParty := fixtureRelyingParty(t, value)
	registration, err := VerifyRegistration(
		registrationJSON(t, value),
		decodeFixture(t, value.Registration.Challenge),
		relyingParty,
	)
	if err != nil {
		t.Fatalf("registration should verify: %v", err)
	}
	return registration, relyingParty
}

func TestVerifiesRegistrationAndAuthentication(t *testing.T) {
	value := loadFixture(t)
	registration, relyingParty := verifyFixtureRegistration(t, value)

	credential := registration.Credential
	if credential.Algorithm != Algorithm {
		t.Fatalf("algorithm = %q", credential.Algorithm)
	}
	if credential.SignCount != 0 {
		t.Fatalf("sign count = %d", credential.SignCount)
	}
	if !bytes.Equal(credential.ID, decodeFixture(t, value.Registration.CredentialID)) {
		t.Fatalf("credential id = %x", credential.ID)
	}
	if !bytes.Equal(registration.AAGUID, bytes.Repeat([]byte{1}, 16)) {
		t.Fatalf("aaguid = %x", registration.AAGUID)
	}
	if !registration.UserVerified {
		t.Fatal("registration should be user verified")
	}
	if len(registration.UserHandle) != 0 {
		t.Fatalf("registration user handle = %x", registration.UserHandle)
	}

	userHandle := decodeFixture(t, value.ExpectedUserHandle)
	if err := VerifyAuthentication(
		authenticationJSON(t, value, value.Authentication.Signature),
		decodeFixture(t, value.Authentication.Challenge),
		relyingParty,
		credential.ID,
		credential.PublicKeyCOSE,
		userHandle,
	); err != nil {
		t.Fatalf("authentication should verify: %v", err)
	}

	updated, err := Authenticate(
		authenticationJSON(t, value, value.Authentication.Signature),
		decodeFixture(t, value.Authentication.Challenge),
		relyingParty,
		credential,
		userHandle,
	)
	if err != nil {
		t.Fatalf("authentication with state should verify: %v", err)
	}
	if updated.SignCount != 1 {
		t.Fatalf("updated sign count = %d", updated.SignCount)
	}
	if updated.BackupEligible || updated.BackupState {
		t.Fatalf("updated backup flags = %+v", updated)
	}
}

func TestRejectsTamperedSignature(t *testing.T) {
	value := loadFixture(t)
	registration, relyingParty := verifyFixtureRegistration(t, value)
	signature := decodeFixture(t, value.Authentication.Signature)
	signature[0] ^= 0x01
	err := VerifyAuthentication(
		authenticationJSON(t, value, base64.RawURLEncoding.EncodeToString(signature)),
		decodeFixture(t, value.Authentication.Challenge),
		relyingParty,
		registration.Credential.ID,
		registration.Credential.PublicKeyCOSE,
		decodeFixture(t, value.ExpectedUserHandle),
	)
	if err != ErrInvalidSignature {
		t.Fatalf("error = %v", err)
	}
}

func TestRejectsOriginAndRPIDMismatch(t *testing.T) {
	value := loadFixture(t)
	challenge := decodeFixture(t, value.Registration.Challenge)

	otherOrigin, err := NewRelyingParty(value.RPID, "https://login.rc.example.test")
	if err != nil {
		t.Fatalf("other origin: %v", err)
	}
	if _, err := VerifyRegistration(registrationJSON(t, value), challenge, otherOrigin); err != ErrOriginMismatch {
		t.Fatalf("origin error = %v", err)
	}

	parentID, err := NewRelyingParty("example.test", value.Origin)
	if err != nil {
		t.Fatalf("parent relying party: %v", err)
	}
	if _, err := VerifyRegistration(registrationJSON(t, value), challenge, parentID); err != ErrRPIDHashMismatch {
		t.Fatalf("RP ID error = %v", err)
	}

	if _, err := NewRelyingParty("evil.example", value.Origin); err != ErrInvalidRelyingPartyID {
		t.Fatalf("error = %v", err)
	}
	if _, err := NewRelyingParty(value.RPID, "http://rc.example.test"); err != ErrInvalidOrigin {
		t.Fatalf("error = %v", err)
	}
}

func TestRejectsAlgorithmMismatch(t *testing.T) {
	value := loadFixture(t)
	registration, relyingParty := verifyFixtureRegistration(t, value)

	publicKey := append([]byte{}, registration.Credential.PublicKeyCOSE...)
	cosePrefix := []byte{0xa5, 0x01, 0x02, 0x03, 0x26}
	index := bytes.Index(publicKey, cosePrefix)
	if index < 0 {
		t.Fatal("fixture credential key is not ES256")
	}
	publicKey[index+4] = 0x27
	err := VerifyAuthentication(
		authenticationJSON(t, value, value.Authentication.Signature),
		decodeFixture(t, value.Authentication.Challenge),
		relyingParty,
		registration.Credential.ID,
		publicKey,
		decodeFixture(t, value.ExpectedUserHandle),
	)
	if err != ErrUnsupportedCoseAlgorithm {
		t.Fatalf("public key error = %v", err)
	}

	attestation := decodeFixture(t, value.Registration.AttestationObject)
	index = bytes.Index(attestation, cosePrefix)
	if index < 0 {
		t.Fatal("fixture attestation is not ES256")
	}
	attestation[index+4] = 0x27
	tampered, err := json.Marshal(map[string]any{
		"id":    value.Registration.CredentialID,
		"rawId": value.Registration.CredentialID,
		"type":  "public-key",
		"response": map[string]any{
			"clientDataJSON":    value.Registration.ClientDataJSON,
			"attestationObject": base64.RawURLEncoding.EncodeToString(attestation),
		},
	})
	if err != nil {
		t.Fatalf("marshal tampered registration: %v", err)
	}
	if _, err := VerifyRegistration(
		tampered,
		decodeFixture(t, value.Registration.Challenge),
		relyingParty,
	); err != ErrUnsupportedCoseAlgorithm {
		t.Fatalf("attestation error = %v", err)
	}
}

func TestCanonicalizesCredentialPublicKey(t *testing.T) {
	value := loadFixture(t)
	registration, _ := verifyFixtureRegistration(t, value)
	key, err := fromCOSEKey(registration.Credential.PublicKeyCOSE)
	if err != nil {
		t.Fatalf("decode credential key: %v", err)
	}
	if !bytes.Equal(key.coseKey(), registration.Credential.PublicKeyCOSE) {
		t.Fatal("credential key should be canonically encoded")
	}
	if len(registration.Credential.PublicKeyCOSE) != 77 {
		t.Fatalf("credential key length = %d", len(registration.Credential.PublicKeyCOSE))
	}
}
