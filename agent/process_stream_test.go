package main

import (
	"encoding/base64"
	"testing"
	"time"
)

func collectProcess(t *testing.T, manager *processManager, processID string, events <-chan wireMessage) (stdout, stderr []byte) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case event := <-events:
			if event.ID != processID {
				continue
			}
			if event.Type == "process.stdout" || event.Type == "process.stderr" {
				data, err := base64.RawURLEncoding.DecodeString(event.Data)
				if err != nil {
					t.Fatal(err)
				}
				if event.Type == "process.stdout" {
					stdout = append(stdout, data...)
				} else {
					stderr = append(stderr, data...)
				}
			}
			if event.Type == "process.exit" {
				if event.ExitCode == nil || *event.ExitCode != 0 {
					t.Fatalf("unexpected process exit: %+v", event)
				}
				return stdout, stderr
			}
		case <-deadline:
			t.Fatal("timed out waiting for process exit")
		}
	}
}

func TestProcessWithoutTerminalPreservesBinaryStreams(t *testing.T) {
	events := make(chan wireMessage, 32)
	manager := newProcessManager()
	manager.attach(func(message wireMessage) error { events <- message; return nil })
	defer manager.shutdown()

	manager.start(wireMessage{Type: "process.start", ID: "binary",
		Command: "printf '\\000\\377A'; printf 'err' >&2"}, "", "")
	stdout, stderr := collectProcess(t, manager, "binary", events)
	if len(stdout) != 3 || stdout[0] != 0 || stdout[1] != 0xff || stdout[2] != 'A' {
		t.Fatalf("stdout was not binary safe: %v", stdout)
	}
	if string(stderr) != "err" {
		t.Fatalf("stderr was not separated: %q", stderr)
	}
}

func TestProcessStdinCloseDeliversEOF(t *testing.T) {
	events := make(chan wireMessage, 32)
	manager := newProcessManager()
	manager.attach(func(message wireMessage) error { events <- message; return nil })
	defer manager.shutdown()

	manager.start(wireMessage{Type: "process.start", ID: "stdin", Command: "cat"}, "", "")
	manager.input("stdin", base64.RawURLEncoding.EncodeToString([]byte("hello")))
	manager.closeInput("stdin")
	stdout, stderr := collectProcess(t, manager, "stdin", events)
	if string(stdout) != "hello" || len(stderr) != 0 {
		t.Fatalf("unexpected stream output stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestProcessTerminalMergesIntoTerminalStream(t *testing.T) {
	events := make(chan wireMessage, 32)
	manager := newProcessManager()
	manager.attach(func(message wireMessage) error { events <- message; return nil })
	defer manager.shutdown()

	manager.start(wireMessage{Type: "process.start", ID: "terminal", Command: "printf out; printf err >&2",
		Terminal: &terminalSpec{Cols: 80, Rows: 24, Term: "xterm-256color"}}, "", "")
	stdout, stderr := collectProcess(t, manager, "terminal", events)
	if string(stdout) != "outerr" {
		t.Fatalf("unexpected terminal stream: %q", stdout)
	}
	if len(stderr) != 0 {
		t.Fatalf("PTY unexpectedly exposed a separate stderr stream: %q", stderr)
	}
}
