package server

import (
	"errors"

	authstore "github.com/OhRats-Technologies/rc/pkg/auth"
)

var (
	errMCPDeviceNotFound = errors.New("device not found")
	errMCPOwnerRequired  = errors.New("Owner access is required for this machine")
)

// authorizeDevice evaluates the current workspace membership of the MCP
// principal. Terminal operations require an Owner; observe operations accept
// any current membership. A missing device and a device outside the principal's
// workspaces intentionally share one error so MCP does not disclose machines.
func (h *McpHandler) authorizeDevice(userID, deviceID string, requireOwner bool) error {
	device, ok := h.registry.Get(deviceID)
	if !ok {
		return errMCPDeviceNotFound
	}
	if h.local || h.auth == nil {
		return nil
	}
	role, err := h.auth.WorkspaceRole(userID, device.WorkspaceID)
	if err != nil {
		return errMCPDeviceNotFound
	}
	if requireOwner && role != "owner" {
		return errMCPOwnerRequired
	}
	return nil
}

func (h *McpHandler) deviceVisible(userID, workspaceID string) bool {
	if h.local || h.auth == nil {
		return true
	}
	_, err := h.auth.WorkspaceRole(userID, workspaceID)
	return err == nil
}

// grantUsable revalidates the durable grant on every request so an expired or
// revoked grant cannot keep working through an unexpired OAuth access token.
func (h *McpHandler) grantUsable(grant *authstore.OAuthGrant) bool {
	if h.local || h.auth == nil {
		return true
	}
	if grant == nil || grant.GrantID == "" {
		return false
	}
	_, err := h.auth.McpGrant(grant.UserID, grant.GrantID)
	return err == nil
}
