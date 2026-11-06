package passkey

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
)

const maxClientDataJSONBytes = 16 * 1024

// decodeBase64URL accepts padded or unpadded base64url and rejects the standard
// base64 alphabet, matching passkey-rp's decode_base64url.
func decodeBase64URL(value string) ([]byte, error) {
	if decoded, err := base64.RawURLEncoding.Strict().DecodeString(value); err == nil {
		return decoded, nil
	}
	if decoded, err := base64.URLEncoding.Strict().DecodeString(value); err == nil {
		return decoded, nil
	}
	return nil, ErrInvalidBase64URL
}

type collectedClientData struct {
	Type        *string `json:"type"`
	Challenge   *string `json:"challenge"`
	Origin      *string `json:"origin"`
	CrossOrigin *bool   `json:"crossOrigin"`
}

// verifyClientData validates the ceremony type, challenge, origin, and
// cross-origin claims in a collected client data document.
func verifyClientData(raw []byte, expectedType string, expectedChallenge []byte, expectedOrigin string) error {
	if len(raw) > maxClientDataJSONBytes {
		return ErrPayloadTooLarge
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return ErrInvalidJSON
	}
	var data collectedClientData
	if err := json.Unmarshal(raw, &data); err != nil {
		return ErrInvalidJSON
	}
	if data.Type == nil || data.Challenge == nil || data.Origin == nil {
		return ErrInvalidJSON
	}
	if *data.Type != expectedType {
		return ErrInvalidClientDataType
	}
	challenge, err := decodeBase64URL(*data.Challenge)
	if err != nil {
		return err
	}
	if !bytes.Equal(challenge, expectedChallenge) {
		return ErrChallengeMismatch
	}
	if *data.Origin != expectedOrigin {
		return ErrOriginMismatch
	}
	_, topOriginPresent := fields["topOrigin"]
	if (data.CrossOrigin != nil && *data.CrossOrigin) || topOriginPresent {
		return ErrCrossOrigin
	}
	return nil
}
