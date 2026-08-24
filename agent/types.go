package main

const version = "0.2.0"

type state struct {
	DeviceID   string `json:"deviceId"`
	PrivateKey string `json:"privateKey"`
}

type enrollRequest struct {
	Token        string   `json:"token"`
	Name         string   `json:"name"`
	Hostname     string   `json:"hostname"`
	Platform     string   `json:"platform"`
	Arch         string   `json:"arch"`
	PublicKey    string   `json:"publicKey"`
	AgentVersion string   `json:"agentVersion"`
	Capabilities []string `json:"capabilities"`
}

type enrollResponse struct {
	DeviceID string `json:"deviceId"`
}

type wireMessage struct {
	Type     string `json:"type"`
	ID       string `json:"id,omitempty"`
	Command  string `json:"command,omitempty"`
	Output   string `json:"output,omitempty"`
	ExitCode int    `json:"exitCode"`
}
