package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	authstore "github.com/OhRats-Technologies/rc/pkg/auth"
)

// currentWorkspaceUser resolves a browser session and returns the user.
func (s *Server) currentWorkspaceUser(w http.ResponseWriter, req *http.Request) (*authstore.User, bool) {
	user, _, err := s.currentSession(req)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return nil, false
	}
	return user, true
}

// requireOwner confirms that the user is an Owner of the workspace.
func (s *Server) requireOwner(w http.ResponseWriter, userID, workspaceID string) bool {
	role, err := s.auth.WorkspaceRole(userID, workspaceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return false
	}
	if role != "owner" {
		writeError(w, http.StatusForbidden, "owner required")
		return false
	}
	return true
}

// publishAudit persists bounded workspace metadata and publishes the live
// event shape consumed by the browser SSE client.
func (s *Server) publishAudit(workspaceID, userID, deviceID, kind string, detail map[string]any) {
	raw, err := json.Marshal(detail)
	if err != nil {
		return
	}
	if s.auth != nil {
		_ = s.auth.RecordAuditEvent(authstore.AuditEvent{
			WorkspaceID: workspaceID, UserID: userID, DeviceID: deviceID,
			Kind: kind, Detail: string(raw),
		})
	}
	s.events.Publish(kind, map[string]any{
		"workspaceId": workspaceID, "userId": userID, "deviceId": deviceID,
		"audit": true, "detail": detail, "at": time.Now().UnixMilli(),
	})
}

func validRole(role string) bool {
	return role == "owner" || role == "operator" || role == "viewer"
}

func trimWorkspaceID(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 128 {
		return ""
	}
	return value
}
