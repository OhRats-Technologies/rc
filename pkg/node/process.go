package node

import (
	"github.com/OhRats-Technologies/rc/pkg/protocol"
)

type StreamKind string

const (
	StreamStdout StreamKind = "stdout"
	StreamStderr StreamKind = "stderr"
)

type ProcessLifetime string

const (
	LifetimeAttached  ProcessLifetime = "attached"
	LifetimeManaged   ProcessLifetime = "managed"
	LifetimeScheduled ProcessLifetime = "scheduled"
)

type ProcessSpec struct {
	ID               string
	Mode             protocol.ExecutionMode
	Environment      protocol.EnvironmentSpec
	Cwd              string
	Terminal         *protocol.TerminalSpec
	SessionID        string
	UserID           string
	AuthorizationID  string
	ScrollbackBytes  uint32
	StdinChunkBytes  uint32
	TerminateGraceMs uint32
	ReattachGraceMs  uint32
	Lifetime         ProcessLifetime
	MaxRuntimeMs     *uint64
}

type JournalChunk struct {
	Stream StreamKind `json:"stream"`
	Cursor uint64     `json:"cursor"`
	Bytes  []byte     `json:"bytes"`
}

type ExecutionRead struct {
	Status          string         `json:"status"`
	Chunks          []JournalChunk `json:"chunks"`
	NextCursor      uint64         `json:"nextCursor"`
	TruncatedBefore uint64         `json:"truncatedBefore"`
	More            bool           `json:"more"`
	ExitCode        *int32         `json:"exitCode,omitempty"`
	Signal          string         `json:"signal,omitempty"`
}

type ProcessEventKind string

const (
	EventStarted ProcessEventKind = "started"
	EventStdout  ProcessEventKind = "stdout"
	EventStderr  ProcessEventKind = "stderr"
	EventExit    ProcessEventKind = "exit"
)

type ProcessEvent struct {
	Kind     ProcessEventKind
	ID       string
	Data     []byte
	ExitCode int32
	Signal   string
}

type ExecutionManager interface {
	ActiveIDs() []string
	Start(spec ProcessSpec) (bool, error)
	Input(id string, data []byte) error
	CloseInput(id string) error
	Resize(id string, cols, rows uint16) error
	Signal(id string, signal string) error
	ExecutionRead(id string, cursor uint64, maxBytes int) (*ExecutionRead, bool)
	ProcessOwner(id string) (userID, sessionID string, ok bool)
	AttachSecure(id, sessionID string) bool
	DetachSecureSession(sessionID string)
	Shutdown()
}
