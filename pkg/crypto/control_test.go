package crypto

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
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
	SharedStatic         string `json:"sharedStatic"`
	SharedEphemeral      string `json:"sharedEphemeral"`
	KeyHex               string `json:"keyHex"`
	NonceC2N             string `json:"nonceC2N"`
	NonceN2C             string `json:"nonceN2C"`
	AadC2N               string `json:"aadC2N"`
	Plaintext            string `json:"plaintext"`
	Ciphertext           string `json:"ciphertext"`
	SessionPayload       string `json:"sessionPayload"`
	ReadyPayload         string `json:"readyPayload"`
	NodeIdentitySeed     string `json:"nodeIdentitySeed"`
	NodeIdentityPublic   string `json:"nodeIdentityPublic"`
	ReadySignature       string `json:"readySignature"`
}

func TestMatchesControlCryptoFixture(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "fixtures", "control-crypto.json")
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture failed: %v", err)
	}

	var v controlCryptoVector
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("unmarshal vector failed: %v", err)
	}

	pub, err := X25519Public(v.ClientPrivate)
	if err != nil || pub != v.ClientPublic {
		t.Fatalf("client public mismatch: got %s, want %s", pub, v.ClientPublic)
	}

	pub, err = X25519Public(v.NodeStaticPrivate)
	if err != nil || pub != v.NodeStaticPublic {
		t.Fatalf("node static public mismatch: got %s, want %s", pub, v.NodeStaticPublic)
	}

	pub, err = X25519Public(v.NodeEphemeralPrivate)
	if err != nil || pub != v.NodeEphemeralPublic {
		t.Fatalf("node ephemeral public mismatch: got %s, want %s", pub, v.NodeEphemeralPublic)
	}

	ss, err := X25519Shared(v.ClientPrivate, v.NodeStaticPublic)
	if err != nil || b64URL.EncodeToString(ss[:]) != v.SharedStatic {
		t.Fatalf("shared static mismatch")
	}

	se, err := X25519Shared(v.ClientPrivate, v.NodeEphemeralPublic)
	if err != nil || b64URL.EncodeToString(se[:]) != v.SharedEphemeral {
		t.Fatalf("shared ephemeral mismatch")
	}

	key, err := DeriveClientKey(v.ClientPrivate, v.NodeStaticPublic, v.NodeEphemeralPublic, v.Challenge, v.DeviceID, v.ClientID)
	if err != nil {
		t.Fatalf("DeriveClientKey failed: %v", err)
	}
	if hex.EncodeToString(key[:]) != v.KeyHex {
		t.Fatalf("derived key mismatch: got %s, want %s", hex.EncodeToString(key[:]), v.KeyHex)
	}

	nonceC2N := FrameNonce(1, v.Sequence)
	if hex.EncodeToString(nonceC2N[:]) != v.NonceC2N {
		t.Fatalf("nonce c2n mismatch")
	}

	nonceN2C := FrameNonce(2, v.Sequence)
	if hex.EncodeToString(nonceN2C[:]) != v.NonceN2C {
		t.Fatalf("nonce n2c mismatch")
	}

	aad := FrameAAD(v.SessionID, v.Sequence, "c2n")
	if string(aad) != v.AadC2N {
		t.Fatalf("aad c2n mismatch")
	}

	ciphertext, err := EncryptFrame(&key, 1, v.Sequence, v.SessionID, "c2n", []byte(v.Plaintext))
	if err != nil || ciphertext != v.Ciphertext {
		t.Fatalf("ciphertext mismatch: got %s, want %s", ciphertext, v.Ciphertext)
	}

	decrypted, err := DecryptFrame(&key, 1, v.Sequence, v.SessionID, "c2n", v.Ciphertext)
	if err != nil || string(decrypted) != v.Plaintext {
		t.Fatalf("decrypted plaintext mismatch: got %s, want %s", string(decrypted), v.Plaintext)
	}

	sp := SessionPayload(v.Challenge, v.DeviceID, v.ClientID, v.ClientPublic)
	if sp != v.SessionPayload {
		t.Fatalf("session payload mismatch")
	}

	rp := ReadyPayload(v.Challenge, v.DeviceID, v.ClientID, v.ClientPublic, v.NodeStaticPublic, v.NodeEphemeralPublic, v.SessionID, "")
	if rp != v.ReadyPayload {
		t.Fatalf("ready payload mismatch")
	}

	sig, err := SignEd25519Seed(v.NodeIdentitySeed, []byte(v.ReadyPayload))
	if err != nil || sig != v.ReadySignature {
		t.Fatalf("signature mismatch: got %s, want %s", sig, v.ReadySignature)
	}

	if err := VerifyEd25519(v.NodeIdentityPublic, []byte(v.ReadyPayload), v.ReadySignature); err != nil {
		t.Fatalf("VerifyEd25519 failed: %v", err)
	}
}
