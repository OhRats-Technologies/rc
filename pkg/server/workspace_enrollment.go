package server

import (
	"net/http"
	"strings"
	"time"
)

func (s *Server) handleCreateEnrollment(w http.ResponseWriter, req *http.Request) {
	user, _, err := s.currentSession(req)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	workspaceID := req.PathValue("id")
	if workspaceID == "" || len(workspaceID) > 128 {
		writeError(w, http.StatusBadRequest, "invalid workspace")
		return
	}
	if s.auth == nil {
		writeError(w, http.StatusServiceUnavailable, "account store is unavailable")
		return
	}
	role, err := s.auth.WorkspaceRole(user.ID, workspaceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}
	if role != "owner" {
		writeError(w, http.StatusForbidden, "owner access required")
		return
	}
	if workspaceID == defaultWorkspaceID {
		if err := s.auth.EnsureWorkspace(defaultWorkspaceID, "Default"); err != nil {
			writeError(w, http.StatusInternalServerError, "workspace could not be created")
			return
		}
	}
	token, expires, err := s.auth.CreateEnrollmentToken(workspaceID, 15*time.Minute)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	server := strings.TrimRight(s.cfg.PublicURL, "/")
	writeJSON(w, http.StatusCreated, map[string]any{
		"token":          token,
		"expiresAt":      expires.UTC(),
		"install":        installCommand(server, token),
		"enroll":         enrollCommand(server, token),
		"installWindows": windowsInstallCommand(server, token),
		"enrollWindows":  windowsEnrollCommand(server, token),
	})
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func powershellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func installCommand(server, token string) string {
	script := server + "/install.sh"
	return "curl -fsSL " + shellQuote(script) + " | sh -s -- " + shellQuote(token) + " " + shellQuote(server)
}

func enrollCommand(server, token string) string {
	return "rc enroll " + shellQuote(token) + " --url " + shellQuote(server) + " && rc service install"
}

func windowsInstallCommand(server, token string) string {
	return "& ([scriptblock]::Create((Invoke-RestMethod -Uri " + powershellQuote(server+"/install.ps1") +
		"))) -Token " + powershellQuote(token) + " -Server " + powershellQuote(server)
}

func windowsEnrollCommand(server, token string) string {
	return "$rc = Join-Path $env:LOCALAPPDATA 'OhRats\\RC\\bin\\rc.exe'; & $rc enroll " +
		powershellQuote(token) + " --url " + powershellQuote(server) +
		"; if ($LASTEXITCODE -eq 0) { & $rc service install }"
}
