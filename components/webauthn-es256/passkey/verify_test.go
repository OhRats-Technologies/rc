package passkey

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

func verifyFixtureRegistration(t *testing.T, value fixture) RegistrationResult {
	t.Helper()
	challenge, err := NewChallenge(decodeFixture(t, value.Registration.Challenge))
	if err != nil {
		t.Fatalf("challenge: %v", err)
	}
	result, err := VerifyRegistration(RegistrationInput{
		RelyingParty:      fixtureRelyingParty(t, value),
		Challenge:         challenge,
		CredentialID:      decodeFixture(t, value.Registration.CredentialID),
		ClientDataJSON:    decodeFixture(t, value.Registration.ClientDataJSON),
		AttestationObject: decodeFixture(t, value.Registration.AttestationObject),
		UserVerification:  UserVerificationRequired,
	})
	if err != nil {
		t.Fatalf("registration should verify: %v", err)
	}
	return result
}

func fixtureAuthenticationInput(t *testing.T, value fixture, credential Credential) AuthenticationInput {
	t.Helper()
	challenge, err := NewChallenge(decodeFixture(t, value.Authentication.Challenge))
	if err != nil {
		t.Fatalf("challenge: %v", err)
	}
	return AuthenticationInput{
		RelyingParty:       fixtureRelyingParty(t, value),
		Challenge:          challenge,
		CredentialID:       decodeFixture(t, value.Authentication.CredentialID),
		ClientDataJSON:     decodeFixture(t, value.Authentication.ClientDataJSON),
		AuthenticatorData:  decodeFixture(t, value.Authentication.AuthenticatorData),
		Signature:          decodeFixture(t, value.Authentication.Signature),
		Credential:         credential,
		ExpectedUserHandle: decodeFixture(t, value.ExpectedUserHandle),
		ResponseUserHandle: decodeFixture(t, value.Authentication.UserHandle),
		ResponsePresent:    true,
		UserHandleMode:     UserHandleIdentified,
		UserVerification:   UserVerificationRequired,
	}
}

func TestVerifiesRegistrationAndAuthentication(t *testing.T) {
	value := loadFixture(t)
	registration := verifyFixtureRegistration(t, value)
	if registration.Credential.Algorithm != "es256" {
		t.Fatalf("algorithm = %q", registration.Credential.Algorithm)
	}
	if registration.Credential.SignCount != 0 {
		t.Fatalf("sign count = %d", registration.Credential.SignCount)
	}
	if !bytes.Equal(registration.AAGUID, bytes.Repeat([]byte{1}, 16)) {
		t.Fatalf("aaguid = %x", registration.AAGUID)
	}
	if !registration.UserVerified {
		t.Fatal("registration should be user verified")
	}

	authentication, err := VerifyAuthentication(
		fixtureAuthenticationInput(t, value, registration.Credential),
	)
	if err != nil {
		t.Fatalf("authentication should verify: %v", err)
	}
	if authentication.Credential.SignCount != 1 || !authentication.SignCountAdvanced {
		t.Fatalf("authentication credential state = %+v", authentication)
	}
	if !authentication.UserVerified {
		t.Fatal("authentication should be user verified")
	}
}

func TestRejectsTamperingAndMissingDiscoverableUserHandles(t *testing.T) {
	value := loadFixture(t)
	registration := verifyFixtureRegistration(t, value)

	tampered := fixtureAuthenticationInput(t, value, registration.Credential)
	tampered.Signature[0] ^= 0x01
	if _, err := VerifyAuthentication(tampered); err == nil {
		t.Fatal("tampered signature should fail")
	}

	discoverable := fixtureAuthenticationInput(t, value, registration.Credential)
	discoverable.UserHandleMode = UserHandleDiscoverable
	discoverable.ResponsePresent = false
	discoverable.ResponseUserHandle = nil
	if _, err := VerifyAuthentication(discoverable); err != ErrUserHandleRequired {
		t.Fatalf("error = %v", err)
	}
}

func TestRejectsOriginAndAttestationMismatches(t *testing.T) {
	value := loadFixture(t)
	challenge, err := NewChallenge(decodeFixture(t, value.Registration.Challenge))
	if err != nil {
		t.Fatalf("challenge: %v", err)
	}
	other, err := NewRelyingParty(value.RPID, "https://login.rc.example.test")
	if err != nil {
		t.Fatalf("other relying party: %v", err)
	}
	_, err = VerifyRegistration(RegistrationInput{
		RelyingParty:      other,
		Challenge:         challenge,
		CredentialID:      decodeFixture(t, value.Registration.CredentialID),
		ClientDataJSON:    decodeFixture(t, value.Registration.ClientDataJSON),
		AttestationObject: decodeFixture(t, value.Registration.AttestationObject),
		UserVerification:  UserVerificationRequired,
	})
	if err != ErrOriginMismatch {
		t.Fatalf("error = %v", err)
	}

	if _, err := NewRelyingParty("evil.example", value.Origin); err != ErrInvalidRelyingPartyID {
		t.Fatalf("error = %v", err)
	}
	if _, err := NewRelyingParty(value.RPID, "http://rc.example.test"); err != ErrInvalidOrigin {
		t.Fatalf("error = %v", err)
	}
}

func TestCanonicalizesCredentialPublicKey(t *testing.T) {
	value := loadFixture(t)
	registration := verifyFixtureRegistration(t, value)
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
