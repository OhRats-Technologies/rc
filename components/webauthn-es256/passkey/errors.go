// Package passkey verifies WebAuthn registration and authentication responses.
//
// It is a Go translation of the passkey-rp 0.1.0 relying-party core: ES256
// credentials on P-256 and the "none" attestation format only. Unsupported
// algorithms or attestation formats fail closed.
package passkey

import "errors"

// Verification failures mirror the upstream PasskeyError display strings.
var (
	ErrInvalidRelyingPartyID = errors.New("relying party id is invalid")
	ErrInvalidOrigin         = errors.New("relying party origin is invalid")
	ErrInvalidChallenge      = errors.New("challenge length must be between 16 and 1024 bytes")
	ErrInvalidBase64URL      = errors.New("base64url data is invalid")
	ErrInvalidJSON           = errors.New("client JSON data is invalid")
	ErrInvalidCBOR           = errors.New("CBOR data is invalid")
	ErrInvalidClientDataType = errors.New("client data type is invalid")
	ErrChallengeMismatch     = errors.New("challenge does not match")
	ErrOriginMismatch        = errors.New("origin does not match relying party")
	ErrCrossOrigin           = errors.New("cross-origin ceremony is not allowed")
	ErrPayloadTooLarge       = errors.New("WebAuthn payload is too large")

	ErrInvalidAuthenticatorData  = errors.New("authenticator data is invalid")
	ErrRPIDHashMismatch          = errors.New("RP ID hash does not match")
	ErrUserPresenceRequired      = errors.New("user presence is required")
	ErrUserVerificationRequired  = errors.New("user verification is required")
	ErrAttestedCredentialMissing = errors.New("attested credential data is missing")
	ErrInvalidAttestation        = errors.New("attestation statement is invalid")
	ErrUnexpectedAttestedData    = errors.New("attested credential data is not allowed in an authentication assertion")
	ErrUnsupportedAttestation    = errors.New("attestation format is not supported")
	ErrInvalidPublicKey          = errors.New("credential public key is invalid")
	ErrUnsupportedCoseAlgorithm  = errors.New("COSE algorithm is not supported")
	ErrUnsupportedCoseKey        = errors.New("COSE key type is not supported")
	ErrInvalidSignature          = errors.New("signature is invalid")
	ErrSignCountRollback         = errors.New("signature counter did not advance")
	ErrEmptyCredentialID         = errors.New("credential id must not be empty")
	ErrCredentialIDTooLarge      = errors.New("credential id is too large")
	ErrCredentialIDMismatch      = errors.New("credential id does not match")
	ErrUserHandleRequired        = errors.New("user handle is required for discoverable authentication")
	ErrUserHandleMismatch        = errors.New("user handle does not match the expected account")
	ErrBackupEligibilityChanged  = errors.New("credential backup eligibility changed")
	ErrInvalidBackupState        = errors.New("credential backup state is invalid")
)
