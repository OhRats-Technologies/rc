package server

import (
	"errors"
	"net/http"

	authstore "github.com/OhRats-Technologies/rc/pkg/auth"
)

func (s *Server) handleWorkspaceAccess(w http.ResponseWriter, req *http.Request) {
	user, ok := s.currentWorkspaceUser(w, req)
	if !ok {
		return
	}
	workspaceID := trimWorkspaceID(req.PathValue("id"))
	if workspaceID == "" || !s.requireOwner(w, user.ID, workspaceID) {
		return
	}
	members, err := s.auth.WorkspaceMemberDetails(workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "workspace access could not be loaded")
		return
	}
	invites, err := s.auth.ListWorkspaceInvites(workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "workspace access could not be loaded")
		return
	}
	memberRows := make([]map[string]any, 0, len(members))
	for _, member := range members {
		memberRows = append(memberRows, map[string]any{
			"userId": member.UserID, "name": member.Name, "role": member.Role,
			"joinedAt": member.JoinedAt.UnixMilli(),
		})
	}
	inviteRows := make([]map[string]any, 0, len(invites))
	for _, invite := range invites {
		inviteRows = append(inviteRows, map[string]any{
			"id": invite.ID, "role": invite.Role,
			"createdAt": invite.CreatedAt.UnixMilli(), "expiresAt": invite.ExpiresAt.UnixMilli(),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"members": memberRows, "invites": inviteRows})
}

func (s *Server) handleWorkspaceMemberRole(w http.ResponseWriter, req *http.Request) {
	user, ok := s.currentWorkspaceUser(w, req)
	if !ok {
		return
	}
	workspaceID := trimWorkspaceID(req.PathValue("id"))
	memberID := trimWorkspaceID(req.PathValue("member"))
	if workspaceID == "" || memberID == "" || !s.requireOwner(w, user.ID, workspaceID) {
		return
	}
	if err := s.consumeStepUp(req, user.ID); err != nil {
		writeError(w, http.StatusUnauthorized, "fresh passkey verification required")
		return
	}
	var input struct {
		Role string `json:"role"`
	}
	if err := decodeJSON(req, &input); err != nil || !validRole(input.Role) {
		writeError(w, http.StatusBadRequest, "invalid role")
		return
	}
	previous, err := s.auth.UpdateWorkspaceMemberRole(workspaceID, memberID, input.Role)
	switch {
	case errors.Is(err, authstore.ErrWorkspaceMemberNotFound):
		writeError(w, http.StatusNotFound, "member not found")
		return
	case errors.Is(err, authstore.ErrLastWorkspaceOwner):
		writeError(w, http.StatusConflict, "promote another owner first")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "member role could not be changed")
		return
	}
	name := memberName(s, memberID)
	s.publishAudit(workspaceID, user.ID, "", "workspace.member.role", map[string]any{
		"memberId": memberID, "name": name, "from": previous, "role": input.Role,
	})
	writeJSON(w, http.StatusOK, map[string]string{"role": input.Role})
}

func (s *Server) handleWorkspaceMemberRemove(w http.ResponseWriter, req *http.Request) {
	user, ok := s.currentWorkspaceUser(w, req)
	if !ok {
		return
	}
	workspaceID := trimWorkspaceID(req.PathValue("id"))
	memberID := trimWorkspaceID(req.PathValue("member"))
	if workspaceID == "" || memberID == "" || !s.requireOwner(w, user.ID, workspaceID) {
		return
	}
	if memberID == user.ID {
		writeError(w, http.StatusConflict, "use Leave workspace for your own account")
		return
	}
	if err := s.consumeStepUp(req, user.ID); err != nil {
		writeError(w, http.StatusUnauthorized, "fresh passkey verification required")
		return
	}
	role, err := s.auth.RemoveWorkspaceMember(workspaceID, memberID)
	switch {
	case errors.Is(err, authstore.ErrWorkspaceMemberNotFound):
		writeError(w, http.StatusNotFound, "member not found")
		return
	case errors.Is(err, authstore.ErrLastWorkspaceOwner):
		writeError(w, http.StatusConflict, "workspace needs an owner")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "member could not be removed")
		return
	}
	s.publishAudit(workspaceID, user.ID, "", "workspace.member.removed", map[string]any{
		"memberId": memberID, "name": memberName(s, memberID), "role": role,
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func memberName(s *Server, userID string) string {
	user, err := s.auth.GetUser(userID)
	if err != nil || user == nil {
		return ""
	}
	return user.Name
}
