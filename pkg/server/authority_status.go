package server

import (
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/OhRats-Technologies/rc/pkg/node"
)

// authorityStatus compares each Node's acknowledged RC Lock with the workspace's current snapshot.
type authorityStatus struct {
	WorkspaceID string
	Hash        string
	Nodes       []*RegisteredDevice
	Pending     []*RegisteredDevice
}

func (s *Server) workspaceAuthority(workspaceID string) (authorityStatus, error) {
	snapshot, err := s.authoritySnapshot(workspaceID)
	if err != nil {
		return authorityStatus{}, err
	}
	status := authorityStatus{WorkspaceID: workspaceID, Hash: node.SnapshotHash(snapshot)}
	for _, device := range s.registry.List() {
		if device.WorkspaceID != workspaceID {
			continue
		}
		device.Online = s.nodes.Online(device.ID)
		status.Nodes = append(status.Nodes, device)
		if device.LockHash != status.Hash {
			status.Pending = append(status.Pending, device)
		}
	}
	sort.Slice(status.Nodes, func(i, j int) bool { return status.Nodes[i].Name < status.Nodes[j].Name })
	sort.Slice(status.Pending, func(i, j int) bool { return status.Pending[i].Name < status.Pending[j].Name })
	return status, nil
}

// pendingNodeSummary lists Nodes that lack the current access, with presence, for operator-facing text.
func pendingNodeSummary(pending []*RegisteredDevice) string {
	parts := make([]string, 0, len(pending))
	for _, device := range pending {
		presence := "offline"
		if device.Online {
			presence = "online"
		}
		parts = append(parts, device.Name+" ("+presence+")")
	}
	return strings.Join(parts, ", ")
}

// LockSyncState classifies one Node's RC Lock relative to the workspace snapshot.
type LockSyncState string

const (
	LockSynced         LockSyncState = "synced"
	LockUnbootstrapped LockSyncState = "unbootstrapped"
	LockStale          LockSyncState = "stale"
)

func classifyLockState(lockHash, currentHash string) LockSyncState {
	switch {
	case lockHash == currentHash && currentHash != "":
		return LockSynced
	case !validAuthorityHash(lockHash):
		return LockUnbootstrapped
	default:
		return LockStale
	}
}

// deviceAuthorityStatus scopes RC Lock state to one specific Node so a synced
// device page can never display another Node's out-of-sync warning.
type deviceAuthorityStatus struct {
	Workspace authorityStatus
	Device    *RegisteredDevice
	State     LockSyncState
}

func (s *Server) deviceAuthority(device *RegisteredDevice) (deviceAuthorityStatus, error) {
	access, err := s.workspaceAuthority(device.WorkspaceID)
	if err != nil {
		return deviceAuthorityStatus{}, err
	}
	return deviceAuthorityStatus{
		Workspace: access,
		Device:    device,
		State:     classifyLockState(device.LockHash, access.Hash),
	}, nil
}

// deviceAuthorityAlert renders the out-of-sync warning on a device's page only when
// that specific Node has not yet accepted the current workspace access.
func deviceAuthorityAlert(status deviceAuthorityStatus, role string) string {
	if status.Device == nil || status.State == LockSynced {
		return ""
	}
	return authorityAlert(status.Workspace, role, true)
}

// authorityAlert renders the out-of-sync warning. Owners get the sync action when withAction is set;
// other roles get guidance because only an Owner can sign an RC Lock transition.
func authorityAlert(status authorityStatus, role string, withAction bool) string {
	if len(status.Pending) == 0 {
		return ""
	}
	action := ""
	switch {
	case role == "owner" && withAction:
		action = `<button class="or-button" type="button" data-authority-sync="` + escape(status.WorkspaceID) + `">SYNC DEVICE ACCESS</button>`
	case role != "owner":
		action = `<p class="meta">An Owner must run Sync device access.</p>`
	}
	return `<div class="authority-alert" role="alert" data-authority-alert="` + escape(status.WorkspaceID) + `">` +
		`<div class="authority-alert-body"><p class="alert-title">RC Lock out of sync</p>` +
		`<p>Not yet accepted by <span data-authority-pending>` + escape(pendingNodeSummary(status.Pending)) + `</span>. ` +
		`Remote commands and MCP are refused on these Nodes until they accept the current workspace access. Offline Nodes accept it when they reconnect.</p>` +
		`<p class="meta" data-authority-status="` + escape(status.WorkspaceID) + `"></p></div>` + action + `</div>`
}

// writeAuthorityState reports how far each Node in the workspace has accepted its current RC Lock.
// The browser uses it to decide whether a sync has settled; it never carries authority.
func (s *Server) writeAuthorityState(w http.ResponseWriter, workspaceID string) {
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
		state := classifyLockState(device.LockHash, hash)
		if state == LockSynced {
			synced++
		}
		states = append(states, map[string]any{
			"id": device.ID, "name": device.Name, "synced": state == LockSynced,
			"lockState": state, "online": s.nodes.Online(device.ID),
		})
		if state == LockStale {
			parents[authorityParentKey(device.LockHash, device.LockGeneration)] = map[string]any{
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
			parents[authorityParentKey(hashValue, generation)] = map[string]any{
				"hash": hashValue, "generation": generation,
			}
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

// authorityAcceptTimeout bounds how long a sync request waits for online Nodes to acknowledge the new lock.
// Offline Nodes are not waited for; they apply the stored transition when they reconnect.
const authorityAcceptTimeout = 10 * time.Second

// awaitAuthority waits until every online Node in the workspace reports digest, then logs which Nodes are
// still pending so a failed sync leaves a server-side record.
func (s *Server) awaitAuthority(workspaceID, digest string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for s.onlineAuthorityPending(workspaceID, digest) > 0 && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	var pending []string
	offline := 0
	for _, device := range s.registry.List() {
		if device.WorkspaceID != workspaceID || device.LockHash == digest {
			continue
		}
		if s.nodes.Online(device.ID) {
			pending = append(pending, device.Name)
		} else {
			offline++
		}
	}
	log.Printf("authority sync workspace=%s hash=%s online_pending=%v offline_pending=%d",
		workspaceID, digest[:12], pending, offline)
}

// onlineAuthorityPending counts online Nodes in the workspace that have not reported digest.
func (s *Server) onlineAuthorityPending(workspaceID, digest string) int {
	count := 0
	for _, device := range s.registry.List() {
		if device.WorkspaceID == workspaceID && device.LockHash != digest && s.nodes.Online(device.ID) {
			count++
		}
	}
	return count
}
