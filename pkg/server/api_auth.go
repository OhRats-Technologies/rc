package server

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	authstore "github.com/OhRats-Technologies/rc/pkg/auth"
	"github.com/OhRats-Technologies/rc/pkg/crypto"
)

const maxAPIKeyBody = 1 << 20

type apiPrincipal struct {
	KeyID  string
	UserID string
	Scopes []string
}

func (p *apiPrincipal) HasScope(required string) bool {
	if p == nil {
		return false
	}
	for _, scope := range p.Scopes {
		if scope == required {
			return true
		}
	}
	return false
}

type apiPrincipalContextKey struct{}
type sessionUserContextKey struct{}
type developmentAccessContextKey struct{}

func apiPrincipalFrom(req *http.Request) *apiPrincipal {
	value, _ := req.Context().Value(apiPrincipalContextKey{}).(*apiPrincipal)
	return value
}

func sessionUserFrom(req *http.Request) *authstore.User {
	value, _ := req.Context().Value(sessionUserContextKey{}).(*authstore.User)
	return value
}

func developmentAccessFrom(req *http.Request) bool {
	value, _ := req.Context().Value(developmentAccessContextKey{}).(bool)
	return value
}

func requestUserID(req *http.Request) (string, bool) {
	if user := sessionUserFrom(req); user != nil {
		return user.ID, true
	}
	if principal := apiPrincipalFrom(req); principal != nil {
		return principal.UserID, true
	}
	return "", false
}

// allowSessionOrAPI accepts either a passkey browser session or a signed API
// key request. API keys must carry the scope required by the route.
func (s *Server) allowSessionOrAPI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if s.cfg.Development {
			ctx := context.WithValue(req.Context(), developmentAccessContextKey{}, true)
			next.ServeHTTP(w, req.WithContext(ctx))
			return
		}
		if strings.TrimSpace(req.Header.Get("X-RC-Key-ID")) != "" {
			principal, status, message := s.authenticateAPIKey(req)
			if status != http.StatusOK {
				writeError(w, status, message)
				return
			}
			if required := apiRequiredScope(req); required != "" && !principal.HasScope(required) {
				writeError(w, http.StatusForbidden, "API key is missing the required scope")
				return
			}
			ctx := context.WithValue(req.Context(), apiPrincipalContextKey{}, principal)
			next.ServeHTTP(w, req.WithContext(ctx))
			return
		}
		user, _, err := s.currentSession(req)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		ctx := context.WithValue(req.Context(), sessionUserContextKey{}, user)
		next.ServeHTTP(w, req.WithContext(ctx))
	})
}

func apiRequiredScope(req *http.Request) string {
	switch {
	case strings.HasPrefix(req.URL.Path, "/api/v1/events"):
		return "read"
	case strings.HasSuffix(req.URL.Path, "/processes") &&
		strings.HasPrefix(req.URL.Path, "/api/v1/devices/"):
		if req.Method == http.MethodGet || req.Method == http.MethodHead {
			return "read"
		}
		return "execute"
	case strings.HasPrefix(req.URL.Path, "/api/v1/processes/"):
		return "read"
	case strings.HasPrefix(req.URL.Path, "/api/v1/devices"):
		if req.Method == http.MethodGet || req.Method == http.MethodHead {
			return "read"
		}
		return "manage-devices"
	case req.URL.Path == "/api/v1/me":
		return "read"
	case strings.HasPrefix(req.URL.Path, "/api/v1/control/"):
		return "execute"
	case strings.HasPrefix(req.URL.Path, "/api/v1/workspaces/") &&
		strings.HasSuffix(req.URL.Path, "/enrollments") && req.Method == http.MethodPost:
		return "manage-devices"
	default:
		return ""
	}
}

func (s *Server) authenticateAPIKey(req *http.Request) (*apiPrincipal, int, string) {
	if s.auth == nil {
		return nil, http.StatusServiceUnavailable, "account store is unavailable"
	}
	keyID := strings.TrimSpace(req.Header.Get("X-RC-Key-ID"))
	timestamp := strings.TrimSpace(req.Header.Get("X-RC-Timestamp"))
	nonce := strings.TrimSpace(req.Header.Get("X-RC-Nonce"))
	signature := strings.TrimSpace(req.Header.Get("X-RC-Signature"))
	if keyID == "" || timestamp == "" || nonce == "" || signature == "" {
		return nil, http.StatusUnauthorized, "missing API key authentication"
	}
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || absInt64(time.Now().Unix()-seconds) > 60 {
		return nil, http.StatusUnauthorized, "expired API key authentication"
	}
	if len(nonce) < 16 || len(nonce) > 128 {
		return nil, http.StatusUnauthorized, "invalid API key authentication"
	}
	key, err := s.auth.APIKey(keyID)
	if err != nil || !key.Active(time.Now()) {
		return nil, http.StatusUnauthorized, "API key is invalid or expired"
	}
	body, status, message := readRequestForSignature(req)
	if status != http.StatusOK {
		return nil, status, message
	}
	err = crypto.VerifyApi(key.PublicKey, signature, crypto.ApiRequest{
		KeyID: keyID, Timestamp: timestamp, Nonce: nonce,
		Method: req.Method, RequestURI: req.URL.RequestURI(), Body: body,
	})
	if err != nil {
		return nil, http.StatusUnauthorized, "API key authentication was rejected"
	}
	fresh, err := s.auth.RememberNonce("client:"+keyID, nonce,
		time.Now().Add(2*time.Minute))
	if err != nil {
		return nil, http.StatusInternalServerError, "API key authentication failed"
	}
	if !fresh {
		return nil, http.StatusConflict, "API key request was replayed"
	}
	_ = s.auth.TouchAPIKey(keyID)
	return &apiPrincipal{KeyID: key.ID, UserID: key.UserID, Scopes: key.Scopes},
		http.StatusOK, ""
}

func readRequestForSignature(req *http.Request) ([]byte, int, string) {
	if req.Body == nil {
		return nil, http.StatusOK, ""
	}
	body, err := io.ReadAll(io.LimitReader(req.Body, maxAPIKeyBody+1))
	if err != nil {
		return nil, http.StatusBadRequest, "request body could not be read"
	}
	if len(body) > maxAPIKeyBody {
		return nil, http.StatusRequestEntityTooLarge, "request body is too large"
	}
	req.Body = io.NopCloser(bytes.NewReader(body))
	return body, http.StatusOK, ""
}

func absInt64(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}
