package auth

import (
	"bytes"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"strings"
)

// ed25519SPKIPrefix is the DER header of an Ed25519 SubjectPublicKeyInfo.
var ed25519SPKIPrefix = []byte{0x30, 0x2a, 0x30, 0x05, 0x06, 0x03, 0x2b, 0x65, 0x70, 0x03, 0x21, 0x00}

var errLegacyKey = errors.New("legacy key is not a valid 32-byte key")

// normalizeKey returns a raw 32-byte key in base64url form. The relay-era
// database stored identity keys as PEM Ed25519 SubjectPublicKeyInfo; the
// Rust-era database and the current schema store the raw key.
func normalizeKey(value string) (string, error) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "-----BEGIN") {
		block, _ := pem.Decode([]byte(value))
		if block == nil || len(block.Bytes) != len(ed25519SPKIPrefix)+32 ||
			!bytes.Equal(block.Bytes[:len(ed25519SPKIPrefix)], ed25519SPKIPrefix) {
			return "", errLegacyKey
		}
		return base64.RawURLEncoding.EncodeToString(block.Bytes[len(ed25519SPKIPrefix):]), nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(raw) != 32 {
		return "", errLegacyKey
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// decodeLegacyBytes accepts the base64 variants the relay-era database used
// for passkey fields: padded or unpadded, standard or URL alphabet.
func decodeLegacyBytes(value string) ([]byte, error) {
	for _, encoding := range []*base64.Encoding{
		base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding,
	} {
		if raw, err := encoding.DecodeString(value); err == nil {
			return raw, nil
		}
	}
	return nil, errors.New("value is not base64")
}
