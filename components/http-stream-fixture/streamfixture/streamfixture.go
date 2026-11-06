// Package streamfixture implements the deterministic HTTP stream provider used
// to exercise kernel streaming behaviour end to end: incremental chunks,
// pending deadlines, endless and oversized producers, provider failures, and
// close accounting.
package streamfixture

import (
	"bytes"
	"strconv"
	"sync"

	witTypes "go.bytecodealliance.org/pkg/wit/types"
	"wit_component/ohrats_rc_http_stream_handler"
	"wit_component/ohrats_rc_http_types"
)

// Generation identifies this fixture build. The removed Rust fixture selected
// a "replacement" generation with `--cfg rc_stream_replacement`; the Go
// component has no equivalent build configuration, so it always reports the
// default "old" generation.
const Generation = "old"

const (
	eventStreamContentType = "text/event-stream"
	oversizedChunkBytes    = 65_537
	totalChunkBytes        = 32
)

type session struct {
	id   string
	kind string
	step uint64
}

type state struct {
	next     uint64
	sessions []session
	closed   uint64
}

var global = struct {
	mutex sync.Mutex
	state state
}{}

type (
	openResult = witTypes.Result[witTypes.Option[ohrats_rc_http_stream_handler.OpenedResponse], string]
	pollResult = witTypes.Result[ohrats_rc_http_stream_handler.PollResult, string]
	opened     = witTypes.Option[ohrats_rc_http_stream_handler.OpenedResponse]
)

// Open registers a session for each recognized fixture stream path. Unknown
// paths are left to later providers.
func Open(request ohrats_rc_http_types.Request) openResult {
	kind := request.Path
	switch kind {
	case "/events", "/generation", "/drain", "/slow", "/endless", "/total", "/oversized", "/failure", "/closed":
	default:
		return witTypes.Ok[opened, string](witTypes.None[ohrats_rc_http_stream_handler.OpenedResponse]())
	}

	global.mutex.Lock()
	defer global.mutex.Unlock()
	global.state.next++
	id := strconv.FormatUint(global.state.next, 10)
	global.state.sessions = append(global.state.sessions, session{id: id, kind: kind})
	return witTypes.Ok[opened, string](witTypes.Some(ohrats_rc_http_stream_handler.OpenedResponse{
		Status: 200,
		Headers: []ohrats_rc_http_types.Header{{
			Name:  "content-type",
			Value: eventStreamContentType,
		}},
		SessionId: id,
	}))
}

// Poll advances one session. Step counters persist across polls so every
// stream has a fixed, deterministic chunk sequence.
func Poll(id string) pollResult {
	global.mutex.Lock()
	defer global.mutex.Unlock()

	closed := global.state.closed
	index := -1
	for i := range global.state.sessions {
		if global.state.sessions[i].id == id {
			index = i
			break
		}
	}
	if index < 0 {
		return witTypes.Err[ohrats_rc_http_stream_handler.PollResult, string]("unknown session")
	}

	current := global.state.sessions[index].step
	global.state.sessions[index].step = current + 1
	switch global.state.sessions[index].kind {
	case "/generation":
		if current == 0 {
			return chunk("data: " + Generation + "\n\n")
		}
	case "/drain":
		switch current {
		case 0:
			return pending(1_000)
		case 1:
			return chunk("data: drained\n\n")
		}
	case "/events":
		switch current {
		case 0:
			return chunk("id: 1\ndata: first\n\n")
		case 1:
			return pending(40)
		case 2:
			return chunk(": heartbeat\n\n")
		case 3:
			return chunk("id: 2\ndata: second\n\n")
		}
	case "/slow":
		switch current {
		case 0:
			return pending(250)
		case 1:
			return chunk("data: slow\n\n")
		}
	case "/endless":
		return pending(25)
	case "/total":
		return bytesChunk(bytes.Repeat([]byte{'t'}, totalChunkBytes))
	case "/oversized":
		if current == 0 {
			return bytesChunk(bytes.Repeat([]byte{'x'}, oversizedChunkBytes))
		}
	case "/failure":
		if current == 0 {
			return chunk("data: before-error\n\n")
		}
		return witTypes.Err[ohrats_rc_http_stream_handler.PollResult, string]("intentional poll failure")
	case "/closed":
		if current == 0 {
			return chunk("data: " + strconv.FormatUint(closed, 10) + "\n\n")
		}
	}
	return done()
}

// Close drops the session and counts every close, even for unknown ids.
func Close(id string, _ ohrats_rc_http_stream_handler.CloseReason) {
	global.mutex.Lock()
	defer global.mutex.Unlock()

	retained := global.state.sessions[:0]
	for _, value := range global.state.sessions {
		if value.id != id {
			retained = append(retained, value)
		}
	}
	global.state.sessions = retained
	global.state.closed++
}

func chunk(value string) pollResult {
	return bytesChunk([]byte(value))
}

func bytesChunk(value []byte) pollResult {
	return witTypes.Ok[ohrats_rc_http_stream_handler.PollResult, string](
		ohrats_rc_http_stream_handler.MakePollResultChunk(value),
	)
}

func pending(milliseconds uint32) pollResult {
	return witTypes.Ok[ohrats_rc_http_stream_handler.PollResult, string](
		ohrats_rc_http_stream_handler.MakePollResultPending(milliseconds),
	)
}

func done() pollResult {
	return witTypes.Ok[ohrats_rc_http_stream_handler.PollResult, string](
		ohrats_rc_http_stream_handler.MakePollResultDone(),
	)
}
