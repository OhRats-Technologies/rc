package webauthn

import (
	"bytes"
	"crypto/sha256"
)

// Algorithm is the only supported WebAuthn credential algorithm.
const Algorithm = "es256"

const (
	maxAttestationObjectBytes = 1024 * 1024
	maxSignatureBytes         = 1024
)

// userVerificationPolicy controls whether the UV flag is mandatory.
type userVerificationPolicy int

const (
	userVerificationRequired userVerificationPolicy = iota
	userVerificationPreferred
)

func (policy userVerificationPolicy) required() bool {
	return policy == userVerificationRequired
}

// userHandleMode selects the account-binding rule for an assertion.
type userHandleMode int

const (
	userHandleIdentified userHandleMode = iota
	userHandleDiscoverable
)

// registrationInput carries one decoded registration ceremony response.
type registrationInput struct {
	relyingParty      RelyingParty
	challenge         []byte
	credentialID      []byte
	clientDataJSON    []byte
	attestationObject []byte
	userVerification  userVerificationPolicy
}

// registrationResult is the outcome of a verified registration response.
type registrationResult struct {
	credential   Credential
	aaguid       []byte
	userVerified bool
}

// authenticationInput carries one decoded authentication ceremony response.
//
// checkBackupEligibility compares the BE flag with stored credential state. It
// must stay enabled when the relying party persists that flag; callers that
// only store the credential ID and public key leave it disabled.
type authenticationInput struct {
	relyingParty           RelyingParty
	challenge              []byte
	credentialID           []byte
	clientDataJSON         []byte
	authenticatorData      []byte
	signature              []byte
	credential             Credential
	expectedUserHandle     []byte
	responseUserHandle     []byte
	responsePresent        bool
	userHandleMode         userHandleMode
	userVerification       userVerificationPolicy
	checkBackupEligibility bool
}

// authenticationResult is the outcome of a verified authentication response.
type authenticationResult struct {
	credential        Credential
	userVerified      bool
	signCountAdvanced bool
}

// verifyRegistration verifies a "none"-attestation creation ceremony.
func verifyRegistration(input registrationInput) (registrationResult, error) {
	relyingParty, err := normalizeRelyingParty(input.relyingParty)
	if err != nil {
		return registrationResult{}, err
	}
	if err := verifyClientData(
		input.clientDataJSON,
		"webauthn.create",
		input.challenge,
		relyingParty.Origin,
	); err != nil {
		return registrationResult{}, err
	}
	data, err := parseNoneAttestation(input.attestationObject)
	if err != nil {
		return registrationResult{}, err
	}
	if err := verifyAuthenticatorPolicy(data, relyingParty, input.userVerification); err != nil {
		return registrationResult{}, err
	}
	if data.attested == nil {
		return registrationResult{}, ErrAttestedCredentialMissing
	}
	if !bytes.Equal(input.credentialID, data.attested.credentialID) {
		return registrationResult{}, ErrCredentialIDMismatch
	}
	credential, err := newCredential(
		data.attested.credentialID,
		data.attested.publicKey,
		data.signCount,
		data.backupEligible(),
		data.backupState(),
	)
	if err != nil {
		return registrationResult{}, err
	}
	return registrationResult{
		credential:   credential,
		aaguid:       copyBytes(data.attested.aaguid),
		userVerified: data.userVerified(),
	}, nil
}

// verifyAuthentication verifies an assertion against trusted credential state.
func verifyAuthentication(input authenticationInput) (authenticationResult, error) {
	relyingParty, err := normalizeRelyingParty(input.relyingParty)
	if err != nil {
		return authenticationResult{}, err
	}
	if len(input.signature) == 0 || len(input.signature) > maxSignatureBytes {
		return authenticationResult{}, ErrInvalidSignature
	}
	if !bytes.Equal(input.credentialID, input.credential.ID) {
		return authenticationResult{}, ErrCredentialIDMismatch
	}
	if err := checkUserHandleBinding(input); err != nil {
		return authenticationResult{}, err
	}
	if err := verifyClientData(
		input.clientDataJSON,
		"webauthn.get",
		input.challenge,
		relyingParty.Origin,
	); err != nil {
		return authenticationResult{}, err
	}
	data, err := parseAuthenticatorData(input.authenticatorData)
	if err != nil {
		return authenticationResult{}, err
	}
	if data.attested != nil {
		return authenticationResult{}, ErrUnexpectedAttestedData
	}
	if err := verifyAuthenticatorPolicy(data, relyingParty, input.userVerification); err != nil {
		return authenticationResult{}, err
	}
	if input.checkBackupEligibility && data.backupEligible() != input.credential.BackupEligible {
		return authenticationResult{}, ErrBackupEligibilityChanged
	}
	key, err := fromCOSEKey(input.credential.PublicKeyCOSE)
	if err != nil {
		return authenticationResult{}, err
	}
	signed := make([]byte, 0, len(input.authenticatorData)+32)
	signed = append(signed, input.authenticatorData...)
	signed = append(signed, hashClientData(input.clientDataJSON)...)
	if err := key.verify(signed, input.signature); err != nil {
		return authenticationResult{}, err
	}
	if err := verifySignCount(input.credential.SignCount, data.signCount); err != nil {
		return authenticationResult{}, err
	}
	credential, err := newCredential(
		input.credential.ID,
		key,
		data.signCount,
		data.backupEligible(),
		data.backupState(),
	)
	if err != nil {
		return authenticationResult{}, err
	}
	return authenticationResult{
		credential:        credential,
		userVerified:      data.userVerified(),
		signCountAdvanced: data.signCount > input.credential.SignCount,
	}, nil
}

func verifyAuthenticatorPolicy(
	data authenticatorData,
	relyingParty RelyingParty,
	policy userVerificationPolicy,
) error {
	if !bytes.Equal(data.rpIDHash, relyingParty.idHash()) {
		return ErrRPIDHashMismatch
	}
	if !data.userPresent() {
		return ErrUserPresenceRequired
	}
	if policy.required() && !data.userVerified() {
		return ErrUserVerificationRequired
	}
	return nil
}

func checkUserHandleBinding(input authenticationInput) error {
	if input.responsePresent {
		if !bytes.Equal(input.responseUserHandle, input.expectedUserHandle) {
			return ErrUserHandleMismatch
		}
		return nil
	}
	if input.userHandleMode == userHandleDiscoverable {
		return ErrUserHandleRequired
	}
	return nil
}

func verifySignCount(stored uint32, current uint32) error {
	if stored == 0 && current == 0 || current > stored {
		return nil
	}
	return ErrSignCountRollback
}

func hashClientData(raw []byte) []byte {
	digest := sha256.Sum256(raw)
	return digest[:]
}
