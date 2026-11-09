package protocol

// Control signaling message types carried by the authenticated Node WebSocket.
const (
	TypeControlChallenge = "control_challenge"
	TypeControlOpen      = "control_open"
	TypeControlWebRTC    = "control_webrtc"
	TypeControlClose     = "control_close"
	TypeControlClosed    = "control_closed"
	TypeControlError     = "control_error"
	TypeProcessState     = "process_state"
)

// TypeControlFrame is the WebRTC DataChannel frame type.
const TypeControlFrame = "control.frame"
