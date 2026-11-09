package server

import "net/http"

func (s *Server) registerWorkspaceRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/workspaces", s.handleWorkspaceList)
	mux.HandleFunc("POST /api/v1/workspaces", s.handleWorkspaceCreate)
	mux.HandleFunc("GET /api/v1/workspaces/{id}", s.handleWorkspaceDetail)
	mux.HandleFunc("PATCH /api/v1/workspaces/{id}", s.handleWorkspaceRename)
	mux.HandleFunc("DELETE /api/v1/workspaces/{id}", s.handleWorkspaceDelete)
	mux.HandleFunc("POST /api/v1/workspaces/{id}/leave", s.handleWorkspaceLeave)
	mux.HandleFunc("POST /workspaces", s.handleWorkspaceCreateForm)
	mux.HandleFunc("POST /workspaces/{id}/rename", s.handleWorkspaceRenameForm)
	mux.HandleFunc("POST /workspaces/{id}/leave", s.handleWorkspaceLeaveForm)
	mux.Handle("POST /api/v1/workspaces/{id}/enrollments", s.allowSessionOrAPI(http.HandlerFunc(s.handleCreateEnrollment)))
	mux.HandleFunc("GET /api/v1/workspaces/{id}/authority", s.handleAuthorityStatus)
	mux.HandleFunc("POST /api/v1/workspaces/{id}/authority/sync", s.handleAuthoritySync)
	mux.HandleFunc("PUT /api/v1/workspaces/{id}/schedule-grants/{schedule}", s.handleScheduleGrantUpsert)
	mux.HandleFunc("DELETE /api/v1/workspaces/{id}/schedule-grants/{schedule}", s.handleScheduleGrantDelete)
	mux.HandleFunc("GET /api/v1/workspaces/{id}/access", s.handleWorkspaceAccess)
	mux.HandleFunc("PATCH /api/v1/workspaces/{id}/members/{member}", s.handleWorkspaceMemberRole)
	mux.HandleFunc("DELETE /api/v1/workspaces/{id}/members/{member}", s.handleWorkspaceMemberRemove)
	mux.HandleFunc("POST /api/v1/workspaces/{id}/invites", s.handleWorkspaceInviteCreate)
	mux.HandleFunc("DELETE /api/v1/workspaces/{id}/invites/{invite}", s.handleWorkspaceInviteRemove)
	mux.HandleFunc("POST /api/v1/workspaces/join", s.handleWorkspaceJoin)
	mux.Handle("GET /api/v1/workspaces/{id}/activity", s.allowSessionOrAPI(http.HandlerFunc(s.handleWorkspaceActivityAPI)))
	mux.HandleFunc("GET /workspaces/{id}/access", s.handleWorkspaceAccessPage)
	mux.HandleFunc("GET /workspaces/{id}/activity", s.handleWorkspaceActivityPage)
	mux.HandleFunc("GET /join", s.handleJoinPage)
}
