package node

import (
	"encoding/json"
	"time"

	"github.com/OhRats-Technologies/rc/pkg/crypto"
	"github.com/OhRats-Technologies/rc/pkg/protocol"
)

func HostedControlAuthority(dir string, proof *protocol.ControlProof, userID string) (*ControlAuthority, error) {
	lock, err := LoadLock(dir)
	if err != nil {
		return nil, err
	}
	snap, err := ValidateSnapshot(lock.Snapshot)
	if err != nil {
		return nil, err
	}
	auth, err := VerifyControlProof(snap, proof, lock.Origin, lock.RpID)
	if err != nil {
		return nil, err
	}
	if auth.Grant.UserID != userID || auth.Role == "viewer" {
		return nil, ErrLockCredential
	}
	return auth, nil
}

func VerifyMcpGrant(dir, grantJSON, signature string, control *ControlAuthority, userID, deviceID string) (*protocol.McpGrantPayload, error) {
	lock, err := LoadLock(dir)
	if err != nil {
		return nil, err
	}
	snap, err := ValidateSnapshot(lock.Snapshot)
	if err != nil {
		return nil, err
	}

	var grant protocol.McpGrantPayload
	if err := json.Unmarshal([]byte(grantJSON), &grant); err != nil {
		return nil, ErrLockMcpGrant
	}

	now := time.Now().UnixMilli()
	if !grant.ValidAudience() || grant.ID == "" || grant.UserID != userID || grant.UserID != control.Grant.UserID {
		return nil, ErrLockMcpGrant
	}
	if grant.IssuedAt > now+60000 || (grant.ExpiresAt != 0 && (grant.ExpiresAt <= now || grant.ExpiresAt <= grant.IssuedAt)) {
		return nil, ErrLockMcpGrant
	}
	if !grant.AllowsDevice(deviceID) || control.Role != "owner" {
		return nil, ErrLockMcpGrant
	}

	hasTerminalScope := false
	for _, sc := range grant.Scopes {
		if sc == "mcp:terminal" {
			hasTerminalScope = true
			break
		}
	}
	if !hasTerminalScope {
		return nil, ErrLockMcpGrant
	}

	digest := SnapshotHash(grantJSON)
	foundGrantInLock := false
	for _, entry := range snap.McpGrants {
		if entry.ID == grant.ID && entry.UserID == userID && entry.Hash == digest {
			foundGrantInLock = true
			break
		}
	}
	if !foundGrantInLock {
		return nil, ErrLockMcpGrant
	}

	payload := grant.SignaturePayload(digest)
	if err := crypto.VerifyEd25519(control.Grant.SigningPublicKey, []byte(payload), signature); err != nil {
		return nil, ErrLockMcpSignature
	}
	return &grant, nil
}

func ScheduleAuthority(dir, scheduleID, deviceID, specHash string) (*protocol.AuthorityScheduleGrant, error) {
	lock, err := LoadLock(dir)
	if err != nil {
		return nil, err
	}
	snap, err := ValidateSnapshot(lock.Snapshot)
	if err != nil {
		return nil, err
	}

	var found *protocol.AuthorityScheduleGrant
	for i := range snap.ScheduleGrants {
		sg := &snap.ScheduleGrants[i]
		if sg.ScheduleID == scheduleID {
			found = sg
			break
		}
	}
	if found == nil || found.DeviceID != deviceID || found.SpecHash != specHash {
		return nil, ErrLockScheduleGrant
	}

	now := time.Now().UnixMilli()
	if found.ExpiresAt != 0 && found.ExpiresAt <= now {
		return nil, ErrLockScheduleGrant
	}

	ownerFound := false
	for _, m := range snap.Members {
		if m.UserID == found.UserID && m.Role == "owner" {
			ownerFound = true
			break
		}
	}
	if !ownerFound {
		return nil, ErrLockScheduleGrant
	}

	return found, nil
}
