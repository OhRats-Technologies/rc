package server

import (
	"strings"

	authstore "github.com/OhRats-Technologies/rc/pkg/auth"
)

func (h *McpHandler) callMachinesList(grant *authstore.OAuthGrant) map[string]any {
	devices := h.registry.List()
	machines := make([]map[string]any, 0, len(devices))
	var text strings.Builder
	for _, device := range devices {
		if !h.deviceVisible(grant.UserID, device.WorkspaceID) {
			continue
		}
		active := h.activeProcesses(device.ID)
		machines = append(machines, map[string]any{
			"id":              device.ID,
			"name":            device.Name,
			"workspaceId":     device.WorkspaceID,
			"hostname":        device.Hostname,
			"platform":        device.Platform,
			"arch":            device.Arch,
			"nodeVersion":     device.Version,
			"online":          device.Online,
			"activeProcesses": active,
		})
		if text.Len() > 0 {
			text.WriteByte('\n')
		}
		state := "offline"
		if device.Online {
			state = "online"
		}
		text.WriteString(device.Name + " — " + state + " — " + device.Platform + "/" + device.Arch +
			" — node " + device.Version + " — id " + device.ID)
	}
	if len(machines) == 0 {
		text.WriteString("No machines are available.")
	}
	return mcpToolText(text.String(), map[string]any{"machines": machines}, false)
}
