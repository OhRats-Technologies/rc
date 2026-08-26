package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

type managedProcess struct {
	cmd          *exec.Cmd
	terminal     *os.File
	stdin        io.WriteCloser
	stdout       io.ReadCloser
	stderr       io.ReadCloser
	lifeline     *os.File
	captures     sync.WaitGroup
	sessionID    string
	userID       string
	secure       bool
	backlog      []wireMessage
	backlogBytes int
}

type processManager struct {
	mu           sync.Mutex
	processes    map[string]*managedProcess
	send         func(wireMessage) error
	pending      []wireMessage
	pendingBytes int
	graceTimer   *time.Timer
	secureSend   func(string, wireMessage) bool
}

const reconnectGrace = 45 * time.Second
const pendingOutputLimit = 8 << 20

func newProcessManager() *processManager {
	return &processManager{processes: map[string]*managedProcess{}}
}

func messageSize(message wireMessage) int {
	return len(message.Output) + len(message.Data) + len(message.Command) + 128
}

func (manager *processManager) queue(message wireMessage) {
	size := messageSize(message)
	for manager.pendingBytes+size > pendingOutputLimit && len(manager.pending) > 0 {
		manager.pendingBytes -= messageSize(manager.pending[0])
		manager.pending = manager.pending[1:]
	}
	manager.pending = append(manager.pending, message)
	manager.pendingBytes += size
}

func (manager *processManager) sendMessage(message wireMessage) {
	manager.mu.Lock()
	send := manager.send
	if send == nil {
		manager.queue(message)
		manager.mu.Unlock()
		return
	}
	manager.mu.Unlock()
	if err := send(message); err != nil {
		manager.mu.Lock()
		if manager.send != nil {
			manager.send = nil
		}
		manager.queue(message)
		manager.mu.Unlock()
	}
}

func (manager *processManager) attach(send func(wireMessage) error) {
	manager.mu.Lock()
	if manager.graceTimer != nil {
		manager.graceTimer.Stop()
		manager.graceTimer = nil
	}
	manager.send = send
	active := make([]string, 0, len(manager.processes))
	for id := range manager.processes {
		active = append(active, id)
	}
	pending := append([]wireMessage(nil), manager.pending...)
	manager.pending = nil
	manager.pendingBytes = 0
	manager.mu.Unlock()
	for _, id := range active {
		manager.sendMessage(wireMessage{Type: "process.started", ID: id})
	}
	for _, message := range pending {
		manager.sendMessage(message)
	}
}

func (manager *processManager) detach() {
	manager.mu.Lock()
	manager.send = nil
	if manager.graceTimer != nil {
		manager.graceTimer.Stop()
	}
	manager.graceTimer = time.AfterFunc(reconnectGrace, manager.shutdown)
	manager.mu.Unlock()
}

func (manager *processManager) handle(message wireMessage) {
	switch message.Type {
	case "process.start":
		manager.start(message, "", "")
	case "process.stdin":
		manager.input(message.ID, message.Data)
	case "process.stdin.close":
		manager.closeInput(message.ID)
	case "process.resize":
		manager.resize(message.ID, message.Cols, message.Rows)
	case "process.signal":
		manager.signal(message.ID, message.Signal)
	}
}

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
	managed := &managedProcess{cmd: cmd, lifeline: writeEnd, sessionID: sessionID, userID: userID, secure: sessionID != ""}
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

func (manager *processManager) input(id, value string) {
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(data) == 0 {
		return
	}
	manager.mu.Lock()
	process := manager.processes[id]
	manager.mu.Unlock()
	if process == nil {
		return
	}
	if process.terminal != nil {
		_, _ = process.terminal.Write(data)
	} else if process.stdin != nil {
		_, _ = process.stdin.Write(data)
	}
}

func (manager *processManager) closeInput(id string) {
	manager.mu.Lock()
	process := manager.processes[id]
	manager.mu.Unlock()
	if process != nil && process.terminal == nil && process.stdin != nil {
		_ = process.stdin.Close()
	}
}

func (manager *processManager) resize(id string, cols, rows int) {
	manager.mu.Lock()
	process := manager.processes[id]
	manager.mu.Unlock()
	if process != nil && process.terminal != nil {
		_ = pty.Setsize(process.terminal, terminalSize(cols, rows))
	}
}

func (manager *processManager) signal(id, value string) {
	manager.mu.Lock()
	process := manager.processes[id]
	manager.mu.Unlock()
	if process == nil {
		return
	}
	if strings.EqualFold(value, "INT") {
		if process.terminal != nil {
			_, _ = process.terminal.Write([]byte{3})
		} else if process.cmd.Process != nil {
			signalSession(process.cmd.Process.Pid, syscall.SIGINT)
		}
		return
	}
	if strings.EqualFold(value, "KILL") {
		_ = process.lifeline.Close()
		return
	}
	if process.cmd.Process != nil {
		_ = syscall.Kill(process.cmd.Process.Pid, syscall.SIGTERM)
	}
}

func (manager *processManager) shutdown() {
	manager.mu.Lock()
	processes := make([]*managedProcess, 0, len(manager.processes))
	for _, process := range manager.processes {
		processes = append(processes, process)
	}
	manager.mu.Unlock()
	for _, process := range processes {
		_ = process.lifeline.Close()
		if process.terminal != nil {
			_ = process.terminal.Close()
		}
		if process.stdin != nil {
			_ = process.stdin.Close()
		}
	}
}

func signalName(signal syscall.Signal) string {
	switch signal {
	case syscall.SIGINT:
		return "SIGINT"
	case syscall.SIGTERM:
		return "SIGTERM"
	case syscall.SIGKILL:
		return "SIGKILL"
	}
	return fmt.Sprintf("SIG%d", signal)
}
