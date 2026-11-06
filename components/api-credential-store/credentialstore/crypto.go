package credentialstore

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	"wit_component/ohrats_rc_api_credentials_types"
)

const maxPathLength = 4096

// verifyRequest checks the proof-of-possession signature over the canonical
// request payload described by docs and the API client.
func verifyRequest(request ohrats_rc_api_credentials_types.Request, storedKey string) error {
	if err := identifier(request.KeyId, "key id"); err != nil {
		return err
	}
	if err := nonce(request.Nonce); err != nil {
		return err
	}
	if err := text(request.TimestampSeconds, "timestamp", 32); err != nil {
		return err
	}
	if err := text(request.Method, "method", 16); err != nil {
		return err
	}
	if !strings.HasPrefix(request.PathAndRawQuery, "/") ||
		len(request.PathAndRawQuery) > maxPathLength ||
		hasControl(request.PathAndRawQuery) {
		return errors.New("invalid request path")
	}
	if err := publicKey(storedKey); err != nil {
		return err
	}
	if err := text(request.Signature, "signature", maxSignature); err != nil {
		return err
	}
	key, err := decodeBase64(storedKey)
	if err != nil {
		return errors.New("invalid public key")
	}
	signature, err := decodeBase64(request.Signature)
	if err != nil {
		return errors.New("invalid signature")
	}
	if len(key) != ed25519.PublicKeySize {
		return errors.New("invalid public key")
	}
	if len(signature) != ed25519.SignatureSize {
		return errors.New("invalid signature")
	}
	if !ed25519.Verify(ed25519.PublicKey(key), []byte(payload(request)), signature) {
		return errors.New("invalid signature")
	}
	return nil
}

// payload builds the canonical signed request string. The method is upper
// cased so clients cannot vary casing to change the signature.
func payload(request ohrats_rc_api_credentials_types.Request) string {
	digest := sha256.Sum256(request.Body)
	return "rc-api-v1\n" +
		request.KeyId + "\n" +
		request.TimestampSeconds + "\n" +
		request.Nonce + "\n" +
		asciiUpper(request.Method) + "\n" +
		request.PathAndRawQuery + "\n" +
		hex.EncodeToString(digest[:])
}

func hash(value string) []byte {
	digest := sha256.Sum256([]byte(value))
	return digest[:]
}

func hexLower(value []byte) string {
	return hex.EncodeToString(value)
}

func asciiUpper(value string) string {
	result := []byte(value)
	for index, character := range result {
		if character >= 'a' && character <= 'z' {
			result[index] = character - ('a' - 'A')
		}
	}
	return string(result)
}
