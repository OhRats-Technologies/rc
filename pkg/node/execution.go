package node

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"syscall"

	"github.com/creack/pty"
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
	master          *os.File
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

	// An interactive request owns a terminal, so the child must run on a PTY.
	// A login shell on pipes is non-interactive: no prompt, no echo, no job
	// control. Windows has no PTY backend here and stays on pipes.
	var master *os.File
	var stdinPipe io.WriteCloser
	var stdoutPipe, stderrPipe io.ReadCloser
	if spec.Terminal != nil {
		if spec.Terminal.Term != "" {
			cmd.Env = append(cmd.Env, "TERM="+spec.Terminal.Term)
		}
		master, err = pty.StartWithSize(cmd, &pty.Winsize{Cols: spec.Terminal.Cols, Rows: spec.Terminal.Rows})
		if err != nil && !errors.Is(err, pty.ErrUnsupported) {
			m.mu.Unlock()
			return false, err
		}
	}
	if master != nil {
		stdinPipe = master
		stdoutPipe = master
	} else {
		var errIn, errOut, errErr error
		stdinPipe, errIn = cmd.StdinPipe()
		stdoutPipe, errOut = cmd.StdoutPipe()
		stderrPipe, errErr = cmd.StderrPipe()
		if errIn != nil || errOut != nil || errErr != nil {
			m.mu.Unlock()
			return false, errors.New("failed creating process pipes")
		}

		if err := cmd.Start(); err != nil {
			_ = stdinPipe.Close()
			m.mu.Unlock()
			return false, err
		}
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
		master:          master,
	}
	m.processes[spec.ID] = p
	m.mu.Unlock()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		journal.streamReader(stdoutPipe, StreamStdout)
	}()
	if stderrPipe != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			journal.streamReader(stderrPipe, StreamStderr)
		}()
	}
	go p.waitExit(&wg)

	return true, nil
}

func (p *runningProcess) waitExit(wg *sync.WaitGroup) {
	wg.Wait()
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
	spec := p.spec
	master := p.master
	p.mu.RUnlock()
	if stdin == nil {
		return errors.New("stdin not available")
	}
	// A PTY applies its own line discipline, so bytes pass through untouched.
	if master != nil {
		_, err := stdin.Write(data)
		return err
	}
	_, err := stdin.Write(terminalInput(spec, data))
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
	master := p.master
	p.mu.RUnlock()
	// Closing the PTY master would tear down the session; the terminal's own
	// EOF (Ctrl-D) is the way to end input.
	if stdin == nil || master != nil {
		return nil
	}
	return stdin.Close()
}

func (m *NativeExecutionManager) Resize(id string, cols, rows uint16) error {
	m.mu.RLock()
	p, ok := m.processes[id]
	m.mu.RUnlock()
	if !ok {
		return errors.New("process not found")
	}
	p.mu.RLock()
	master := p.master
	p.mu.RUnlock()
	if master == nil {
		return nil
	}
	return pty.Setsize(master, &pty.Winsize{Cols: cols, Rows: rows})
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
	if runtime.GOOS == "windows" || sigName == "KILL" {
		return p.cmd.Process.Kill()
	}
	switch sigName {
	case "INT":
		return p.cmd.Process.Signal(syscall.SIGINT)
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
		if p.master != nil {
			_ = p.master.Close()
		}
		p.mu.Unlock()
	}
}
