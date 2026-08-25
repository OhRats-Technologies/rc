package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func authorityJSONHash(snapshot []byte) string {
	digest := sha256.Sum256(snapshot)
	return hex.EncodeToString(digest[:])
}

func signAuthorityTransition(generation uint64, fromHash string, snapshot []byte, privateKey ed25519.PrivateKey) string {
	payload := fmt.Sprintf("rc-authority-v3\n%d\n%s\n%s", generation, fromHash, authorityJSONHash(snapshot))
	return base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, []byte(payload)))
}

func TestAuthoritySnapshotReplayIsRejected(t *testing.T) {
	initial, proof, privateKey := controlFixture(t, "owner")
	dir := t.TempDir()
	initialJSON, _ := json.Marshal(initial)
	if err := saveLock(dir, string(initialJSON), "https://rc.ohrats.party", "rc.ohrats.party"); err != nil {
		t.Fatal(err)
	}

	withKey := initial
	withKey.APIKeys = []authorityAPIKey{{
		ID: "temporary-key", UserID: "user",
		PublicKey: base64.RawURLEncoding.EncodeToString(privateKey.Public().(ed25519.PublicKey)),
		Scopes:    []string{"read"},
	}}
	withKeyJSON, _ := json.Marshal(withKey)
	initialHash := authorityJSONHash(initialJSON)
	withKeySignature := signAuthorityTransition(0, initialHash, withKeyJSON, privateKey)
	if err := syncLock(dir, string(withKeyJSON), initialHash, 0, proof, withKeySignature); err != nil {
		t.Fatal(err)
	}

	revoked := initial
	revokedJSON, _ := json.Marshal(revoked)
	withKeyHash := authorityJSONHash(withKeyJSON)
	revokedSignature := signAuthorityTransition(1, withKeyHash, revokedJSON, privateKey)
	if err := syncLock(dir, string(revokedJSON), withKeyHash, 1, proof, revokedSignature); err != nil {
		t.Fatal(err)
	}

	if err := syncLock(dir, string(withKeyJSON), initialHash, 0, proof, withKeySignature); err == nil {
		t.Fatal("replayed owner-signed authority snapshot restored a revoked API key")
	}
}

func TestExpiredApiKeyIsRejectedByNodeAuthority(t *testing.T) {
	snapshot, _, privateKey := controlFixture(t, "owner")
	snapshot.APIKeys = []authorityAPIKey{{
		ID: "expired", UserID: "user", PublicKey: base64.RawURLEncoding.EncodeToString(privateKey.Public().(ed25519.PublicKey)),
		Scopes: []string{"execute"}, ExpiresAt: time.Now().Add(-time.Minute).UnixMilli(),
	}}
	if key, _ := apiAuthority(snapshot, "expired"); key != nil {
		t.Fatal("Node accepted an expired API key from RC Lock")
	}
	snapshot.APIKeys[0].ExpiresAt = 0
	if key, _ := apiAuthority(snapshot, "expired"); key == nil {
		t.Fatal("Node rejected an until-revoked API key")
	}
}

func TestProcessPermitCannotBeClaimedByAnotherUser(t *testing.T) {
	processes := newProcessManager()
	defer processes.shutdown()
	manager := &controlManager{
		processes: processes,
		send:      func(wireMessage) error { return nil },
		sessions: map[string]*controlSession{
			"attacker-session": {userID: "attacker", canExecute: true},
			"victim-session":   {userID: "victim", canExecute: true},
		},
		pendingStarts: map[string]pendingSecureStart{},
	}
	manager.queueSecureStart("attacker-session", "attacker", wireMessage{Type: "process.start", ID: "victim-process", Command: "printf attacker"})
	manager.permitSecureStart(wireMessage{Type: "process.permit", ID: "victim-process", UserID: "victim"})
	processes.mu.Lock()
	defer processes.mu.Unlock()
	if len(processes.processes) != 0 {
		t.Fatal("a process permit for the victim released the attacker's pending command")
	}
}
