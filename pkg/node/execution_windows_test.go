//go:build windows

package node

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/OhRats-Technologies/rc/pkg/protocol"
)

func TestWindowsLoginShellTerminalInput(t *testing.T) {
	shell := os.Getenv("ComSpec")
	if shell == "" {
		t.Skip("ComSpec is not configured")
	}
	t.Setenv("RC_SHELL", shell)
	manager := NewNativeExecutionManager()
	defer manager.Shutdown()
	started, err := manager.Start(ProcessSpec{
		ID: "proc_windows_input", UserID: "test", SessionID: "sess_windows",
		Mode:        protocol.ExecutionMode{Kind: protocol.ExecutionModeSystemLoginShell},
		Environment: protocol.EnvironmentSpec{Base: protocol.EnvironmentBaseInherit},
		Terminal:    &protocol.TerminalSpec{Cols: 80, Rows: 24, Term: "xterm-256color"},
	})
	if err != nil || !started {
		t.Fatalf("start Windows login shell = %v, %v", started, err)
	}
	if err := manager.Input("proc_windows_input", []byte("echo RC_WINDOWS_INPUT_OK\r")); err != nil {
		t.Fatalf("terminal input = %v", err)
	}
	var output strings.Builder
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		read, ok := manager.ExecutionRead("proc_windows_input", 0, 64*1024)
		if !ok {
			break
		}
		output.Reset()
		for _, chunk := range read.Chunks {
			output.Write(chunk.Bytes)
		}
		if strings.Contains(output.String(), "RC_WINDOWS_INPUT_OK") {
			_ = manager.Input("proc_windows_input", []byte("exit\r"))
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("Windows login shell did not execute terminal input: %q", output.String())
}
