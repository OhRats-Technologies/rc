package main

import (
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
	cmd      *exec.Cmd
	terminal *os.File
	lifeline *os.File
}

type processManager struct {
	mu           sync.Mutex
	processes    map[string]*managedProcess
	send         func(wireMessage) error
	pending      []wireMessage
	pendingBytes int
	graceTimer   *time.Timer
}

const reconnectGrace = 45 * time.Second
const pendingOutputLimit = 8 << 20

func newProcessManager() *processManager {
	return &processManager{processes: map[string]*managedProcess{}}
}

func messageSize(message wireMessage) int {
	return len(message.Output) + len(message.Input) + len(message.Command) + 128
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
		manager.start(message)
	case "process.input":
		manager.input(message.ID, message.Input)
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

func (manager *processManager) start(message wireMessage) {
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
	terminal, err := pty.StartWithSize(cmd, terminalSize(message.Cols, message.Rows))
	_ = readEnd.Close()
	if err != nil {
		_ = writeEnd.Close()
		manager.mu.Unlock()
		manager.sendMessage(wireMessage{Type: "process.output", ID: message.ID, Output: fmt.Sprintf("process start failed: %v\r\n", err)})
		code := -1
		manager.sendMessage(wireMessage{Type: "process.exit", ID: message.ID, ExitCode: &code})
		return
	}
	managed := &managedProcess{cmd: cmd, terminal: terminal, lifeline: writeEnd}
	manager.processes[message.ID] = managed
	manager.mu.Unlock()
	manager.sendMessage(wireMessage{Type: "process.started", ID: message.ID})
	go manager.capture(message.ID, managed)
	go manager.wait(message.ID, managed)
}

func (manager *processManager) capture(id string, managed *managedProcess) {
	buffer := make([]byte, 16*1024)
	for {
		n, err := managed.terminal.Read(buffer)
		if n > 0 {
			manager.sendMessage(wireMessage{Type: "process.output", ID: id, Output: string(buffer[:n])})
		}
		if err != nil {
			return
		}
	}
}

func (manager *processManager) wait(id string, managed *managedProcess) {
	err := managed.cmd.Wait()
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
	_ = managed.terminal.Close()
	manager.sendMessage(wireMessage{Type: "process.exit", ID: id, ExitCode: &code, Signal: signal})
}

func (manager *processManager) input(id, value string) {
	manager.mu.Lock()
	process := manager.processes[id]
	manager.mu.Unlock()
	if process != nil && value != "" {
		_, _ = io.WriteString(process.terminal, value)
	}
}

func (manager *processManager) resize(id string, cols, rows int) {
	manager.mu.Lock()
	process := manager.processes[id]
	manager.mu.Unlock()
	if process != nil {
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
		_, _ = process.terminal.Write([]byte{3})
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
		_ = process.terminal.Close()
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
