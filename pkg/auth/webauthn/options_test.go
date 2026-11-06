package webauthn

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestCreationAndRequestOptions(t *testing.T) {
	rp, err := NewRelyingParty("rc.example.test", "https://rc.example.test")
	if err != nil {
		t.Fatalf("relying party: %v", err)
	}
	if !rp.Secure {
		t.Fatal("https relying party should be secure")
	}
	existing := []byte("existing-credential")
	challenge, options, err := CreationOptions("user-handle", "Alice", rp, [][]byte{existing})
	if err != nil {
		t.Fatalf("creation options: %v", err)
	}
	if len(challenge) != 32 {
		t.Fatalf("challenge length = %d", len(challenge))
	}
	if options["challenge"] != base64.RawURLEncoding.EncodeToString(challenge) {
		t.Fatalf("challenge option = %v", options["challenge"])
	}
	if options["attestation"] != "none" || options["timeout"] != CeremonyTimeoutMs {
		t.Fatalf("options = %v", options)
	}
	params, ok := options["pubKeyCredParams"].([]any)
	if !ok || len(params) != 1 {
		t.Fatalf("pubKeyCredParams = %v", options["pubKeyCredParams"])
	}
	param := params[0].(map[string]any)
	if param["alg"] != COSEAlgorithmES256 || param["type"] != "public-key" {
		t.Fatalf("parameter = %v", param)
	}
	selection := options["authenticatorSelection"].(map[string]any)
	if selection["residentKey"] != "required" || selection["requireResidentKey"] != true ||
		selection["userVerification"] != "required" {
		t.Fatalf("authenticatorSelection = %v", selection)
	}
	rpOption := options["rp"].(map[string]any)
	if rpOption["id"] != rp.ID || rpOption["name"] != "RC" {
		t.Fatalf("rp = %v", rpOption)
	}
	userOption := options["user"].(map[string]any)
	if userOption["id"] != base64.RawURLEncoding.EncodeToString([]byte("user-handle")) ||
		userOption["name"] != "Alice" || userOption["displayName"] != "Alice" {
		t.Fatalf("user = %v", userOption)
	}
	exclude := options["excludeCredentials"].([]any)
	if len(exclude) != 1 ||
		exclude[0].(map[string]any)["id"] != base64.RawURLEncoding.EncodeToString(existing) {
		t.Fatalf("excludeCredentials = %v", exclude)
	}

	newChallenge, requestOptions, err := RequestOptions([][]byte{existing}, rp)
	if err != nil {
		t.Fatalf("request options: %v", err)
	}
	if len(newChallenge) != 32 {
		t.Fatalf("challenge length = %d", len(newChallenge))
	}
	if requestOptions["rpId"] != rp.ID || requestOptions["userVerification"] != "required" {
		t.Fatalf("request options = %v", requestOptions)
	}
	allow := requestOptions["allowCredentials"].([]any)
	if len(allow) != 1 ||
		allow[0].(map[string]any)["id"] != base64.RawURLEncoding.EncodeToString(existing) {
		t.Fatalf("allowCredentials = %v", allow)
	}

	if _, _, err := RequestOptions(nil, rp); err != ErrNoCredentials {
		t.Fatalf("empty credentials error = %v", err)
	}
	if _, _, err := CreationOptions("", "Alice", rp, nil); err != ErrInvalidUserHandle {
		t.Fatalf("empty user error = %v", err)
	}
	if _, _, err := CreationOptions("user", " ", rp, nil); err != ErrInvalidName {
		t.Fatalf("empty name error = %v", err)
	}
	if _, _, err := CreationOptions("user", "Alice", RelyingParty{ID: "other.test", Origin: "https://rc.example.test"}, nil); err != ErrInvalidRelyingPartyID {
		t.Fatalf("relying party error = %v", err)
	}
}

func TestRelyingPartyFromDeploymentURL(t *testing.T) {
	rp, err := RelyingPartyFromURL("https://rc.example.test/")
	if err != nil {
		t.Fatalf("from URL: %v", err)
	}
	if rp.ID != "rc.example.test" || rp.Origin != "https://rc.example.test" || !rp.Secure {
		t.Fatalf("relying party = %+v", rp)
	}

	rp, err = RelyingPartyFromRequest("", "http", "localhost:8080", [][2]string{
		{"X-Forwarded-Proto", "https"},
		{"X-Forwarded-Host", "rc.example.test"},
	})
	if err != nil {
		t.Fatalf("from forwarded request: %v", err)
	}
	if rp.ID != "rc.example.test" || rp.Origin != "https://rc.example.test" || !rp.Secure {
		t.Fatalf("forwarded relying party = %+v", rp)
	}

	rp, err = RelyingPartyFromRequest("https://rc.example.test", "http", "localhost:8080", nil)
	if err != nil || rp.ID != "rc.example.test" {
		t.Fatalf("configured relying party = %+v, %v", rp, err)
	}

	if _, err := RelyingPartyFromURL("http://rc.example.test"); err == nil {
		t.Fatal("plain HTTP outside localhost should fail")
	}
	if _, err := RelyingPartyFromURL(""); err == nil {
		t.Fatal("empty public URL should fail")
	}
}

func TestRejectsMalformedCeremonyResponses(t *testing.T) {
	value := loadFixture(t)
	registration, relyingParty := verifyFixtureRegistration(t, value)
	challenge := decodeFixture(t, value.Authentication.Challenge)
	userHandle := decodeFixture(t, value.ExpectedUserHandle)

	if _, err := VerifyRegistration([]byte("not json"), decodeFixture(t, value.Registration.Challenge), relyingParty); err != ErrInvalidResponse {
		t.Fatalf("invalid JSON error = %v", err)
	}
	missing, err := json.Marshal(map[string]any{"id": "a", "rawId": "a", "type": "public-key"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if _, err := VerifyRegistration(missing, decodeFixture(t, value.Registration.Challenge), relyingParty); err != ErrInvalidResponse {
		t.Fatalf("missing response error = %v", err)
	}
	wrongType, err := json.Marshal(map[string]any{
		"id":    value.Registration.CredentialID,
		"rawId": value.Registration.CredentialID,
		"type":  "password",
		"response": map[string]any{
			"clientDataJSON":    value.Registration.ClientDataJSON,
			"attestationObject": value.Registration.AttestationObject,
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if _, err := VerifyRegistration(wrongType, decodeFixture(t, value.Registration.Challenge), relyingParty); err != ErrInvalidResponse {
		t.Fatalf("type error = %v", err)
	}
	if _, err := VerifyRegistration(
		registrationJSON(t, value),
		bytes.Repeat([]byte{7}, 32),
		relyingParty,
	); err != ErrChallengeMismatch {
		t.Fatalf("challenge error = %v", err)
	}

	response := authenticationJSON(t, value, value.Authentication.Signature)
	if err := VerifyAuthentication(response, challenge, relyingParty, nil, registration.Credential.PublicKeyCOSE, userHandle); err != ErrEmptyCredentialID {
		t.Fatalf("empty credential error = %v", err)
	}
	if err := VerifyAuthentication(response, challenge, relyingParty, registration.Credential.ID, registration.Credential.PublicKeyCOSE, nil); err != ErrInvalidUserHandle {
		t.Fatalf("empty user handle error = %v", err)
	}
	if err := VerifyAuthentication(response, challenge, relyingParty, registration.Credential.ID, registration.Credential.PublicKeyCOSE, []byte("another-user")); err != ErrUserHandleMismatch {
		t.Fatalf("user handle error = %v", err)
	}
	if err := VerifyAuthentication(response, challenge, relyingParty, []byte("another-credential"), registration.Credential.PublicKeyCOSE, userHandle); err != ErrCredentialIDMismatch {
		t.Fatalf("credential mismatch error = %v", err)
	}
	withoutUserHandle := map[string]any{
		"id":    value.Authentication.CredentialID,
		"rawId": value.Authentication.CredentialID,
		"type":  "public-key",
		"response": map[string]any{
			"clientDataJSON":    value.Authentication.ClientDataJSON,
			"authenticatorData": value.Authentication.AuthenticatorData,
			"signature":         value.Authentication.Signature,
		},
	}
	strippedJSON, err := json.Marshal(withoutUserHandle)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := VerifyAuthentication(strippedJSON, challenge, relyingParty, registration.Credential.ID, registration.Credential.PublicKeyCOSE, userHandle); err != nil {
		t.Fatalf("identified authentication without user handle should verify: %v", err)
	}
}
