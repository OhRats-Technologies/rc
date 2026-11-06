package protocol

type NodeToServer struct {
	Type                  string              `json:"type"`
	Hello                 *NodeHello          `json:"hello,omitempty"`
	IDs                   []string            `json:"ids,omitempty"`
	ID                    string              `json:"id,omitempty"`
	UserID                string              `json:"user_id,omitempty"`
	ExitCode              int32               `json:"exit_code,omitempty"`
	Signal                string              `json:"signal,omitempty"`
	Error                 string              `json:"error,omitempty"`
	RequestID             string              `json:"request_id,omitempty"`
	Challenge             string              `json:"challenge,omitempty"`
	SessionID             string              `json:"session_id,omitempty"`
	TransportPublicKey    string              `json:"transport_public_key,omitempty"`
	EphemeralPublicKey    string              `json:"ephemeral_public_key,omitempty"`
	Signature             string              `json:"signature,omitempty"`
	Attempts              []ControlIceAttempt `json:"attempts,omitempty"`
	SDP                   string              `json:"sdp,omitempty"`
	Hash                  string              `json:"hash,omitempty"`
	Generation            uint64              `json:"generation,omitempty"`
	Data                  string              `json:"data,omitempty"`
	ProcessID             string              `json:"process_id,omitempty"`
	Status                string              `json:"status,omitempty"`
	Chunks                []McpExecutionChunk `json:"chunks,omitempty"`
	NextCursor            uint64              `json:"next_cursor,omitempty"`
	TruncatedBeforeCursor uint64              `json:"truncated_before_cursor,omitempty"`
	OutputPending         bool                `json:"output_pending,omitempty"`
	Accepted              bool                `json:"accepted,omitempty"`
	MimeType              string              `json:"mime_type,omitempty"`
	SizeBytes             uint64              `json:"size_bytes,omitempty"`
	OK                    bool                `json:"ok,omitempty"`
	Version               string              `json:"version,omitempty"`
}

type ServerToNode struct {
	Type               string          `json:"type"`
	Snapshot           string          `json:"snapshot,omitempty"`
	PreviousHash       string          `json:"previous_hash,omitempty"`
	PreviousGeneration uint64          `json:"previous_generation,omitempty"`
	Grant              string          `json:"grant,omitempty"`
	CredentialID       string          `json:"credential_id,omitempty"`
	Assertion          string          `json:"assertion,omitempty"`
	Signature          string          `json:"signature,omitempty"`
	ID                 string          `json:"id,omitempty"`
	UserID             string          `json:"user_id,omitempty"`
	RequestID          string          `json:"request_id,omitempty"`
	Challenge          string          `json:"challenge,omitempty"`
	ClientID           string          `json:"client_id,omitempty"`
	PublicKey          string          `json:"public_key,omitempty"`
	IceServers         []IceServer     `json:"ice_servers,omitempty"`
	SessionID          string          `json:"session_id,omitempty"`
	SDP                string          `json:"sdp,omitempty"`
	Mode               ControlIceMode  `json:"mode,omitempty"`
	ExecMode           *ExecutionMode  `json:"mode_exec,omitempty"`
	ProcessID          string          `json:"process_id,omitempty"`
	Command            string          `json:"command,omitempty"`
	Cwd                string          `json:"cwd,omitempty"`
	Terminal           *TerminalSpec   `json:"terminal,omitempty"`
	Data               string          `json:"data,omitempty"`
	Cols               uint16          `json:"cols,omitempty"`
	Rows               uint16          `json:"rows,omitempty"`
	Signal             string          `json:"signal,omitempty"`
	Environment        EnvironmentSpec `json:"environment,omitempty"`
	MaxRuntimeSeconds  *uint64         `json:"max_runtime_seconds,omitempty"`
	McpGrant           string          `json:"mcp_grant,omitempty"`
	McpSignature       string          `json:"mcp_signature,omitempty"`
	ControlGrant       string          `json:"control_grant,omitempty"`
	ControlAssertion   string          `json:"control_assertion,omitempty"`
	Cursor             uint64          `json:"cursor,omitempty"`
	WaitSeconds        uint64          `json:"wait_seconds,omitempty"`
	Path               string          `json:"path,omitempty"`
	EOF                bool            `json:"eof,omitempty"`
}
