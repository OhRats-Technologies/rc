package node

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/OhRats-Technologies/rc/pkg/crypto"
	"github.com/OhRats-Technologies/rc/pkg/protocol"
	"github.com/OhRats-Technologies/rc/pkg/transport"
)

type fakeExecutionManager struct {
	mu       sync.Mutex
	started  bool
	owner    string
	attached string
}

func (f *fakeExecutionManager) ActiveIDs() []string { return nil }
func (f *fakeExecutionManager) Start(spec ProcessSpec) (bool, error) {
	f.mu.Lock()
	f.started = true
	f.owner = spec.UserID
	f.attached = spec.SessionID
	f.mu.Unlock()
	return true, nil
}
func (f *fakeExecutionManager) Input(string, []byte) error          { return nil }
func (f *fakeExecutionManager) CloseInput(string) error             { return nil }
func (f *fakeExecutionManager) Resize(string, uint16, uint16) error { return nil }
func (f *fakeExecutionManager) Signal(string, string) error         { return nil }
func (f *fakeExecutionManager) ExecutionRead(string, uint64, int) (*ExecutionRead, bool) {
	exit := int32(0)
	return &ExecutionRead{
		Status: "completed", NextCursor: 5, ExitCode: &exit,
		Chunks: []JournalChunk{{Stream: StreamStdout, Cursor: 5, Bytes: []byte("hello")}},
	}, true
}
func (f *fakeExecutionManager) ProcessOwner(string) (string, string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.owner, f.attached, f.started
}
func (f *fakeExecutionManager) AttachSecure(_ string, sessionID string) bool {
	f.mu.Lock()
	f.attached = sessionID
	f.mu.Unlock()
	return f.started
}
func (f *fakeExecutionManager) DetachSecureSession(string) {}
func (f *fakeExecutionManager) Shutdown()                  {}

func TestAuthorizedWebRTCControlSessionAndLockInvalidation(t *testing.T) {
	state, err := GenerateNodeState("dev_control")
	if err != nil {
		t.Fatal(err)
	}
	exec := &fakeExecutionManager{}
	manager := NewControlManager(state, t.TempDir(), exec)
	challenge := manager.CreateChallenge("req_challenge")

	signingPublic, signingPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	clientPrivate := randomControlValue(32)
	clientPublic, err := crypto.X25519Public(clientPrivate)
	if err != nil {
		t.Fatal(err)
	}
	authority := &sessionAuthority{
		UserID: "user_control", Role: "owner", CanExecute: true, CanManage: true,
		PublicKey: base64.RawURLEncoding.EncodeToString(signingPublic),
	}
	signature := base64.RawURLEncoding.EncodeToString(ed25519.Sign(signingPrivate,
		[]byte(crypto.SessionPayload(challenge, state.DeviceID, "client_control", clientPublic))))
	ready, err := manager.OpenAuthorizedSession(challenge, "client_control", clientPublic, signature, authority, nil)
	if err != nil {
		t.Fatal(err)
	}
	identityPublic, err := state.IdentityPublicKey()
	if err != nil {
		t.Fatal(err)
	}
	readyPayload := crypto.ReadyPayload(challenge, state.DeviceID, "client_control", clientPublic,
		ready.TransportPublicKey, ready.EphemeralPublicKey, ready.SessionID,
		protocol.ControlAttemptsPayload(ready.Attempts))
	if err := crypto.VerifyEd25519(identityPublic, []byte(readyPayload), ready.Signature); err != nil {
		t.Fatalf("node handshake signature = %v", err)
	}
	clientKey, err := crypto.DeriveClientKey(clientPrivate, ready.TransportPublicKey,
		ready.EphemeralPublicKey, challenge, state.DeviceID, "client_control")
	if err != nil {
		t.Fatal(err)
	}

	peer, err := transport.NewPeer(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	offer, err := peer.Offer()
	if err != nil {
		t.Fatal(err)
	}
	answer, err := manager.AnswerWebRTC(ready.SessionID, offer, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := peer.AcceptAnswer(answer); err != nil {
		t.Fatal(err)
	}
	if err := peer.WaitOpen(10 * time.Second); err != nil {
		t.Fatal(err)
	}

	start, _ := json.Marshal(protocol.ControlMessage{
		Type: protocol.TypeProcessStart, ID: "process_control",
		Mode:        &protocol.ExecutionMode{Kind: protocol.ExecutionModeArgv, Program: "fixture"},
		Environment: &protocol.EnvironmentSpec{Base: protocol.EnvironmentBaseInherit},
	})
	ciphertext, err := crypto.EncryptFrame(&clientKey, 1, 1, ready.SessionID, "c2n", start)
	if err != nil {
		t.Fatal(err)
	}
	frame, _ := json.Marshal(protocol.ControlTransportMessage{
		Type: protocol.TypeControlFrame, SessionID: ready.SessionID,
		Sequence: 1, Ciphertext: ciphertext,
	})
	if err := peer.Send(frame); err != nil {
		t.Fatal(err)
	}

	gotTypes := map[string]bool{}
	deadline := time.Now().Add(5 * time.Second)
	for len(gotTypes) < 3 && time.Now().Before(deadline) {
		message, err := peer.Receive(time.Second)
		if err != nil {
			continue
		}
		var frame protocol.ControlTransportMessage
		if json.Unmarshal(message, &frame) != nil || frame.Type != protocol.TypeControlFrame {
			t.Fatalf("unexpected frame = %s", message)
		}
		plain, err := crypto.DecryptFrame(&clientKey, 2, frame.Sequence, ready.SessionID, "n2c", frame.Ciphertext)
		if err != nil {
			t.Fatal(err)
		}
		var control protocol.ControlMessage
		if json.Unmarshal(plain, &control) != nil {
			t.Fatal("invalid control message")
		}
		gotTypes[control.Type] = true
		if control.Type == protocol.TypeProcessStdout &&
			!strings.Contains(string(mustDecode(t, control.Data)), "hello") {
			t.Fatalf("stdout = %q", control.Data)
		}
	}
	for _, kind := range []string{protocol.TypeProcessStarted, protocol.TypeProcessStdout, protocol.TypeProcessExit} {
		if !gotTypes[kind] {
			t.Fatalf("missing %s: %+v", kind, gotTypes)
		}
	}

	manager.CloseAllSessions()
	if _, err := manager.HandleDirectFrame(ready.SessionID, 2, "ciphertext"); err == nil {
		t.Fatal("lock invalidation left the encrypted session usable")
	}
}

func mustDecode(t *testing.T, value string) []byte {
	t.Helper()
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

func TestControlProcessOwnershipDeniesCrossUserAndStaleWriter(t *testing.T) {
	state, err := GenerateNodeState("dev_owner")
	if err != nil {
		t.Fatal(err)
	}
	exec := &fakeExecutionManager{started: true, owner: "alice", attached: "sess_alice"}
	manager := NewControlManager(state, t.TempDir(), exec)
	alice := &ControlSession{SessionID: "sess_alice", UserID: "alice", CanExecute: true}
	bob := &ControlSession{SessionID: "sess_bob", UserID: "bob", CanExecute: true}
	staleAlice := &ControlSession{SessionID: "sess_stale", UserID: "alice", CanExecute: true}
	messages := []*protocol.ControlMessage{
		{Type: protocol.TypeProcessAttach, ID: "proc_owner"},
		{Type: protocol.TypeProcessStdin, ID: "proc_owner", Data: "eA"},
		{Type: protocol.TypeProcessResize, ID: "proc_owner", Cols: 80, Rows: 24},
		{Type: protocol.TypeProcessSignal, ID: "proc_owner", Signal: "TERM"},
	}
	for _, message := range messages {
		if _, err := manager.dispatchControlMessage(bob, message); err == nil {
			t.Fatalf("bob was allowed to %s another user's process", message.Type)
		}
	}
	for _, message := range messages[1:] {
		if _, err := manager.dispatchControlMessage(staleAlice, message); err == nil {
			t.Fatalf("stale session was allowed to %s another session's process", message.Type)
		}
	}
	for _, message := range messages[1:] {
		if _, err := manager.dispatchControlMessage(alice, message); err != nil {
			t.Fatalf("owner was denied %s: %v", message.Type, err)
		}
	}
	if _, err := manager.dispatchControlMessage(alice,
		&protocol.ControlMessage{Type: protocol.TypeProcessAttach, ID: "proc_owner"}); err != nil {
		t.Fatalf("owner attach = %v", err)
	}
}
