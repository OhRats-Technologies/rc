package node

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/OhRats-Technologies/rc/pkg/crypto"
	"github.com/OhRats-Technologies/rc/pkg/platform"
	"github.com/OhRats-Technologies/rc/pkg/protocol"
)

var (
	ErrLockMissing             = errors.New("RC Lock is not initialized")
	ErrLockCorrupt             = errors.New("existing RC Lock is unreadable; refusing server bootstrap")
	ErrLockSnapshot            = errors.New("invalid RC Lock authority snapshot")
	ErrLockOrigin              = errors.New("invalid RC server origin")
	ErrLockApiKey              = errors.New("API control key rejected")
	ErrLockGrant               = errors.New("invalid control grant")
	ErrLockGrantExpired        = errors.New("expired control grant")
	ErrLockCredential          = errors.New("control credential is not authorized")
	ErrLockStaleTransition     = errors.New("stale RC Lock authority transition")
	ErrLockWorkspaceMismatch   = errors.New("workspace authority mismatch")
	ErrLockOwnerRequired       = errors.New("owner authorization required for RC Lock sync")
	ErrLockAuthoritySignature  = errors.New("invalid RC Lock authority signature")
	ErrLockMcpGrant            = errors.New("MCP grant rejected")
	ErrLockMcpSignature        = errors.New("MCP grant signature rejected")
	ErrLockScheduleGrant       = errors.New("schedule authority rejected")
	ErrLockGenerationExhausted = errors.New("RC Lock generation exhausted")
)

type ApiControlAuthority struct {
	UserID           string
	Role             string
	PublicKey        string
	CanExecute       bool
	CanManageDevices bool
}

type ControlAuthority struct {
	Grant protocol.ControlGrant
	Role  string
}

func SnapshotHash(snapshot string) string {
	sum := sha256.Sum256([]byte(snapshot))
	return hex.EncodeToString(sum[:])
}

func ValidateSnapshot(snapshot string) (*protocol.AuthoritySnapshot, error) {
	var s protocol.AuthoritySnapshot
	if err := json.Unmarshal([]byte(snapshot), &s); err != nil {
		return nil, ErrLockSnapshot
	}
	if s.V != 1 || strings.TrimSpace(s.WorkspaceID) == "" {
		return nil, ErrLockSnapshot
	}
	return &s, nil
}

func LoadLock(dir string) (*protocol.LockState, error) {
	path := LockPath(dir)
	if err := validatePrivateFile(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrLockMissing
		}
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrLockMissing
		}
		return nil, err
	}
	var lock protocol.LockState
	if err := json.Unmarshal(data, &lock); err != nil {
		return nil, ErrLockCorrupt
	}
	if _, err := ValidateSnapshot(lock.Snapshot); err != nil {
		return nil, err
	}
	if lock.Origin == "" || lock.RpID == "" {
		return nil, ErrLockCorrupt
	}
	return &lock, nil
}

func BootstrapLock(dir, snapshot, server string) error {
	path := LockPath(dir)
	if _, err := os.Stat(path); err == nil {
		_, err := LoadLock(dir)
		if err != nil {
			return ErrLockCorrupt
		}
		return nil
	}
	if _, err := ValidateSnapshot(snapshot); err != nil {
		return err
	}
	parsed, err := url.Parse(server)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return ErrLockOrigin
	}
	origin := fmt.Sprintf("%s://%s", parsed.Scheme, parsed.Host)
	lock := protocol.LockState{
		Snapshot:   snapshot,
		Origin:     origin,
		RpID:       parsed.Hostname(),
		Generation: 0,
	}
	return saveJSON(dir, path, lock)
}

func LockMetadata(dir string) (string, uint64) {
	lock, err := LoadLock(dir)
	if err != nil {
		return "", 0
	}
	return SnapshotHash(lock.Snapshot), lock.Generation
}

func SyncLock(dir, snapshotJSON, previousHash string, previousGeneration uint64, proof *protocol.ControlProof, signature string) error {
	current, err := LoadLock(dir)
	if err != nil {
		return err
	}
	oldSnap, err := ValidateSnapshot(current.Snapshot)
	if err != nil {
		return err
	}
	nextSnap, err := ValidateSnapshot(snapshotJSON)
	if err != nil {
		return err
	}
	if oldSnap.WorkspaceID != nextSnap.WorkspaceID {
		return ErrLockWorkspaceMismatch
	}
	currentHash := SnapshotHash(current.Snapshot)
	if previousHash != currentHash || previousGeneration != current.Generation {
		return ErrLockStaleTransition
	}

	auth, err := VerifyControlProof(oldSnap, proof, current.Origin, current.RpID)
	if err != nil || auth.Role != "owner" {
		return ErrLockOwnerRequired
	}

	payload := fmt.Sprintf("rc-authority-v3\n%d\n%s\n%s", current.Generation, currentHash, SnapshotHash(snapshotJSON))
	if err := crypto.VerifyEd25519(auth.Grant.SigningPublicKey, []byte(payload), signature); err != nil {
		return ErrLockAuthoritySignature
	}

	current.Snapshot = snapshotJSON
	current.Generation++
	return saveJSON(dir, LockPath(dir), current)
}

func VerifyControlProof(snapshot *protocol.AuthoritySnapshot, proof *protocol.ControlProof, origin, rpID string) (*ControlAuthority, error) {
	var grant protocol.ControlGrant
	if err := json.Unmarshal([]byte(proof.Grant), &grant); err != nil {
		return nil, ErrLockGrant
	}
	if grant.V != 1 || grant.ClientID == "" || grant.UserID == "" {
		return nil, ErrLockGrant
	}

	now := time.Now().UnixMilli()
	if grant.IssuedAt > now+60000 || (grant.ExpiresAt != 0 && (grant.ExpiresAt <= now || grant.ExpiresAt <= grant.IssuedAt)) {
		return nil, ErrLockGrantExpired
	}

	var foundMember *protocol.AuthorityMember
	var foundCred *protocol.AuthorityCredential
	for i := range snapshot.Members {
		m := &snapshot.Members[i]
		for j := range m.Credentials {
			c := &m.Credentials[j]
			if c.ID == proof.CredentialID {
				foundMember = m
				foundCred = c
				break
			}
		}
		if foundMember != nil {
			break
		}
	}
	if foundMember == nil || foundMember.UserID != grant.UserID || (foundMember.Role != "owner" && foundMember.Role != "operator") {
		return nil, ErrLockCredential
	}

	challenge := crypto.ControlGrantChallenge(proof.Grant)
	if err := crypto.VerifyWebauthnAssertion(proof.Assertion, proof.CredentialID, foundCred.PublicKey, challenge, origin, rpID); err != nil {
		return nil, ErrLockCredential
	}

	return &ControlAuthority{
		Grant: grant,
		Role:  foundMember.Role,
	}, nil
}
