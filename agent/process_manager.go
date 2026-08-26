package main

import (
	"encoding/base64"
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
	edgeSession  string
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
