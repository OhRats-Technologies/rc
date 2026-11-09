package server

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	authstore "github.com/OhRats-Technologies/rc/pkg/auth"
	"github.com/OhRats-Technologies/rc/pkg/node"
	"github.com/OhRats-Technologies/rc/pkg/protocol"
)

func hasCapability(values []string, required string) bool {
	for _, value := range values {
		if value == required {
			return true
		}
	}
	return false
}

func (h *McpHandler) nodeReady(deviceID string) error {
	device, ok := h.registry.Get(deviceID)
	if !ok {
		return authstore.ErrDeviceNotFound
	}
	if !hasCapability(device.Capabilities, "mcp-account-v2") {
		return errors.New("Node upgrade required for account-wide MCP access; run rc upgrade")
	}
	if !hasCapability(device.Capabilities, "execution-v2") {
		return errors.New("Node upgrade required for execution-v2; run rc upgrade")
	}
	if h.authorityHash == nil {
		return nil
	}
	hash, err := h.authorityHash(device.WorkspaceID)
	if err != nil {
		return err
	}
	if device.LockHash != hash {
		return fmt.Errorf("MCP grant is not synchronized with this Node; open RC and run Sync device access for workspace %s", device.WorkspaceID)
	}
	return nil
}

// mcpGrant returns the durable Owner-authorized grant for this OAuth token.
func (h *McpHandler) mcpGrant(grant *authstore.OAuthGrant) (*authstore.McpGrantRecord, error) {
	if h.auth == nil || grant.GrantID == "" {
		return nil, authstore.ErrOAuthInvalidGrant
	}
	return h.auth.McpGrant(grant.UserID, grant.GrantID)
}

func (h *McpHandler) remoteProcessRun(ctx context.Context, deviceID string, spec node.ProcessSpec, grant *authstore.OAuthGrant, wait uint64) map[string]any {
	if h.nodes == nil || !h.nodes.Online(deviceID) {
		return mcpToolError("Node is offline")
	}
	if err := h.nodeReady(deviceID); err != nil {
		return mcpToolError(err.Error())
	}
	record, err := h.mcpGrant(grant)
	if err != nil {
		return mcpToolError("MCP grant is not authorized")
	}
	timeout := time.Duration(wait+10) * time.Second
	message := protocol.ServerToNode{
		Type:              "mcp_start",
		ProcessID:         spec.ID,
		UserID:            grant.UserID,
		ExecMode:          &spec.Mode,
		Cwd:               spec.Cwd,
		Environment:       spec.Environment,
		MaxRuntimeSeconds: secondsFromMillis(spec.MaxRuntimeMs),
		WaitSeconds:       wait,
		McpGrant:          record.Grant,
		McpSignature:      record.Signature,
		ControlGrant:      record.ControlGrant,
		CredentialID:      record.CredentialID,
		ControlAssertion:  record.ControlAssertion,
	}
	h.bindProcess(spec.ID, grant, deviceID)
	response, err := h.nodes.Request(ctx, deviceID, message, timeout)
	if err != nil {
		reason := "Node command failed: " + err.Error()
		result := mcpProcessResult{ProcessID: spec.ID, Status: "lost", Chunks: []mcpOutputChunk{}, Error: &reason}
		return mcpToolText(processResultText(result), result, true)
	}
	result := mcpResultFromNode(response, spec.ID)
	return mcpToolText(processResultText(result), result, processResultFailed(result))
}

func (h *McpHandler) remoteProcessStatus(ctx context.Context, deviceID, processID string, cursor, wait uint64, grant *authstore.OAuthGrant) map[string]any {
	if h.nodes == nil || !h.nodes.Online(deviceID) {
		reason := "Node is offline"
		result := mcpProcessResult{ProcessID: processID, Status: "lost", Chunks: []mcpOutputChunk{}, Error: &reason}
		return mcpToolText(processResultText(result), result, true)
	}
	if err := h.nodeReady(deviceID); err != nil {
		reason := err.Error()
		result := mcpProcessResult{ProcessID: processID, Status: "lost", Chunks: []mcpOutputChunk{}, Error: &reason}
		return mcpToolText(processResultText(result), result, true)
	}
	record, err := h.mcpGrant(grant)
	if err != nil {
		return mcpToolError("MCP grant is not authorized")
	}
	response, err := h.nodes.Request(ctx, deviceID, protocol.ServerToNode{
		Type:             "mcp_status",
		ProcessID:        processID,
		Cursor:           cursor,
		WaitSeconds:      wait,
		McpGrant:         record.Grant,
		McpSignature:     record.Signature,
		ControlGrant:     record.ControlGrant,
		CredentialID:     record.CredentialID,
		ControlAssertion: record.ControlAssertion,
	}, time.Duration(wait+10)*time.Second)
	if err != nil {
		reason := "Node command failed: " + err.Error()
		result := mcpProcessResult{ProcessID: processID, Status: "lost", Chunks: []mcpOutputChunk{}, Error: &reason}
		return mcpToolText(processResultText(result), result, true)
	}
	result := mcpResultFromNode(response, processID)
	return mcpToolText(processResultText(result), result, processResultFailed(result))
}

func (h *McpHandler) remoteProcessInput(ctx context.Context, deviceID, processID, data string, eof bool, grant *authstore.OAuthGrant) map[string]any {
	if h.nodes == nil || !h.nodes.Online(deviceID) {
		return mcpToolError("Node is offline")
	}
	if err := h.nodeReady(deviceID); err != nil {
		return mcpToolError(err.Error())
	}
	record, err := h.mcpGrant(grant)
	if err != nil {
		return mcpToolError("MCP grant is not authorized")
	}
	response, err := h.nodes.Request(ctx, deviceID, protocol.ServerToNode{
		Type:             "mcp_input",
		ProcessID:        processID,
		Data:             base64.StdEncoding.EncodeToString([]byte(data)),
		EOF:              eof,
		McpGrant:         record.Grant,
		McpSignature:     record.Signature,
		ControlGrant:     record.ControlGrant,
		CredentialID:     record.CredentialID,
		ControlAssertion: record.ControlAssertion,
	}, 15*time.Second)
	if err != nil {
		return mcpToolError("Node command failed: " + err.Error())
	}
	if !response.Accepted {
		return mcpToolError("process input was rejected")
	}
	return mcpToolText("accepted", map[string]any{
		"deviceId": deviceID, "processId": processID,
		"acceptedBytes": len(data), "stdinClosed": eof,
	}, false)
}

func (h *McpHandler) remoteProcessCancel(ctx context.Context, deviceID, processID, signal string, grant *authstore.OAuthGrant) map[string]any {
	if h.nodes == nil || !h.nodes.Online(deviceID) {
		return mcpToolError("Node is offline")
	}
	if err := h.nodeReady(deviceID); err != nil {
		return mcpToolError(err.Error())
	}
	record, err := h.mcpGrant(grant)
	if err != nil {
		return mcpToolError("MCP grant is not authorized")
	}
	response, err := h.nodes.Request(ctx, deviceID, protocol.ServerToNode{
		Type:             "mcp_cancel",
		ProcessID:        processID,
		Signal:           signal,
		McpGrant:         record.Grant,
		McpSignature:     record.Signature,
		ControlGrant:     record.ControlGrant,
		CredentialID:     record.CredentialID,
		ControlAssertion: record.ControlAssertion,
	}, 15*time.Second)
	if err != nil {
		return mcpToolError("Node command failed: " + err.Error())
	}
	if !response.Accepted {
		return mcpToolError("process cancellation was rejected")
	}
	return mcpToolText("requested "+signal, map[string]any{
		"processId": processID, "signal": signal, "accepted": true,
	}, false)
}

func (h *McpHandler) remoteImageView(ctx context.Context, deviceID, path string, grant *authstore.OAuthGrant) map[string]any {
	if h.nodes == nil || !h.nodes.Online(deviceID) {
		return mcpToolError("Node is offline")
	}
	if err := h.nodeReady(deviceID); err != nil {
		return mcpToolError(err.Error())
	}
	record, err := h.mcpGrant(grant)
	if err != nil {
		return mcpToolError("MCP grant is not authorized")
	}
	image, err := h.nodes.Image(ctx, deviceID, path, record.Grant, record.Signature, record.ControlGrant, record.CredentialID, record.ControlAssertion)
	if err != nil {
		return mcpToolError("image could not be read from the Node: " + err.Error())
	}
	return map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": "Image from " + path + " (" + image.mimeType + ", " + itoa(len(image.data)) + " bytes)."},
			{"type": "image", "data": base64.StdEncoding.EncodeToString(image.data), "mimeType": image.mimeType},
		},
		"structuredContent": map[string]any{
			"deviceId": deviceID, "path": path, "mimeType": image.mimeType, "sizeBytes": len(image.data),
		},
	}
}

func mcpResultFromNode(message protocol.NodeToServer, processID string) mcpProcessResult {
	status := message.Status
	switch status {
	case "running":
	case "completed", "exited":
		status = "exited"
	default:
		status = "lost"
	}
	result := mcpProcessResult{
		ProcessID:             processID,
		Status:                status,
		Chunks:                make([]mcpOutputChunk, 0, len(message.Chunks)),
		NextCursor:            message.NextCursor,
		OutputPending:         message.OutputPending,
		TruncatedBeforeCursor: message.TruncatedBeforeCursor,
	}
	if status != "running" {
		exit := message.ExitCode
		result.ExitCode = &exit
	}
	if message.Signal != "" {
		signal := message.Signal
		result.Signal = &signal
	}
	if message.Error != "" {
		text := message.Error
		result.Error = &text
	}
	for _, chunk := range message.Chunks {
		last := len(result.Chunks) - 1
		if last >= 0 && result.Chunks[last].Stream == chunk.Stream &&
			result.Chunks[last].Encoding == chunk.Encoding {
			result.Chunks[last].Data += chunk.Data
			result.Chunks[last].Cursor = chunk.Cursor
			continue
		}
		result.Chunks = append(result.Chunks, mcpOutputChunk{
			Stream:   chunk.Stream,
			Cursor:   chunk.Cursor,
			Encoding: chunk.Encoding,
			Data:     chunk.Data,
		})
	}
	return result
}

func secondsFromMillis(value *uint64) *uint64 {
	if value == nil {
		return nil
	}
	seconds := *value / 1000
	return &seconds
}
