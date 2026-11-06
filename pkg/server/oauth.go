package server

import (
	"net/http"
	"strings"
)

const mcpResourcePath = "/mcp"

// issuer resolves the OAuth authorization-server issuer. The configured public
// URL wins; otherwise the request origin is used so local deployments work.
func (s *Server) issuer(req *http.Request) string {
	if public := strings.TrimSpace(s.cfg.PublicURL); public != "" {
		return strings.TrimRight(public, "/")
	}
	scheme := "http"
	if req.TLS != nil {
		scheme = "https"
	}
	host := req.Host
	if s.cfg.TrustProxy {
		if forwarded := headerFirst(req.Header.Get("X-Forwarded-Proto")); forwarded == "http" || forwarded == "https" {
			scheme = forwarded
		}
		if forwarded := headerFirst(req.Header.Get("X-Forwarded-Host")); forwarded != "" {
			host = forwarded
		}
	}
	return scheme + "://" + host
}

// mcpResource is the immutable resource identifier for the Remote MCP surface.
func (s *Server) mcpResource(req *http.Request) string {
	return s.issuer(req) + mcpResourcePath
}

func headerFirst(value string) string {
	if index := strings.IndexByte(value, ','); index >= 0 {
		value = value[:index]
	}
	return strings.TrimSpace(value)
}

func writeOAuthError(w http.ResponseWriter, status int, code, description string) {
	writeJSON(w, status, map[string]string{
		"error":             code,
		"error_description": description,
	})
}

// requireOAuth protects Remote MCP with a resource-bound Bearer access token.
// Development mode keeps the unauthenticated local control plane.
func (s *Server) requireOAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if s.cfg.Development {
			next.ServeHTTP(w, req)
			return
		}
		if s.auth == nil {
			writeError(w, http.StatusServiceUnavailable, "account store is unavailable")
			return
		}
		token, ok := bearerToken(req)
		if !ok {
			s.writeOAuthChallenge(w, req)
			return
		}
		if _, err := s.auth.LookupOAuthAccess(token, s.mcpResource(req)); err != nil {
			s.writeOAuthChallenge(w, req)
			return
		}
		next.ServeHTTP(w, req)
	})
}

func bearerToken(req *http.Request) (string, bool) {
	scheme, value, found := strings.Cut(strings.TrimSpace(req.Header.Get("Authorization")), " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	token := strings.TrimSpace(value)
	if token == "" {
		return "", false
	}
	return token, true
}

func (s *Server) writeOAuthChallenge(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+
		s.issuer(req)+`/.well-known/oauth-protected-resource"`)
	writeError(w, http.StatusUnauthorized, "authentication required")
}
