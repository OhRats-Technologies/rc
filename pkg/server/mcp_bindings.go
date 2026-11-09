package server

import authstore "github.com/OhRats-Technologies/rc/pkg/auth"

func (h *McpHandler) bindProcess(id string, grant *authstore.OAuthGrant, deviceID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.processes[id] = mcpProcessBinding{
		DeviceID: deviceID, UserID: grant.UserID, ClientID: grant.ClientID, GrantID: grant.GrantID,
	}
}

func (h *McpHandler) processBinding(id, deviceID string, grant *authstore.OAuthGrant) (mcpProcessBinding, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	binding, ok := h.processes[id]
	if !ok || binding.DeviceID != deviceID {
		return mcpProcessBinding{}, false
	}
	if binding.UserID != grant.UserID || binding.ClientID != grant.ClientID {
		return mcpProcessBinding{}, false
	}
	return binding, true
}

// releaseGrant drops process bindings owned by a revoked MCP grant so a new
// grant cannot inherit them and hosted bookkeeping does not grow forever.
func (h *McpHandler) releaseGrant(grantID string) {
	if grantID == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, binding := range h.processes {
		if binding.GrantID == grantID {
			delete(h.processes, id)
		}
	}
}

// releaseUser drops bindings for an account after deletion or workspace loss.
func (h *McpHandler) releaseUser(userID string) {
	if userID == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, binding := range h.processes {
		if binding.UserID == userID {
			delete(h.processes, id)
		}
	}
}

func (h *McpHandler) activeProcesses(deviceID string) int {
	if h.exec == nil {
		return 0
	}
	h.mu.Lock()
	ids := make([]string, 0)
	for id, binding := range h.processes {
		if binding.DeviceID == deviceID {
			ids = append(ids, id)
		}
	}
	h.mu.Unlock()
	if !h.local || h.exec == nil {
		return len(ids)
	}
	active := 0
	for _, id := range ids {
		if read, ok := h.exec.ExecutionRead(id, 0, 1); ok && read.Status == "running" {
			active++
		}
	}
	return active
}
