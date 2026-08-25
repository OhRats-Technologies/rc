package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"
)

func randomURLBytes(size int) string {
	bytes := make([]byte, size)
	_, _ = rand.Read(bytes)
	return base64.RawURLEncoding.EncodeToString(bytes)
}

func sessionPayload(challenge, deviceID, clientID, publicKey string) string {
	return "rc-session-v1\n" + challenge + "\n" + deviceID + "\n" + clientID + "\n" + publicKey
}

func readyPayload(challenge, deviceID, clientID, publicKey, transportKey, ephemeralKey, sessionID string) string {
	return "rc-ready-v2\n" + challenge + "\n" + deviceID + "\n" + clientID + "\n" + publicKey + "\n" + transportKey + "\n" + ephemeralKey + "\n" + sessionID
}

func x25519Shared(privateEncoded, publicEncoded string) ([]byte, error) {
	privateBytes, err := base64.RawURLEncoding.DecodeString(privateEncoded)
	if err != nil {
		return nil, err
	}
	publicBytes, err := base64.RawURLEncoding.DecodeString(publicEncoded)
	if err != nil {
		return nil, err
	}
	privateKey, err := ecdh.X25519().NewPrivateKey(privateBytes)
	if err != nil {
		return nil, err
	}
	publicKey, err := ecdh.X25519().NewPublicKey(publicBytes)
	if err != nil {
		return nil, err
	}
	return privateKey.ECDH(publicKey)
}

func deriveSessionAEAD(sharedStatic, sharedEphemeral []byte, challenge, deviceID, clientID string) (cipher.AEAD, error) {
	material := make([]byte, 0, len(sharedStatic)+len(sharedEphemeral))
	material = append(material, sharedStatic...)
	material = append(material, sharedEphemeral...)
	salt := sha256.Sum256([]byte(challenge))
	key, err := hkdf.Key(sha256.New, material, salt[:], "rc-e2e-v2\n"+deviceID+"\n"+clientID, 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func deriveNodeAEAD(staticPrivate, ephemeralPrivate, clientPublic, challenge, deviceID, clientID string) (cipher.AEAD, error) {
	sharedStatic, err := x25519Shared(staticPrivate, clientPublic)
	if err != nil {
		return nil, err
	}
	sharedEphemeral, err := x25519Shared(ephemeralPrivate, clientPublic)
	if err != nil {
		return nil, err
	}
	return deriveSessionAEAD(sharedStatic, sharedEphemeral, challenge, deviceID, clientID)
}

func deriveClientAEAD(clientPrivate, nodeStaticPublic, nodeEphemeralPublic, challenge, deviceID, clientID string) (cipher.AEAD, error) {
	sharedStatic, err := x25519Shared(clientPrivate, nodeStaticPublic)
	if err != nil {
		return nil, err
	}
	sharedEphemeral, err := x25519Shared(clientPrivate, nodeEphemeralPublic)
	if err != nil {
		return nil, err
	}
	return deriveSessionAEAD(sharedStatic, sharedEphemeral, challenge, deviceID, clientID)
}

func frameNonce(direction byte, sequence uint64) []byte {
	nonce := make([]byte, 12)
	nonce[0] = direction
	binary.BigEndian.PutUint64(nonce[4:], sequence)
	return nonce
}

func frameAAD(sessionID string, sequence uint64, direction string) []byte {
	return []byte(fmt.Sprintf("rc-frame-v1\n%s\n%d\n%s", sessionID, sequence, direction))
}
