package apiclient

import "github.com/OhRats-Technologies/rc/pkg/protocol"

type Device struct {
	ID                 string   `json:"id"`
	WorkspaceID        string   `json:"workspace_id,omitempty"`
	Name               string   `json:"name"`
	Workspace          string   `json:"workspace_name,omitempty"`
	Version            string   `json:"version,omitempty"`
	Capabilities       []string `json:"capabilities,omitempty"`
	Online             bool     `json:"online,omitempty"`
	IdentityPublicKey  string   `json:"identity_public_key,omitempty"`
	TransportPublicKey string   `json:"transport_public_key,omitempty"`
}

func (d *Device) Supports(capability string) bool {
	for _, c := range d.Capabilities {
		if c == capability {
			return true
		}
	}
	return false
}

type ControlReady struct {
	SessionID          string                       `json:"sessionId"`
	TransportPublicKey string                       `json:"transportPublicKey"`
	EphemeralPublicKey string                       `json:"ephemeralPublicKey"`
	Signature          string                       `json:"signature"`
	IceServers         []protocol.IceServer         `json:"iceServers,omitempty"`
	Attempts           []protocol.ControlIceAttempt `json:"attempts"`
}

type ControlChallenge struct {
	Challenge string `json:"challenge"`
}

type WebRtcAnswer struct {
	SDP string `json:"sdp"`
}

type ControlOpen struct {
	DeviceID  string `json:"deviceId"`
	Challenge string `json:"challenge"`
	ClientID  string `json:"clientId"`
	PublicKey string `json:"publicKey"`
	Signature string `json:"signature"`
}
