package server

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"sort"
	"time"

	authstore "github.com/OhRats-Technologies/rc/pkg/auth"
	"github.com/OhRats-Technologies/rc/pkg/auth/webauthn"
	"github.com/OhRats-Technologies/rc/pkg/node"
	"github.com/OhRats-Technologies/rc/pkg/protocol"
)

func (s *Server) authoritySnapshot(workspaceID string) (string, error) {
	devices := s.registry.List()
	authorityDevices := make([]protocol.AuthorityDevice, 0)
	for _, device := range devices {
		if device.WorkspaceID != workspaceID {
			continue
		}
		authorityDevices = append(authorityDevices, protocol.AuthorityDevice{
			ID: device.ID, IdentityPublicKey: device.IdentityPublicKey,
			TransportPublicKey: device.TransportPublicKey,
		})
	}
	sort.Slice(authorityDevices, func(i, j int) bool { return authorityDevices[i].ID < authorityDevices[j].ID })

	members, err := s.auth.WorkspaceMembers(workspaceID)
	if err != nil {
		return "", err
	}
	authorityMembers := make([]protocol.AuthorityMember, 0, len(members))
	owners := make(map[string]bool)
	for _, member := range members {
		passkeys, err := s.auth.PasskeysForUser(member.UserID)
		if err != nil {
			return "", err
		}
		credentials := make([]protocol.AuthorityCredential, 0, len(passkeys))
		for _, passkey := range passkeys {
			authorityKey, err := webauthn.AuthorityPublicKey(passkey.PublicKey)
			if err != nil {
				return "", err
			}
			credentials = append(credentials, protocol.AuthorityCredential{
				ID:        base64.RawURLEncoding.EncodeToString(passkey.CredentialID),
				PublicKey: authorityKey,
			})
		}
		authorityMembers = append(authorityMembers, protocol.AuthorityMember{
			UserID: member.UserID, Role: member.Role, Credentials: credentials,
		})
		if member.Role == "owner" {
			owners[member.UserID] = true
		}
	}
	sort.Slice(authorityMembers, func(i, j int) bool { return authorityMembers[i].UserID < authorityMembers[j].UserID })

	keys, err := s.auth.ActiveAPIKeysForWorkspace(workspaceID)
	if err != nil {
		return "", err
	}
	authorityKeys := make([]protocol.AuthorityApiKey, 0, len(keys))
	for _, key := range keys {
		authorityKeys = append(authorityKeys, protocol.AuthorityApiKey{
			ID: key.ID, UserID: key.UserID, PublicKey: key.PublicKey,
			Scopes: key.Scopes, ExpiresAt: msOrZero(key.ExpiresAt),
		})
	}
	sort.Slice(authorityKeys, func(i, j int) bool { return authorityKeys[i].ID < authorityKeys[j].ID })

	schedules, err := s.auth.ActiveScheduleGrantsForWorkspace(workspaceID)
	if err != nil {
		return "", err
	}
	authoritySchedules := make([]protocol.AuthorityScheduleGrant, 0, len(schedules))
	for _, grant := range schedules {
		authoritySchedules = append(authoritySchedules, protocol.AuthorityScheduleGrant{
			ScheduleID: grant.ScheduleID, DeviceID: grant.DeviceID,
			UserID: grant.UserID, SpecHash: grant.SpecHash,
			MaxRuntimeMs: grant.MaxRuntimeMs, ExpiresAt: msOrZero(grant.ExpiresAt),
		})
	}
	sort.Slice(authoritySchedules, func(i, j int) bool {
		return authoritySchedules[i].ScheduleID < authoritySchedules[j].ScheduleID
	})

	grants, err := s.auth.ActiveMcpGrants()
	if err != nil {
		return "", err
	}
	authorityGrants := make([]protocol.AuthorityMcpGrant, 0)
	for _, record := range grants {
		if !owners[record.UserID] {
			continue
		}
		var payload protocol.McpGrantPayload
		if json.Unmarshal([]byte(record.Grant), &payload) != nil || !payload.ValidAudience() ||
			!hasScope(payload.Scopes, authstore.OAuthScopeTerminal) {
			continue
		}
		authorityGrants = append(authorityGrants, protocol.AuthorityMcpGrant{
			ID: record.ID, UserID: record.UserID, Hash: node.SnapshotHash(record.Grant),
		})
	}
	sort.Slice(authorityGrants, func(i, j int) bool { return authorityGrants[i].ID < authorityGrants[j].ID })

	snapshot := protocol.AuthoritySnapshot{
		V: 1, WorkspaceID: workspaceID,
		Devices: authorityDevices, Members: authorityMembers, ApiKeys: authorityKeys,
		McpGrants: authorityGrants, ScheduleGrants: authoritySchedules,
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func (s *Server) handleAuthorityStatus(w http.ResponseWriter, req *http.Request) {
	user, _, err := s.currentSession(req)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	workspaceID := req.PathValue("id")
	if _, err := s.auth.WorkspaceRole(user.ID, workspaceID); err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}
	snapshot, err := s.authoritySnapshot(workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "authority snapshot failed")
		return
	}
	hash := node.SnapshotHash(snapshot)
	devices := make([]*RegisteredDevice, 0)
	for _, device := range s.registry.List() {
		if device.WorkspaceID == workspaceID {
			devices = append(devices, device)
		}
	}
	synced := 0
	states := make([]map[string]any, 0, len(devices))
	parents := make(map[string]map[string]any)
	for _, device := range devices {
		if device.LockHash == hash {
			synced++
		}
		states = append(states, map[string]any{
			"id": device.ID, "synced": device.LockHash == hash, "online": s.nodes.Online(device.ID),
		})
		if validAuthorityHash(device.LockHash) && device.LockHash != hash {
			parents[device.LockHash] = map[string]any{
				"hash": device.LockHash, "generation": device.LockGeneration,
			}
		}
	}
	pending, err := s.auth.AuthorityDeliveryParents(workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "authority state failed")
		return
	}
	for _, parent := range pending {
		hashValue, _ := parent[0].(string)
		generation, _ := parent[1].(uint64)
		if hashValue != "" && hashValue != hash {
			parents[hashValue] = map[string]any{"hash": hashValue, "generation": generation}
		}
	}
	parentList := make([]map[string]any, 0, len(parents))
	for _, parent := range parents {
		parentList = append(parentList, parent)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"hash": hash, "devices": len(devices), "synced": synced,
		"deviceStates": states, "parents": parentList,
	})
}

func (s *Server) handleAuthoritySync(w http.ResponseWriter, req *http.Request) {
	user, _, err := s.currentSession(req)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	workspaceID := req.PathValue("id")
	role, err := s.auth.WorkspaceRole(user.ID, workspaceID)
	if err != nil || role != "owner" {
		writeError(w, http.StatusForbidden, "owner required")
		return
	}
	var input struct {
		ClientID    string `json:"clientId"`
		Transitions []struct {
			FromHash   string `json:"fromHash"`
			Generation uint64 `json:"generation"`
			Signature  string `json:"signature"`
		} `json:"transitions"`
	}
	if err := decodeJSON(req, &input); err != nil || len(input.Transitions) == 0 || len(input.Transitions) > 100 {
		writeError(w, http.StatusBadRequest, "invalid authority transition")
		return
	}
	snapshot, err := s.authoritySnapshot(workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "authority snapshot failed")
		return
	}
	digest := node.SnapshotHash(snapshot)
	signatures := make(map[string]string)
	for _, transition := range input.Transitions {
		from := transition.FromHash
		if !validAuthorityHash(from) {
			writeError(w, http.StatusBadRequest, "invalid authority transition")
			return
		}
		payload := "rc-authority-v3\n" + itoa(int(transition.Generation)) + "\n" + from + "\n" + digest
		if !s.verifyControlSignature(user.ID, input.ClientID, payload, transition.Signature) {
			writeError(w, http.StatusForbidden, "invalid authority transition signature")
			return
		}
		signatures[from+"\x00"+itoa(int(transition.Generation))] = transition.Signature
	}
	proof, err := s.controlProof(user.ID, input.ClientID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "control client authorization expired")
		return
	}
	var grant protocol.ControlGrant
	_ = json.Unmarshal([]byte(proof.Grant), &grant)
	expires := time.Time{}
	if grant.ExpiresAt != 0 {
		expires = time.UnixMilli(grant.ExpiresAt).UTC()
	}
	devices := s.registry.List()
	online := 0
	for _, device := range devices {
		if device.WorkspaceID != workspaceID {
			continue
		}
		parents := make([][2]any, 0)
		if device.LockHash != "" {
			parents = append(parents, [2]any{device.LockHash, device.LockGeneration})
		}
		pending, _ := s.auth.AuthorityDeliveryParentsForDevice(device.ID)
		for _, parent := range pending {
			parents = append(parents, parent)
		}
		for _, parent := range parents {
			from, _ := parent[0].(string)
			generation, _ := parent[1].(uint64)
			signature, ok := signatures[from+"\x00"+itoa(int(generation))]
			if !ok {
				continue
			}
			message := protocol.ServerToNode{
				Type: "lock_sync", Snapshot: snapshot,
				PreviousHash: from, PreviousGeneration: generation,
				Grant: proof.Grant, CredentialID: proof.CredentialID,
				Assertion: proof.Assertion, Signature: signature,
			}
			encoded, _ := json.Marshal(message)
			_ = s.auth.PutAuthorityDelivery(authstore.AuthorityDelivery{
				DeviceID: device.ID, PreviousHash: from, Generation: generation,
				NextHash: digest, Message: string(encoded), ExpiresAt: expires,
			})
		}
		if s.deliverAuthority(device.ID) {
			online++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "devices": len(devices), "online": online,
		"lockHash": digest, "at": time.Now().UnixMilli(),
	})
}
