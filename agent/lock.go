package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
)

type authorityCredential struct {
	ID        string `json:"id"`
	PublicKey string `json:"publicKey"`
}
type authorityMember struct {
	UserID      string                `json:"userId"`
	Role        string                `json:"role"`
	Credentials []authorityCredential `json:"credentials"`
}
type authorityAPIKey struct {
	ID        string   `json:"id"`
	UserID    string   `json:"userId"`
	PublicKey string   `json:"publicKey"`
	Scopes    []string `json:"scopes"`
}
type authoritySnapshot struct {
	V           int               `json:"v"`
	WorkspaceID string            `json:"workspaceId"`
	Members     []authorityMember `json:"members"`
	APIKeys     []authorityAPIKey `json:"apiKeys"`
}
type lockState struct {
	Snapshot string `json:"snapshot"`
	Origin   string `json:"origin"`
	RPID     string `json:"rpId"`
}
type controlGrant struct {
	V                int    `json:"v"`
	ClientID         string `json:"clientId"`
	UserID           string `json:"userId"`
	SigningPublicKey string `json:"signingPublicKey"`
	IssuedAt         int64  `json:"issuedAt"`
	ExpiresAt        int64  `json:"expiresAt"`
}
type controlProof struct {
	Grant        string `json:"grant"`
	CredentialID string `json:"credentialId"`
	Assertion    string `json:"assertion"`
}

func authorityMemberForUser(snapshot authoritySnapshot, userID string) *authorityMember {
	for index := range snapshot.Members {
		if snapshot.Members[index].UserID == userID {
			return &snapshot.Members[index]
		}
	}
	return nil
}

func apiAuthority(snapshot authoritySnapshot, keyID string) (*authorityAPIKey, *authorityMember) {
	for index := range snapshot.APIKeys {
		key := &snapshot.APIKeys[index]
		if key.ID == keyID {
			return key, authorityMemberForUser(snapshot, key.UserID)
		}
	}
	return nil, nil
}

func hasScope(scopes []string, scope string) bool {
	for _, value := range scopes {
		if value == scope {
			return true
		}
	}
	return false
}

func lockPath(dir string) string { return filepath.Join(dir, "lock.json") }

func loadLock(dir string) (lockState, error) {
	var value lockState
	data, err := os.ReadFile(lockPath(dir))
	if err != nil {
		return value, err
	}
	err = json.Unmarshal(data, &value)
	return value, err
}

func saveLock(dir, snapshot, origin, rpID string) error {
	var parsed authoritySnapshot
	if json.Unmarshal([]byte(snapshot), &parsed) != nil || parsed.V != 1 || parsed.WorkspaceID == "" {
		return errors.New("invalid RC Lock authority snapshot")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(lockState{Snapshot: snapshot, Origin: origin, RPID: rpID}, "", "  ")
	return os.WriteFile(lockPath(dir), data, 0600)
}

func lockHash(dir string) string {
	value, err := loadLock(dir)
	if err != nil || value.Snapshot == "" {
		return ""
	}
	digest := sha256.Sum256([]byte(value.Snapshot))
	return hex.EncodeToString(digest[:])
}

func bootstrapLock(dir, snapshot, serverURL string) error {
	if _, err := os.Stat(lockPath(dir)); err == nil {
		if _, loadErr := loadLock(dir); loadErr != nil {
			return errors.New("existing RC Lock is unreadable; refusing server bootstrap")
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	parsed, err := url.Parse(serverURL)
	if err != nil || parsed.Scheme == "" || parsed.Hostname() == "" {
		return errors.New("invalid RC server origin")
	}
	return saveLock(dir, snapshot, parsed.Scheme+"://"+parsed.Host, parsed.Hostname())
}

func memberCredential(snapshot authoritySnapshot, credentialID string) (*authorityMember, *authorityCredential) {
	for index := range snapshot.Members {
		member := &snapshot.Members[index]
		for keyIndex := range member.Credentials {
			credential := &member.Credentials[keyIndex]
			if credential.ID == credentialID {
				return member, credential
			}
		}
	}
	return nil, nil
}

func grantChallenge(grant string) string {
	digest := sha256.Sum256([]byte("rc-control-grant-v1\n" + grant))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func verifyControlProof(snapshot authoritySnapshot, proof controlProof, origin, rpID string) (controlGrant, string, error) {
	var grant controlGrant
	if err := json.Unmarshal([]byte(proof.Grant), &grant); err != nil || grant.V != 1 || grant.ClientID == "" {
		return grant, "", errors.New("invalid control grant")
	}
	now := time.Now().UnixMilli()
	if grant.ExpiresAt <= now || grant.IssuedAt > now+60_000 || grant.ExpiresAt-grant.IssuedAt > int64(31*24*time.Hour/time.Millisecond) {
		return grant, "", errors.New("expired control grant")
	}
	member, credential := memberCredential(snapshot, proof.CredentialID)
	if member == nil || credential == nil || member.UserID != grant.UserID || member.Role == "viewer" {
		return grant, "", errors.New("control credential is not authorized")
	}
	assertionBytes := []byte(proof.Assertion)
	parsed, err := protocol.ParseCredentialRequestResponseBytes(assertionBytes)
	if err != nil {
		return grant, "", errors.New("invalid passkey assertion")
	}
	rawID := base64.RawURLEncoding.EncodeToString(parsed.RawID)
	if rawID != proof.CredentialID {
		return grant, "", errors.New("passkey credential mismatch")
	}
	credentialBytes, err := base64.StdEncoding.DecodeString(credential.PublicKey)
	if err != nil {
		return grant, "", errors.New("invalid stored passkey")
	}
	err = parsed.Verify(grantChallenge(proof.Grant), rpID, "",
		[]string{origin}, nil, protocol.TopOriginDefaultVerificationMode,
		false, true, true, credentialBytes)
	if err != nil {
		return grant, "", errors.New("passkey grant verification failed")
	}
	return grant, member.Role, nil
}

func syncLock(dir, snapshotJSON string, proof controlProof, signature string) error {
	current, err := loadLock(dir)
	if err != nil {
		return errors.New("RC Lock is not initialized")
	}
	var oldSnapshot, nextSnapshot authoritySnapshot
	if json.Unmarshal([]byte(current.Snapshot), &oldSnapshot) != nil || json.Unmarshal([]byte(snapshotJSON), &nextSnapshot) != nil {
		return errors.New("invalid authority snapshot")
	}
	if oldSnapshot.WorkspaceID != nextSnapshot.WorkspaceID {
		return errors.New("workspace authority mismatch")
	}
	grant, role, err := verifyControlProof(oldSnapshot, proof, current.Origin, current.RPID)
	if err != nil || role != "owner" {
		return errors.New("owner authorization required for RC Lock sync")
	}
	publicKey, err := base64.RawURLEncoding.DecodeString(grant.SigningPublicKey)
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		return errors.New("invalid client public key")
	}
	digest := sha256.Sum256([]byte(snapshotJSON))
	payload := "rc-authority-v1\n" + hex.EncodeToString(digest[:])
	sig, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil || !ed25519.Verify(ed25519.PublicKey(publicKey), []byte(payload), sig) {
		return errors.New("invalid RC Lock authority signature")
	}
	return saveLock(dir, snapshotJSON, current.Origin, current.RPID)
}
