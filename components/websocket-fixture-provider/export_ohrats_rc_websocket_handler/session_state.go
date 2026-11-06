package export_ohrats_rc_websocket_handler

import (
	"wit_component/ohrats_rc_net_byte_stream_host"
	"wit_component/ohrats_rc_websocket_types"
)

type sessionState struct {
	stream       *ohrats_rc_net_byte_stream_host.Connection
	pendingWrite []uint8
	queued       []ohrats_rc_websocket_types.Outbound
	closed       bool
}

// flushWrite drains as much of the pending write buffer as the stream accepts.
// An empty return value means the flush succeeded.
func flushWrite(state *sessionState) string {
	if len(state.pendingWrite) == 0 {
		return ""
	}
	result := state.stream.Write(state.pendingWrite)
	if result.IsErr() {
		return result.Err()
	}
	write := result.Ok()
	if write.Tag() != ohrats_rc_net_byte_stream_host.WriteResult_Accepted {
		return ""
	}
	count := write.Accepted()
	if count == 0 || count > uint64(len(state.pendingWrite)) {
		return "invalid write progress"
	}
	state.pendingWrite = state.pendingWrite[int(count):]
	return ""
}

// payloadLen reports the encoded payload length used for poll budgeting. The
// close-frame budget counts only the reason bytes, matching the fixture contract.
func payloadLen(frame ohrats_rc_websocket_types.Outbound) uint64 {
	switch frame.Tag() {
	case ohrats_rc_websocket_types.Outbound_Message:
		message := frame.Message()
		switch message.Tag() {
		case ohrats_rc_websocket_types.Message_Text:
			return uint64(len(message.Text()))
		case ohrats_rc_websocket_types.Message_Binary:
			return uint64(len(message.Binary()))
		case ohrats_rc_websocket_types.Message_Ping:
			return uint64(len(message.Ping()))
		case ohrats_rc_websocket_types.Message_Pong:
			return uint64(len(message.Pong()))
		}
	case ohrats_rc_websocket_types.Outbound_Close:
		return uint64(len(frame.Close().Reason))
	}
	return 0
}
