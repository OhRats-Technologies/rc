package server

import (
	"net/http"

	authstore "github.com/OhRats-Technologies/rc/pkg/auth"
)

func (s *Server) visibleDevices(userID string) []*RegisteredDevice {
	devices := s.registry.List()
	if s.auth == nil || userID == "" {
		return devices
	}
	visible := make([]*RegisteredDevice, 0, len(devices))
	for _, device := range devices {
		if _, err := s.auth.WorkspaceRole(userID, device.WorkspaceID); err == nil {
			visible = append(visible, device)
		}
	}
	return visible
}

// requestRole resolves the workspace role of the authenticated API key or
// browser session attached by allowSessionOrAPI.
func (r *DeviceRegistry) requestRole(req *http.Request, workspaceID string) (string, bool) {
	if r.store == nil || developmentAccessFrom(req) {
		return "owner", true
	}
	userID := ""
	if user := sessionUserFrom(req); user != nil {
		userID = user.ID
	} else if principal := apiPrincipalFrom(req); principal != nil {
		userID = principal.UserID
	}
	if userID == "" {
		return "", false
	}
	role, err := r.store.WorkspaceRole(userID, workspaceID)
	if err != nil {
		return "", false
	}
	return role, true
}

func (r *DeviceRegistry) requestDevices(req *http.Request) ([]*RegisteredDevice, bool) {
	if r.store == nil || developmentAccessFrom(req) {
		return r.List(), true
	}
	userID := ""
	if user := sessionUserFrom(req); user != nil {
		userID = user.ID
	} else if principal := apiPrincipalFrom(req); principal != nil {
		userID = principal.UserID
	}
	if userID == "" {
		return nil, false
	}
	return newVisibleDevices(r.store, userID, r.List()), true
}

func newVisibleDevices(store *authstore.Store, userID string, devices []*RegisteredDevice) []*RegisteredDevice {
	visible := make([]*RegisteredDevice, 0, len(devices))
	for _, device := range devices {
		if _, err := store.WorkspaceRole(userID, device.WorkspaceID); err == nil {
			visible = append(visible, device)
		}
	}
	return visible
}
