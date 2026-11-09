package node

import (
	"runtime"

	"github.com/OhRats-Technologies/rc/pkg/protocol"
)

// terminalInput applies the minimal line discipline a pipe-backed interactive
// shell needs. xterm sends CR for Enter, Windows console shells expect CRLF,
// and Unix shells expect LF. Non-terminal and exact-byte streams are untouched.
func terminalInput(spec ProcessSpec, data []byte) []byte {
	if spec.Terminal == nil || spec.Mode.Kind != protocol.ExecutionModeSystemLoginShell {
		return data
	}
	return normalizeTerminalInput(data, runtime.GOOS == "windows")
}

func normalizeTerminalInput(data []byte, crlf bool) []byte {
	out := make([]byte, 0, len(data)+8)
	for i := 0; i < len(data); i++ {
		value := data[i]
		if value != '\r' {
			out = append(out, value)
			continue
		}
		if crlf {
			out = append(out, '\r')
			if i+1 < len(data) && data[i+1] == '\n' {
				continue
			}
			out = append(out, '\n')
			continue
		}
		if i+1 < len(data) && data[i+1] == '\n' {
			i++
		}
		out = append(out, '\n')
	}
	return out
}
