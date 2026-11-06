package webauthn

import (
	"crypto/rand"
	"encoding/base64"
	"strings"
)

const (
	// COSEAlgorithmES256 is the COSE algorithm identifier persisted with every
	// credential this package accepts.
	COSEAlgorithmES256 int64 = -7

	// CeremonyTimeoutMs is the browser ceremony timeout advertised in options.
	CeremonyTimeoutMs uint64 = 5 * 60 * 1000

	maxCredentials   = 1000
	maxUserNameBytes = 256
	relyingPartyName = "RC"
)

// CreationOptions builds a WebAuthn registration ceremony for an account,
// excluding credential IDs the account already registered.
func CreationOptions(
	userID string,
	name string,
	rp RelyingParty,
	existingCredentialIDs [][]byte,
) ([]byte, map[string]any, error) {
	relyingParty, err := normalizeRelyingParty(rp)
	if err != nil {
		return nil, nil, err
	}
	displayName, err := validateUserName(name)
	if err != nil {
		return nil, nil, err
	}
	if err := validateUserHandle([]byte(userID)); err != nil {
		return nil, nil, err
	}
	if len(existingCredentialIDs) > maxCredentials {
		return nil, nil, ErrTooManyCredentials
	}
	challenge, err := randomBytes(32)
	if err != nil {
		return nil, nil, err
	}
	exclude := make([]any, 0, len(existingCredentialIDs))
	for _, credentialID := range existingCredentialIDs {
		if len(credentialID) == 0 {
			return nil, nil, ErrEmptyCredentialID
		}
		exclude = append(exclude, map[string]any{
			"id":   encodeBase64URL(credentialID),
			"type": "public-key",
		})
	}
	options := map[string]any{
		"challenge": encodeBase64URL(challenge),
		"rp": map[string]any{
			"id":   relyingParty.ID,
			"name": relyingPartyName,
		},
		"user": map[string]any{
			"id":          encodeBase64URL([]byte(userID)),
			"name":        displayName,
			"displayName": displayName,
		},
		"pubKeyCredParams": []any{
			map[string]any{"type": "public-key", "alg": COSEAlgorithmES256},
		},
		"timeout":     CeremonyTimeoutMs,
		"attestation": "none",
		"authenticatorSelection": map[string]any{
			"residentKey":        "required",
			"requireResidentKey": true,
			"userVerification":   "required",
		},
		"excludeCredentials": exclude,
	}
	return challenge, options, nil
}

// RequestOptions builds a WebAuthn authentication ceremony allowing the given
// credentials.
func RequestOptions(credentials [][]byte, rp RelyingParty) ([]byte, map[string]any, error) {
	relyingParty, err := normalizeRelyingParty(rp)
	if err != nil {
		return nil, nil, err
	}
	if len(credentials) == 0 {
		return nil, nil, ErrNoCredentials
	}
	if len(credentials) > maxCredentials {
		return nil, nil, ErrTooManyCredentials
	}
	challenge, err := randomBytes(32)
	if err != nil {
		return nil, nil, err
	}
	allow := make([]any, 0, len(credentials))
	for _, credentialID := range credentials {
		if len(credentialID) == 0 {
			return nil, nil, ErrEmptyCredentialID
		}
		allow = append(allow, map[string]any{
			"id":   encodeBase64URL(credentialID),
			"type": "public-key",
		})
	}
	options := map[string]any{
		"challenge":        encodeBase64URL(challenge),
		"rpId":             relyingParty.ID,
		"timeout":          CeremonyTimeoutMs,
		"userVerification": "required",
		"allowCredentials": allow,
	}
	return challenge, options, nil
}

func validateUserName(value string) (string, error) {
	name := strings.TrimSpace(value)
	if len(name) == 0 || len(name) > maxUserNameBytes {
		return "", ErrInvalidName
	}
	return name, nil
}

func encodeBase64URL(value []byte) string {
	return base64.RawURLEncoding.EncodeToString(value)
}

func randomBytes(size int) ([]byte, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return nil, err
	}
	return value, nil
}
