package main

import (
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
)

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "__process-runner" {
		os.Exit(runProcessRunner())
	}
	os.Exit(m.Run())
}

func encodedX25519(t *testing.T) (string, string) {
	t.Helper()
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(key.Bytes()), base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes())
}

func TestControlSessionKeyAgreementHasEphemeralContribution(t *testing.T) {
	staticPrivate, staticPublic := encodedX25519(t)
	ephemeralPrivate, ephemeralPublic := encodedX25519(t)
	clientPrivate, clientPublic := encodedX25519(t)
	node, err := deriveNodeAEAD(staticPrivate, ephemeralPrivate, clientPublic, "challenge", "device", "client")
	if err != nil {
		t.Fatal(err)
	}
	client, err := deriveClientAEAD(clientPrivate, staticPublic, ephemeralPublic, "challenge", "device", "client")
	if err != nil {
		t.Fatal(err)
	}
	plaintext := []byte("secret terminal bytes")
	ciphertext := node.Seal(nil, frameNonce(2, 1), plaintext, frameAAD("session", 1, "n2c"))
	opened, err := client.Open(nil, frameNonce(2, 1), ciphertext, frameAAD("session", 1, "n2c"))
	if err != nil || string(opened) != string(plaintext) {
		t.Fatalf("interoperability failed: %v", err)
	}

	_, wrongEphemeral := encodedX25519(t)
	wrong, err := deriveClientAEAD(clientPrivate, staticPublic, wrongEphemeral, "challenge", "device", "client")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wrong.Open(nil, frameNonce(2, 1), ciphertext, frameAAD("session", 1, "n2c")); err == nil {
		t.Fatal("ciphertext decrypted without the Node ephemeral key")
	}
}

func TestEncryptedProcessOutputOnlyLeavesAsCiphertext(t *testing.T) {
	staticPrivate, staticPublic := encodedX25519(t)
	ephemeralPrivate, ephemeralPublic := encodedX25519(t)
	clientPrivate, clientPublic := encodedX25519(t)
	nodeAEAD, err := deriveNodeAEAD(staticPrivate, ephemeralPrivate, clientPublic, "challenge", "device", "client")
	if err != nil {
		t.Fatal(err)
	}
	clientAEAD, err := deriveClientAEAD(clientPrivate, staticPublic, ephemeralPublic, "challenge", "device", "client")
	if err != nil {
		t.Fatal(err)
	}

	outbound := make(chan wireMessage, 64)
	processes := newProcessManager()
	defer processes.shutdown()
	manager := &controlManager{processes: processes, send: func(message wireMessage) error { outbound <- message; return nil },
		sessions:   map[string]*controlSession{"session": {aead: nodeAEAD, clientID: "client", userID: "user", role: "owner", canExecute: true}},
		challenges: map[string]time.Time{}}
	processes.setSecureSender(manager.sendFrame)
	command := wireMessage{Type: "process.start", ID: "secret-process", Command: "printf 'phase34-secret'", Cols: 80, Rows: 24}
	plain, _ := json.Marshal(command)
	ciphertext := clientAEAD.Seal(nil, frameNonce(1, 1), plain, frameAAD("session", 1, "c2n"))
	if err := manager.receiveFrame(wireMessage{Type: "control.frame", SessionID: "session", Sequence: 1,
		Ciphertext: base64.RawURLEncoding.EncodeToString(ciphertext)}); err != nil {
		t.Fatal(err)
	}

	deadline := time.After(5 * time.Second)
	sawSecret, sawExit := false, false
	for !sawExit {
		select {
		case message := <-outbound:
			if message.Type == "process.output" || message.Output == "phase34-secret" {
				t.Fatalf("plaintext terminal output escaped encrypted control: %+v", message)
			}
			if message.Type != "control.frame" {
				continue
			}
			opened, err := clientAEAD.Open(nil, frameNonce(2, message.Sequence), mustDecodeURL(t, message.Ciphertext),
				frameAAD("session", message.Sequence, "n2c"))
			if err != nil {
				t.Fatal(err)
			}
			var inner wireMessage
			if err := json.Unmarshal(opened, &inner); err != nil {
				t.Fatal(err)
			}
			if inner.Type == "process.output" && inner.Output == "phase34-secret" {
				sawSecret = true
			}
			if inner.Type == "process.exit" {
				sawExit = true
			}
		case <-deadline:
			t.Fatal("timed out waiting for encrypted process output")
		}
	}
	if !sawSecret {
		t.Fatal("encrypted terminal output was not delivered to the authorized client")
	}
}

func mustDecodeURL(t *testing.T, value string) []byte {
	t.Helper()
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

type webAuthnFixture struct {
	credentialID string
	publicKey    string
	privateKey   *ecdsa.PrivateKey
}

func newWebAuthnFixture(t *testing.T) webAuthnFixture {
	t.Helper()
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	rawID := make([]byte, 32)
	if _, err := rand.Read(rawID); err != nil {
		t.Fatal(err)
	}
	x := privateKey.PublicKey.X.FillBytes(make([]byte, 32))
	y := privateKey.PublicKey.Y.FillBytes(make([]byte, 32))
	cose, err := cbor.Marshal(map[int]any{1: 2, 3: -7, -1: 1, -2: x, -3: y})
	if err != nil {
		t.Fatal(err)
	}
	return webAuthnFixture{credentialID: base64.RawURLEncoding.EncodeToString(rawID),
		publicKey: base64.StdEncoding.EncodeToString(cose), privateKey: privateKey}
}

func assertionForGrant(t *testing.T, fixture webAuthnFixture, grant, origin, rpID string) string {
	t.Helper()
	clientJSON, err := json.Marshal(map[string]any{"type": "webauthn.get", "challenge": grantChallenge(grant), "origin": origin, "crossOrigin": false})
	if err != nil {
		t.Fatal(err)
	}
	rpHash := sha256.Sum256([]byte(rpID))
	authData := make([]byte, 37)
	copy(authData, rpHash[:])
	authData[32] = 0x05
	clientHash := sha256.Sum256(clientJSON)
	signed := append(append([]byte{}, authData...), clientHash[:]...)
	digest := sha256.Sum256(signed)
	signature, err := ecdsa.SignASN1(rand.Reader, fixture.privateKey, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	response := map[string]any{"id": fixture.credentialID, "rawId": fixture.credentialID, "type": "public-key",
		"response": map[string]any{"clientDataJSON": base64.RawURLEncoding.EncodeToString(clientJSON),
			"authenticatorData": base64.RawURLEncoding.EncodeToString(authData), "signature": base64.RawURLEncoding.EncodeToString(signature)}}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func controlFixture(t *testing.T, role string) (authoritySnapshot, controlProof, ed25519.PrivateKey) {
	t.Helper()
	passkey := newWebAuthnFixture(t)
	clientPublic, clientPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	grantBytes, _ := json.Marshal(controlGrant{V: 1, ClientID: "client", UserID: "user",
		SigningPublicKey: base64.RawURLEncoding.EncodeToString(clientPublic), IssuedAt: now, ExpiresAt: now + int64(30*24*time.Hour/time.Millisecond)})
	grant := string(grantBytes)
	snapshot := authoritySnapshot{V: 1, WorkspaceID: "workspace", Members: []authorityMember{{
		UserID: "user", Role: role, Credentials: []authorityCredential{{ID: passkey.credentialID, PublicKey: passkey.publicKey}},
	}}}
	proof := controlProof{Grant: grant, CredentialID: passkey.credentialID,
		Assertion: assertionForGrant(t, passkey, grant, "https://rc.ohrats.party", "rc.ohrats.party")}
	return snapshot, proof, clientPrivate
}

func TestPasskeyGrantAndOwnerSignedLockSync(t *testing.T) {
	snapshot, proof, clientPrivate := controlFixture(t, "owner")
	grant, role, err := verifyControlProof(snapshot, proof, "https://rc.ohrats.party", "rc.ohrats.party")
	if err != nil || role != "owner" || grant.ClientID != "client" {
		t.Fatalf("grant rejected: %v %s", err, role)
	}
	dir := t.TempDir()
	current, _ := json.Marshal(snapshot)
	if err := saveLock(dir, string(current), "https://rc.ohrats.party", "rc.ohrats.party"); err != nil {
		t.Fatal(err)
	}
	next := snapshot
	next.APIKeys = []authorityAPIKey{{ID: "api", UserID: "user", PublicKey: base64.RawURLEncoding.EncodeToString(clientPrivate.Public().(ed25519.PublicKey)), Scopes: []string{"read"}}}
	nextJSON, _ := json.Marshal(next)
	digest := sha256.Sum256(nextJSON)
	signature := ed25519.Sign(clientPrivate, []byte("rc-authority-v1\n"+hex.EncodeToString(digest[:])))
	if err := syncLock(dir, string(nextJSON), proof, base64.RawURLEncoding.EncodeToString(signature)); err != nil {
		t.Fatal(err)
	}
	locked, err := loadLock(dir)
	if err != nil {
		t.Fatal(err)
	}
	if locked.Snapshot != string(nextJSON) {
		t.Fatal("signed authority snapshot was not persisted")
	}
}

func TestLockRejectsNonOwnerAndBootstrapIsTOFUOnly(t *testing.T) {
	snapshot, proof, clientPrivate := controlFixture(t, "operator")
	dir := t.TempDir()
	original, _ := json.Marshal(snapshot)
	if err := bootstrapLock(dir, string(original), "https://rc.ohrats.party"); err != nil {
		t.Fatal(err)
	}
	mutated := snapshot
	mutated.Members[0].Role = "owner"
	nextJSON, _ := json.Marshal(mutated)
	digest := sha256.Sum256(nextJSON)
	signature := ed25519.Sign(clientPrivate, []byte("rc-authority-v1\n"+hex.EncodeToString(digest[:])))
	if err := syncLock(dir, string(nextJSON), proof, base64.RawURLEncoding.EncodeToString(signature)); err == nil {
		t.Fatal("operator changed RC Lock authority")
	}
	if err := bootstrapLock(dir, string(nextJSON), "https://evil.invalid"); err != nil {
		t.Fatal(err)
	}
	locked, err := loadLock(dir)
	if err != nil {
		t.Fatal(err)
	}
	if locked.Snapshot != string(original) || locked.Origin != "https://rc.ohrats.party" {
		t.Fatal("bootstrap overwrote an existing lock")
	}
	if info, err := os.Stat(lockPath(dir)); err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("lock permissions: %v %v", info, err)
	}
}

func TestBootstrapRefusesCorruptExistingLock(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(lockPath(dir), []byte("not-json"), 0600); err != nil {
		t.Fatal(err)
	}
	snapshot, _, _ := controlFixture(t, "owner")
	encoded, _ := json.Marshal(snapshot)
	if err := bootstrapLock(dir, string(encoded), "https://rc.ohrats.party"); err == nil {
		t.Fatal("corrupt existing RC Lock was replaced by server bootstrap")
	}
	data, _ := os.ReadFile(lockPath(dir))
	if string(data) != "not-json" {
		t.Fatal("corrupt existing RC Lock was modified")
	}
}
