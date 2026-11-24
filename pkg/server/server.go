package server

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"

	authstore "github.com/OhRats-Technologies/rc/pkg/auth"
	"github.com/OhRats-Technologies/rc/pkg/node"
	"github.com/OhRats-Technologies/rc/pkg/protocol"
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
	MCPAccessTTLMinutes    uint64
}

type Server struct {
	cfg       Config
	httpSrv   *http.Server
	registry  *DeviceRegistry
	events    *EventHub
	mcp       *McpHandler
	auth      *authstore.Store
	nodes     *NodeHub
	control    *controlHub
	processes  *processHub
	assetVer   string
	stopReaper chan struct{}
	reapOnce   sync.Once

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
	nodes := newNodeHub()
	reg.isConnected = nodes.Online
	mcp.nodes = nodes
	mcp.local = cfg.Development

	s := &Server{
		cfg:        cfg,
		registry:   reg,
		events:     events,
		mcp:        mcp,
		nodes:      nodes,
		assetVer:   computeAssetVersion(cfg.AssetsDir),
		stopReaper: make(chan struct{}),
	}
	go s.runPresenceReaper()
	s.control = newControlHub()
	s.processes = newProcessHub()
	mcp.challenge = s.writeOAuthChallenge
	nodes.onHello = s.handleNodeHello
	nodes.onLock = s.handleNodeLockState
	s.control.wire(nodes)
	nodes.onControlClosed = func(deviceID, sessionID string) {
		session, ok := s.control.get(sessionID)
		if ok && session.DeviceID == deviceID {
			s.control.remove(sessionID)
		}
	}
	nodes.onProcessState = func(deviceID string, message protocol.NodeToServer) {
		exit := message.ExitCode
		s.processes.update(message.ProcessID, message.Status, &exit, message.Signal, message.Error)
		kind := "process.started"
		if message.Status != "running" {
			kind = "process.exited"
		}
		s.events.Publish(kind, map[string]any{
			"deviceId": deviceID, "processId": message.ProcessID,
			"exitCode": message.ExitCode, "signal": message.Signal, "error": message.Error,
		})
	}
	nodes.onPresence = func(deviceID string, online bool) {
		if online {
			s.registry.markOnline(deviceID, true)
			s.events.Publish("device.online", map[string]any{"deviceId": deviceID})
			return
		}
		s.control.releaseDevice(deviceID)
		s.markOffline(deviceID)
	}
	if cfg.Development && cfg.DevelopmentEnrollToken != "" {
		reg.AddToken(cfg.DevelopmentEnrollToken)
	}
	s.initAuth()
	mcp.auth = s.auth
	mcp.authorityHash = func(workspaceID string) (string, error) {
		snapshot, err := s.authoritySnapshot(workspaceID)
		if err != nil {
			return "", err
		}
		return node.SnapshotHash(snapshot), nil
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /api/v1/health", s.handleAPIHealth)
	mux.HandleFunc("GET /api/v1/status", s.handleAPIStatus)
	mux.HandleFunc("POST /api/v1/node/enroll", reg.HandleEnroll)
	mux.HandleFunc("GET /api/v1/node/self", s.handleNodeSelf)
	mux.HandleFunc("POST /api/v1/node/heartbeat", s.handleNodeHeartbeat)
	mux.HandleFunc("GET /api/v1/node/connect", s.handleNodeConnect)
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
	mux.Handle("GET /api/v1/me", s.allowSessionOrAPI(http.HandlerFunc(s.handleMe)))
	mux.HandleFunc("GET /api/v1/control/clients/{id}", s.handleControlClientStatus)
	mux.HandleFunc("POST /api/v1/control/authorize/options", s.handleControlAuthorizeOptions)
	mux.HandleFunc("POST /api/v1/control/authorize/verify", s.handleControlAuthorizeVerify)
	mux.Handle("POST /api/v1/control/challenge", s.allowSessionOrAPI(http.HandlerFunc(s.handleControlChallenge)))
	mux.Handle("POST /api/v1/control/open", s.allowSessionOrAPI(http.HandlerFunc(s.handleControlOpen)))
	mux.Handle("GET /api/v1/control/pipe/{session}", s.allowSessionOrAPI(http.HandlerFunc(s.handleControlPipe)))
	mux.Handle("DELETE /api/v1/control/{session}", s.allowSessionOrAPI(http.HandlerFunc(s.handleControlClose)))
	mux.HandleFunc("POST /account/logout", s.handleLogout)
	mux.HandleFunc("POST /account/delete", s.handleAccountDelete)
	mux.HandleFunc("DELETE /api/v1/account", s.handleAccountDelete)
	mux.HandleFunc("GET /account", s.handleAccountPage)
	mux.HandleFunc("GET /api", s.handleAPIPage)
	mux.HandleFunc("GET /api/keys/new", s.handleAPIPage)
	mux.HandleFunc("GET /integrations/mcp", s.handleMCPPage)
	mux.HandleFunc("POST /account/name", s.handleAccountName)
	mux.HandleFunc("GET /devices", s.handleDevicesPage)
	mux.HandleFunc("GET /devices/enroll", s.handleEnrollPage)
	mux.HandleFunc("GET /devices/{id}", s.handleDevicePage)
	mux.HandleFunc("POST /devices/{id}/rename", s.handleDeviceRenameForm)
	s.registerWorkspaceRoutes(mux)
	mux.Handle("GET /api/v1/devices", s.allowSessionOrAPI(http.HandlerFunc(reg.HandleListDevices)))
	mux.Handle("/api/v1/devices/", s.allowSessionOrAPI(http.HandlerFunc(reg.HandleDeviceRoutes)))
	mux.Handle("GET /api/v1/events", s.allowSessionOrAPI(events))
	s.registerProcessAPIRoutes(mux)
	mux.HandleFunc("GET /api/v1/tokens", s.handleListAPITokens)
	mux.HandleFunc("POST /api/v1/tokens", s.handleCreateAPIToken)
	mux.HandleFunc("DELETE /api/v1/tokens/{id}", s.handleDeleteAPIToken)
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", s.handleOAuthAuthorizationServerMetadata)
	mux.HandleFunc("GET /.well-known/oauth-protected-resource", s.handleOAuthProtectedResourceMetadata)
	mux.HandleFunc("GET /.well-known/oauth-protected-resource/mcp", s.handleOAuthProtectedResourceMetadata)
	mux.HandleFunc("POST /oauth/register", s.handleOAuthRegister)
	mux.HandleFunc("GET /oauth/authorize", s.handleOAuthAuthorize)
	mux.HandleFunc("POST /oauth/authorize/prepare", s.handleOAuthPrepare)
	mux.HandleFunc("POST /oauth/authorize/approve", s.handleOAuthApproveGrant)
	mux.HandleFunc("POST /oauth/authorize/cancel", s.handleOAuthCancel)
	mux.HandleFunc("POST /oauth/authorize/switch-account", s.handleOAuthSwitchAccount)
	mux.HandleFunc("DELETE /oauth/grants/{id}", s.handleOAuthRevokeGrant)
	mux.HandleFunc("POST /oauth/token", s.handleOAuthToken)
	mux.HandleFunc("POST /oauth/revoke", s.handleOAuthRevoke)
	mux.Handle("POST /mcp", s.requireOAuth(mcp))

	// Static assets and public pages.
	mux.HandleFunc("GET /assets/", s.handleAssets)
	mux.HandleFunc("GET /install.sh", s.handleInstaller("install.sh"))
	mux.HandleFunc("GET /install.ps1", s.handleInstaller("install.ps1"))
	mux.HandleFunc("GET /android-enroll.sh", s.handleInstaller("android-enroll.sh"))
	mux.HandleFunc("GET /{$}", s.handlePublicPage)
	mux.HandleFunc("GET /docs", s.handlePublicPage)
	mux.HandleFunc("GET /docs/{topic}", s.handlePublicPage)
	mux.HandleFunc("GET /login", s.handleLoginPage)
	mux.HandleFunc("GET /signup", s.handleSignupPage)

	s.httpSrv = &http.Server{
		Addr:         cfg.Addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 90 * time.Second,
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
