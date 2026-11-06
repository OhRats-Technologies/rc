package protocol

import (
	"fmt"
	"strings"
)

type IceServer struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username,omitempty"`
	Credential string   `json:"credential,omitempty"`
}

type ControlIceMode string

const (
	ControlIceModeHost  ControlIceMode = "host"
	ControlIceModeStun  ControlIceMode = "stun"
	ControlIceModeRelay ControlIceMode = "relay"
)

type ControlRouteClass string

const (
	ControlRouteClassDirectHost ControlRouteClass = "direct-host"
	ControlRouteClassDirectStun ControlRouteClass = "direct-stun"
	ControlRouteClassTurnRelay  ControlRouteClass = "turn-relay"
	ControlRouteClassUnknown    ControlRouteClass = "unknown"
)

type ControlIceAttempt struct {
	Mode             ControlIceMode    `json:"mode"`
	Route            ControlRouteClass `json:"route"`
	GatherTimeoutMs  uint32            `json:"gatherTimeoutMs"`
	ConnectTimeoutMs uint32            `json:"connectTimeoutMs"`
	RetryDelayMs     uint32            `json:"retryDelayMs"`
}

func ControlAttemptsPayload(attempts []ControlIceAttempt) string {
	parts := make([]string, len(attempts))
	for i, a := range attempts {
		parts[i] = fmt.Sprintf("%s:%s:%d:%d:%d", a.Mode, a.Route, a.GatherTimeoutMs, a.ConnectTimeoutMs, a.RetryDelayMs)
	}
	return strings.Join(parts, ",")
}

type NodeHello struct {
	Version            string   `json:"version"`
	Hostname           string   `json:"hostname"`
	Platform           string   `json:"platform"`
	Arch               string   `json:"arch"`
	Capabilities       []string `json:"capabilities,omitempty"`
	TransportPublicKey string   `json:"transportPublicKey"`
	LockHash           string   `json:"lockHash,omitempty"`
	LockGeneration     uint64   `json:"lockGeneration,omitempty"`
}

type McpExecutionChunk struct {
	Stream string `json:"stream"`
	Cursor uint64 `json:"cursor"`
	Data   string `json:"data"`
}
