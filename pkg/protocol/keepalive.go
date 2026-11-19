package protocol

import "time"

// TypeKeepalive is an empty control-channel message. Proxies such as Cloudflare close a WebSocket
// that carries no traffic for about two minutes, so both sides send it while the channel is idle.
const TypeKeepalive = "keepalive"

// CapabilityKeepalive is announced in NodeHello by a Node that sends keepalives. The server enforces
// IdleTimeout only for such Nodes, so older Nodes that send none are not disconnected.
const CapabilityKeepalive = "keepalive"

const (
	// KeepaliveInterval is how often each side sends TypeKeepalive on an idle control channel.
	KeepaliveInterval = 25 * time.Second
	// IdleTimeout closes a keepalive-capable control channel that receives no frame for this long.
	IdleTimeout = 90 * time.Second
)
