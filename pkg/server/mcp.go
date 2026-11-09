package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	authstore "github.com/OhRats-Technologies/rc/pkg/auth"
	"github.com/OhRats-Technologies/rc/pkg/node"
	"github.com/OhRats-Technologies/rc/pkg/version"
)

const (
	mcpProtocolVersion       = "2026-07-28"
	mcpLegacyProtocolVersion = "2025-11-25"
)

var mcpSupportedVersions = []string{
	mcpProtocolVersion,
	mcpLegacyProtocolVersion,
	"2025-06-18",
	"2025-03-26",
	"2024-11-05",
}

type McpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type McpResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Result  any    `json:"result,omitempty"`
	Error   any    `json:"error,omitempty"`
}

type mcpProcessBinding struct {
	DeviceID string
	UserID   string
	ClientID string
	GrantID  string
}

type McpHandler struct {
	registry      *DeviceRegistry
	exec          node.ExecutionManager
	nodes         *NodeHub
	auth          *authstore.Store
	authorityHash func(string) (string, error)
	local         bool
	challenge     func(http.ResponseWriter, *http.Request, string, bool)

	mu        sync.Mutex
	processes map[string]mcpProcessBinding
}

func NewMcpHandler(reg *DeviceRegistry, exec node.ExecutionManager) *McpHandler {
	return &McpHandler{
		registry:  reg,
		exec:      exec,
		processes: make(map[string]mcpProcessBinding),
	}
}

func (h *McpHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("x-rc-request-id", randomID(16))
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	var req McpRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeRPC(w, http.StatusBadRequest, McpResponse{
			JSONRPC: "2.0", ID: nil,
			Error: map[string]any{"code": -32700, "message": "Parse error"},
		})
		return
	}
	if req.JSONRPC != "2.0" || strings.TrimSpace(req.Method) == "" {
		h.writeRPC(w, http.StatusBadRequest, McpResponse{
			JSONRPC: "2.0", ID: req.ID,
			Error: map[string]any{"code": -32600, "message": "Invalid Request"},
		})
		return
	}
	if err := validateMCPProtocolVersion(r.Header.Get("MCP-Protocol-Version")); err != nil {
		h.writeRPC(w, http.StatusBadRequest, McpResponse{
			JSONRPC: "2.0", ID: req.ID,
			Error: map[string]any{"code": -32022, "message": err.Error()},
		})
		return
	}
	if header := strings.TrimSpace(r.Header.Get("Mcp-Method")); header != "" && header != req.Method {
		h.writeRPC(w, http.StatusBadRequest, McpResponse{
			JSONRPC: "2.0", ID: req.ID,
			Error: map[string]any{"code": -32600, "message": "Mcp-Method header does not match request"},
		})
		return
	}
	if isNotification(req.Method) {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	grant := oauthGrantFromRequest(r)
	if grant == nil {
		h.writeAuthError(w, r, authstore.OAuthScopeObserve, false)
		return
	}
	if !h.grantUsable(grant) {
		h.writeAuthError(w, r, "", false)
		return
	}
	switch req.Method {
	case "initialize":
		h.handleInitialize(w, r, req)
	case "server/discover":
		h.handleServerDiscover(w, req)
	case "ping":
		h.writeRPC(w, http.StatusOK, McpResponse{JSONRPC: "2.0", ID: req.ID, Result: struct{}{}})
	case "tools/list":
		h.handleToolsList(w, r, req, grant)
	case "tools/call":
		h.handleToolsCall(w, r, req, grant)
	case "machines_list", "process_run", "process_status", "process_input", "process_cancel", "image_view":
		h.handleLegacyCall(w, r, req, grant)
	default:
		h.writeRPC(w, http.StatusOK, McpResponse{
			JSONRPC: "2.0", ID: req.ID,
			Error: map[string]any{"code": -32601, "message": "Method not found"},
		})
	}
}

func (h *McpHandler) handleInitialize(w http.ResponseWriter, r *http.Request, req McpRequest) {
	var params struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	_ = json.Unmarshal(req.Params, &params)
	protocol := params.ProtocolVersion
	if !supportedMCPVersion(protocol) {
		protocol = mcpProtocolVersion
	}
	h.writeRPC(w, http.StatusOK, McpResponse{
		JSONRPC: "2.0", ID: req.ID,
		Result: map[string]any{
			"protocolVersion": protocol,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "RC", "version": version.Version},
			"instructions":    "Use only the machines and capabilities explicitly granted by the user.",
		},
	})
}

func (h *McpHandler) handleServerDiscover(w http.ResponseWriter, req McpRequest) {
	h.writeRPC(w, http.StatusOK, McpResponse{
		JSONRPC: "2.0", ID: req.ID,
		Result: map[string]any{
			"resultType":        "complete",
			"supportedVersions": mcpSupportedVersions,
			"capabilities":      map[string]any{"tools": map[string]any{}},
			"instructions":      "Use only the machines and capabilities explicitly granted by the user.",
			"ttlMs":             300000,
			"cacheScope":        "public",
			"_meta": map[string]any{
				"io.modelcontextprotocol/serverInfo": map[string]any{
					"name": "RC", "version": version.Version,
				},
			},
		},
	})
}

func (h *McpHandler) handleToolsList(w http.ResponseWriter, r *http.Request, req McpRequest, grant *authstore.OAuthGrant) {
	if !hasAnyMCPScope(grant) {
		h.writeAuthError(w, r, authstore.OAuthScopeObserve, true)
		return
	}
	terminal := grant.HasScope(authstore.OAuthScopeTerminal)
	result := map[string]any{"tools": mcpTools(terminal)}
	if isModernMCP(r) {
		result["resultType"] = "complete"
		result["ttlMs"] = 30000
		result["cacheScope"] = "private"
	}
	h.writeRPC(w, http.StatusOK, McpResponse{
		JSONRPC: "2.0", ID: req.ID,
		Result: result,
	})
}

func (h *McpHandler) handleToolsCall(w http.ResponseWriter, r *http.Request, req McpRequest, grant *authstore.OAuthGrant) {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || strings.TrimSpace(params.Name) == "" {
		h.writeRPC(w, http.StatusOK, McpResponse{
			JSONRPC: "2.0", ID: req.ID,
			Error: map[string]any{"code": -32602, "message": "Invalid params"},
		})
		return
	}
	required := registeredMCPScope(params.Name)
	if required == "" {
		h.writeRPC(w, http.StatusOK, McpResponse{
			JSONRPC: "2.0", ID: req.ID,
			Error: map[string]any{"code": -32602, "message": "Tool is not available: " + params.Name},
		})
		return
	}
	if !grant.HasScope(required) {
		h.writeAuthError(w, r, required, true)
		return
	}
	if header := strings.TrimSpace(r.Header.Get("Mcp-Name")); header != "" && header != params.Name {
		h.writeRPC(w, http.StatusBadRequest, McpResponse{
			JSONRPC: "2.0", ID: req.ID,
			Error: map[string]any{"code": -32600, "message": "Mcp-Name header does not match tool call"},
		})
		return
	}
	result := h.callTool(r.Context(), params.Name, params.Arguments, grant)
	if isModernMCP(r) {
		result["resultType"] = "complete"
	}
	h.writeRPC(w, http.StatusOK, McpResponse{JSONRPC: "2.0", ID: req.ID, Result: result})
}

func (h *McpHandler) handleLegacyCall(w http.ResponseWriter, r *http.Request, req McpRequest, grant *authstore.OAuthGrant) {
	required := registeredMCPScope(req.Method)
	if !grant.HasScope(required) {
		h.writeAuthError(w, r, required, true)
		return
	}
	result := h.callTool(r.Context(), req.Method, req.Params, grant)
	var response any = result
	if structured, ok := result["structuredContent"]; ok {
		response = structured
	}
	h.writeRPC(w, http.StatusOK, McpResponse{JSONRPC: "2.0", ID: req.ID, Result: response})
}

func (h *McpHandler) callTool(ctx context.Context, name string, raw json.RawMessage, grant *authstore.OAuthGrant) map[string]any {
	switch name {
	case "machines_list":
		return h.callMachinesList(grant)
	case "image_view":
		return h.callImageView(ctx, raw, grant)
	case "process_run":
		return h.callProcessRun(ctx, raw, grant)
	case "process_status":
		return h.callProcessStatus(ctx, raw, grant)
	case "process_input":
		return h.callProcessInput(ctx, raw, grant)
	case "process_cancel":
		return h.callProcessCancel(ctx, raw, grant)
	default:
		return mcpToolError("tool is not available: " + name)
	}
}

func (h *McpHandler) writeRPC(w http.ResponseWriter, status int, response McpResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(response)
}

func (h *McpHandler) writeAuthError(w http.ResponseWriter, req *http.Request, scope string, insufficient bool) {
	if h.challenge != nil {
		h.challenge(w, req, scope, insufficient)
		return
	}
	if insufficient {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "insufficient_scope"})
		return
	}
	writeError(w, http.StatusUnauthorized, "authentication required")
}

func isModernMCP(r *http.Request) bool {
	return strings.TrimSpace(r.Header.Get("MCP-Protocol-Version")) == mcpProtocolVersion
}
