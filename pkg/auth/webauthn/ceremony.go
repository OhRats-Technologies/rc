package webauthn

import "encoding/json"

const maxUserHandleBytes = 64

// Registration is the outcome of a verified registration response.
//
// UserHandle is only populated when the client echoes one; the WebAuthn
// registration response does not normally carry it, so rely on the account
// recorded with the ceremony instead.
type Registration struct {
	Credential   Credential
	UserHandle   []byte
	AAGUID       []byte
	UserVerified bool
}

// VerifyRegistration verifies a client registration response against the
// server-side challenge and relying party.
func VerifyRegistration(
	responseJSON []byte,
	challenge []byte,
	rp RelyingParty,
) (Registration, error) {
	parsed, inner, err := parseCredentialResponse[registrationResponse](responseJSON)
	if err != nil {
		return Registration{}, err
	}
	if parsed.RawID == nil || parsed.ID == nil || parsed.Type == nil ||
		*parsed.Type != "public-key" ||
		inner.ClientDataJSON == nil || inner.AttestationObject == nil {
		return Registration{}, ErrInvalidResponse
	}
	credentialID, err := decodeCredentialID(*parsed.RawID)
	if err != nil {
		return Registration{}, err
	}
	expectedChallenge, err := newChallenge(challenge)
	if err != nil {
		return Registration{}, err
	}
	clientData, err := decodeClientData(*inner.ClientDataJSON)
	if err != nil {
		return Registration{}, err
	}
	attestation, err := decodeAttestation(*inner.AttestationObject)
	if err != nil {
		return Registration{}, err
	}
	result, err := verifyRegistration(registrationInput{
		relyingParty:      rp,
		challenge:         expectedChallenge,
		credentialID:      credentialID,
		clientDataJSON:    clientData,
		attestationObject: attestation,
		userVerification:  userVerificationRequired,
	})
	if err != nil {
		return Registration{}, err
	}
	var userHandle []byte
	if inner.UserHandle != nil {
		userHandle, err = decodeUserHandle(*inner.UserHandle)
		if err != nil {
			return Registration{}, err
		}
	}
	return Registration{
		Credential:   result.credential,
		UserHandle:   userHandle,
		AAGUID:       result.aaguid,
		UserVerified: result.userVerified,
	}, nil
}

// VerifyAuthentication verifies a client assertion against the stored
// credential ID and COSE public key. The expected user handle is the account
// identifier carried in the registration ceremony.
func VerifyAuthentication(
	responseJSON []byte,
	challenge []byte,
	rp RelyingParty,
	storedCredentialID []byte,
	storedPublicKey []byte,
	userHandle []byte,
) error {
	_, err := authenticate(
		responseJSON,
		challenge,
		rp,
		Credential{ID: storedCredentialID, PublicKeyCOSE: storedPublicKey},
		userHandle,
		false,
	)
	return err
}

// Authenticate verifies a client assertion against complete stored credential
// state and returns the updated state, including the signature counter and
// backup flags. Use it when the relying party persists those flags.
func Authenticate(
	responseJSON []byte,
	challenge []byte,
	rp RelyingParty,
	credential Credential,
	userHandle []byte,
) (Credential, error) {
	result, err := authenticate(responseJSON, challenge, rp, credential, userHandle, true)
	if err != nil {
		return Credential{}, err
	}
	return result.credential, nil
}

func authenticate(
	responseJSON []byte,
	challenge []byte,
	rp RelyingParty,
	credential Credential,
	userHandle []byte,
	checkBackupEligibility bool,
) (authenticationResult, error) {
	parsed, inner, err := parseCredentialResponse[authenticationResponse](responseJSON)
	if err != nil {
		return authenticationResult{}, err
	}
	if parsed.RawID == nil || parsed.ID == nil || parsed.Type == nil ||
		*parsed.Type != "public-key" ||
		inner.ClientDataJSON == nil || inner.AuthenticatorData == nil ||
		inner.Signature == nil {
		return authenticationResult{}, ErrInvalidResponse
	}
	credentialID, err := decodeCredentialID(*parsed.RawID)
	if err != nil {
		return authenticationResult{}, err
	}
	expectedChallenge, err := newChallenge(challenge)
	if err != nil {
		return authenticationResult{}, err
	}
	clientData, err := decodeClientData(*inner.ClientDataJSON)
	if err != nil {
		return authenticationResult{}, err
	}
	authenticatorData, err := decodeAuthenticatorData(*inner.AuthenticatorData)
	if err != nil {
		return authenticationResult{}, err
	}
	signature, err := decodeSignature(*inner.Signature)
	if err != nil {
		return authenticationResult{}, err
	}
	if err := validateUserHandle(userHandle); err != nil {
		return authenticationResult{}, err
	}
	var responseUserHandle []byte
	if inner.UserHandle != nil {
		responseUserHandle, err = decodeUserHandle(*inner.UserHandle)
		if err != nil {
			return authenticationResult{}, err
		}
	}
	stored, err := decodeCredential(credential)
	if err != nil {
		return authenticationResult{}, err
	}
	return verifyAuthentication(authenticationInput{
		relyingParty:           rp,
		challenge:              expectedChallenge,
		credentialID:           credentialID,
		clientDataJSON:         clientData,
		authenticatorData:      authenticatorData,
		signature:              signature,
		credential:             stored,
		expectedUserHandle:     userHandle,
		responseUserHandle:     responseUserHandle,
		responsePresent:        inner.UserHandle != nil,
		userHandleMode:         userHandleIdentified,
		userVerification:       userVerificationRequired,
		checkBackupEligibility: checkBackupEligibility,
	})
}

type credentialResponse struct {
	RawID    *string         `json:"rawId"`
	ID       *string         `json:"id"`
	Type     *string         `json:"type"`
	Response json.RawMessage `json:"response"`
}

type registrationResponse struct {
	ClientDataJSON    *string `json:"clientDataJSON"`
	AttestationObject *string `json:"attestationObject"`
	UserHandle        *string `json:"userHandle"`
}

type authenticationResponse struct {
	ClientDataJSON    *string `json:"clientDataJSON"`
	AuthenticatorData *string `json:"authenticatorData"`
	Signature         *string `json:"signature"`
	UserHandle        *string `json:"userHandle"`
}

func parseCredentialResponse[T any](raw []byte) (credentialResponse, T, error) {
	var outer credentialResponse
	var inner T
	if err := json.Unmarshal(raw, &outer); err != nil {
		return outer, inner, ErrInvalidResponse
	}
	if err := json.Unmarshal(outer.Response, &inner); err != nil {
		return outer, inner, ErrInvalidResponse
	}
	return outer, inner, nil
}

func validateUserHandle(value []byte) error {
	if len(value) == 0 || len(value) > maxUserHandleBytes {
		return ErrInvalidUserHandle
	}
	return nil
}

func decodeCredentialID(value string) ([]byte, error) {
	return decodeField(value, ErrCredentialIDEncoding)
}

func decodeClientData(value string) ([]byte, error) {
	return decodeField(value, ErrClientDataEncoding)
}

func decodeAttestation(value string) ([]byte, error) {
	return decodeField(value, ErrAttestationEncoding)
}

func decodeAuthenticatorData(value string) ([]byte, error) {
	return decodeField(value, ErrAuthenticatorDataEncoding)
}

func decodeSignature(value string) ([]byte, error) {
	return decodeField(value, ErrSignatureEncoding)
}

func decodeUserHandle(value string) ([]byte, error) {
	decoded, err := decodeField(value, ErrUserHandleEncoding)
	if err != nil {
		return nil, err
	}
	if err := validateUserHandle(decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}

func decodeField(value string, failure error) ([]byte, error) {
	decoded, err := decodeBase64URL(value)
	if err != nil {
		return nil, failure
	}
	return decoded, nil
}
