package protocol

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
	Stream   string `json:"stream"`
	Cursor   uint64 `json:"cursor"`
	Encoding string `json:"encoding"`
	Data     string `json:"data"`
}
