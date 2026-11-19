package server

import (
	"sort"
	"strings"

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
		`<p>` + escape(pendingNodeSummary(status.Pending)) + ` has not accepted the current workspace access. ` +
		`Remote commands and MCP are refused on these Nodes until they accept it. Offline Nodes accept it when they reconnect.</p>` +
		`<p class="meta" data-authority-status="` + escape(status.WorkspaceID) + `"></p></div>` + action + `</div>`
}
