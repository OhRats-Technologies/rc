package webauthn

import (
	"crypto/elliptic"
	"encoding/base64"
	"encoding/json"
	"math/big"
)

// AuthorityPublicKey converts a CBOR COSE key into the JSON authority key
// material used by Node RC Lock verification.
func AuthorityPublicKey(input []byte) (string, error) {
	key, err := fromCOSEKey(input)
	if err != nil {
		return "", err
	}
	if !elliptic.P256().IsOnCurve(new(big.Int).SetBytes(key.x), new(big.Int).SetBytes(key.y)) {
		return "", ErrInvalidPublicKey
	}
	encoded, err := json.Marshal(map[string]any{
		"type_": "ES256",
		"key": map[string]any{"EC_EC2": map[string]any{
			"curve": "SECP256R1",
			"x":     base64.RawURLEncoding.EncodeToString(key.x),
			"y":     base64.RawURLEncoding.EncodeToString(key.y),
		}},
	})
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(encoded), nil
}
