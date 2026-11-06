package mesh

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"

	"github.com/OhRats-Technologies/rc/pkg/crypto"
)

var b64URL = base64.RawURLEncoding
var ErrPublicKey = errors.New("invalid mesh identity public key")

func PeerIDFromPublicKey(publicKey string) (PeerID, error) {
	bytes, err := b64URL.DecodeString(publicKey)
	if err != nil || len(bytes) != 32 {
		return "", ErrPublicKey
	}
	digest := sha256.Sum256(bytes)
	return NewPeerID(b64URL.EncodeToString(digest[:20]))
}

func SignPayload(seed string, domain string, payload []byte) (string, error) {
	msg := make([]byte, 0, len(domain)+1+len(payload))
	msg = append(msg, []byte(domain)...)
	msg = append(msg, '\n')
	msg = append(msg, payload...)
	return crypto.SignEd25519Seed(seed, msg)
}

func VerifyPayload(publicKey string, domain string, payload []byte, signature string) error {
	msg := make([]byte, 0, len(domain)+1+len(payload))
	msg = append(msg, []byte(domain)...)
	msg = append(msg, '\n')
	msg = append(msg, payload...)
	return crypto.VerifyEd25519(publicKey, msg, signature)
}
