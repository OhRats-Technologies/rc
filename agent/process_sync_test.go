package main

import (
	"encoding/json"
	"testing"
)

func TestProcessManagerAttachSyncsActiveProcessIDs(t *testing.T) {
	manager := newProcessManager()
	manager.processes["process-a"] = &managedProcess{}
	var messages []wireMessage
	manager.attach(func(message wireMessage) error {
		messages = append(messages, message)
		return nil
	})
	defer manager.detach()

	if len(messages) < 2 {
		t.Fatalf("expected sync plus started event, got %d messages", len(messages))
	}
	if messages[0].Type != "process.sync" {
		t.Fatalf("first attach message = %q, want process.sync", messages[0].Type)
	}
	if messages[0].ProcessIDs == nil || len(*messages[0].ProcessIDs) != 1 || (*messages[0].ProcessIDs)[0] != "process-a" {
		t.Fatalf("unexpected synced ids: %#v", messages[0].ProcessIDs)
	}
}

func TestProcessSyncSerializesEmptyIDs(t *testing.T) {
	empty := []string{}
	encoded, err := json.Marshal(wireMessage{Type: "process.sync", ProcessIDs: &empty})
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"type":"process.sync","ids":[]}` {
		t.Fatalf("empty process sync = %s", encoded)
	}
}
