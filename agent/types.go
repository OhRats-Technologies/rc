package main

const version = "0.15.4"

type state struct {
	DeviceID            string `json:"deviceId"`
	PrivateKey          string `json:"privateKey"`
	TransportPrivateKey string `json:"transportPrivateKey"`
	TransportPublicKey  string `json:"transportPublicKey"`
}

type enrollRequest struct {
	Token              string   `json:"token"`
	Name               string   `json:"name"`
	Hostname           string   `json:"hostname"`
	Platform           string   `json:"platform"`
	Arch               string   `json:"arch"`
	PublicKey          string   `json:"publicKey"`
	TransportPublicKey string   `json:"transportPublicKey"`
	AgentVersion       string   `json:"agentVersion"`
	Capabilities       []string `json:"capabilities"`
}

type enrollResponse struct {
	DeviceID string `json:"deviceId"`
}

type iceServer struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username,omitempty"`
	Credential string   `json:"credential,omitempty"`
}

type terminalSpec struct {
	Cols int    `json:"cols,omitempty"`
	Rows int    `json:"rows,omitempty"`
	Term string `json:"term,omitempty"`
}

type wireMessage struct {
	Type               string        `json:"type"`
	ID                 string        `json:"id,omitempty"`
	Command            string        `json:"command,omitempty"`
	Cwd                string        `json:"cwd,omitempty"`
	Data               string        `json:"data,omitempty"`
	Signal             string        `json:"signal,omitempty"`
	Cols               int           `json:"cols,omitempty"`
	Rows               int           `json:"rows,omitempty"`
	Terminal           *terminalSpec `json:"terminal,omitempty"`
	Output             string        `json:"output,omitempty"`
	ExitCode           *int          `json:"exitCode,omitempty"`
	AgentVersion       string        `json:"agentVersion,omitempty"`
	Hostname           string        `json:"hostname,omitempty"`
	Platform           string        `json:"platform,omitempty"`
	Arch               string        `json:"arch,omitempty"`
	Capabilities       []string      `json:"capabilities,omitempty"`
	TransportPublicKey string        `json:"transportPublicKey,omitempty"`
	EphemeralPublicKey string        `json:"ephemeralPublicKey,omitempty"`
	LockHash           string        `json:"lockHash,omitempty"`
	LockGeneration     uint64        `json:"lockGeneration,omitempty"`
	PreviousHash       string        `json:"previousHash,omitempty"`
	PreviousGeneration uint64        `json:"previousGeneration,omitempty"`
	UserID             string        `json:"userId,omitempty"`
	RequestID          string        `json:"requestId,omitempty"`
	Challenge          string        `json:"challenge,omitempty"`
	ClientID           string        `json:"clientId,omitempty"`
	Grant              string        `json:"grant,omitempty"`
	CredentialID       string        `json:"credentialId,omitempty"`
	Assertion          string        `json:"assertion,omitempty"`
	PublicKey          string        `json:"publicKey,omitempty"`
	Signature          string        `json:"signature,omitempty"`
	SessionID          string        `json:"sessionId,omitempty"`
	Sequence           uint64        `json:"sequence,omitempty"`
	Ciphertext         string        `json:"ciphertext,omitempty"`
	SDP                string        `json:"sdp,omitempty"`
	IceServers         []iceServer   `json:"iceServers,omitempty"`
	Snapshot           string        `json:"snapshot,omitempty"`
	McpGrant           string        `json:"mcpGrant,omitempty"`
	McpSignature       string        `json:"mcpSignature,omitempty"`
}
