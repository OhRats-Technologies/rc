package server

import (
	"errors"
	"net/http"
	"strings"
	"time"

	authstore "github.com/OhRats-Technologies/rc/pkg/auth"
	"github.com/OhRats-Technologies/rc/pkg/node"
)

const maxScheduleRuntimeMs = 30 * 24 * 60 * 60 * 1000

type scheduleGrantInput struct {
	ClientID     string `json:"clientId"`
	DeviceID     string `json:"deviceId"`
	SpecHash     string `json:"specHash"`
	MaxRuntimeMs uint64 `json:"maxRuntimeMs"`
	ExpiresAt    int64  `json:"expiresAt"`
}

func (s *Server) handleScheduleGrantUpsert(w http.ResponseWriter, req *http.Request) {
	user, _, err := s.currentSession(req)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	workspaceID := req.PathValue("id")
	if !s.scheduleOwner(w, user.ID, workspaceID) {
		return
	}
	var input scheduleGrantInput
	if err := decodeJSON(req, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	scheduleID := strings.TrimSpace(req.PathValue("schedule"))
	if err := validScheduleGrant(scheduleID, input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := s.freshControlProof(user.ID, input.ClientID, 5*time.Minute); err != nil {
		writeError(w, http.StatusUnauthorized, "fresh passkey-backed control authorization required")
		return
	}
	expires := time.Time{}
	if input.ExpiresAt != 0 {
		expires = time.UnixMilli(input.ExpiresAt).UTC()
	}
	err = s.auth.PutScheduleGrant(authstore.ScheduleGrant{
		ScheduleID: scheduleID, WorkspaceID: workspaceID, DeviceID: input.DeviceID,
		UserID: user.ID, SpecHash: strings.ToLower(input.SpecHash),
		MaxRuntimeMs: input.MaxRuntimeMs, ExpiresAt: expires, CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		if errors.Is(err, authstore.ErrScheduleGrantNotFound) {
			writeError(w, http.StatusNotFound, "device not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "schedule authority could not be stored")
		return
	}
	s.writeScheduleAuthority(w, workspaceID, scheduleID)
}

func (s *Server) handleScheduleGrantDelete(w http.ResponseWriter, req *http.Request) {
	user, _, err := s.currentSession(req)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	workspaceID := req.PathValue("id")
	if !s.scheduleOwner(w, user.ID, workspaceID) {
		return
	}
	var input struct {
		ClientID string `json:"clientId"`
	}
	if err := decodeJSON(req, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	scheduleID := strings.TrimSpace(req.PathValue("schedule"))
	if scheduleID == "" || len(scheduleID) > 128 {
		writeError(w, http.StatusBadRequest, "invalid schedule authority permit")
		return
	}
	if _, err := s.controlProof(user.ID, input.ClientID); err != nil {
		writeError(w, http.StatusUnauthorized, "passkey-backed control authorization required")
		return
	}
	removed, err := s.auth.DeleteScheduleGrant(workspaceID, scheduleID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "schedule authority could not be removed")
		return
	}
	if !removed {
		writeError(w, http.StatusNotFound, "schedule authority not found")
		return
	}
	s.writeScheduleAuthority(w, workspaceID, scheduleID)
}

func (s *Server) scheduleOwner(w http.ResponseWriter, userID, workspaceID string) bool {
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

func (s *Server) writeScheduleAuthority(w http.ResponseWriter, workspaceID, scheduleID string) {
	snapshot, err := s.authoritySnapshot(workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "authority snapshot failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "scheduleId": scheduleID, "workspaceId": workspaceID,
		"lockHash": node.SnapshotHash(snapshot),
	})
}

func validScheduleGrant(scheduleID string, input scheduleGrantInput) error {
	if scheduleID == "" || len(scheduleID) > 128 ||
		strings.TrimSpace(input.DeviceID) == "" || !validAuthorityHash(input.SpecHash) ||
		input.MaxRuntimeMs == 0 || input.MaxRuntimeMs > maxScheduleRuntimeMs ||
		input.ExpiresAt < 0 ||
		(input.ExpiresAt != 0 && input.ExpiresAt <= time.Now().UnixMilli()) {
		return errors.New("invalid schedule authority permit")
	}
	return nil
}
