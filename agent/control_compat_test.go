package main

import (
	"crypto/ecdh"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

type controlCryptoVector struct {
	Challenge            string `json:"challenge"`
	DeviceID             string `json:"deviceId"`
	ClientID             string `json:"clientId"`
	SessionID            string `json:"sessionId"`
	Sequence             uint64 `json:"sequence"`
	ClientPrivate        string `json:"clientPrivate"`
	ClientPublic         string `json:"clientPublic"`
	NodeStaticPrivate    string `json:"nodeStaticPrivate"`
	NodeStaticPublic     string `json:"nodeStaticPublic"`
	NodeEphemeralPrivate string `json:"nodeEphemeralPrivate"`
	NodeEphemeralPublic  string `json:"nodeEphemeralPublic"`
	KeyHex               string `json:"keyHex"`
	NonceC2N             string `json:"nonceC2N"`
	AADc2N               string `json:"aadC2N"`
	Plaintext            string `json:"plaintext"`
	Ciphertext           string `json:"ciphertext"`
	SessionPayload       string `json:"sessionPayload"`
	ReadyPayload         string `json:"readyPayload"`
	NodeIdentitySeed     string `json:"nodeIdentitySeed"`
	ReadySignature       string `json:"readySignature"`
}

func TestControlCryptoCompatibilityVector(t *testing.T) {
	data, err := os.ReadFile("../fixtures/control-crypto.json")
	if err != nil {
		t.Fatal(err)
	}
	var vector controlCryptoVector
	if err := json.Unmarshal(data, &vector); err != nil {
		t.Fatal(err)
	}
	client, err := deriveClientAEAD(vector.ClientPrivate, vector.NodeStaticPublic, vector.NodeEphemeralPublic,
		vector.Challenge, vector.DeviceID, vector.ClientID)
	if err != nil {
		t.Fatal(err)
	}
	node, err := deriveNodeAEAD(vector.NodeStaticPrivate, vector.NodeEphemeralPrivate, vector.ClientPublic,
		vector.Challenge, vector.DeviceID, vector.ClientID)
	if err != nil {
		t.Fatal(err)
	}
	ciphertext := client.Seal(nil, frameNonce(1, vector.Sequence), []byte(vector.Plaintext),
		frameAAD(vector.SessionID, vector.Sequence, "c2n"))
	if base64.RawURLEncoding.EncodeToString(ciphertext) != vector.Ciphertext {
		t.Fatal("Go ciphertext drifted from compatibility vector")
	}
	opened, err := node.Open(nil, frameNonce(1, vector.Sequence), ciphertext,
		frameAAD(vector.SessionID, vector.Sequence, "c2n"))
	if err != nil || string(opened) != vector.Plaintext {
		t.Fatalf("Go node/client key agreement drifted: %v", err)
	}
	if hex.EncodeToString(frameNonce(1, vector.Sequence)) != vector.NonceC2N ||
		string(frameAAD(vector.SessionID, vector.Sequence, "c2n")) != vector.AADc2N {
		t.Fatal("frame construction drifted")
	}
	if sessionPayload(vector.Challenge, vector.DeviceID, vector.ClientID, vector.ClientPublic) != vector.SessionPayload ||
		readyPayload(vector.Challenge, vector.DeviceID, vector.ClientID, vector.ClientPublic,
			vector.NodeStaticPublic, vector.NodeEphemeralPublic, vector.SessionID) != vector.ReadyPayload {
		t.Fatal("handshake canonical payload drifted")
	}
	seed, err := base64.RawURLEncoding.DecodeString(vector.NodeIdentitySeed)
	if err != nil {
		t.Fatal(err)
	}
	signature := ed25519.Sign(ed25519.NewKeyFromSeed(seed), []byte(vector.ReadyPayload))
	if base64.RawURLEncoding.EncodeToString(signature) != vector.ReadySignature {
		t.Fatal("ready signature drifted")
	}
	assertX25519Public(t, vector.ClientPrivate, vector.ClientPublic)
}

func assertX25519Public(t *testing.T, privateEncoded, expected string) {
	t.Helper()
	privateBytes, err := base64.RawURLEncoding.DecodeString(privateEncoded)
	if err != nil {
		t.Fatal(err)
	}
	privateKey, err := ecdh.X25519().NewPrivateKey(privateBytes)
	if err != nil {
		t.Fatal(err)
	}
	if base64.RawURLEncoding.EncodeToString(privateKey.PublicKey().Bytes()) != expected {
		t.Fatal("X25519 public key drifted")
	}
}
