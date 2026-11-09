package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	authstore "github.com/OhRats-Technologies/rc/pkg/auth"
	"github.com/OhRats-Technologies/rc/pkg/node"
	"github.com/OhRats-Technologies/rc/pkg/protocol"
)

type mcpOutputChunk struct {
	Stream   string `json:"stream"`
	Cursor   uint64 `json:"cursor"`
	Encoding string `json:"encoding"`
	Data     string `json:"data"`
}

type mcpProcessResult struct {
	ProcessID             string           `json:"processId"`
	Status                string           `json:"status"`
	Chunks                []mcpOutputChunk `json:"chunks"`
	ExitCode              *int32           `json:"exitCode"`
	Signal                *string          `json:"signal"`
	Error                 *string          `json:"error"`
	NextCursor            uint64           `json:"nextCursor"`
	OutputPending         bool             `json:"outputPending"`
	TruncatedBeforeCursor uint64           `json:"truncatedBeforeCursor"`
}

func (h *McpHandler) callProcessStatus(ctx context.Context, raw json.RawMessage, grant *authstore.OAuthGrant) map[string]any {
	var args struct {
		DeviceID    string  `json:"deviceId"`
		ProcessID   string  `json:"processId"`
		Cursor      uint64  `json:"cursor"`
		Offset      uint64  `json:"offset"`
		WaitSeconds *uint64 `json:"waitSeconds"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return mcpToolError("invalid process_status arguments")
	}
	if args.ProcessID == "" {
		return mcpToolError("processId is required")
	}
	if err := h.authorizeDevice(grant.UserID, args.DeviceID, false); err != nil {
		return mcpToolError(err.Error())
	}
	if h.local {
		if _, ok := h.processBinding(args.ProcessID, args.DeviceID, grant); !ok {
			return mcpToolError("process not found")
		}
	}
	cursor := args.Cursor
	if cursor == 0 && args.Offset > 0 {
		cursor = args.Offset
	}
	var wait uint64
	if args.WaitSeconds != nil {
		wait = *args.WaitSeconds
	}
	if !h.local {
		return h.remoteProcessStatus(ctx, args.DeviceID, args.ProcessID, cursor, wait, grant)
	}
	if h.exec == nil {
		return mcpToolError("execution control plane is unavailable")
	}
	return h.processResult(args.ProcessID, args.DeviceID, grant, cursor, wait, ctx, false)
}

func (h *McpHandler) callProcessInput(ctx context.Context, raw json.RawMessage, grant *authstore.OAuthGrant) map[string]any {
	var args struct {
		DeviceID  string `json:"deviceId"`
		ProcessID string `json:"processId"`
		Data      string `json:"data"`
		Encoding  string `json:"encoding"`
		EOF       bool   `json:"eof"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return mcpToolError("invalid process_input arguments")
	}
	if err := h.authorizeDevice(grant.UserID, args.DeviceID, true); err != nil {
		return mcpToolError(err.Error())
	}
	if h.local {
		if _, ok := h.processBinding(args.ProcessID, args.DeviceID, grant); !ok {
			return mcpToolError("process not found")
		}
	}
	if args.Data == "" && !args.EOF {
		return mcpToolError("data or eof is required")
	}
	data, err := decodeProcessInput(args.Data, args.Encoding)
	if err != nil {
		return mcpToolError(err.Error())
	}
	if len(data) > protocol.ProcessInputChunkLimit {
		return mcpToolError("process input chunk exceeds the transport limit")
	}
	if !h.local {
		return h.remoteProcessInput(ctx, args.DeviceID, args.ProcessID, string(data), args.EOF, grant)
	}
	if h.exec == nil {
		return mcpToolError("execution control plane is unavailable")
	}
	if len(data) > 0 {
		if err := h.exec.Input(args.ProcessID, data); err != nil {
			return mcpToolError("process input was rejected")
		}
	}
	if args.EOF {
		if err := h.exec.CloseInput(args.ProcessID); err != nil {
			return mcpToolError("process stdin could not be closed")
		}
	}
	return mcpToolText("accepted "+itoa(len(data))+" bytes", map[string]any{
		"deviceId":      args.DeviceID,
		"processId":     args.ProcessID,
		"acceptedBytes": len(data),
		"stdinClosed":   args.EOF,
	}, false)
}

func (h *McpHandler) callProcessCancel(ctx context.Context, raw json.RawMessage, grant *authstore.OAuthGrant) map[string]any {
	var args struct {
		DeviceID  string `json:"deviceId"`
		ProcessID string `json:"processId"`
		Signal    string `json:"signal"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return mcpToolError("invalid process_cancel arguments")
	}
	if err := h.authorizeDevice(grant.UserID, args.DeviceID, true); err != nil {
		return mcpToolError(err.Error())
	}
	if h.local {
		if _, ok := h.processBinding(args.ProcessID, args.DeviceID, grant); !ok {
			return mcpToolError("process not found")
		}
	}
	signal := strings.ToUpper(strings.TrimSpace(args.Signal))
	if signal == "" {
		signal = "TERM"
	}
	if signal != "INT" && signal != "TERM" && signal != "KILL" {
		return mcpToolError("signal must be INT, TERM, or KILL")
	}
	if !h.local {
		return h.remoteProcessCancel(ctx, args.DeviceID, args.ProcessID, signal, grant)
	}
	if h.exec == nil {
		return mcpToolError("execution control plane is unavailable")
	}
	if err := h.exec.Signal(args.ProcessID, signal); err != nil {
		return mcpToolError("process cancellation was rejected")
	}
	return mcpToolText("requested "+signal, map[string]any{
		"processId": args.ProcessID,
		"signal":    signal,
		"accepted":  true,
	}, false)
}

func (h *McpHandler) processResult(
	processID, deviceID string,
	grant *authstore.OAuthGrant,
	cursor, waitSeconds uint64,
	ctx context.Context,
	waitForExit bool,
) map[string]any {
	if _, ok := h.processBinding(processID, deviceID, grant); !ok {
		return mcpToolError("process not found")
	}
	waitSeconds = minUint64(waitSeconds, 60)
	if waitSeconds > 0 {
		deadline := time.Now().Add(time.Duration(waitSeconds) * time.Second)
	waitLoop:
		for time.Now().Before(deadline) {
			read, ok := h.exec.ExecutionRead(processID, cursor, 256*1024)
			if !ok || read.Status != "running" || (!waitForExit && len(read.Chunks) > 0) {
				break
			}
			select {
			case <-ctx.Done():
				break waitLoop
			case <-time.After(25 * time.Millisecond):
			}
		}
	}
	read, ok := h.exec.ExecutionRead(processID, cursor, 256*1024)
	if !ok {
		reason := "process state is no longer available"
		return mcpToolText(reason, mcpProcessResult{
			ProcessID: processID,
			Status:    "lost",
			Chunks:    []mcpOutputChunk{},
			Error:     &reason,
		}, true)
	}
	result := mcpProcessResultFromRead(processID, read)
	return mcpToolText(processResultText(result), result, processResultFailed(result))
}

func mcpProcessResultFromRead(processID string, read *node.ExecutionRead) mcpProcessResult {
	status := read.Status
	if status == "completed" {
		status = "exited"
	}
	if status != "running" && status != "exited" {
		status = "lost"
	}
	result := mcpProcessResult{
		ProcessID:             processID,
		Status:                status,
		Chunks:                make([]mcpOutputChunk, 0, len(read.Chunks)),
		ExitCode:              read.ExitCode,
		NextCursor:            read.NextCursor,
		OutputPending:         read.More,
		TruncatedBeforeCursor: read.TruncatedBefore,
	}
	if read.Signal != "" {
		signal := read.Signal
		result.Signal = &signal
	}
	for _, chunk := range read.Chunks {
		value := encodeMCPOutput(chunk.Bytes)
		last := len(result.Chunks) - 1
		if last >= 0 && result.Chunks[last].Stream == string(chunk.Stream) &&
			result.Chunks[last].Encoding == value.encoding {
			result.Chunks[last].Data += value.data
			result.Chunks[last].Cursor = chunk.Cursor
			continue
		}
		result.Chunks = append(result.Chunks, mcpOutputChunk{
			Stream:   string(chunk.Stream),
			Cursor:   chunk.Cursor,
			Encoding: value.encoding,
			Data:     value.data,
		})
	}
	return result
}

type encodedOutput struct {
	encoding string
	data     string
}

func encodeMCPOutput(data []byte) encodedOutput {
	if utf8.Valid(data) {
		return encodedOutput{encoding: "text", data: string(data)}
	}
	return encodedOutput{encoding: "base64", data: base64.StdEncoding.EncodeToString(data)}
}

func processResultText(result mcpProcessResult) string {
	var text strings.Builder
	for _, chunk := range result.Chunks {
		if chunk.Stream == "stderr" {
			text.WriteString("[stderr] ")
		}
		if chunk.Encoding == "text" {
			text.WriteString(chunk.Data)
		} else {
			text.WriteString("[base64 output: " + chunk.Data + "]")
		}
	}
	if result.TruncatedBeforeCursor > 0 {
		text.WriteString("\n[Earlier output before cursor " + utoa(result.TruncatedBeforeCursor) + " is no longer buffered.]")
	}
	if text.Len() > 0 && !strings.HasSuffix(text.String(), "\n") {
		text.WriteByte('\n')
	}
	switch result.Status {
	case "running":
		text.WriteString("Process " + result.ProcessID + " is still running.")
	case "exited":
		if result.ExitCode != nil {
			text.WriteString("Exit " + itoa(int(*result.ExitCode)) + ".")
		} else {
			text.WriteString("Process exited.")
		}
	default:
		text.WriteString("Process state is lost.")
		if result.Error != nil {
			text.WriteString(" " + *result.Error + ".")
		}
	}
	if result.OutputPending {
		text.WriteString(" More buffered output is available at nextCursor.")
	}
	return text.String()
}
