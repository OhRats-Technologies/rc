package server

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/OhRats-Technologies/rc/pkg/server/pages"
	"github.com/OhRats-Technologies/rc/pkg/version"
)

func (s *Server) userCount() int {
	if s.auth == nil {
		return 0
	}
	count, err := s.auth.UserCount()
	if err != nil {
		return 0
	}
	return count
}

func (s *Server) publicSignupConfigured() bool {
	return s.cfg.PublicSignup && s.cfg.TurnstileSiteKey != "" && s.cfg.TurnstileSecretKey != ""
}

func (s *Server) pageOptions() pages.Options {
	return pages.Options{
		PublicURL:    s.cfg.PublicURL,
		Version:      version.Version,
		PublicSignup: s.publicSignupConfigured(),
		SiteKey:      s.cfg.TurnstileSiteKey,
	}
}

func (s *Server) writePage(w http.ResponseWriter, status int, body string, cacheControl string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", cacheControl)
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

func (s *Server) handlePublicPage(w http.ResponseWriter, req *http.Request) {
	if req.URL.Path == "/" {
		s.handleHomePage(w, req)
		return
	}
	page, ok := pages.Lookup(req.URL.Path)
	if !ok {
		http.NotFound(w, req)
		return
	}
	s.writePage(w, http.StatusOK, pages.Public(page, s.pageOptions()), "public, max-age=0, must-revalidate")
}

func (s *Server) handleHomePage(w http.ResponseWriter, req *http.Request) {
	if s.userCount() == 0 {
		options := s.pageOptions()
		options.Authorized = s.requireSetupAuthorizationSilent(req)
		s.writePage(w, http.StatusOK, pages.Auth(pages.Setup, options), "no-store")
		return
	}
	if _, _, err := s.currentSession(req); err == nil {
		http.Redirect(w, req, "/devices", http.StatusSeeOther)
		return
	}
	next := req.URL.Query().Get("next")
	if safeNext(next) && next != "/devices" {
		options := s.pageOptions()
		options.Next = next
		s.writePage(w, http.StatusOK, pages.Auth(pages.Login, options), "no-store")
		return
	}
	s.writePage(w, http.StatusOK, pages.Public(pages.Landing, s.pageOptions()), "public, max-age=0, must-revalidate")
}

func (s *Server) handleLoginPage(w http.ResponseWriter, req *http.Request) {
	if s.userCount() == 0 {
		http.Redirect(w, req, "/", http.StatusSeeOther)
		return
	}
	if _, _, err := s.currentSession(req); err == nil {
		http.Redirect(w, req, "/devices", http.StatusSeeOther)
		return
	}
	options := s.pageOptions()
	if next := req.URL.Query().Get("next"); safeNext(next) {
		options.Next = next
	}
	s.writePage(w, http.StatusOK, pages.Auth(pages.Login, options), "no-store")
}

func (s *Server) handleSignupPage(w http.ResponseWriter, req *http.Request) {
	if !s.publicSignupConfigured() || s.userCount() == 0 {
		http.Redirect(w, req, "/login", http.StatusSeeOther)
		return
	}
	if _, _, err := s.currentSession(req); err == nil {
		http.Redirect(w, req, "/devices", http.StatusSeeOther)
		return
	}
	s.writePage(w, http.StatusOK, pages.Auth(pages.Signup, s.pageOptions()), "no-store")
}

func (s *Server) handleAPIHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (s *Server) handleAPIStatus(w http.ResponseWriter, req *http.Request) {
	users := s.userCount()
	setupRequired := users == 0
	writeJSON(w, http.StatusOK, map[string]any{
		"setupRequired":   setupRequired,
		"setupAuthorized": setupRequired && s.requireSetupAuthorizationSilent(req),
		"publicSignup":    users > 0 && s.publicSignupConfigured(),
		"version":         version.Version,
	})
}

// requireSetupAuthorizationSilent reports whether the request already carries
// valid first-instance setup authorization. It never writes a response.
func (s *Server) requireSetupAuthorizationSilent(req *http.Request) bool {
	if s.cfg.SetupToken == "" {
		return true
	}
	candidates := []string{req.Header.Get("X-RC-Setup-Token")}
	if cookie, err := req.Cookie("rc_setup"); err == nil {
		candidates = append(candidates, cookie.Value)
	}
	candidates = append(candidates, req.URL.Query().Get("token"))
	for _, value := range candidates {
		if subtle.ConstantTimeCompare([]byte(value), []byte(s.cfg.SetupToken)) == 1 {
			return true
		}
	}
	return false
}

func safeNext(value string) bool {
	return strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "//")
}
