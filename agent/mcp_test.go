package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"
)

func mcpFixture(t *testing.T, role, kind, command string) (string, wireMessage, ed25519.PrivateKey) {
	t.Helper()
	snapshot, proof, privateKey := controlFixture(t, role)
	scope := "mcp:terminal"
	actions := []mcpActionGrant{}
	if kind == "action" {
		scope = "mcp:actions"
		digest := sha256.Sum256([]byte(command + "\n"))
		actions = []mcpActionGrant{{ID: "action", Hash: hex.EncodeToString(digest[:])}}
	}
	grant := mcpGrant{V: 1, ID: "mcp", UserID: "user", DeviceIDs: []string{"device"}, Scopes: []string{scope}, Actions: actions,
		IssuedAt: time.Now().UnixMilli(), ExpiresAt: time.Now().Add(time.Hour).UnixMilli()}
	raw, _ := json.Marshal(grant)
	digest := sha256.Sum256(raw)
	snapshot.MCPGrants = []authorityMcpGrant{{ID: grant.ID, UserID: grant.UserID, Hash: hex.EncodeToString(digest[:])}}
	dir := t.TempDir()
	snapshotJSON, _ := json.Marshal(snapshot)
	if err := saveLock(dir, string(snapshotJSON), "https://rc.ohrats.party", "rc.ohrats.party"); err != nil {
		t.Fatal(err)
	}
	signature := ed25519.Sign(privateKey, []byte(mcpGrantSignaturePayload(string(raw))))
	return dir, wireMessage{Type: "mcp.process.start", ID: "process", UserID: "user", Command: command, McpKind: kind,
		ActionID: "action", McpGrant: string(raw), McpSignature: base64.RawURLEncoding.EncodeToString(signature),
		Grant: proof.Grant, CredentialID: proof.CredentialID, Assertion: proof.Assertion}, privateKey
}

func TestMcpTerminalRequiresActiveOwnerGrant(t *testing.T) {
	dir, message, _ := mcpFixture(t, "owner", "terminal", "printf ok")
	if _, err := verifyMcpProcess(dir, "device", message); err != nil {
		t.Fatal(err)
	}
	lock, _ := loadLock(dir)
	var snapshot authoritySnapshot
	_ = json.Unmarshal([]byte(lock.Snapshot), &snapshot)
	snapshot.MCPGrants = nil
	withoutGrant, _ := json.Marshal(snapshot)
	if err := saveLockGeneration(dir, string(withoutGrant), lock.Origin, lock.RPID, lock.Generation+1); err != nil {
		t.Fatal(err)
	}
	if _, err := verifyMcpProcess(dir, "device", message); err == nil {
		t.Fatal("revoked MCP grant remained executable after RC Lock removal")
	}
}

func TestMcpActionIsBoundToApprovedDefinition(t *testing.T) {
	dir, message, _ := mcpFixture(t, "owner", "action", "printf approved")
	if _, err := verifyMcpProcess(dir, "device", message); err != nil {
		t.Fatal(err)
	}
	message.Command = "printf tampered"
	if _, err := verifyMcpProcess(dir, "device", message); err == nil {
		t.Fatal("edited Action command passed an old MCP grant")
	}
}

func TestMcpExecutionRejectsOperatorSigner(t *testing.T) {
	dir, message, _ := mcpFixture(t, "operator", "terminal", "printf no")
	if _, err := verifyMcpProcess(dir, "device", message); err == nil {
		t.Fatal("operator created an executable MCP grant")
	}
}

func TestMcpExecutionRejectsOverlongGrantLifetime(t *testing.T) {
	dir, message, _ := mcpFixture(t, "owner", "terminal", "printf no")
	var grant mcpGrant
	_ = json.Unmarshal([]byte(message.McpGrant), &grant)
	grant.ExpiresAt = grant.IssuedAt + int64(400*24*time.Hour/time.Millisecond)
	raw, _ := json.Marshal(grant)
	message.McpGrant = string(raw)
	if _, err := verifyMcpProcess(dir, "device", message); err == nil {
		t.Fatal("Node accepted an MCP grant longer than its authorization ceiling")
	}
}

func TestMcpExecutionAllowsUntilRevokedGrant(t *testing.T) {
	dir, message, privateKey := mcpFixture(t, "owner", "terminal", "printf ok")
	lock, _ := loadLock(dir)
	var snapshot authoritySnapshot
	_ = json.Unmarshal([]byte(lock.Snapshot), &snapshot)
	var grant mcpGrant
	_ = json.Unmarshal([]byte(message.McpGrant), &grant)
	grant.ExpiresAt = 0
	raw, _ := json.Marshal(grant)
	digest := sha256.Sum256(raw)
	snapshot.MCPGrants = []authorityMcpGrant{{ID: grant.ID, UserID: grant.UserID, Hash: hex.EncodeToString(digest[:])}}
	snapshotJSON, _ := json.Marshal(snapshot)
	if err := saveLockGeneration(dir, string(snapshotJSON), lock.Origin, lock.RPID, lock.Generation); err != nil {
		t.Fatal(err)
	}
	message.McpGrant = string(raw)
	message.McpSignature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, []byte(mcpGrantSignaturePayload(string(raw)))))
	if _, err := verifyMcpProcess(dir, "device", message); err != nil {
		t.Fatalf("until-revoked MCP grant was rejected: %v", err)
	}
}

func TestMcpOwnerGrantRunsThroughExistingProcessManager(t *testing.T) {
	dir, message, _ := mcpFixture(t, "owner", "terminal", "printf mcp-ok")
	outbound := make(chan wireMessage, 16)
	manager := newProcessManager()
	manager.attach(func(message wireMessage) error { outbound <- message; return nil })
	defer manager.shutdown()
	if err := handleMcpProcess(dir, "device", manager, message); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(5 * time.Second)
	sawOutput, sawExit := false, false
	for !sawExit {
		select {
		case event := <-outbound:
			if event.Type == "process.output" && event.ID == message.ID && event.Output == "mcp-ok" {
				sawOutput = true
			}
			if event.Type == "process.exit" && event.ID == message.ID {
				sawExit = true
			}
		case <-deadline:
			t.Fatal("timed out waiting for MCP process")
		}
	}
	if !sawOutput {
		t.Fatal("authorized MCP process output was not delivered")
	}
}
