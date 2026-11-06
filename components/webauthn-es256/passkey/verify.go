package passkey

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

// UserVerificationPolicy controls whether the UV flag is mandatory.
type UserVerificationPolicy int

const (
	UserVerificationRequired UserVerificationPolicy = iota
	UserVerificationPreferred
)

func (policy UserVerificationPolicy) required() bool {
	return policy == UserVerificationRequired
}

// UserHandleMode selects the account-binding rule for an assertion.
type UserHandleMode int

const (
	UserHandleIdentified UserHandleMode = iota
	UserHandleDiscoverable
)

// Credential is validated credential state persisted by the relying party.
type Credential struct {
	ID             []byte
	Algorithm      string
	PublicKeyCOSE  []byte
	SignCount      uint32
	BackupEligible bool
	BackupState    bool
}

// NewChallenge validates a server-side ceremony challenge.
func NewChallenge(value []byte) ([]byte, error) {
	if len(value) < 16 || len(value) > 1024 {
		return nil, ErrInvalidChallenge
	}
	return copyBytes(value), nil
}

// DecodeCredential validates stored credential state before a ceremony.
func DecodeCredential(value Credential) (Credential, error) {
	key, err := fromCOSEKey(value.PublicKeyCOSE)
	if err != nil {
		return Credential{}, err
	}
	if value.BackupState && !value.BackupEligible {
		return Credential{}, ErrInvalidBackupState
	}
	if len(value.ID) == 0 {
		return Credential{}, ErrEmptyCredentialID
	}
	if len(value.ID) > maxCredentialIDBytes {
		return Credential{}, ErrCredentialIDTooLarge
	}
	value.ID = copyBytes(value.ID)
	value.Algorithm = Algorithm
	value.PublicKeyCOSE = key.coseKey()
	return value, nil
}

// RegistrationInput carries one decoded registration ceremony response.
type RegistrationInput struct {
	RelyingParty      RelyingParty
	Challenge         []byte
	CredentialID      []byte
	ClientDataJSON    []byte
	AttestationObject []byte
	UserVerification  UserVerificationPolicy
}

// RegistrationResult is the outcome of a verified registration response.
type RegistrationResult struct {
	Credential   Credential
	AAGUID       []byte
	UserVerified bool
}

// AuthenticationInput carries one decoded authentication ceremony response.
type AuthenticationInput struct {
	RelyingParty       RelyingParty
	Challenge          []byte
	CredentialID       []byte
	ClientDataJSON     []byte
	AuthenticatorData  []byte
	Signature          []byte
	Credential         Credential
	ExpectedUserHandle []byte
	ResponseUserHandle []byte
	ResponsePresent    bool
	UserHandleMode     UserHandleMode
	UserVerification   UserVerificationPolicy
}

// AuthenticationResult is the outcome of a verified authentication response.
type AuthenticationResult struct {
	Credential        Credential
	UserVerified      bool
	SignCountAdvanced bool
}

// VerifyRegistration verifies a "none"-attestation creation ceremony.
func VerifyRegistration(input RegistrationInput) (RegistrationResult, error) {
	if err := verifyClientData(
		input.ClientDataJSON,
		"webauthn.create",
		input.Challenge,
		input.RelyingParty.Origin,
	); err != nil {
		return RegistrationResult{}, err
	}
	data, err := parseNoneAttestation(input.AttestationObject)
	if err != nil {
		return RegistrationResult{}, err
	}
	if err := verifyAuthenticatorPolicy(data, input.RelyingParty, input.UserVerification); err != nil {
		return RegistrationResult{}, err
	}
	if data.attested == nil {
		return RegistrationResult{}, ErrAttestedCredentialMissing
	}
	if !bytes.Equal(input.CredentialID, data.attested.credentialID) {
		return RegistrationResult{}, ErrCredentialIDMismatch
	}
	credential, err := newCredential(
		data.attested.credentialID,
		data.attested.publicKey,
		data.signCount,
		data.backupEligible(),
		data.backupState(),
	)
	if err != nil {
		return RegistrationResult{}, err
	}
	return RegistrationResult{
		Credential:   credential,
		AAGUID:       copyBytes(data.attested.aaguid),
		UserVerified: data.userVerified(),
	}, nil
}

// VerifyAuthentication verifies an assertion against trusted credential state.
func VerifyAuthentication(input AuthenticationInput) (AuthenticationResult, error) {
	if len(input.Signature) == 0 || len(input.Signature) > maxSignatureBytes {
		return AuthenticationResult{}, ErrInvalidSignature
	}
	if !bytes.Equal(input.CredentialID, input.Credential.ID) {
		return AuthenticationResult{}, ErrCredentialIDMismatch
	}
	if err := verifyUserHandle(input); err != nil {
		return AuthenticationResult{}, err
	}
	if err := verifyClientData(
		input.ClientDataJSON,
		"webauthn.get",
		input.Challenge,
		input.RelyingParty.Origin,
	); err != nil {
		return AuthenticationResult{}, err
	}
	data, err := parseAuthenticatorData(input.AuthenticatorData)
	if err != nil {
		return AuthenticationResult{}, err
	}
	if data.attested != nil {
		return AuthenticationResult{}, ErrUnexpectedAttestedData
	}
	if err := verifyAuthenticatorPolicy(data, input.RelyingParty, input.UserVerification); err != nil {
		return AuthenticationResult{}, err
	}
	if data.backupEligible() != input.Credential.BackupEligible {
		return AuthenticationResult{}, ErrBackupEligibilityChanged
	}
	key, err := fromCOSEKey(input.Credential.PublicKeyCOSE)
	if err != nil {
		return AuthenticationResult{}, err
	}
	signed := make([]byte, 0, len(input.AuthenticatorData)+32)
	signed = append(signed, input.AuthenticatorData...)
	signed = append(signed, hashClientData(input.ClientDataJSON)...)
	if err := key.verify(signed, input.Signature); err != nil {
		return AuthenticationResult{}, err
	}
	if err := verifySignCount(input.Credential.SignCount, data.signCount); err != nil {
		return AuthenticationResult{}, err
	}
	credential, err := newCredential(
		input.Credential.ID,
		key,
		data.signCount,
		data.backupEligible(),
		data.backupState(),
	)
	if err != nil {
		return AuthenticationResult{}, err
	}
	return AuthenticationResult{
		Credential:        credential,
		UserVerified:      data.userVerified(),
		SignCountAdvanced: data.signCount > input.Credential.SignCount,
	}, nil
}

func newCredential(
	id []byte,
	key publicKey,
	signCount uint32,
	backupEligible bool,
	backupState bool,
) (Credential, error) {
	if len(id) == 0 {
		return Credential{}, ErrEmptyCredentialID
	}
	if len(id) > maxCredentialIDBytes {
		return Credential{}, ErrCredentialIDTooLarge
	}
	if backupState && !backupEligible {
		return Credential{}, ErrInvalidBackupState
	}
	return Credential{
		ID:             copyBytes(id),
		Algorithm:      Algorithm,
		PublicKeyCOSE:  key.coseKey(),
		SignCount:      signCount,
		BackupEligible: backupEligible,
		BackupState:    backupState,
	}, nil
}

func verifyAuthenticatorPolicy(
	data authenticatorData,
	relyingParty RelyingParty,
	policy UserVerificationPolicy,
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

func verifyUserHandle(input AuthenticationInput) error {
	if input.ResponsePresent {
		if !bytes.Equal(input.ResponseUserHandle, input.ExpectedUserHandle) {
			return ErrUserHandleMismatch
		}
		return nil
	}
	if input.UserHandleMode == UserHandleDiscoverable {
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
