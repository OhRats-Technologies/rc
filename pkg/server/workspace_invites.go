package server

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	authstore "github.com/OhRats-Technologies/rc/pkg/auth"
	"github.com/OhRats-Technologies/rc/pkg/version"

	"github.com/OhRats-Technologies/rc/pkg/server/pages"
)

const workspaceInviteLifetime = 4 * time.Hour

func (s *Server) handleWorkspaceInviteCreate(w http.ResponseWriter, req *http.Request) {
	user, ok := s.currentWorkspaceUser(w, req)
	if !ok {
		return
	}
	workspaceID := trimWorkspaceID(req.PathValue("id"))
	if workspaceID == "" || !s.requireOwner(w, user.ID, workspaceID) {
		return
	}
	var input struct {
		Role string `json:"role"`
	}
	if err := decodeJSON(req, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	role := input.Role
	if role != "viewer" {
		role = "operator"
	}
	id, token, err := s.auth.CreateWorkspaceInvite(workspaceID, user.ID, role, time.Now().Add(workspaceInviteLifetime))
	if errors.Is(err, authstore.ErrWorkspaceInviteLimit) {
		writeError(w, http.StatusConflict, "pending invite limit reached (25)")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "invite could not be created")
		return
	}
	s.publishAudit(workspaceID, user.ID, "", "workspace.invite.created", map[string]any{
		"inviteId": id, "role": role,
	})
	base := strings.TrimRight(s.cfg.PublicURL, "/")
	if base == "" {
		base = "http://" + req.Host
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"token": token, "url": base + "/join?invite=" + url.QueryEscape(token),
		"expiresAt": time.Now().Add(workspaceInviteLifetime).UnixMilli(),
	})
}

func (s *Server) handleWorkspaceInviteRemove(w http.ResponseWriter, req *http.Request) {
	user, ok := s.currentWorkspaceUser(w, req)
	if !ok {
		return
	}
	workspaceID := trimWorkspaceID(req.PathValue("id"))
	inviteID := trimWorkspaceID(req.PathValue("invite"))
	if workspaceID == "" || inviteID == "" || !s.requireOwner(w, user.ID, workspaceID) {
		return
	}
	removed, err := s.auth.RevokeWorkspaceInvite(workspaceID, inviteID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "invite could not be revoked")
		return
	}
	if !removed {
		writeError(w, http.StatusNotFound, "invite not found")
		return
	}
	s.publishAudit(workspaceID, user.ID, "", "workspace.invite.revoked", map[string]any{"inviteId": inviteID})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleWorkspaceJoin(w http.ResponseWriter, req *http.Request) {
	user, ok := s.currentWorkspaceUser(w, req)
	if !ok {
		return
	}
	var input struct {
		Token string `json:"token"`
	}
	if err := decodeJSON(req, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid invite")
		return
	}
	token := strings.TrimSpace(input.Token)
	if token == "" {
		writeError(w, http.StatusBadRequest, "invalid invite")
		return
	}
	workspaceID, role, err := s.auth.ConsumeWorkspaceInvite(token, user.ID)
	if errors.Is(err, authstore.ErrWorkspaceInviteNotFound) {
		writeError(w, http.StatusUnauthorized, "invalid or expired invite")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "invite could not be accepted")
		return
	}
	s.publishAudit(workspaceID, user.ID, "", "workspace.member.joined", map[string]any{"role": role})
	writeJSON(w, http.StatusOK, map[string]string{"workspaceId": workspaceID})
}

func (s *Server) handleJoinPage(w http.ResponseWriter, req *http.Request) {
	_, _, err := s.currentSession(req)
	token := strings.TrimSpace(req.URL.Query().Get("invite"))
	if err != nil {
		next := req.URL.RequestURI()
		http.Redirect(w, req, "/login?next="+url.QueryEscape(next), http.StatusSeeOther)
		return
	}
	options := pages.Options{PublicURL: s.cfg.PublicURL, Version: version.Version, Invite: token}
	if token == "" || !s.auth.WorkspaceInviteValid(token) {
		s.writePage(w, http.StatusOK, pages.Auth(pages.InvalidInvite, options), "no-store")
		return
	}
	s.writePage(w, http.StatusOK, pages.Auth(pages.Join, options), "no-store")
}
