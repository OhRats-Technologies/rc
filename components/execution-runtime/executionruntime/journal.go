package executionruntime

import (
	processRuntime "wit_component/ohrats_rc_process_runtime"
	processTypes "wit_component/ohrats_rc_process_types"
)

// journal is the bounded, component-local output journal of one execution.
// Chunks are addressed by absolute byte cursor so a controller may reattach
// and resume reading. Bytes beyond the scrollback limit are dropped from the
// front and reported as `truncated-before`. Adjacent appends from the same
// stream coalesce while stdout/stderr boundaries stay distinct.
type journal struct {
	chunks    []processRuntime.OutputChunk
	next      uint64
	truncated uint64
	limit     int
	bytes     int
}

func newJournal(limit uint32) *journal {
	return &journal{limit: int(limit)}
}

// push appends one stream read. The bytes are copied because host read results
// point into reusable linear memory.
func (j *journal) push(stream processTypes.StreamKind, value []uint8) {
	if len(value) == 0 {
		return
	}
	cursor := j.next
	j.next = saturatingAdd(j.next, uint64(len(value)))
	if len(value) > j.limit {
		skipped := len(value) - j.limit
		value = value[skipped:]
		cursor = saturatingAdd(cursor, uint64(skipped))
	}
	owned := append([]uint8(nil), value...)
	j.bytes += len(owned)
	if last := len(j.chunks) - 1; last >= 0 &&
		j.chunks[last].Kind == stream &&
		saturatingAdd(j.chunks[last].Cursor, uint64(len(j.chunks[last].Bytes))) == cursor {
		j.chunks[last].Bytes = append(j.chunks[last].Bytes, owned...)
	} else {
		j.chunks = append(j.chunks, processRuntime.OutputChunk{Kind: stream, Cursor: cursor, Bytes: owned})
	}
	for j.bytes > j.limit {
		if len(j.chunks) == 0 {
			break
		}
		remove := j.bytes - j.limit
		front := &j.chunks[0]
		if remove >= len(front.Bytes) {
			j.bytes -= len(front.Bytes)
			j.chunks = j.chunks[1:]
			if len(j.chunks) == 0 {
				j.chunks = nil
			}
			continue
		}
		front.Bytes = front.Bytes[remove:]
		front.Cursor = saturatingAdd(front.Cursor, uint64(remove))
		j.bytes -= remove
	}
	if len(j.chunks) > 0 {
		j.truncated = j.chunks[0].Cursor
	} else {
		j.truncated = j.next
	}
}

// read returns at most max bytes starting at cursor, the cursor after the
// returned bytes, and whether newer bytes remain.
func (j *journal) read(cursor uint64, max uint64) ([]processRuntime.OutputChunk, uint64, bool) {
	if cursor < j.truncated {
		cursor = j.truncated
	}
	budget := max
	var result []processRuntime.OutputChunk
	for _, chunk := range j.chunks {
		end := saturatingAdd(chunk.Cursor, uint64(len(chunk.Bytes)))
		if end <= cursor {
			continue
		}
		if cursor < chunk.Cursor {
			break
		}
		offset := int(cursor - chunk.Cursor)
		if offset >= len(chunk.Bytes) {
			break
		}
		count := uint64(len(chunk.Bytes) - offset)
		if count > budget {
			count = budget
		}
		if count == 0 {
			break
		}
		result = append(result, processRuntime.OutputChunk{
			Kind:   chunk.Kind,
			Cursor: cursor,
			Bytes:  append([]uint8(nil), chunk.Bytes[offset:offset+int(count)]...),
		})
		cursor = saturatingAdd(cursor, count)
		budget -= count
		if budget == 0 {
			break
		}
	}
	return result, cursor, cursor < j.next
}

func (j *journal) truncatedBefore() uint64 {
	return j.truncated
}
