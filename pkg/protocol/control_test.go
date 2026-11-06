package protocol

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestExactArgvRoundTripsWithoutQuoting(t *testing.T) {
	values := []string{
		"",
		"hello world",
		"'",
		"\"",
		"\\",
		"line\nbreak",
		"emoji 🐀",
		"-leading",
	}

	cwd := "/tmp/with space"
	message := ControlMessage{
		Type: TypeProcessStart,
		ID:   "process-1",
		Mode: &ExecutionMode{
			Kind:    ExecutionModeArgv,
			Program: "fixture",
			Args:    values,
		},
		Cwd:         &cwd,
		Environment: &EnvironmentSpec{Base: EnvironmentBaseInherit},
	}

	encoded, err := json.Marshal(message)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded ControlMessage
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.Type != TypeProcessStart || decoded.Mode == nil || decoded.Mode.Program != "fixture" {
		t.Fatalf("unexpected decoded message: %+v", decoded)
	}
	if !reflect.DeepEqual(decoded.Mode.Args, values) {
		t.Fatalf("args mismatch: got %+v, want %+v", decoded.Mode.Args, values)
	}
}

func TestLoginShellIntentContainsNoUnixCommand(t *testing.T) {
	message := ControlMessage{
		Type: TypeProcessStart,
		ID:   "process-1",
		Mode: &ExecutionMode{
			Kind: ExecutionModeSystemLoginShell,
		},
		Environment: &EnvironmentSpec{Base: EnvironmentBaseInherit},
		Terminal: &TerminalSpec{
			Cols: 80,
			Rows: 24,
			Term: "xterm-256color",
		},
	}

	encoded, err := json.Marshal(message)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	s := string(encoded)
	if !strings.Contains(s, `"kind":"systemLoginShell"`) {
		t.Fatalf("expected kind systemLoginShell in %s", s)
	}
	if strings.Contains(s, "SHELL") || strings.Contains(s, "exec") {
		t.Fatalf("login shell intent must not contain unix command strings: %s", s)
	}
}

func TestStartupErrorRoundTripsOnProcessExit(t *testing.T) {
	code := int32(127)
	message := ControlMessage{
		Type:     TypeProcessExit,
		ID:       "process-1",
		ExitCode: &code,
		Error:    "The Node could not find the configured system shell",
	}

	encoded, err := json.Marshal(message)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded ControlMessage
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.Type != TypeProcessExit || decoded.ExitCode == nil || *decoded.ExitCode != 127 || decoded.Error != message.Error {
		t.Fatalf("exit mismatch: got %+v, want %+v", decoded, message)
	}
}

func TestScheduleHashBindsExecutionButNotMutableDisplayState(t *testing.T) {
	name := "Nightly"
	maxRuntime := uint64(60000)
	schedule := ScheduleDefinition{
		ID:       "schedule-1",
		Name:     &name,
		Cron:     "0 1 * * *",
		Timezone: "America/Toronto",
		Mode: ExecutionMode{
			Kind:    ExecutionModeArgv,
			Program: "fixture",
			Args:    []string{"hello world"},
		},
		Enabled:      true,
		Misfire:      ScheduleMisfireSkip,
		MaxRuntimeMs: &maxRuntime,
		CreatedBy:    "owner",
		CreatedAtMs:  1,
	}

	initial := ScheduleSpecHash(&schedule)

	// Mutate display / state fields that should not affect spec hash
	renamed := "Renamed"
	schedule.Name = &renamed
	schedule.Enabled = false
	schedule.CreatedAtMs = 2
	schedule.PermitHash = "server-copy"
	if ScheduleSpecHash(&schedule) != initial {
		t.Fatalf("hash changed when mutable fields were modified")
	}

	// Mutating cron MUST change the hash
	schedule.Cron = "0 2 * * *"
	if ScheduleSpecHash(&schedule) == initial {
		t.Fatalf("hash must change when cron is modified")
	}
}
