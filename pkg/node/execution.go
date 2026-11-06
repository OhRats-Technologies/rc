package node

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"syscall"

	"github.com/OhRats-Technologies/rc/pkg/protocol"
)

type runningProcess struct {
	mu              sync.RWMutex
	spec            ProcessSpec
	cmd             *exec.Cmd
	stdin           io.WriteCloser
	status          string
	exitCode        *int32
	signal          string
	journal         *processJournal
	attachedSession string
	done            chan struct{}
}

// NativeExecutionManager provides direct native process execution.
type NativeExecutionManager struct {
	mu        sync.RWMutex
	processes map[string]*runningProcess
}

// NewNativeExecutionManager creates a native process manager.
func NewNativeExecutionManager() *NativeExecutionManager {
	return &NativeExecutionManager{
		processes: make(map[string]*runningProcess),
	}
}

func (m *NativeExecutionManager) ActiveIDs() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var ids []string
	for id, p := range m.processes {
		p.mu.RLock()
		if p.status == "starting" || p.status == "running" {
			ids = append(ids, id)
		}
		p.mu.RUnlock()
	}
	return ids
}

func (m *NativeExecutionManager) Start(spec ProcessSpec) (bool, error) {
	m.mu.Lock()
	if _, exists := m.processes[spec.ID]; exists {
		m.mu.Unlock()
		return false, errors.New("process ID already exists")
	}

	cmd, err := buildCommand(spec)
	if err != nil {
		m.mu.Unlock()
		return false, err
	}

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		m.mu.Unlock()
		return false, err
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdinPipe.Close()
		m.mu.Unlock()
		return false, err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		_ = stdinPipe.Close()
		_ = stdoutPipe.Close()
		m.mu.Unlock()
		return false, err
	}

	if err := cmd.Start(); err != nil {
		_ = stdinPipe.Close()
		_ = stdoutPipe.Close()
		_ = stderrPipe.Close()
		m.mu.Unlock()
		return false, err
	}

	journal := newProcessJournal()
	p := &runningProcess{
		spec:            spec,
		cmd:             cmd,
		stdin:           stdinPipe,
		status:          "running",
		journal:         journal,
		attachedSession: spec.SessionID,
		done:            make(chan struct{}),
	}
	m.processes[spec.ID] = p
	m.mu.Unlock()

	go journal.streamReader(stdoutPipe, StreamStdout)
	go journal.streamReader(stderrPipe, StreamStderr)
	go p.waitExit()

	return true, nil
}

func buildCommand(spec ProcessSpec) (*exec.Cmd, error) {
	var cmd *exec.Cmd

	switch spec.Mode.Kind {
	case protocol.ExecutionModeArgv:
		if spec.Mode.Program == "" {
			return nil, errors.New("missing program for argv execution")
		}
		cmd = exec.Command(spec.Mode.Program, spec.Mode.Args...)

	case protocol.ExecutionModeSystemLoginShell:
		shell := os.Getenv("SHELL")
		if shell == "" {
			if runtime.GOOS == "windows" {
				shell = "powershell.exe"
			} else {
				shell = "/bin/sh"
			}
		}
		cmd = exec.Command(shell)

	case protocol.ExecutionModeRcShell, protocol.ExecutionModeSystemShell:
		cmdStr := spec.Mode.Command
		if cmdStr == "" && spec.Mode.Script != "" {
			cmdStr = spec.Mode.Script
		}
		if cmdStr == "" {
			return nil, errors.New("missing command for shell execution")
		}
		if runtime.GOOS == "windows" {
			cmd = exec.Command("cmd.exe", "/c", cmdStr)
		} else {
			cmd = exec.Command("/bin/sh", "-c", cmdStr)
		}

	default:
		return nil, errors.New("unsupported execution mode")
	}

	if spec.Cwd != "" {
		cmd.Dir = spec.Cwd
	}

	if spec.Environment.Base == protocol.EnvironmentBaseClean {
		cmd.Env = []string{}
	} else {
		cmd.Env = os.Environ()
	}
	for _, change := range spec.Environment.Changes {
		if change.Value != nil {
			cmd.Env = append(cmd.Env, change.Name+"="+*change.Value)
		}
	}

	return cmd, nil
}

func (p *runningProcess) waitExit() {
	err := p.cmd.Wait()
	p.mu.Lock()
	defer p.mu.Unlock()

	var exitCode int32 = 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = int32(exitErr.ExitCode())
		} else {
			exitCode = 1
		}
	}
	p.exitCode = &exitCode
	p.status = "completed"
	close(p.done)
}

func (m *NativeExecutionManager) Input(id string, data []byte) error {
	m.mu.RLock()
	p, ok := m.processes[id]
	m.mu.RUnlock()
	if !ok {
		return errors.New("process not found")
	}
	p.mu.RLock()
	stdin := p.stdin
	p.mu.RUnlock()
	if stdin == nil {
		return errors.New("stdin not available")
	}
	_, err := stdin.Write(data)
	return err
}

func (m *NativeExecutionManager) CloseInput(id string) error {
	m.mu.RLock()
	p, ok := m.processes[id]
	m.mu.RUnlock()
	if !ok {
		return errors.New("process not found")
	}
	p.mu.RLock()
	stdin := p.stdin
	p.mu.RUnlock()
	if stdin == nil {
		return nil
	}
	return stdin.Close()
}

func (m *NativeExecutionManager) Resize(id string, cols, rows uint16) error {
	return nil
}

func (m *NativeExecutionManager) Signal(id string, sigName string) error {
	m.mu.RLock()
	p, ok := m.processes[id]
	m.mu.RUnlock()
	if !ok {
		return errors.New("process not found")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd == nil || p.cmd.Process == nil {
		return nil
	}

	p.signal = sigName
	switch sigName {
	case "INT":
		return p.cmd.Process.Signal(syscall.SIGINT)
	case "KILL":
		return p.cmd.Process.Kill()
	default:
		return p.cmd.Process.Signal(syscall.SIGTERM)
	}
}

func (m *NativeExecutionManager) ExecutionRead(id string, cursor uint64, maxBytes int) (*ExecutionRead, bool) {
	m.mu.RLock()
	p, ok := m.processes[id]
	m.mu.RUnlock()
	if !ok {
		return nil, false
	}

	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.journal.read(cursor, maxBytes, p.status, p.exitCode, p.signal), true
}

func (m *NativeExecutionManager) AttachSecure(id, sessionID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.processes[id]
	if !ok {
		return false
	}
	p.mu.Lock()
	p.attachedSession = sessionID
	p.mu.Unlock()
	return true
}

func (m *NativeExecutionManager) DetachSecureSession(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range m.processes {
		p.mu.Lock()
		if p.attachedSession == sessionID {
			p.attachedSession = ""
		}
		p.mu.Unlock()
	}
}

func (m *NativeExecutionManager) Shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range m.processes {
		p.mu.Lock()
		if p.cmd != nil && p.cmd.Process != nil && p.status == "running" {
			_ = p.cmd.Process.Kill()
		}
		p.mu.Unlock()
	}
}
