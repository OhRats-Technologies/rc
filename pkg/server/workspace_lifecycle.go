package server

import (
	"errors"
	"net/http"
	"strings"

	authstore "github.com/OhRats-Technologies/rc/pkg/auth"
)

func (s *Server) handleWorkspaceList(w http.ResponseWriter, req *http.Request) {
	user, ok := s.currentWorkspaceUser(w, req)
	if !ok {
		return
	}
	workspaces, err := s.auth.WorkspacesForUser(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "workspaces could not be loaded")
		return
	}
	rows := make([]map[string]any, 0, len(workspaces))
	for _, workspace := range workspaces {
		rows = append(rows, map[string]any{"id": workspace.ID, "name": workspace.Name, "role": workspace.Role})
	}
	writeJSON(w, http.StatusOK, map[string]any{"workspaces": rows})
}

func (s *Server) handleWorkspaceCreate(w http.ResponseWriter, req *http.Request) {
	user, ok := s.currentWorkspaceUser(w, req)
	if !ok {
		return
	}
	var input struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(req, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	workspace, err := s.auth.CreateWorkspace(input.Name, user.ID)
	if errors.Is(err, authstore.ErrWorkspaceLimit) {
		writeError(w, http.StatusConflict, "workspace limit reached (10)")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.publishAudit(workspace.ID, user.ID, "", "workspace.created", map[string]any{"name": workspace.Name})
	writeJSON(w, http.StatusCreated, map[string]any{"id": workspace.ID, "name": workspace.Name})
}

func (s *Server) handleWorkspaceCreateForm(w http.ResponseWriter, req *http.Request) {
	user, ok := s.currentWorkspaceUser(w, req)
	if !ok {
		return
	}
	workspace, err := s.auth.CreateWorkspace(req.FormValue("name"), user.ID)
	if err != nil {
		http.Redirect(w, req, "/devices", http.StatusSeeOther)
		return
	}
	s.publishAudit(workspace.ID, user.ID, "", "workspace.created", map[string]any{"name": workspace.Name})
	http.Redirect(w, req, "/devices", http.StatusSeeOther)
}

func (s *Server) handleWorkspaceDetail(w http.ResponseWriter, req *http.Request) {
	user, ok := s.currentWorkspaceUser(w, req)
	if !ok {
		return
	}
	workspaceID := trimWorkspaceID(req.PathValue("id"))
	if workspaceID == "" {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}
	role, err := s.auth.WorkspaceRole(user.ID, workspaceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}
	workspace, err := s.auth.WorkspaceByID(workspaceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}
	devices := make([]map[string]any, 0)
	for _, device := range s.visibleDevices(user.ID) {
		if device.WorkspaceID == workspaceID {
			devices = append(devices, map[string]any{
				"id": device.ID, "name": device.Name, "workspaceId": device.WorkspaceID,
				"hostname": device.Hostname, "platform": device.Platform, "arch": device.Arch,
				"online": device.Online,
			})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"workspace": map[string]any{"id": workspace.ID, "name": workspace.Name, "role": role},
		"devices":   devices,
	})
}

func (s *Server) handleWorkspaceRename(w http.ResponseWriter, req *http.Request) {
	user, ok := s.currentWorkspaceUser(w, req)
	if !ok {
		return
	}
	workspaceID := trimWorkspaceID(req.PathValue("id"))
	if workspaceID == "" || !s.requireOwner(w, user.ID, workspaceID) {
		return
	}
	var input struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(req, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if err := s.auth.RenameWorkspace(workspaceID, input.Name); err != nil {
		writeError(w, http.StatusBadRequest, "workspace could not be renamed")
		return
	}
	s.publishAudit(workspaceID, user.ID, "", "workspace.renamed", map[string]any{"name": strings.TrimSpace(input.Name)})
	writeJSON(w, http.StatusOK, map[string]string{"name": strings.TrimSpace(input.Name)})
}

func (s *Server) handleWorkspaceRenameForm(w http.ResponseWriter, req *http.Request) {
	user, ok := s.currentWorkspaceUser(w, req)
	if !ok {
		return
	}
	workspaceID := trimWorkspaceID(req.PathValue("id"))
	if workspaceID == "" || !s.requireOwner(w, user.ID, workspaceID) {
		return
	}
	name := strings.TrimSpace(req.FormValue("name"))
	if err := s.auth.RenameWorkspace(workspaceID, name); err != nil {
		writeError(w, http.StatusBadRequest, "workspace could not be renamed")
		return
	}
	s.publishAudit(workspaceID, user.ID, "", "workspace.renamed", map[string]any{"name": name})
	http.Redirect(w, req, "/devices", http.StatusSeeOther)
}

func (s *Server) handleWorkspaceDelete(w http.ResponseWriter, req *http.Request) {
	user, ok := s.currentWorkspaceUser(w, req)
	if !ok {
		return
	}
	workspaceID := trimWorkspaceID(req.PathValue("id"))
	if workspaceID == "" || !s.requireOwner(w, user.ID, workspaceID) {
		return
	}
	for _, device := range s.registry.List() {
		if device.WorkspaceID == workspaceID {
			_ = s.registry.remove(device.ID)
		}
	}
	if err := s.auth.DeleteWorkspace(workspaceID); err != nil {
		writeError(w, http.StatusInternalServerError, "workspace could not be deleted")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleWorkspaceLeave(w http.ResponseWriter, req *http.Request) {
	user, ok := s.currentWorkspaceUser(w, req)
	if !ok {
		return
	}
	workspaceID := trimWorkspaceID(req.PathValue("id"))
	if workspaceID == "" {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}
	role, err := s.auth.LeaveWorkspace(workspaceID, user.ID)
	if errors.Is(err, authstore.ErrLastWorkspaceOwner) {
		writeError(w, http.StatusConflict, "promote another owner before leaving")
		return
	}
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}
	s.publishAudit(workspaceID, user.ID, "", "workspace.member.left", map[string]any{"role": role})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleWorkspaceLeaveForm(w http.ResponseWriter, req *http.Request) {
	user, ok := s.currentWorkspaceUser(w, req)
	if !ok {
		return
	}
	workspaceID := trimWorkspaceID(req.PathValue("id"))
	if workspaceID == "" {
		http.Redirect(w, req, "/devices", http.StatusSeeOther)
		return
	}
	if _, err := s.auth.LeaveWorkspace(workspaceID, user.ID); err != nil {
		http.Redirect(w, req, "/devices", http.StatusSeeOther)
		return
	}
	http.Redirect(w, req, "/devices", http.StatusSeeOther)
}
