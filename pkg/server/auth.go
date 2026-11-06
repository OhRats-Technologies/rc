package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	authstore "github.com/OhRats-Technologies/rc/pkg/auth"
	"github.com/OhRats-Technologies/rc/pkg/auth/webauthn"
)

const (
	sessionCookieName = "rc_session"
	stepUpCookieName  = "rc_stepup"
	ceremonyTTL       = 5 * time.Minute
)

func (s *Server) initAuth() {
	if strings.TrimSpace(s.cfg.DataDir) == "" {
		return
	}
	path := s.cfg.DBPath
	if path == "" {
		path = filepath.Join(s.cfg.DataDir, "rc.db")
	}
	store, err := authstore.OpenDataDir(path)
	if err != nil {
		// Serving without the store would silently discard every account and device.
		log.Fatalf("account store: %v", err)
	}
	if err := s.registry.attachStore(store); err != nil {
		log.Fatalf("device store: %v", err)
	}
	s.auth = store
	s.registry.Consume = store.ConsumeEnrollmentToken
}

// Close releases the account store.
func (s *Server) Close() error {
	if s.auth == nil {
		return nil
	}
	return s.auth.Close()
}

func (s *Server) relyingParty(req *http.Request) (webauthn.RelyingParty, error) {
	scheme := "http"
	if req.TLS != nil {
		scheme = "https"
	}
	headers := [][2]string{}
	if s.cfg.TrustProxy {
		headers = append(headers,
			[2]string{"x-forwarded-proto", req.Header.Get("X-Forwarded-Proto")},
			[2]string{"x-forwarded-host", req.Header.Get("X-Forwarded-Host")},
		)
	}
	return webauthn.RelyingPartyFromRequest(s.cfg.PublicURL, scheme, req.Host, headers)
}

func (s *Server) currentSession(req *http.Request) (*authstore.User, *authstore.Session, error) {
	if s.auth == nil {
		return nil, nil, errors.New("account store is unavailable")
	}
	cookie, err := req.Cookie(sessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return nil, nil, errors.New("session required")
	}
	session, err := s.auth.LookupSession(cookie.Value)
	if err != nil {
		return nil, nil, err
	}
	user, err := s.auth.GetUser(session.UserID)
	if err != nil {
		return nil, nil, err
	}
	return user, session, nil
}

func (s *Server) setSessionCookie(w http.ResponseWriter, name, token string, expires time.Time, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  expires,
		MaxAge:   int(time.Until(expires).Seconds()),
	})
}

func clearSessionCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", HttpOnly: true, MaxAge: -1})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func decodeJSON(req *http.Request, out any) error {
	decoder := json.NewDecoder(io.LimitReader(req.Body, 64<<10))
	decoder.DisallowUnknownFields()
	return decoder.Decode(out)
}

func (s *Server) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if _, _, err := s.currentSession(req); err != nil {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		next.ServeHTTP(w, req)
	})
}

// allowSessionOrDev permits dev mode or a valid passkey session. It is used for
// browser-facing device and SSE reads.
func (s *Server) allowSessionOrDev(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if s.cfg.Development {
			next.ServeHTTP(w, req)
			return
		}
		if _, _, err := s.currentSession(req); err != nil {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		next.ServeHTTP(w, req)
	})
}

func (s *Server) requireSetupAuthorization(w http.ResponseWriter, req *http.Request) bool {
	if s.cfg.SetupToken == "" {
		return true
	}
	value := req.Header.Get("X-RC-Setup-Token")
	if value == "" {
		if cookie, err := req.Cookie("rc_setup"); err == nil {
			value = cookie.Value
		}
	}
	if value == "" {
		value = req.URL.Query().Get("token")
	}
	if subtle.ConstantTimeCompare([]byte(value), []byte(s.cfg.SetupToken)) != 1 {
		writeError(w, http.StatusForbidden, "setup authorization required")
		return false
	}
	return true
}

func (s *Server) handleSetupLink(w http.ResponseWriter, req *http.Request) {
	token := req.PathValue("token")
	if s.cfg.SetupToken == "" || subtle.ConstantTimeCompare([]byte(token), []byte(s.cfg.SetupToken)) != 1 {
		http.NotFound(w, req)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "rc_setup",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   strings.HasPrefix(s.cfg.PublicURL, "https://"),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   30 * 60,
	})
	http.Redirect(w, req, "/", http.StatusSeeOther)
}

func (s *Server) handleMe(w http.ResponseWriter, req *http.Request) {
	user, _, err := s.currentSession(req)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	stepUp := false
	if cookie, err := req.Cookie(stepUpCookieName); err == nil && cookie.Value != "" {
		if _, err := s.auth.LookupSession(cookie.Value); err == nil {
			stepUp = true
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user": map[string]string{"id": user.ID, "name": user.Name},
		"workspaces": []map[string]string{
			{"id": "ws_default", "name": "Default", "role": "owner"},
		},
		"stepUp": stepUp,
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, req *http.Request) {
	for _, name := range []string{sessionCookieName, stepUpCookieName} {
		if cookie, err := req.Cookie(name); err == nil && cookie.Value != "" && s.auth != nil {
			_ = s.auth.RevokeSession(cookie.Value)
		}
		clearSessionCookie(w, name)
	}
	http.Redirect(w, req, "/", http.StatusSeeOther)
}

func (s *Server) handleCreateEnrollment(w http.ResponseWriter, req *http.Request) {
	if _, _, err := s.currentSession(req); err != nil {
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

func randomID(size int) string {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(value)
}
