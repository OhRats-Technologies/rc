package node

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"unicode/utf8"
)

func hasGrantScope(scopes []string, required string) bool {
	for _, scope := range scopes {
		if scope == "mcp" || scope == required {
			return true
		}
		if required == "mcp:observe" && scope == "mcp:terminal" {
			return true
		}
	}
	return false
}

func encodeNodeOutput(data []byte) (string, string) {
	if utf8.Valid(data) {
		return "text", string(data)
	}
	return "base64", base64.StdEncoding.EncodeToString(data)
}

func nodeImageMimeType(data []byte) string {
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

func minU64(value, maximum uint64) uint64 {
	if value > maximum {
		return maximum
	}
	return value
}
