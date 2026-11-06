package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"
)

var b64URL = base64.RawURLEncoding

func SessionPayload(challenge, deviceID, clientID, publicKey string) string {
	return fmt.Sprintf("rc-session-v1\n%s\n%s\n%s\n%s", challenge, deviceID, clientID, publicKey)
}

func ReadyPayload(challenge, deviceID, clientID, publicKey, transportKey, ephemeralKey, sessionID, attemptPlan string) string {
	if attemptPlan == "" {
		return fmt.Sprintf("rc-ready-v2\n%s\n%s\n%s\n%s\n%s\n%s\n%s",
			challenge, deviceID, clientID, publicKey, transportKey, ephemeralKey, sessionID)
	}
	return fmt.Sprintf("rc-ready-v3\n%s\n%s\n%s\n%s\n%s\n%s\n%s\n%s",
		challenge, deviceID, clientID, publicKey, transportKey, ephemeralKey, sessionID, attemptPlan)
}

func FrameNonce(direction byte, sequence uint64) [12]byte {
	var nonce [12]byte
	nonce[0] = direction
	binary.BigEndian.PutUint64(nonce[4:], sequence)
	return nonce
}

func FrameAAD(sessionID string, sequence uint64, direction string) []byte {
	return []byte(fmt.Sprintf("rc-frame-v1\n%s\n%d\n%s", sessionID, sequence, direction))
}

func DecodeX25519(value string) ([32]byte, error) {
	var out [32]byte
	b, err := b64URL.DecodeString(value)
	if err != nil {
		return out, ErrBase64
	}
	if len(b) != 32 {
		return out, ErrKeyLength
	}
	copy(out[:], b)
	return out, nil
}

func X25519Public(private string) (string, error) {
	privBytes, err := DecodeX25519(private)
	if err != nil {
		return "", err
	}
	k, err := ecdh.X25519().NewPrivateKey(privBytes[:])
	if err != nil {
		return "", err
	}
	return b64URL.EncodeToString(k.PublicKey().Bytes()), nil
}

func X25519Shared(private, public string) ([32]byte, error) {
	var out [32]byte
	privBytes, err := DecodeX25519(private)
	if err != nil {
		return out, err
	}
	pubBytes, err := DecodeX25519(public)
	if err != nil {
		return out, err
	}
	privKey, err := ecdh.X25519().NewPrivateKey(privBytes[:])
	if err != nil {
		return out, err
	}
	pubKey, err := ecdh.X25519().NewPublicKey(pubBytes[:])
	if err != nil {
		return out, err
	}
	shared, err := privKey.ECDH(pubKey)
	if err != nil {
		return out, err
	}
	copy(out[:], shared)
	return out, nil
}

func DeriveSessionKey(sharedStatic, sharedEphemeral *[32]byte, challenge, deviceID, clientID string) ([32]byte, error) {
	var key [32]byte
	var material [64]byte
	copy(material[:32], sharedStatic[:])
	copy(material[32:], sharedEphemeral[:])

	salt := sha256.Sum256([]byte(challenge))
	info := []byte(fmt.Sprintf("rc-e2e-v2\n%s\n%s", deviceID, clientID))

	// HKDF-Extract (RFC 5869)
	mac := hmac.New(sha256.New, salt[:])
	mac.Write(material[:])
	prk := mac.Sum(nil)

	// HKDF-Expand (32 bytes = 1 block with counter 0x01)
	mac = hmac.New(sha256.New, prk)
	mac.Write(info)
	mac.Write([]byte{0x01})
	copy(key[:], mac.Sum(nil))

	return key, nil
}

func DeriveClientKey(clientPrivate, nodeStaticPublic, nodeEphemeralPublic, challenge, deviceID, clientID string) ([32]byte, error) {
	sStatic, err := X25519Shared(clientPrivate, nodeStaticPublic)
	if err != nil {
		return [32]byte{}, err
	}
	sEphemeral, err := X25519Shared(clientPrivate, nodeEphemeralPublic)
	if err != nil {
		return [32]byte{}, err
	}
	return DeriveSessionKey(&sStatic, &sEphemeral, challenge, deviceID, clientID)
}

func DeriveNodeKey(staticPrivate, ephemeralPrivate, clientPublic, challenge, deviceID, clientID string) ([32]byte, error) {
	sStatic, err := X25519Shared(staticPrivate, clientPublic)
	if err != nil {
		return [32]byte{}, err
	}
	sEphemeral, err := X25519Shared(ephemeralPrivate, clientPublic)
	if err != nil {
		return [32]byte{}, err
	}
	return DeriveSessionKey(&sStatic, &sEphemeral, challenge, deviceID, clientID)
}

func EncryptFrame(key *[32]byte, direction byte, sequence uint64, sessionID, label string, plaintext []byte) (string, error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", ErrKeyLength
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", ErrKeyLength
	}
	nonce := FrameNonce(direction, sequence)
	aad := FrameAAD(sessionID, sequence, label)
	ciphertext := gcm.Seal(nil, nonce[:], plaintext, aad)
	return b64URL.EncodeToString(ciphertext), nil
}

func DecryptFrame(key *[32]byte, direction byte, sequence uint64, sessionID, label string, ciphertext string) ([]byte, error) {
	data, err := b64URL.DecodeString(ciphertext)
	if err != nil {
		return nil, ErrBase64
	}
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, ErrKeyLength
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, ErrKeyLength
	}
	nonce := FrameNonce(direction, sequence)
	aad := FrameAAD(sessionID, sequence, label)
	plaintext, err := gcm.Open(nil, nonce[:], data, aad)
	if err != nil {
		return nil, ErrAuthentication
	}
	return plaintext, nil
}

func SignEd25519Seed(seed string, payload []byte) (string, error) {
	seedBytes, err := b64URL.DecodeString(seed)
	if err != nil {
		return "", ErrBase64
	}
	if len(seedBytes) != 32 {
		return "", ErrKeyLength
	}
	key := ed25519.NewKeyFromSeed(seedBytes)
	sig := ed25519.Sign(key, payload)
	return b64URL.EncodeToString(sig), nil
}

func VerifyEd25519(public string, payload []byte, signature string) error {
	pubBytes, err := b64URL.DecodeString(public)
	if err != nil {
		return ErrBase64
	}
	if len(pubBytes) != 32 {
		return ErrKeyLength
	}
	sigBytes, err := b64URL.DecodeString(signature)
	if err != nil {
		return ErrBase64
	}
	if len(sigBytes) != 64 {
		return ErrKeyLength
	}
	if !ed25519.Verify(pubBytes, payload, sigBytes) {
		return ErrSignature
	}
	return nil
}
