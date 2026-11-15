package protocol

// Control signaling message types carried by the authenticated Node WebSocket.
const (
	TypeControlChallenge = "control_challenge"
	TypeControlOpen      = "control_open"
	TypeControlClose     = "control_close"
	TypeControlClosed    = "control_closed"
	TypeControlError     = "control_error"
	TypeProcessState     = "process_state"
)

// TypeControlFrame is the encrypted control frame type relayed by the server
// between a browser/CLI client and its Node over authenticated WebSockets.
const TypeControlFrame = "control.frame"
