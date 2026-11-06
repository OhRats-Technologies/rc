package credentialfixture

import (
	"crypto/sha256"
	"encoding/asn1"
	"errors"
	"math/big"
)

// k=1 is deliberately fixture-only. Its point is the P-256 generator, so r is
// known and signing needs only scalar arithmetic instead of a fuel-heavy point
// multiplication inside the constrained guest command.
var fixtureR = [32]byte{
	0x6b, 0x17, 0xd1, 0xf2, 0xe1, 0x2c, 0x42, 0x47, 0xf8, 0xbc, 0xe6, 0xe5, 0x63, 0xa4, 0x40, 0xf2,
	0x77, 0x03, 0x7d, 0x81, 0x2d, 0xeb, 0x33, 0xa0, 0xf4, 0xa1, 0x39, 0x45, 0xd8, 0x98, 0xc2, 0x96,
}

// p256Order is the order of the P-256 group.
var p256Order, _ = new(big.Int).SetString("ffffffff00000000ffffffffffffffffbce6faada7179e84f3b9cac2fc632551", 16)

type ecdsaSignature struct {
	R *big.Int
	S *big.Int
}

// fixtureSignature signs with the fixture private key 3 and the fixed nonce
// r = 1. Scalars stay valid ECDSA, so the real ES256 verifier accepts them.
func fixtureSignature(message []byte) ([]byte, error) {
	digest := sha256.Sum256(message)
	z := new(big.Int).SetBytes(digest[:])
	z.Mod(z, p256Order)
	private := new(big.Int).SetBytes(fixturePrivateKey[:])
	r := new(big.Int).SetBytes(fixtureR[:])
	s := new(big.Int).Mul(r, private)
	s.Add(s, z)
	s.Mod(s, p256Order)
	if r.Sign() == 0 || s.Sign() == 0 {
		return nil, errors.New("invalid fixture signature")
	}
	return asn1.Marshal(ecdsaSignature{R: r, S: s})
}
