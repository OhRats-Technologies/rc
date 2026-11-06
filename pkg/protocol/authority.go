package protocol

import "fmt"

type AuthorityCredential struct {
	ID        string `json:"id"`
	PublicKey string `json:"publicKey"`
}

type AuthorityMember struct {
	UserID      string                `json:"userId"`
	Role        string                `json:"role"`
	Credentials []AuthorityCredential `json:"credentials,omitempty"`
}

type AuthorityApiKey struct {
	ID        string   `json:"id"`
	UserID    string   `json:"userId"`
	PublicKey string   `json:"publicKey"`
	Scopes    []string `json:"scopes,omitempty"`
	ExpiresAt int64    `json:"expiresAt,omitempty"`
}

type AuthorityMcpGrant struct {
	ID     string `json:"id"`
	UserID string `json:"userId"`
	Hash   string `json:"hash"`
}

type AuthorityScheduleGrant struct {
	ScheduleID   string `json:"scheduleId"`
	DeviceID     string `json:"deviceId"`
	UserID       string `json:"userId"`
	SpecHash     string `json:"specHash"`
	MaxRuntimeMs uint64 `json:"maxRuntimeMs"`
	ExpiresAt    int64  `json:"expiresAt,omitempty"`
}

type AuthorityDevice struct {
	ID                 string `json:"id"`
	IdentityPublicKey  string `json:"identityPublicKey"`
	TransportPublicKey string `json:"transportPublicKey"`
}

type McpGrantPayload struct {
	V          uint32   `json:"v"`
	ID         string   `json:"id"`
	UserID     string   `json:"userId"`
	ClientID   string   `json:"clientId"`
	ClientName string   `json:"clientName"`
	Audience   *string  `json:"audience,omitempty"`
	DeviceIDs  []string `json:"deviceIds,omitempty"`
	Scopes     []string `json:"scopes,omitempty"`
	IssuedAt   int64    `json:"issuedAt"`
	ExpiresAt  int64    `json:"expiresAt"`
}

func (p *McpGrantPayload) ValidAudience() bool {
	// Strict Version-2 MCP grants: Version 1 grants are deprecated and rejected.
	// Version 2 requires audience: "account" and an empty deviceIds list.
	return p.V == 2 && p.Audience != nil && *p.Audience == "account" && len(p.DeviceIDs) == 0
}

func (p *McpGrantPayload) AllowsDevice(deviceID string) bool {
	if !p.ValidAudience() {
		return false
	}
	return true
}

func (p *McpGrantPayload) SignaturePayload(digest string) string {
	return fmt.Sprintf("rc-mcp-grant-v%d\n%s", p.V, digest)
}

type AuthoritySnapshot struct {
	V              uint32                   `json:"v"`
	WorkspaceID    string                   `json:"workspaceId"`
	Devices        []AuthorityDevice        `json:"devices,omitempty"`
	Members        []AuthorityMember        `json:"members,omitempty"`
	ApiKeys        []AuthorityApiKey        `json:"apiKeys,omitempty"`
	McpGrants      []AuthorityMcpGrant      `json:"mcpGrants,omitempty"`
	ScheduleGrants []AuthorityScheduleGrant `json:"scheduleGrants,omitempty"`
}

type ControlGrant struct {
	V                uint32 `json:"v"`
	ClientID         string `json:"clientId"`
	UserID           string `json:"userId"`
	SigningPublicKey string `json:"signingPublicKey"`
	IssuedAt         int64  `json:"issuedAt"`
	ExpiresAt        int64  `json:"expiresAt"`
}

type ControlProof struct {
	Grant        string `json:"grant"`
	CredentialID string `json:"credentialId"`
	Assertion    string `json:"assertion"`
}

type LockState struct {
	Snapshot   string `json:"snapshot"`
	Origin     string `json:"origin"`
	RpID       string `json:"rpId"`
	Generation uint64 `json:"generation"`
}
