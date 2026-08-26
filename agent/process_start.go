package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/creack/pty"
)

func terminalSize(cols, rows int) *pty.Winsize {
	if cols < 2 || cols > 500 {
		cols = 80
	}
	if rows < 2 || rows > 500 {
		rows = 24
	}
	return &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)}
}

func (manager *processManager) start(message wireMessage, sessionID, userID string) {
	manager.startWith(message, sessionID, userID, sessionID != "", "")
}

func (manager *processManager) startSsh(message wireMessage, userID, edgeSession string) {
	manager.startWith(message, "", userID, false, edgeSession)
}

func (manager *processManager) startWith(message wireMessage, sessionID, userID string, secure bool, edgeSession string) {
	if message.ID == "" || strings.TrimSpace(message.Command) == "" {
		return
	}
	manager.mu.Lock()
	if _, exists := manager.processes[message.ID]; exists {
		manager.mu.Unlock()
		return
	}
	executable, err := os.Executable()
	if err != nil {
		manager.mu.Unlock()
		return
	}
	readEnd, writeEnd, err := os.Pipe()
	if err != nil {
		manager.mu.Unlock()
		return
	}
	cmd := exec.Command(executable, "__process-runner")
	cmd.Env = append(os.Environ(), "OHRATS_PROCESS_COMMAND="+message.Command, "OHRATS_PROCESS_CWD="+message.Cwd)
	cmd.ExtraFiles = []*os.File{readEnd}
	managed := &managedProcess{cmd: cmd, lifeline: writeEnd, sessionID: sessionID, userID: userID, secure: secure, edgeSession: edgeSession}
	if message.Terminal != nil {
		term := strings.TrimSpace(message.Terminal.Term)
		if term == "" {
			term = "xterm-256color"
		}
		cmd.Env = append(cmd.Env, "OHRATS_PROCESS_TERMINAL=1", "TERM="+term)
		managed.terminal, err = pty.StartWithSize(cmd, terminalSize(message.Terminal.Cols, message.Terminal.Rows))
	} else {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		managed.stdin, err = cmd.StdinPipe()
		if err == nil {
			managed.stdout, err = cmd.StdoutPipe()
		}
		if err == nil {
			managed.stderr, err = cmd.StderrPipe()
		}
		if err == nil {
			err = cmd.Start()
		}
	}
	_ = readEnd.Close()
	if err != nil {
		_ = writeEnd.Close()
		manager.mu.Unlock()
		failed := &managedProcess{sessionID: sessionID, userID: userID, secure: sessionID != ""}
		manager.emitBytes(failed, "process.stderr", message.ID, []byte(fmt.Sprintf("process start failed: %v\n", err)))
		code := -1
		manager.emit(failed, wireMessage{Type: "process.exit", ID: message.ID, ExitCode: &code})
		return
	}
	manager.processes[message.ID] = managed
	manager.mu.Unlock()
	manager.emit(managed, wireMessage{Type: "process.started", ID: message.ID})
	if managed.terminal != nil {
		managed.captures.Add(1)
		go manager.capture(message.ID, "process.stdout", managed, managed.terminal)
	} else {
		managed.captures.Add(2)
		go manager.capture(message.ID, "process.stdout", managed, managed.stdout)
		go manager.capture(message.ID, "process.stderr", managed, managed.stderr)
	}
	go manager.wait(message.ID, managed)
}

func (manager *processManager) emitBytes(managed *managedProcess, kind, id string, data []byte) {
	if len(data) == 0 {
		return
	}
	manager.emit(managed, wireMessage{Type: kind, ID: id, Data: base64.RawURLEncoding.EncodeToString(data)})
}

func (manager *processManager) capture(id, kind string, managed *managedProcess, reader io.Reader) {
	defer managed.captures.Done()
	if reader == nil {
		return
	}
	buffer := make([]byte, 16*1024)
	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			manager.emitBytes(managed, kind, id, buffer[:n])
		}
		if err != nil {
			return
		}
	}
}

func (manager *processManager) wait(id string, managed *managedProcess) {
	err := managed.cmd.Wait()
	managed.captures.Wait()
	code, signal := 0, ""
	if err != nil {
		code = -1
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			code = exitError.ExitCode()
			if status, ok := exitError.Sys().(syscall.WaitStatus); ok && status.Signaled() {
				signal = signalName(status.Signal())
			}
		}
	}
	manager.mu.Lock()
	delete(manager.processes, id)
	manager.mu.Unlock()
	_ = managed.lifeline.Close()
	if managed.terminal != nil {
		_ = managed.terminal.Close()
	}
	if managed.stdin != nil {
		_ = managed.stdin.Close()
	}
	manager.emit(managed, wireMessage{Type: "process.exit", ID: id, ExitCode: &code, Signal: signal})
}
