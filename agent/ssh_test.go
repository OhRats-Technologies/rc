package main

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

func sshFixture(t *testing.T, role string) (string, wireMessage) {
	t.Helper()
	snapshot, proof, _ := controlFixture(t, role)
	dir := t.TempDir()
	raw, _ := json.Marshal(snapshot)
	if err := saveLock(dir, string(raw), "https://rc.ohrats.party", "rc.ohrats.party"); err != nil {
		t.Fatal(err)
	}
	return dir, wireMessage{Type: "ssh.process.start", ID: "ssh-process", SessionID: "ssh-session", UserID: "user",
		Command: "printf ssh-ok", Grant: proof.Grant, CredentialID: proof.CredentialID, Assertion: proof.Assertion}
}

func TestSshProcessRequiresOperatorOrOwnerGrant(t *testing.T) {
	for _, role := range []string{"owner", "operator"} {
		dir, message := sshFixture(t, role)
		if userID, err := verifySshProcessStart(dir, message); err != nil || userID != "user" {
			t.Fatalf("%s SSH grant rejected: %v", role, err)
		}
	}
	dir, message := sshFixture(t, "viewer")
	if _, err := verifySshProcessStart(dir, message); err == nil {
		t.Fatal("viewer SSH grant was accepted")
	}
	message.UserID = "other"
	if _, err := verifySshProcessStart(dir, message); err == nil {
		t.Fatal("mismatched SSH user was accepted")
	}
}

func TestSshProcessRunsThroughGenericProcessStreams(t *testing.T) {
	dir, message := sshFixture(t, "owner")
	events := make(chan wireMessage, 16)
	manager := newProcessManager()
	manager.attach(func(message wireMessage) error { events <- message; return nil })
	defer manager.shutdown()
	if err := handleSshProcess(dir, manager, message); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(5 * time.Second)
	sawOutput, sawExit := false, false
	for !sawExit {
		select {
		case event := <-events:
			if event.Type == "process.stdout" && event.ID == message.ID {
				data, err := base64.RawURLEncoding.DecodeString(event.Data)
				if err != nil {
					t.Fatal(err)
				}
				if string(data) == "ssh-ok" {
					sawOutput = true
				}
			}
			if event.Type == "process.exit" && event.ID == message.ID {
				sawExit = true
			}
		case <-deadline:
			t.Fatal("timed out waiting for SSH process")
		}
	}
	if !sawOutput {
		t.Fatal("SSH process output was not delivered")
	}
}

func TestSshProcessCommandsAreSessionBound(t *testing.T) {
	dir, message := sshFixture(t, "owner")
	message.Command = "cat"
	events := make(chan wireMessage, 16)
	manager := newProcessManager()
	manager.attach(func(message wireMessage) error { events <- message; return nil })
	defer manager.shutdown()
	if err := handleSshProcess(dir, manager, message); err != nil {
		t.Fatal(err)
	}
	if err := handleSshProcess(dir, manager, wireMessage{Type: "ssh.process.stdin", ID: message.ID, SessionID: "wrong",
		Data: base64.RawURLEncoding.EncodeToString([]byte("bad"))}); err == nil {
		t.Fatal("SSH process accepted input from another edge session")
	}
	if err := handleSshProcess(dir, manager, wireMessage{Type: "ssh.process.stdin", ID: message.ID, SessionID: message.SessionID,
		Data: base64.RawURLEncoding.EncodeToString([]byte("ok"))}); err != nil {
		t.Fatal(err)
	}
	if err := handleSshProcess(dir, manager, wireMessage{Type: "ssh.process.stdin.close", ID: message.ID, SessionID: message.SessionID}); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(5 * time.Second)
	for {
		select {
		case event := <-events:
			if event.Type == "process.stdout" {
				data, _ := base64.RawURLEncoding.DecodeString(event.Data)
				if string(data) == "ok" {
					return
				}
			}
		case <-deadline:
			t.Fatal("timed out waiting for session-bound stdin")
		}
	}
}
