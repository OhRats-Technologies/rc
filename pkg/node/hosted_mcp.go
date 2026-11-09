package node

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/OhRats-Technologies/rc/pkg/protocol"
)

type hostedProcess struct {
	grantID   string
	userID    string
	clientID  string
	expiresAt time.Time
}

// HostedMCP executes commands requested by the hosted RC MCP surface. The
// Node verifies the Owner/passkey-authorized RC Lock grant before touching a
// process and owns all process journals.
type HostedMCP struct {
	stateDir string
	state    *NodeState
	exec     ExecutionManager

	sendMu sync.RWMutex
	send   func(protocol.NodeToServer) error

	mu        sync.Mutex
	processes map[string]hostedProcess
}

func NewHostedMCP(stateDir string, state *NodeState, exec ExecutionManager) *HostedMCP {
	return &HostedMCP{
		stateDir:  stateDir,
		state:     state,
		exec:      exec,
		processes: make(map[string]hostedProcess),
	}
}

func (m *HostedMCP) SetSend(send func(protocol.NodeToServer) error) {
	m.sendMu.Lock()
	m.send = send
	m.sendMu.Unlock()
}

func (m *HostedMCP) sendMessage(message protocol.NodeToServer) error {
	m.sendMu.RLock()
	send := m.send
	m.sendMu.RUnlock()
	if send == nil {
		return errors.New("Node control channel is unavailable")
	}
	return send(message)
}

func (m *HostedMCP) verify(message protocol.ServerToNode, scope string) (*protocol.McpGrantPayload, error) {
	var payload protocol.McpGrantPayload
	if err := json.Unmarshal([]byte(message.McpGrant), &payload); err != nil {
		return nil, errors.New("invalid execution grant")
	}
	proof := &protocol.ControlProof{
		Grant: message.ControlGrant, CredentialID: message.CredentialID,
		Assertion: message.ControlAssertion,
	}
	control, err := HostedControlAuthority(m.stateDir, proof, payload.UserID)
	if err != nil {
		return nil, err
	}
	grant, err := VerifyMcpGrant(m.stateDir, message.McpGrant, message.McpSignature,
		control, payload.UserID, m.state.DeviceID)
	if err != nil {
		return nil, err
	}
	if !hasGrantScope(grant.Scopes, scope) {
		return nil, errors.New("execution grant is missing the required scope")
	}
	return grant, nil
}

func (m *HostedMCP) Handle(message protocol.ServerToNode) {
	switch message.Type {
	case "mcp_start":
		m.start(message)
	case "mcp_status":
		m.status(message)
	case "mcp_input":
		m.input(message)
	case "mcp_cancel":
		m.cancel(message)
	case "mcp_image":
		m.image(message)
	}
}

func (m *HostedMCP) start(message protocol.ServerToNode) {
	grant, err := m.verify(message, "mcp:terminal")
	if err != nil {
		m.sendStatus(message.RequestID, message.ProcessID, nil, err)
		return
	}
	if message.ExecMode == nil || message.ProcessID == "" {
		m.sendStatus(message.RequestID, message.ProcessID, nil, errors.New("invalid execution request"))
		return
	}
	spec := ProcessSpec{
		ID:          message.ProcessID,
		Mode:        *message.ExecMode,
		Environment: message.Environment,
		Cwd:         message.Cwd,
		UserID:      grant.UserID,
		Lifetime:    LifetimeManaged,
	}
	if message.MaxRuntimeSeconds != nil && *message.MaxRuntimeSeconds > 0 {
		ms := *message.MaxRuntimeSeconds * 1000
		spec.MaxRuntimeMs = &ms
	}
	started, err := m.exec.Start(spec)
	if err != nil || !started {
		m.sendStatus(message.RequestID, message.ProcessID, nil,
			errors.New("execution could not be started"))
		return
	}
	m.mu.Lock()
	for id, process := range m.processes {
		if !process.expiresAt.IsZero() && process.expiresAt.Before(time.Now()) {
			delete(m.processes, id)
		}
	}
	expiresAt := time.Time{}
	if grant.ExpiresAt > 0 {
		expiresAt = time.UnixMilli(grant.ExpiresAt)
	}
	m.processes[message.ProcessID] = hostedProcess{
		grantID: grant.ID, userID: grant.UserID, clientID: grant.ClientID, expiresAt: expiresAt,
	}
	m.mu.Unlock()
	if spec.MaxRuntimeMs != nil && *spec.MaxRuntimeMs > 0 {
		processID := message.ProcessID
		time.AfterFunc(time.Duration(*spec.MaxRuntimeMs)*time.Millisecond, func() {
			_ = m.exec.Signal(processID, "KILL")
		})
	}
	if !expiresAt.IsZero() {
		processID := message.ProcessID
		time.AfterFunc(time.Until(expiresAt), func() {
			_ = m.exec.Signal(processID, "KILL")
		})
	}
	read := m.waitForExit(message.ProcessID, 0, message.WaitSeconds)
	m.markCompleted(message.ProcessID, read)
	m.sendStatus(message.RequestID, message.ProcessID, read, nil)
}

func (m *HostedMCP) status(message protocol.ServerToNode) {
	grant, err := m.verify(message, "mcp:observe")
	if err != nil {
		m.sendStatus(message.RequestID, message.ProcessID, nil, err)
		return
	}
	if !m.owned(message.ProcessID, grant) {
		m.sendStatus(message.RequestID, message.ProcessID, nil, errors.New("process not found"))
		return
	}
	read := m.waitForOutput(message.ProcessID, message.Cursor, message.WaitSeconds)
	m.markCompleted(message.ProcessID, read)
	m.sendStatus(message.RequestID, message.ProcessID, read, nil)
}

func (m *HostedMCP) input(message protocol.ServerToNode) {
	grant, err := m.verify(message, "mcp:terminal")
	if err != nil {
		m.sendOperation(message.RequestID, message.ProcessID, false)
		return
	}
	if !m.owned(message.ProcessID, grant) {
		m.sendOperation(message.RequestID, message.ProcessID, false)
		return
	}
	data, err := base64.StdEncoding.DecodeString(message.Data)
	if err != nil {
		m.sendOperation(message.RequestID, message.ProcessID, false)
		return
	}
	if len(data) > 0 {
		if err := m.exec.Input(message.ProcessID, data); err != nil {
			m.sendOperation(message.RequestID, message.ProcessID, false)
			return
		}
	}
	if message.EOF {
		if err := m.exec.CloseInput(message.ProcessID); err != nil {
			m.sendOperation(message.RequestID, message.ProcessID, false)
			return
		}
	}
	m.sendOperation(message.RequestID, message.ProcessID, true)
}

func (m *HostedMCP) cancel(message protocol.ServerToNode) {
	grant, err := m.verify(message, "mcp:terminal")
	if err != nil {
		m.sendOperation(message.RequestID, message.ProcessID, false)
		return
	}
	if !m.owned(message.ProcessID, grant) {
		m.sendOperation(message.RequestID, message.ProcessID, false)
		return
	}
	if err := m.exec.Signal(message.ProcessID, message.Signal); err != nil {
		m.sendOperation(message.RequestID, message.ProcessID, false)
		return
	}
	m.sendOperation(message.RequestID, message.ProcessID, true)
}

// AuthorityChanged terminates active MCP executions whose grant is no longer
// present in the accepted RC Lock snapshot. This makes revocation effective
// for already-running work, not only for the next request.
func (m *HostedMCP) AuthorityChanged(snapshot *protocol.AuthoritySnapshot) {
	allowed := make(map[string]bool, len(snapshot.McpGrants))
	for _, grant := range snapshot.McpGrants {
		allowed[grant.ID] = true
	}
	var terminate []string
	m.mu.Lock()
	for id, process := range m.processes {
		if process.grantID == "" || !allowed[process.grantID] {
			terminate = append(terminate, id)
			delete(m.processes, id)
		}
	}
	m.mu.Unlock()
	if m.exec == nil {
		return
	}
	for _, id := range terminate {
		_ = m.exec.Signal(id, "KILL")
	}
}

func (m *HostedMCP) owned(processID string, grant *protocol.McpGrantPayload) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	owner, ok := m.processes[processID]
	if !ok || (!owner.expiresAt.IsZero() && owner.expiresAt.Before(time.Now())) {
		if ok {
			delete(m.processes, processID)
		}
		return false
	}
	return owner.grantID == grant.ID && owner.userID == grant.UserID &&
		owner.clientID == grant.ClientID
}

func (m *HostedMCP) markCompleted(processID string, read *ExecutionRead) {
	if read == nil || read.Status == "running" {
		return
	}
	m.mu.Lock()
	owner, ok := m.processes[processID]
	if ok {
		owner.expiresAt = time.Now().Add(5 * time.Minute)
		m.processes[processID] = owner
	}
	m.mu.Unlock()
}

func (m *HostedMCP) waitForExit(processID string, cursor, waitSeconds uint64) *ExecutionRead {
	waitSeconds = minU64(waitSeconds, 60)
	deadline := time.Now().Add(time.Duration(waitSeconds) * time.Second)
	for {
		read, ok := m.exec.ExecutionRead(processID, cursor, 256*1024)
		if !ok {
			return nil
		}
		if read.Status != "running" || time.Now().After(deadline) {
			return read
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func (m *HostedMCP) waitForOutput(processID string, cursor, waitSeconds uint64) *ExecutionRead {
	waitSeconds = minU64(waitSeconds, 60)
	deadline := time.Now().Add(time.Duration(waitSeconds) * time.Second)
	for {
		read, ok := m.exec.ExecutionRead(processID, cursor, 256*1024)
		if !ok {
			return nil
		}
		if read.Status != "running" || len(read.Chunks) > 0 || time.Now().After(deadline) {
			return read
		}
		time.Sleep(25 * time.Millisecond)
	}
}
