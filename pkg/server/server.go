package server

import (
	"context"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	authstore "github.com/OhRats-Technologies/rc/pkg/auth"
	"github.com/OhRats-Technologies/rc/pkg/node"
)

// Config describes one rc-server process.
//
// Development enables in-memory enrollment tokens and the MCP surface used by
// local tests and demos. Signed Node presence, passkey setup/login sessions,
// durable workspace enrollment tokens, authenticated SSE, and MCP OAuth work
// without it. MCP OAuth requires the durable account store and fails closed
// without one.
type Config struct {
	Addr                   string
	DataDir                string
	AssetsDir              string
	DBPath                 string
	SetupToken             string
	PublicURL              string
	PublicSignup           bool
	Development            bool
	DevelopmentEnrollToken string
	TurnstileSiteKey       string
	TurnstileSecretKey     string
	TrustProxy             bool
	TurnTokenID            string
	TurnAPIToken           string
	MCPAccessTTLMinutes    uint64
}

type Server struct {
	cfg      Config
	httpSrv  *http.Server
	registry *DeviceRegistry
	events   *EventHub
	mcp      *McpHandler
	auth     *authstore.Store
	turn     *turnProvider

	mu         sync.RWMutex
	listenAddr string
}

func New(cfg Config) *Server {
	if cfg.Addr == "" {
		cfg.Addr = ":8080"
	}
	if cfg.AssetsDir == "" {
		cfg.AssetsDir = "dist/assets"
	}

	reg := NewDeviceRegistry()
	events := NewEventHub()
	execMgr := node.NewNativeExecutionManager()
	mcp := NewMcpHandler(reg, execMgr)

	s := &Server{
		cfg:      cfg,
		registry: reg,
		events:   events,
		mcp:      mcp,
		turn:     newTurnProvider(cfg.TurnTokenID, cfg.TurnAPIToken),
	}
	if cfg.Development && cfg.DevelopmentEnrollToken != "" {
		reg.AddToken(cfg.DevelopmentEnrollToken)
	}
	s.initAuth()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /api/v1/health", s.handleAPIHealth)
	mux.HandleFunc("GET /api/v1/status", s.handleAPIStatus)
	mux.HandleFunc("POST /api/v1/node/enroll", reg.HandleEnroll)
	mux.HandleFunc("GET /api/v1/node/self", s.handleNodeSelf)
	mux.HandleFunc("POST /api/v1/node/heartbeat", s.handleNodeHeartbeat)
	mux.HandleFunc("GET /api/v1/node/ice", s.handleNodeICE)
	mux.HandleFunc("GET /setup/{token}", s.handleSetupLink)
	mux.HandleFunc("POST /api/v1/auth/setup/options", s.handleSetupOptions)
	mux.HandleFunc("POST /api/v1/auth/setup/verify", s.handleSetupVerify)
	mux.HandleFunc("POST /api/v1/auth/login/options", s.handleLoginOptions)
	mux.HandleFunc("POST /api/v1/auth/login/verify", s.handleLoginVerify)
	mux.HandleFunc("POST /api/v1/auth/register/options", s.handleRegisterOptions)
	mux.HandleFunc("POST /api/v1/auth/register/verify", s.handleRegisterVerify)
	mux.HandleFunc("POST /api/v1/auth/signup/options", s.handleSignupOptions)
	mux.HandleFunc("POST /api/v1/auth/step-up/options", s.handleStepUpOptions)
	mux.HandleFunc("POST /api/v1/auth/step-up/verify", s.handleStepUpVerify)
	mux.HandleFunc("GET /api/v1/me", s.handleMe)
	mux.HandleFunc("POST /account/logout", s.handleLogout)
	mux.HandleFunc("GET /account", s.handleAccountPage)
	mux.HandleFunc("POST /account/name", s.handleAccountName)
	mux.HandleFunc("GET /devices", s.handleDevicesPage)
	mux.HandleFunc("GET /devices/enroll", s.handleEnrollPage)
	mux.HandleFunc("GET /devices/{id}", s.handleDevicePage)
	mux.HandleFunc("POST /devices/{id}/rename", s.handleDeviceRenameForm)
	mux.HandleFunc("POST /api/v1/workspaces/{id}/enrollments", s.handleCreateEnrollment)
	mux.Handle("GET /api/v1/devices", s.allowSessionOrDev(http.HandlerFunc(reg.HandleListDevices)))
	mux.Handle("/api/v1/devices/", s.allowSessionOrDev(http.HandlerFunc(reg.HandleDeviceRoutes)))
	mux.Handle("GET /api/v1/events", s.allowSessionOrDev(events))
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", s.handleOAuthAuthorizationServerMetadata)
	mux.HandleFunc("GET /.well-known/oauth-protected-resource", s.handleOAuthProtectedResourceMetadata)
	mux.HandleFunc("POST /oauth/register", s.handleOAuthRegister)
	mux.HandleFunc("GET /oauth/authorize", s.handleOAuthAuthorize)
	mux.HandleFunc("POST /oauth/token", s.handleOAuthToken)
	mux.Handle("POST /mcp", s.requireOAuth(mcp))

	// Static assets and public pages.
	mux.HandleFunc("GET /assets/", s.handleAssets)
	mux.HandleFunc("GET /{$}", s.handlePublicPage)
	mux.HandleFunc("GET /docs", s.handlePublicPage)
	mux.HandleFunc("GET /docs/{topic}", s.handlePublicPage)
	mux.HandleFunc("GET /login", s.handleLoginPage)
	mux.HandleFunc("GET /signup", s.handleSignupPage)

	s.httpSrv = &http.Server{
		Addr:         cfg.Addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
	}

	return s
}

// Listen binds the configured address. Tests and rc-server print the resolved
// address (including the kernel-assigned port) from the returned listener.
func (s *Server) Listen() (net.Listener, error) {
	ln, err := net.Listen("tcp", s.cfg.Addr)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.listenAddr = ln.Addr().String()
	s.mu.Unlock()
	return ln, nil
}

func (s *Server) Serve(ln net.Listener) error {
	return s.httpSrv.Serve(ln)
}

func (s *Server) Start() error {
	ln, err := s.Listen()
	if err != nil {
		return err
	}
	return s.Serve(ln)
}

func (s *Server) Shutdown(ctx context.Context) error {
	err := s.httpSrv.Shutdown(ctx)
	if closeErr := s.Close(); err == nil {
		err = closeErr
	}
	return err
}

// Addr reports the bound address once Listen or Start succeeded.
func (s *Server) Addr() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.listenAddr
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func (s *Server) handleAssets(w http.ResponseWriter, r *http.Request) {
	rel := strings.TrimPrefix(r.URL.Path, "/assets/")
	if rel == "" || strings.HasPrefix(rel, "/") || !filepath.IsLocal(filepath.FromSlash(rel)) {
		http.NotFound(w, r)
		return
	}
	fullPath := filepath.Join(s.cfg.AssetsDir, filepath.FromSlash(path.Clean(rel)))
	fi, err := os.Stat(fullPath)
	if err != nil || fi.IsDir() {
		http.NotFound(w, r)
		return
	}
	if contentHashed(path.Base(fullPath)) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=0, must-revalidate")
	}
	http.ServeFile(w, r, fullPath)
}

// contentHashed reports names produced by the client bundler's content-hash
// scheme. Other assets keep a stable name and must be revalidated.
func contentHashed(name string) bool {
	if strings.HasPrefix(name, "chunk-") {
		return true
	}
	extensions := map[string]bool{
		"css": true, "js": true, "mjs": true, "png": true, "jpg": true, "jpeg": true,
		"gif": true, "svg": true, "webp": true, "ico": true, "woff": true, "woff2": true,
		"ttf": true, "otf": true, "json": true, "map": true, "txt": true, "wasm": true,
	}
	for index := 0; index < len(name); index++ {
		if name[index] != '.' {
			continue
		}
		digest := name[index+1:]
		if end := strings.IndexByte(digest, '.'); end >= 0 {
			digest = digest[:end]
		}
		if len(digest) >= 6 && !extensions[digest] {
			return true
		}
	}
	return false
}
