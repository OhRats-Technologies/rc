package crypto

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

type ApiRequest struct {
	KeyID      string
	Timestamp  string
	Nonce      string
	Method     string
	RequestURI string
	Body       []byte
}

func ApiPayload(keyID, timestamp, nonce, method, requestURI string, body []byte) string {
	digest := sha256.Sum256(body)
	return fmt.Sprintf("rc-api-v1\n%s\n%s\n%s\n%s\n%s\n%s",
		keyID, timestamp, nonce, method, requestURI, hex.EncodeToString(digest[:]))
}

func SignApiSeed(seed, keyID, timestamp, nonce, method, requestURI string, body []byte) (string, error) {
	seedBytes, err := b64URL.DecodeString(seed)
	if err != nil {
		return "", ErrBase64
	}
	if len(seedBytes) != 32 {
		return "", ErrKeyLength
	}
	payload := ApiPayload(keyID, timestamp, nonce, method, requestURI, body)
	key := ed25519.NewKeyFromSeed(seedBytes)
	sig := ed25519.Sign(key, []byte(payload))
	return b64URL.EncodeToString(sig), nil
}

func VerifyApi(publicKey, signature string, req ApiRequest) error {
	payload := ApiPayload(req.KeyID, req.Timestamp, req.Nonce, req.Method, req.RequestURI, req.Body)
	return VerifyEd25519(publicKey, []byte(payload), signature)
}
