package node

import (
	"crypto/ed25519"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/OhRats-Technologies/rc/pkg/crypto"
	"github.com/OhRats-Technologies/rc/pkg/protocol"
)

func TestNodeStateSaveAndLoad(t *testing.T) {
	temp := t.TempDir()

	state, err := GenerateNodeState("node_dev_1")
	if err != nil {
		t.Fatalf("GenerateNodeState failed: %v", err)
	}

	if err := SaveState(temp, state); err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}

	loaded, err := LoadState(temp)
	if err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}

	if loaded.DeviceID != "node_dev_1" || loaded.IdentitySeed != state.IdentitySeed {
		t.Fatalf("loaded state mismatch: got %+v, want %+v", loaded, state)
	}
}

func TestLockBootstrapAndSync(t *testing.T) {
	temp := t.TempDir()

	ownerPub, ownerPriv, _ := ed25519.GenerateKey(nil)
	ownerPubStr := b64URL.EncodeToString(ownerPub)
	_ = ownerPriv

	snap := protocol.AuthoritySnapshot{
		V:           1,
		WorkspaceID: "ws_001",
		Members: []protocol.AuthorityMember{
			{
				UserID: "usr_owner",
				Role:   "owner",
				Credentials: []protocol.AuthorityCredential{
					{
						ID:        "cred_1",
						PublicKey: ownerPubStr,
					},
				},
			},
		},
	}

	snapJSON, _ := json.Marshal(snap)
	server := "https://rc.example.test"

	if err := BootstrapLock(temp, string(snapJSON), server); err != nil {
		t.Fatalf("BootstrapLock failed: %v", err)
	}

	lock, err := LoadLock(temp)
	if err != nil {
		t.Fatalf("LoadLock failed: %v", err)
	}
	if lock.Generation != 0 || lock.RpID != "rc.example.test" {
		t.Fatalf("unexpected lock state: %+v", lock)
	}

	hash, gen := LockMetadata(temp)
	if hash != SnapshotHash(string(snapJSON)) || gen != 0 {
		t.Fatalf("LockMetadata mismatch: hash=%s, gen=%d", hash, gen)
	}
}

func TestControlSessionEstablishmentAndEncryption(t *testing.T) {
	temp := t.TempDir()
	state, err := GenerateNodeState("dev_node_rtc")
	if err != nil {
		t.Fatalf("GenerateNodeState failed: %v", err)
	}

	mgr := NewControlManager(state, temp, nil)
	ch := mgr.CreateChallenge("req_100")
	if ch == "" {
		t.Fatal("empty challenge")
	}

	clientPriv := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	clientPub, err := crypto.X25519Public(clientPriv)
	if err != nil {
		t.Fatalf("client X25519Public failed: %v", err)
	}

	nodeTransportPub, err := state.TransportPublicKey()
	if err != nil {
		t.Fatalf("node TransportPublicKey failed: %v", err)
	}

	clientKey, err := crypto.DeriveClientKey(
		clientPriv,
		nodeTransportPub,
		nodeTransportPub,
		ch,
		state.DeviceID,
		"req_100",
	)
	if err != nil {
		t.Fatalf("DeriveClientKey failed: %v", err)
	}

	session, err := mgr.OpenSession("req_100", "sess_xyz", clientPub)
	if err != nil {
		t.Fatalf("OpenSession failed: %v", err)
	}

	// Client sends encrypted frame to Node
	msg := protocol.ControlMessage{
		Type: protocol.TypeProcessStdin,
		ID:   "p1",
		Data: "ls -la\n",
	}
	msgBytes, _ := json.Marshal(msg)

	ciphertext, err := crypto.EncryptFrame(&clientKey, 1, 1, "sess_xyz", "c2n", msgBytes)
	if err != nil {
		t.Fatalf("EncryptFrame failed: %v", err)
	}

	decrypted, err := session.DecryptFrame(1, ciphertext)
	if err != nil || string(decrypted) != string(msgBytes) {
		t.Fatalf("DecryptFrame failed: decrypted=%s, err=%v", string(decrypted), err)
	}

	// Test binary framing over HandleRawFrame
	binFrame, err := protocol.EncodeBinaryFrame(1, 1, []byte(ciphertext))
	if err != nil {
		t.Fatalf("EncodeBinaryFrame failed: %v", err)
	}
	if _, err := mgr.HandleRawFrame("sess_xyz", binFrame); err != nil {
		t.Fatalf("HandleRawFrame failed with binary frame: %v", err)
	}
}

func init() {
	_ = os.Setenv("RC_STATE_DIR", filepath.Join(os.TempDir(), "rc-test-state"))
	_ = time.Second
}
