package server

import (
	"fmt"
	"strings"

	authstore "github.com/OhRats-Technologies/rc/pkg/auth"
)

func mcpToolError(message string) map[string]any {
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": message}},
		"isError": true,
	}
}

func mcpToolText(text string, structured any, isError bool) map[string]any {
	result := map[string]any{
		"content":           []map[string]any{{"type": "text", "text": text}},
		"structuredContent": structured,
	}
	if isError {
		result["isError"] = true
	}
	return result
}

func registeredMCPScope(name string) string {
	switch name {
	case "machines_list", "process_status":
		return authstore.OAuthScopeObserve
	case "image_view", "process_run", "process_input", "process_cancel":
		return authstore.OAuthScopeTerminal
	default:
		return ""
	}
}

func hasAnyMCPScope(grant *authstore.OAuthGrant) bool {
	return grant.HasScope(authstore.OAuthScopeObserve) || grant.HasScope(authstore.OAuthScopeTerminal)
}

func isNotification(method string) bool {
	return strings.HasPrefix(method, "notifications/")
}

func validateMCPProtocolVersion(value string) error {
	value = strings.TrimSpace(value)
	if value == "" || supportedMCPVersion(value) {
		return nil
	}
	return fmt.Errorf("Unsupported protocol version: %s", value)
}

func supportedMCPVersion(value string) bool {
	for _, version := range mcpSupportedVersions {
		if version == value {
			return true
		}
	}
	return false
}
