package node

import (
	"encoding/base64"
	"io"
	"os"
	"strings"

	"github.com/OhRats-Technologies/rc/pkg/protocol"
)

func (m *HostedMCP) image(message protocol.ServerToNode) {
	if _, err := m.verify(message, "mcp:terminal"); err != nil {
		m.sendMessage(protocol.NodeToServer{Type: "mcp_image_result", RequestID: message.RequestID, Error: err.Error()})
		return
	}
	path := strings.TrimSpace(message.Path)
	if path == "" {
		m.sendMessage(protocol.NodeToServer{Type: "mcp_image_result", RequestID: message.RequestID, Error: "path must not be empty"})
		return
	}
	file, err := os.Open(path)
	if err != nil {
		m.sendMessage(protocol.NodeToServer{Type: "mcp_image_result", RequestID: message.RequestID, Error: "image could not be read"})
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, int64(protocol.McpImageMaxBytes)+1))
	if err != nil || len(data) > protocol.McpImageMaxBytes {
		m.sendMessage(protocol.NodeToServer{Type: "mcp_image_result", RequestID: message.RequestID, Error: "image exceeds the 8 MiB limit"})
		return
	}
	mimeType := nodeImageMimeType(data)
	if mimeType == "" {
		m.sendMessage(protocol.NodeToServer{Type: "mcp_image_result", RequestID: message.RequestID, Error: "unsupported image type"})
		return
	}
	for start := 0; start < len(data); start += protocol.McpImageChunkBytes {
		end := start + protocol.McpImageChunkBytes
		if end > len(data) {
			end = len(data)
		}
		if err := m.sendMessage(protocol.NodeToServer{
			Type: "mcp_image_chunk", RequestID: message.RequestID,
			Data: base64.RawURLEncoding.EncodeToString(data[start:end]),
		}); err != nil {
			return
		}
	}
	m.sendMessage(protocol.NodeToServer{
		Type: "mcp_image_result", RequestID: message.RequestID,
		MimeType: mimeType, SizeBytes: uint64(len(data)),
	})
}
