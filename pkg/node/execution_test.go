package node

import (
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/OhRats-Technologies/rc/pkg/protocol"
)

func TestNativeExecutionExactArgv(t *testing.T) {
	mgr := NewNativeExecutionManager()
	defer mgr.Shutdown()

	program := "echo"
	args := []string{"hello", "rc", "exact"}
	if runtime.GOOS == "windows" {
		program = "cmd.exe"
		args = []string{"/c", "echo", "hello", "rc", "exact"}
	}

	spec := ProcessSpec{
		ID: "proc_exact_1",
		Mode: protocol.ExecutionMode{
			Kind:    protocol.ExecutionModeArgv,
			Program: program,
			Args:    args,
		},
	}

	started, err := mgr.Start(spec)
	if err != nil || !started {
		t.Fatalf("Start failed: %v", err)
	}

	// Wait for process completion
	var read *ExecutionRead
	for i := 0; i < 50; i++ {
		time.Sleep(20 * time.Millisecond)
		var ok bool
		read, ok = mgr.ExecutionRead("proc_exact_1", 0, 4096)
		if ok && read.Status == "completed" {
			break
		}
	}

	if read == nil || read.Status != "completed" {
		t.Fatalf("expected completed status, got: %+v", read)
	}
	if read.ExitCode == nil || *read.ExitCode != 0 {
		t.Fatalf("unexpected exit code: %v", read.ExitCode)
	}

	var out strings.Builder
	for _, chunk := range read.Chunks {
		out.Write(chunk.Bytes)
	}
	if !strings.Contains(out.String(), "hello rc exact") {
		t.Fatalf("expected output 'hello rc exact', got %q", out.String())
	}
}

func TestNativeExecutionInputAndSignal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping cat test on windows")
	}
	mgr := NewNativeExecutionManager()
	defer mgr.Shutdown()

	spec := ProcessSpec{
		ID: "proc_cat_1", UserID: "alice", SessionID: "sess_one",
		Mode: protocol.ExecutionMode{
			Kind:    protocol.ExecutionModeArgv,
			Program: "cat",
		},
	}

	started, err := mgr.Start(spec)
	if err != nil || !started {
		t.Fatalf("Start failed: %v", err)
	}

	active := mgr.ActiveIDs()
	if len(active) != 1 || active[0] != "proc_cat_1" {
		t.Fatalf("expected active proc_cat_1, got %v", active)
	}
	owner, session, ok := mgr.ProcessOwner("proc_cat_1")
	if !ok || owner != "alice" || session != "sess_one" {
		t.Fatalf("process owner = %q, %q, %v", owner, session, ok)
	}
	if !mgr.AttachSecure("proc_cat_1", "sess_two") {
		t.Fatal("AttachSecure failed")
	}
	_, session, _ = mgr.ProcessOwner("proc_cat_1")
	if session != "sess_two" {
		t.Fatalf("attached session = %q", session)
	}

	if err := mgr.Input("proc_cat_1", []byte("stream input\n")); err != nil {
		t.Fatalf("Input failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)
	read, ok := mgr.ExecutionRead("proc_cat_1", 0, 4096)
	if !ok || len(read.Chunks) == 0 {
		t.Fatalf("failed reading echoed input: %+v", read)
	}

	if err := mgr.Signal("proc_cat_1", "KILL"); err != nil {
		t.Fatalf("Signal failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)
	read, _ = mgr.ExecutionRead("proc_cat_1", 0, 4096)
	if read.Status != "completed" {
		t.Fatalf("expected completed after KILL, got: %s", read.Status)
	}
}
