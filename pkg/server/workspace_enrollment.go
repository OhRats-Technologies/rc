package server

import (
	"net/http"
	"strings"
	"time"
)

func (s *Server) handleCreateEnrollment(w http.ResponseWriter, req *http.Request) {
	workspaceID := req.PathValue("id")
	if workspaceID == "" || len(workspaceID) > 128 {
		writeError(w, http.StatusBadRequest, "invalid workspace")
		return
	}
	if s.auth == nil {
		writeError(w, http.StatusServiceUnavailable, "account store is unavailable")
		return
	}
	if !developmentAccessFrom(req) {
		userID, ok := requestUserID(req)
		if !ok {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		role, err := s.auth.WorkspaceRole(userID, workspaceID)
		if err != nil {
			writeError(w, http.StatusNotFound, "workspace not found")
			return
		}
		if role != "owner" {
			writeError(w, http.StatusForbidden, "owner access required")
			return
		}
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
		"installAndroid": androidInstallCommand(server, token),
		"enrollAndroid":  androidEnrollCommand(server, token),
	})
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

// deviceNamePlaceholder is the literal the operator replaces before running
// the command. Enrollment otherwise falls back to the OS hostname, which is
// rarely the name the device should carry in the sidebar.
const deviceNamePlaceholder = "device-name"

func powershellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func installCommand(server, token string) string {
	script := server + "/install.sh"
	return "curl -fsSL " + shellQuote(script) + " | sh -s -- " + shellQuote(token) + " " + shellQuote(server) + " " + shellQuote(deviceNamePlaceholder)
}

func enrollCommand(server, token string) string {
	return "rc enroll " + shellQuote(token) + " --url " + shellQuote(server) + " --name " + shellQuote(deviceNamePlaceholder) + " && rc service install"
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

// androidInstallCommand enrolls an Android device through Termux and proot.
// The native kernel links Wasmtime through cgo and RC has no Android service
// manager, so rc runs as a Linux/arm64 binary inside a Debian rootfs. The
// script is idempotent: it reuses an existing rootfs and skips a live node.
func androidInstallCommand(server, token string) string {
	return "sh -c \"$(curl -fsSL " + shellQuote(server+"/android-enroll.sh") + ")\" -- " +
		shellQuote(token) + " " + shellQuote(server) + " " + shellQuote(deviceNamePlaceholder)
}

// androidEnrollCommand enrolls inside an existing proot rootfs. Termux UIDs
// are VPN-locked on locked-down devices, so enrollment runs as root.
func androidEnrollCommand(server, token string) string {
	return "su -c \"" + androidProotPrefix + " /root/rc enroll " + token +
		" --url " + server + " --name " + deviceNamePlaceholder + "\""
}

const androidProotPrefix = "export PROOT_LOADER=/data/data/com.termux/files/usr/libexec/proot/loader HOME=/root; " +
	"/data/data/com.termux/files/usr/bin/proot -r /data/data/com.termux/files/home/debian " +
	"-b /dev -b /proc -b /sys -b /sdcard -w /root"
