package node

import "github.com/OhRats-Technologies/rc/pkg/protocol"

func (m *HostedMCP) sendStatus(requestID, processID string, read *ExecutionRead, failure error) {
	message := protocol.NodeToServer{
		Type: "mcp_status_result", RequestID: requestID, ProcessID: processID,
		Status: "lost", Chunks: []protocol.McpExecutionChunk{},
	}
	if failure != nil {
		message.Error = failure.Error()
		m.sendMessage(message)
		return
	}
	if read == nil {
		message.Error = "process state is no longer available"
		m.sendMessage(message)
		return
	}
	message.Status = read.Status
	message.NextCursor = read.NextCursor
	message.TruncatedBeforeCursor = read.TruncatedBefore
	message.OutputPending = read.More
	message.Signal = read.Signal
	if read.ExitCode != nil {
		message.ExitCode = *read.ExitCode
	}
	for _, chunk := range read.Chunks {
		encoding, data := encodeNodeOutput(chunk.Bytes)
		message.Chunks = append(message.Chunks, protocol.McpExecutionChunk{
			Stream: string(chunk.Stream), Cursor: chunk.Cursor, Encoding: encoding, Data: data,
		})
	}
	m.sendMessage(message)
}

func (m *HostedMCP) sendOperation(requestID, processID string, accepted bool) {
	m.sendMessage(protocol.NodeToServer{
		Type: "mcp_operation_result", RequestID: requestID, ProcessID: processID, Accepted: accepted,
	})
}
