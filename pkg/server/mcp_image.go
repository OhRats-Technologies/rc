package server

import (
	"bytes"
	"context"

	"encoding/base64"
	"encoding/json"
	authstore "github.com/OhRats-Technologies/rc/pkg/auth"
	"io"
	"net/http"
	"os"
	"strings"
)

const maxMCPImageBytes = 8 << 20

func (h *McpHandler) callImageView(ctx context.Context, raw json.RawMessage, grant *authstore.OAuthGrant) map[string]any {
	var args struct {
		DeviceID string `json:"deviceId"`
		Path     string `json:"path"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return mcpToolError("invalid image_view arguments")
	}
	if err := h.authorizeDevice(grant.UserID, args.DeviceID, true); err != nil {
		return mcpToolError(err.Error())
	}
	path := strings.TrimSpace(args.Path)
	if !h.local {
		return h.remoteImageView(ctx, args.DeviceID, path, grant)
	}
	if path == "" {
		return mcpToolError("path must not be empty")
	}
	file, err := os.Open(path)
	if err != nil {
		return mcpToolError("image could not be read")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxMCPImageBytes+1))
	if err != nil {
		return mcpToolError("image could not be read")
	}
	if len(data) > maxMCPImageBytes {
		return mcpToolError("image exceeds the 8 MiB limit")
	}
	mimeType := imageMimeType(data)
	if mimeType == "" {
		return mcpToolError("only PNG, JPEG, WebP, and GIF images are supported")
	}
	return map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": "Image from " + path + " (" + mimeType + ", " + itoa(len(data)) + " bytes)."},
			{"type": "image", "data": base64.StdEncoding.EncodeToString(data), "mimeType": mimeType},
		},
		"structuredContent": map[string]any{
			"deviceId":  args.DeviceID,
			"path":      path,
			"mimeType":  mimeType,
			"sizeBytes": len(data),
		},
	}
}

func imageMimeType(data []byte) string {
	mimeType := http.DetectContentType(data)
	if mimeType == "image/png" || mimeType == "image/jpeg" ||
		mimeType == "image/webp" || mimeType == "image/gif" {
		return mimeType
	}
	if len(data) >= 12 && bytes.HasPrefix(data, []byte("RIFF")) && string(data[8:12]) == "WEBP" {
		return "image/webp"
	}
	return ""
}
