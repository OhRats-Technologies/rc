// Package verifier adapts the portable passkey verifier to the component's
// WIT wire types.
package verifier

import (
	"errors"
	"fmt"

	"wit_component/ohrats_rc_webauthn_types"
	"wit_component/passkey"
)

const maxUserHandleBytes = 64

// Registration verifies a WebAuthn registration response.
func Registration(
	algorithm string,
	value ohrats_rc_webauthn_types.RegistrationRequest,
) (ohrats_rc_webauthn_types.VerifiedRegistration, error) {
	if err := requireAlgorithm(algorithm); err != nil {
		return ohrats_rc_webauthn_types.VerifiedRegistration{}, err
	}
	relyingParty, err := passkey.NewRelyingParty(value.RelyingParty.Id, value.RelyingParty.Origin)
	if err != nil {
		return ohrats_rc_webauthn_types.VerifiedRegistration{}, err
	}
	challenge, err := passkey.NewChallenge(value.Challenge)
	if err != nil {
		return ohrats_rc_webauthn_types.VerifiedRegistration{}, err
	}
	result, err := passkey.VerifyRegistration(passkey.RegistrationInput{
		RelyingParty:      relyingParty,
		Challenge:         challenge,
		CredentialID:      value.CredentialId,
		ClientDataJSON:    value.ClientDataJson,
		AttestationObject: value.AttestationObject,
		UserVerification:  policy(value.UserVerification),
	})
	if err != nil {
		return ohrats_rc_webauthn_types.VerifiedRegistration{}, err
	}
	return ohrats_rc_webauthn_types.VerifiedRegistration{
		Credential:   wireCredential(result.Credential),
		Aaguid:       result.AAGUID,
		UserVerified: result.UserVerified,
	}, nil
}

// Authentication verifies a WebAuthn authentication response.
func Authentication(
	algorithm string,
	value ohrats_rc_webauthn_types.AuthenticationRequest,
) (ohrats_rc_webauthn_types.VerifiedAuthentication, error) {
	if err := requireAlgorithm(algorithm); err != nil {
		return ohrats_rc_webauthn_types.VerifiedAuthentication{}, err
	}
	if err := validateUserHandle(value.ExpectedUserHandle, true); err != nil {
		return ohrats_rc_webauthn_types.VerifiedAuthentication{}, err
	}
	responseUserHandle := []byte{}
	responsePresent := value.ResponseUserHandle.IsSome()
	if responsePresent {
		responseUserHandle = value.ResponseUserHandle.Some()
		if err := validateUserHandle(responseUserHandle, true); err != nil {
			return ohrats_rc_webauthn_types.VerifiedAuthentication{}, err
		}
	}
	relyingParty, err := passkey.NewRelyingParty(value.RelyingParty.Id, value.RelyingParty.Origin)
	if err != nil {
		return ohrats_rc_webauthn_types.VerifiedAuthentication{}, err
	}
	challenge, err := passkey.NewChallenge(value.Challenge)
	if err != nil {
		return ohrats_rc_webauthn_types.VerifiedAuthentication{}, err
	}
	credential, err := storedCredential(value.Credential)
	if err != nil {
		return ohrats_rc_webauthn_types.VerifiedAuthentication{}, err
	}
	decoded, err := passkey.DecodeCredential(credential)
	if err != nil {
		return ohrats_rc_webauthn_types.VerifiedAuthentication{}, err
	}
	result, err := passkey.VerifyAuthentication(passkey.AuthenticationInput{
		RelyingParty:       relyingParty,
		Challenge:          challenge,
		CredentialID:       value.CredentialId,
		ClientDataJSON:     value.ClientDataJson,
		AuthenticatorData:  value.AuthenticatorData,
		Signature:          value.Signature,
		Credential:         decoded,
		ExpectedUserHandle: value.ExpectedUserHandle,
		ResponseUserHandle: responseUserHandle,
		ResponsePresent:    responsePresent,
		UserHandleMode:     userHandleMode(value.UserHandleMode),
		UserVerification:   policy(value.UserVerification),
	})
	if err != nil {
		return ohrats_rc_webauthn_types.VerifiedAuthentication{}, err
	}
	return ohrats_rc_webauthn_types.VerifiedAuthentication{
		Credential:        wireCredential(result.Credential),
		UserVerified:      result.UserVerified,
		SignCountAdvanced: result.SignCountAdvanced,
	}, nil
}

func requireAlgorithm(value string) error {
	if value == passkey.Algorithm {
		return nil
	}
	return fmt.Errorf("unsupported WebAuthn algorithm %q", value)
}

func policy(value ohrats_rc_webauthn_types.UserVerification) passkey.UserVerificationPolicy {
	if value == ohrats_rc_webauthn_types.UserVerification_Preferred {
		return passkey.UserVerificationPreferred
	}
	return passkey.UserVerificationRequired
}

func userHandleMode(value ohrats_rc_webauthn_types.UserHandleMode) passkey.UserHandleMode {
	if value == ohrats_rc_webauthn_types.UserHandleMode_Discoverable {
		return passkey.UserHandleDiscoverable
	}
	return passkey.UserHandleIdentified
}

func validateUserHandle(value []byte, required bool) error {
	if len(value) <= maxUserHandleBytes && (!required || len(value) != 0) {
		return nil
	}
	return errors.New("invalid WebAuthn user handle")
}

func wireCredential(value passkey.Credential) ohrats_rc_webauthn_types.StoredCredential {
	return ohrats_rc_webauthn_types.StoredCredential{
		Id:             value.ID,
		Algorithm:      value.Algorithm,
		PublicKeyCose:  value.PublicKeyCOSE,
		SignCount:      value.SignCount,
		BackupEligible: value.BackupEligible,
		BackupState:    value.BackupState,
	}
}

func storedCredential(value ohrats_rc_webauthn_types.StoredCredential) (passkey.Credential, error) {
	if err := requireAlgorithm(value.Algorithm); err != nil {
		return passkey.Credential{}, err
	}
	return passkey.Credential{
		ID:             value.Id,
		Algorithm:      value.Algorithm,
		PublicKeyCOSE:  value.PublicKeyCose,
		SignCount:      value.SignCount,
		BackupEligible: value.BackupEligible,
		BackupState:    value.BackupState,
	}, nil
}
