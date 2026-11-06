package passkey

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"math/big"
)

const maxCOSEKeyBytes = 16 * 1024

// PublicKey is a validated ES256 credential key on P-256.
type publicKey struct {
	x []byte
	y []byte
}

// fromCOSEKey parses a complete CBOR-encoded COSE key. Trailing data is
// rejected, matching passkey-rp's PublicKey::from_cose_key.
func fromCOSEKey(input []byte) (publicKey, error) {
	if len(input) > maxCOSEKeyBytes {
		return publicKey{}, ErrPayloadTooLarge
	}
	key, consumed, err := parseCOSEKeyPrefix(input)
	if err != nil {
		return publicKey{}, err
	}
	if consumed != len(input) {
		return publicKey{}, ErrInvalidPublicKey
	}
	return key, nil
}

// parseCOSEKeyPrefix parses one COSE key and reports how many bytes it used.
func parseCOSEKeyPrefix(input []byte) (publicKey, int, error) {
	value, consumed, err := decodeCBOR(input)
	if err != nil {
		return publicKey{}, 0, ErrInvalidCBOR
	}
	if consumed > maxCOSEKeyBytes {
		return publicKey{}, 0, ErrPayloadTooLarge
	}
	mapping, ok := value.(cborMap)
	if !ok {
		return publicKey{}, 0, ErrInvalidPublicKey
	}
	kty, err := expectInteger(mapping, 1)
	if err != nil {
		return publicKey{}, 0, err
	}
	if kty != 2 {
		return publicKey{}, 0, ErrUnsupportedCoseKey
	}
	algorithm, err := expectInteger(mapping, 3)
	if err != nil {
		return publicKey{}, 0, err
	}
	if algorithm != -7 {
		return publicKey{}, 0, ErrUnsupportedCoseAlgorithm
	}
	curve, err := expectInteger(mapping, -1)
	if err != nil {
		return publicKey{}, 0, err
	}
	if curve != 1 {
		return publicKey{}, 0, ErrUnsupportedCoseKey
	}
	x, err := expectBytes(mapping, -2)
	if err != nil {
		return publicKey{}, 0, err
	}
	y, err := expectBytes(mapping, -3)
	if err != nil {
		return publicKey{}, 0, err
	}
	if len(x) != 32 || len(y) != 32 {
		return publicKey{}, 0, ErrInvalidPublicKey
	}
	if !elliptic.P256().IsOnCurve(new(big.Int).SetBytes(x), new(big.Int).SetBytes(y)) {
		return publicKey{}, 0, ErrInvalidPublicKey
	}
	return publicKey{x: x, y: y}, consumed, nil
}

func expectInteger(mapping cborMap, key int64) (int64, error) {
	value, found := mapInt(mapping, key)
	if !found {
		return 0, ErrInvalidPublicKey
	}
	number, ok := value.(int64)
	if !ok {
		return 0, ErrInvalidPublicKey
	}
	return number, nil
}

func expectBytes(mapping cborMap, key int64) ([]byte, error) {
	value, found := mapInt(mapping, key)
	if !found {
		return nil, ErrInvalidPublicKey
	}
	raw, ok := value.([]byte)
	if !ok {
		return nil, ErrInvalidPublicKey
	}
	return raw, nil
}

// verify checks a DER-encoded ECDSA P-256 SHA-256 signature over data.
func (key publicKey) verify(data []byte, signature []byte) error {
	value := &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     new(big.Int).SetBytes(key.x),
		Y:     new(big.Int).SetBytes(key.y),
	}
	digest := sha256.Sum256(data)
	if !ecdsa.VerifyASN1(value, digest[:], signature) {
		return ErrInvalidSignature
	}
	return nil
}

// coseKey re-encodes the validated key in the canonical CBOR map order used by
// serde_cbor: non-negative integer keys before negative integer keys.
func (key publicKey) coseKey() []byte {
	result := []byte{0xa5, 0x01, 0x02, 0x03, 0x26, 0x20, 0x01, 0x21, 0x58, 0x20}
	result = append(result, key.x...)
	result = append(result, 0x22, 0x58, 0x20)
	return append(result, key.y...)
}
