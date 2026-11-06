package identityhttp

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"

	witTypes "go.bytecodealliance.org/pkg/wit/types"
	"wit_component/ohrats_rc_webauthn_types"
)

const (
	ceremonyTTLMs    uint64 = 5 * 60 * 1000
	maxCredentials          = 1000
	passkeyAlgorithm        = "es256"
)

// RegistrationResult carries the verified first credential of a new account.
type RegistrationResult struct {
	UserID      string
	DisplayName string
	Credential  ohrats_rc_webauthn_types.StoredCredential
}

type registrationMeta struct {
	DisplayName string `json:"display_name"`
}

type ceremonyState struct {
	Challenge string `json:"challenge"`
	RPID      string `json:"rp_id"`
	Origin    string `json:"origin"`
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
}

type authenticationResponse struct {
	ClientDataJSON    *string `json:"clientDataJSON"`
	AuthenticatorData *string `json:"authenticatorData"`
	Signature         *string `json:"signature"`
	UserHandle        *string `json:"userHandle"`
}

func parseCredentialResponse[T any](raw json.RawMessage) (credentialResponse, T, error) {
	var outer credentialResponse
	var inner T
	if err := json.Unmarshal(raw, &outer); err != nil {
		return outer, inner, errors.New("invalid passkey response")
	}
	if err := json.Unmarshal(outer.Response, &inner); err != nil {
		return outer, inner, errors.New("invalid passkey response")
	}
	return outer, inner, nil
}

func wireRelyingParty(value ceremonyState) ohrats_rc_webauthn_types.RelyingParty {
	return ohrats_rc_webauthn_types.RelyingParty{Id: value.RPID, Origin: value.Origin}
}

func decodeBase64URL(value string, label string) ([]byte, error) {
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(value)
	if err != nil {
		return nil, errors.New("invalid " + label)
	}
	return decoded, nil
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

func randomID() (string, error) {
	return randomToken(18)
}

func randomUserID() (string, error) {
	return randomToken(16)
}

func randomToken(size int) (string, error) {
	value, err := randomBytes(size)
	if err != nil {
		return "", err
	}
	return encodeBase64URL(value), nil
}

func optionValue[T any](value witTypes.Option[T]) (T, bool) {
	if value.IsSome() {
		return value.Some(), true
	}
	var zero T
	return zero, false
}
