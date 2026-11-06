package crypto

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

func NodeHttpPayload(deviceID, timestamp, nonce, method, path string, body []byte) string {
	digest := sha256.Sum256(body)
	return fmt.Sprintf("rc-node-http-v1\n%s\n%s\n%s\n%s\n%s\n%s",
		deviceID, timestamp, nonce, method, path, hex.EncodeToString(digest[:]))
}

func SignNodeHttp(seed, deviceID, timestamp, nonce, method, path string, body []byte) (string, error) {
	seedBytes, err := b64URL.DecodeString(seed)
	if err != nil {
		return "", ErrBase64
	}
	if len(seedBytes) != 32 {
		return "", ErrKeyLength
	}
	payload := NodeHttpPayload(deviceID, timestamp, nonce, method, path, body)
	key := ed25519.NewKeyFromSeed(seedBytes)
	sig := ed25519.Sign(key, []byte(payload))
	return b64URL.EncodeToString(sig), nil
}

func VerifyNodeHttp(publicKey, signature, payload string) error {
	return VerifyEd25519(publicKey, []byte(payload), signature)
}
