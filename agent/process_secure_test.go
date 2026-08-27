package main

import (
	"encoding/base64"
	"testing"
)

func TestSecureScrollbackReplaysDeliveredOutputOnAttach(t *testing.T) {
	manager := newProcessManager()
	process := &managedProcess{secure: true, sessionID: "first", userID: "user"}
	manager.processes["process"] = process
	var sent []wireMessage
	manager.setSecureSender(func(sessionID string, message wireMessage) bool {
		sent = append(sent, message)
		return true
	})

	first := wireMessage{Type: "process.stdout", ID: "process", Data: base64.RawURLEncoding.EncodeToString([]byte("before refresh\n"))}
	manager.emit(process, first)
	if len(sent) != 1 || len(process.scrollback) != 1 {
		t.Fatalf("live output was not both delivered and retained: sent=%d scrollback=%d", len(sent), len(process.scrollback))
	}

	manager.detachSecureSession("first")
	second := wireMessage{Type: "process.stdout", ID: "process", Data: base64.RawURLEncoding.EncodeToString([]byte("while detached\n"))}
	manager.emit(process, second)
	if len(sent) != 1 || len(process.scrollback) != 2 {
		t.Fatalf("detached output retention failed: sent=%d scrollback=%d", len(sent), len(process.scrollback))
	}

	manager.attachSecure(process, "second")
	if len(sent) != 3 {
		t.Fatalf("reattach replayed %d messages, want 2 historical messages", len(sent)-1)
	}
	if len(process.scrollback) != 2 {
		t.Fatalf("reattach cleared scrollback: %d", len(process.scrollback))
	}
}
