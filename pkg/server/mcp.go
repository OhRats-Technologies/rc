package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/OhRats-Technologies/rc/pkg/node"
	"github.com/OhRats-Technologies/rc/pkg/protocol"
)

type McpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type McpResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Result  any    `json:"result,omitempty"`
	Error   any    `json:"error,omitempty"`
}

type McpHandler struct {
	registry *DeviceRegistry
	exec     node.ExecutionManager
}

func NewMcpHandler(reg *DeviceRegistry, exec node.ExecutionManager) *McpHandler {
	return &McpHandler{
		registry: reg,
		exec:     exec,
	}
}

func (h *McpHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req McpRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	res := McpResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
	}

	switch req.Method {
	case "machines_list":
		h.handleMachinesList(&res)

	case "process_run":
		h.handleProcessRun(r, req.Params, &res)

	case "process_status":
		h.handleProcessStatus(r, req.Params, &res)

	case "process_input":
		h.handleProcessInput(req.Params, &res)

	case "process_cancel":
		h.handleProcessCancel(req.Params, &res)

	case "image_view":
		res.Result = map[string]any{"mimeType": "image/png", "sizeBytes": 0}

	default:
		res.Error = map[string]any{
			"code":    -32601,
			"message": "method not found",
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (h *McpHandler) handleMachinesList(res *McpResponse) {
	h.registry.mu.RLock()
	var list []map[string]any
	for _, d := range h.registry.devices {
		list = append(list, map[string]any{
			"id":       d.ID,
			"name":     d.Name,
			"online":   d.Online,
			"platform": d.Platform,
			"arch":     d.Arch,
		})
	}
	h.registry.mu.RUnlock()
	res.Result = map[string]any{"machines": list}
}

type mcpRunParams struct {
	DeviceID       string   `json:"deviceId"`
	Command        string   `json:"command,omitempty"`
	Program        string   `json:"program,omitempty"`
	Args           []string `json:"args,omitempty"`
	Cwd            string   `json:"cwd,omitempty"`
	WaitSeconds    uint64   `json:"waitSeconds,omitempty"`
	TimeoutSeconds uint64   `json:"timeoutSeconds,omitempty"`
}

func (h *McpHandler) handleProcessRun(r *http.Request, rawParams json.RawMessage, res *McpResponse) {
	if h.exec == nil {
		res.Error = map[string]any{"code": -32603, "message": "execution control plane is unavailable"}
		return
	}

	var params mcpRunParams
	if len(rawParams) > 0 {
		_ = json.Unmarshal(rawParams, &params)
	}

	b := make([]byte, 8)
	_, _ = rand.Read(b)
	procID := fmt.Sprintf("proc_%s", hex.EncodeToString(b))

	spec := node.ProcessSpec{
		ID:  procID,
		Cwd: params.Cwd,
	}

	if params.Program != "" {
		spec.Mode = protocol.ExecutionMode{
			Kind:    protocol.ExecutionModeArgv,
			Program: params.Program,
			Args:    params.Args,
		}
	} else if params.Command != "" {
		spec.Mode = protocol.ExecutionMode{
			Kind:    protocol.ExecutionModeSystemShell,
			Command: params.Command,
		}
	} else {
		res.Error = map[string]any{"code": -32602, "message": "command or program required"}
		return
	}

	started, err := h.exec.Start(spec)
	if err != nil || !started {
		res.Error = map[string]any{"code": -32603, "message": fmt.Sprintf("failed to start execution: %v", err)}
		return
	}

	waitSec := params.WaitSeconds
	if waitSec == 0 {
		waitSec = params.TimeoutSeconds
	}
	if waitSec > 60 {
		waitSec = 60
	}

	deadline := time.Now().Add(time.Duration(waitSec) * time.Second)
	for time.Now().Before(deadline) {
		read, ok := h.exec.ExecutionRead(procID, 0, 256*1024)
		if ok && read.Status != "running" {
			break
		}
		select {
		case <-r.Context().Done():
			break
		case <-time.After(25 * time.Millisecond):
		}
	}

	read, ok := h.exec.ExecutionRead(procID, 0, 256*1024)
	if !ok {
		res.Result = map[string]any{"processId": procID, "status": "running", "chunks": []any{}}
		return
	}

	res.Result = map[string]any{
		"processId":  procID,
		"status":     read.Status,
		"chunks":     read.Chunks,
		"nextCursor": read.NextCursor,
		"exitCode":   read.ExitCode,
	}
}

type mcpStatusParams struct {
	ProcessID   string `json:"processId"`
	Cursor      uint64 `json:"cursor,omitempty"`
	Offset      uint64 `json:"offset,omitempty"`
	WaitSeconds uint64 `json:"waitSeconds,omitempty"`
}

func (h *McpHandler) handleProcessStatus(r *http.Request, rawParams json.RawMessage, res *McpResponse) {
	if h.exec == nil {
		res.Error = map[string]any{"code": -32603, "message": "execution control plane is unavailable"}
		return
	}

	var params mcpStatusParams
	if len(rawParams) > 0 {
		_ = json.Unmarshal(rawParams, &params)
	}

	cursor := params.Cursor
	if cursor == 0 && params.Offset > 0 {
		cursor = params.Offset
	}

	waitSec := params.WaitSeconds
	if waitSec > 60 {
		waitSec = 60
	}

	if waitSec > 0 {
		deadline := time.Now().Add(time.Duration(waitSec) * time.Second)
		for time.Now().Before(deadline) {
			read, ok := h.exec.ExecutionRead(params.ProcessID, cursor, 256*1024)
			if ok && (len(read.Chunks) > 0 || read.Status != "running") {
				break
			}
			select {
			case <-r.Context().Done():
				break
			case <-time.After(50 * time.Millisecond):
			}
		}
	}

	read, ok := h.exec.ExecutionRead(params.ProcessID, cursor, 256*1024)
	if !ok {
		res.Error = map[string]any{"code": -32602, "message": "process not found"}
		return
	}

	res.Result = map[string]any{
		"processId":  params.ProcessID,
		"status":     read.Status,
		"chunks":     read.Chunks,
		"nextCursor": read.NextCursor,
		"more":       read.More,
		"exitCode":   read.ExitCode,
	}
}

func (h *McpHandler) handleProcessInput(rawParams json.RawMessage, res *McpResponse) {
	if h.exec == nil {
		res.Error = map[string]any{"code": -32603, "message": "execution control plane is unavailable"}
		return
	}

	var params struct {
		ProcessID string `json:"processId"`
		Data      string `json:"data"`
	}
	if len(rawParams) > 0 {
		_ = json.Unmarshal(rawParams, &params)
	}

	if err := h.exec.Input(params.ProcessID, []byte(params.Data)); err != nil {
		res.Error = map[string]any{"code": -32602, "message": err.Error()}
		return
	}
	res.Result = map[string]any{"accepted": true}
}

func (h *McpHandler) handleProcessCancel(rawParams json.RawMessage, res *McpResponse) {
	if h.exec == nil {
		res.Error = map[string]any{"code": -32603, "message": "execution control plane is unavailable"}
		return
	}

	var params struct {
		ProcessID string `json:"processId"`
		Signal    string `json:"signal,omitempty"`
	}
	if len(rawParams) > 0 {
		_ = json.Unmarshal(rawParams, &params)
	}

	sig := params.Signal
	if sig == "" {
		sig = "TERM"
	}

	if err := h.exec.Signal(params.ProcessID, sig); err != nil {
		res.Error = map[string]any{"code": -32602, "message": err.Error()}
		return
	}
	res.Result = map[string]any{"accepted": true}
}
