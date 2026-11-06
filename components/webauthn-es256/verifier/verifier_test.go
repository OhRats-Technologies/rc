package verifier

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"testing"

	witTypes "go.bytecodealliance.org/pkg/wit/types"
	"wit_component/ohrats_rc_webauthn_types"
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

func registrationRequest(t *testing.T, value fixture) ohrats_rc_webauthn_types.RegistrationRequest {
	t.Helper()
	return ohrats_rc_webauthn_types.RegistrationRequest{
		RelyingParty: ohrats_rc_webauthn_types.RelyingParty{
			Id:     value.RPID,
			Origin: value.Origin,
		},
		Challenge:         decodeFixture(t, value.Registration.Challenge),
		CredentialId:      decodeFixture(t, value.Registration.CredentialID),
		ClientDataJson:    decodeFixture(t, value.Registration.ClientDataJSON),
		AttestationObject: decodeFixture(t, value.Registration.AttestationObject),
		UserVerification:  ohrats_rc_webauthn_types.UserVerification_Required,
	}
}

func authenticationRequest(
	t *testing.T,
	value fixture,
	credential ohrats_rc_webauthn_types.StoredCredential,
) ohrats_rc_webauthn_types.AuthenticationRequest {
	t.Helper()
	return ohrats_rc_webauthn_types.AuthenticationRequest{
		RelyingParty: ohrats_rc_webauthn_types.RelyingParty{
			Id:     value.RPID,
			Origin: value.Origin,
		},
		Challenge:          decodeFixture(t, value.Authentication.Challenge),
		CredentialId:       decodeFixture(t, value.Authentication.CredentialID),
		ClientDataJson:     decodeFixture(t, value.Authentication.ClientDataJSON),
		AuthenticatorData:  decodeFixture(t, value.Authentication.AuthenticatorData),
		Signature:          decodeFixture(t, value.Authentication.Signature),
		Credential:         credential,
		ExpectedUserHandle: decodeFixture(t, value.ExpectedUserHandle),
		ResponseUserHandle: witTypes.Some(decodeFixture(t, value.Authentication.UserHandle)),
		UserHandleMode:     ohrats_rc_webauthn_types.UserHandleMode_Identified,
		UserVerification:   ohrats_rc_webauthn_types.UserVerification_Required,
	}
}

func TestVerifiesRegistrationAndAuthentication(t *testing.T) {
	value := loadFixture(t)
	registration, err := Registration(value.Algorithm, registrationRequest(t, value))
	if err != nil {
		t.Fatalf("registration should verify: %v", err)
	}
	if registration.Credential.Algorithm != "es256" || registration.Credential.SignCount != 0 {
		t.Fatalf("registration credential = %+v", registration.Credential)
	}
	if !bytes.Equal(registration.Aaguid, bytes.Repeat([]byte{1}, 16)) || !registration.UserVerified {
		t.Fatalf("registration result = %+v", registration)
	}
	authentication, err := Authentication(
		value.Algorithm,
		authenticationRequest(t, value, registration.Credential),
	)
	if err != nil {
		t.Fatalf("authentication should verify: %v", err)
	}
	if authentication.Credential.SignCount != 1 ||
		!authentication.SignCountAdvanced ||
		!authentication.UserVerified {
		t.Fatalf("authentication result = %+v", authentication)
	}
}

func TestRejectsAlgorithmAndUserHandleMismatches(t *testing.T) {
	value := loadFixture(t)
	registration, err := Registration(value.Algorithm, registrationRequest(t, value))
	if err != nil {
		t.Fatalf("registration should verify: %v", err)
	}
	if _, err := Registration("rs256", registrationRequest(t, value)); err == nil ||
		err.Error() != `unsupported WebAuthn algorithm "rs256"` {
		t.Fatalf("error = %v", err)
	}
	credential := registration.Credential
	credential.Algorithm = "rs256"
	if _, err := Authentication(value.Algorithm, authenticationRequest(t, value, credential)); err == nil {
		t.Fatal("mismatched credential algorithm should fail")
	}

	discoverable := authenticationRequest(t, value, registration.Credential)
	discoverable.UserHandleMode = ohrats_rc_webauthn_types.UserHandleMode_Discoverable
	discoverable.ResponseUserHandle = witTypes.None[[]byte]()
	if _, err := Authentication(value.Algorithm, discoverable); err == nil ||
		err.Error() != "user handle is required for discoverable authentication" {
		t.Fatalf("error = %v", err)
	}

	empty := authenticationRequest(t, value, registration.Credential)
	empty.ExpectedUserHandle = []byte{}
	if _, err := Authentication(value.Algorithm, empty); err == nil ||
		err.Error() != "invalid WebAuthn user handle" {
		t.Fatalf("error = %v", err)
	}

	oversized := authenticationRequest(t, value, registration.Credential)
	oversized.ResponseUserHandle = witTypes.Some(bytes.Repeat([]byte{1}, 65))
	if _, err := Authentication(value.Algorithm, oversized); err == nil ||
		err.Error() != "invalid WebAuthn user handle" {
		t.Fatalf("error = %v", err)
	}
}

func TestRejectsTamperedSignatureAndWrongOrigin(t *testing.T) {
	value := loadFixture(t)
	registration, err := Registration(value.Algorithm, registrationRequest(t, value))
	if err != nil {
		t.Fatalf("registration should verify: %v", err)
	}
	tampered := authenticationRequest(t, value, registration.Credential)
	tampered.Signature = append([]byte{}, tampered.Signature...)
	tampered.Signature[0] ^= 0x01
	if _, err := Authentication(value.Algorithm, tampered); err == nil {
		t.Fatal("tampered signature should fail")
	}

	request := registrationRequest(t, value)
	request.RelyingParty.Origin = "https://other.example.test"
	if _, err := Registration(value.Algorithm, request); err == nil {
		t.Fatal("origin mismatch should fail")
	}
}
