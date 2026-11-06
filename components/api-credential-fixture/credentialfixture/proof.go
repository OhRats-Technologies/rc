package credentialfixture

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strconv"
	"time"

	"wit_component/ohrats_rc_api_credentials_types"
)

// signed builds a proof-of-possession request exactly the way a real API
// client does: Ed25519 over the canonical payload bound to the body digest.
func signed(seed byte, id string, atMs uint64, nonce string, method string, path string, body []byte) ohrats_rc_api_credentials_types.Request {
	timestamp := strconv.FormatUint(atMs/1000, 10)
	digest := sha256.Sum256(body)
	payload := "rc-api-v1\n" + id + "\n" + timestamp + "\n" + nonce + "\n" +
		method + "\n" + path + "\n" + hex.EncodeToString(digest[:])
	signature := ed25519.Sign(signingKey(seed), []byte(payload))
	return ohrats_rc_api_credentials_types.Request{
		KeyId:            id,
		TimestampSeconds: timestamp,
		Nonce:            nonce,
		Method:           method,
		PathAndRawQuery:  path,
		Body:             body,
		Signature:        base64.RawURLEncoding.EncodeToString(signature),
	}
}

// key returns the base64url public key a client would register for the seed.
func key(seed byte) string {
	public := signingKey(seed).Public().(ed25519.PublicKey)
	return base64.RawURLEncoding.EncodeToString(public)
}

func signingKey(seed byte) ed25519.PrivateKey {
	return ed25519.NewKeyFromSeed(bytes.Repeat([]byte{seed}, ed25519.SeedSize))
}

func nowMs() uint64 {
	value := time.Now().UnixMilli()
	if value < 0 {
		return 0
	}
	return uint64(value)
}
