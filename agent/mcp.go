package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"
)

type mcpActionGrant struct {
	ID   string `json:"id"`
	Hash string `json:"hash"`
}

type mcpGrant struct {
	V         int              `json:"v"`
	ID        string           `json:"id"`
	UserID    string           `json:"userId"`
	DeviceIDs []string         `json:"deviceIds"`
	Scopes    []string         `json:"scopes"`
	Actions   []mcpActionGrant `json:"actions"`
	IssuedAt  int64            `json:"issuedAt"`
	ExpiresAt int64            `json:"expiresAt"`
}

func mcpGrantSignaturePayload(raw string) string {
	digest := sha256.Sum256([]byte(raw))
	return "rc-mcp-grant-v1\n" + hex.EncodeToString(digest[:])
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func actionAllowed(grant mcpGrant, actionID, command, cwd string) bool {
	digest := sha256.Sum256([]byte(command + "\n" + cwd))
	hash := hex.EncodeToString(digest[:])
	for _, action := range grant.Actions {
		if action.ID == actionID && action.Hash == hash {
			return true
		}
	}
	return false
}

func mcpGrantActive(snapshot authoritySnapshot, grant mcpGrant, raw string) bool {
	digest := sha256.Sum256([]byte(raw))
	hash := hex.EncodeToString(digest[:])
	for _, active := range snapshot.MCPGrants {
		if active.ID == grant.ID && active.UserID == grant.UserID && active.Hash == hash {
			return true
		}
	}
	return false
}

func verifyMcpProcess(stateDir, deviceID string, message wireMessage) (string, error) {
	lock, err := loadLock(stateDir)
	if err != nil {
		return "", errors.New("RC Lock is not initialized")
	}
	var snapshot authoritySnapshot
	if json.Unmarshal([]byte(lock.Snapshot), &snapshot) != nil {
		return "", errors.New("invalid RC Lock state")
	}
	proof := controlProof{Grant: message.Grant, CredentialID: message.CredentialID, Assertion: message.Assertion}
	control, role, err := verifyControlProof(snapshot, proof, lock.Origin, lock.RPID)
	if err != nil || role != "owner" {
		return "", errors.New("MCP execution requires an Owner grant")
	}
	var grant mcpGrant
	if json.Unmarshal([]byte(message.McpGrant), &grant) != nil || grant.V != 1 || grant.ID == "" || grant.UserID != control.UserID || grant.UserID != message.UserID {
		return "", errors.New("invalid MCP grant")
	}
	now := time.Now().UnixMilli()
	if grant.IssuedAt > now+60_000 || (grant.ExpiresAt != 0 && (grant.ExpiresAt <= now || grant.ExpiresAt <= grant.IssuedAt ||
		grant.ExpiresAt-grant.IssuedAt > int64(366*24*time.Hour/time.Millisecond))) || !containsString(grant.DeviceIDs, deviceID) {
		return "", errors.New("MCP grant is expired or not valid for this device")
	}
	if !mcpGrantActive(snapshot, grant, message.McpGrant) {
		return "", errors.New("MCP grant is not active in RC Lock")
	}
	publicKey, err := base64.RawURLEncoding.DecodeString(control.SigningPublicKey)
	signature, sigErr := base64.RawURLEncoding.DecodeString(message.McpSignature)
	if err != nil || sigErr != nil || len(publicKey) != ed25519.PublicKeySize || !ed25519.Verify(ed25519.PublicKey(publicKey), []byte(mcpGrantSignaturePayload(message.McpGrant)), signature) {
		return "", errors.New("invalid MCP grant signature")
	}
	if message.McpKind == "terminal" && containsString(grant.Scopes, "mcp:terminal") {
		return grant.UserID, nil
	}
	if message.McpKind == "action" && containsString(grant.Scopes, "mcp:actions") && actionAllowed(grant, message.ActionID, message.Command, message.Cwd) {
		return grant.UserID, nil
	}
	return "", errors.New("MCP grant does not allow this command")
}

func handleMcpProcess(stateDir, deviceID string, manager *processManager, message wireMessage) error {
	userID, err := verifyMcpProcess(stateDir, deviceID, message)
	if err != nil {
		return err
	}
	manager.start(message, "", userID)
	return nil
}
