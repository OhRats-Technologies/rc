// Package webauthnfixture checks the installed WebAuthn verifier against the
// deterministic ES256 fixture shared with the portable verifier tests.
package webauthnfixture

import (
	_ "embed"
	"encoding/base64"
	"encoding/json"

	witTypes "go.bytecodealliance.org/pkg/wit/types"
	"wit_component/ohrats_rc_webauthn_types"
)

//go:embed webauthn-es256.json
var fixtureJSON []byte

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

func load() (fixture, error) {
	var value fixture
	if err := json.Unmarshal(fixtureJSON, &value); err != nil {
		return fixture{}, err
	}
	return value, nil
}

func (value fixture) registrationRequest() (ohrats_rc_webauthn_types.RegistrationRequest, error) {
	challenge, err := decode(value.Registration.Challenge)
	if err != nil {
		return ohrats_rc_webauthn_types.RegistrationRequest{}, err
	}
	credentialID, err := decode(value.Registration.CredentialID)
	if err != nil {
		return ohrats_rc_webauthn_types.RegistrationRequest{}, err
	}
	clientDataJSON, err := decode(value.Registration.ClientDataJSON)
	if err != nil {
		return ohrats_rc_webauthn_types.RegistrationRequest{}, err
	}
	attestationObject, err := decode(value.Registration.AttestationObject)
	if err != nil {
		return ohrats_rc_webauthn_types.RegistrationRequest{}, err
	}
	return ohrats_rc_webauthn_types.RegistrationRequest{
		RelyingParty: ohrats_rc_webauthn_types.RelyingParty{
			Id:     value.RPID,
			Origin: value.Origin,
		},
		Challenge:         challenge,
		CredentialId:      credentialID,
		ClientDataJson:    clientDataJSON,
		AttestationObject: attestationObject,
		UserVerification:  ohrats_rc_webauthn_types.UserVerification_Required,
	}, nil
}

func (value fixture) authenticationRequest(
	credential ohrats_rc_webauthn_types.StoredCredential,
) (ohrats_rc_webauthn_types.AuthenticationRequest, error) {
	fields := make([][]byte, 7)
	for index, source := range []string{
		value.Authentication.Challenge,
		value.Authentication.CredentialID,
		value.Authentication.ClientDataJSON,
		value.Authentication.AuthenticatorData,
		value.Authentication.Signature,
		value.ExpectedUserHandle,
		value.Authentication.UserHandle,
	} {
		decoded, err := decode(source)
		if err != nil {
			return ohrats_rc_webauthn_types.AuthenticationRequest{}, err
		}
		fields[index] = decoded
	}
	return ohrats_rc_webauthn_types.AuthenticationRequest{
		RelyingParty: ohrats_rc_webauthn_types.RelyingParty{
			Id:     value.RPID,
			Origin: value.Origin,
		},
		Challenge:          fields[0],
		CredentialId:       fields[1],
		ClientDataJson:     fields[2],
		AuthenticatorData:  fields[3],
		Signature:          fields[4],
		Credential:         credential,
		ExpectedUserHandle: fields[5],
		ResponseUserHandle: witTypes.Some(fields[6]),
		UserHandleMode:     ohrats_rc_webauthn_types.UserHandleMode_Identified,
		UserVerification:   ohrats_rc_webauthn_types.UserVerification_Required,
	}, nil
}

func decode(value string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(value)
}
