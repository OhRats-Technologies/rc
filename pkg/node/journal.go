package node

import (
	"io"
	"sync"
)

const defaultMaxJournalBytes = 256 * 1024 // 256 KiB rolling buffer

type processJournal struct {
	mu              sync.RWMutex
	chunks          []JournalChunk
	cursor          uint64
	truncatedBefore uint64
	totalBytes      int
}

func newProcessJournal() *processJournal {
	return &processJournal{}
}

func (j *processJournal) appendChunk(kind StreamKind, data []byte) {
	j.mu.Lock()
	defer j.mu.Unlock()

	j.cursor += uint64(len(data))
	chunk := JournalChunk{
		Stream: kind,
		Cursor: j.cursor,
		Bytes:  append([]byte(nil), data...),
	}
	j.chunks = append(j.chunks, chunk)
	j.totalBytes += len(data)

	// Rolling buffer truncation
	for j.totalBytes > defaultMaxJournalBytes && len(j.chunks) > 1 {
		j.totalBytes -= len(j.chunks[0].Bytes)
		j.truncatedBefore = j.chunks[0].Cursor
		j.chunks = j.chunks[1:]
	}
}

func (j *processJournal) streamReader(r io.Reader, kind StreamKind) {
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			j.appendChunk(kind, buf[:n])
		}
		if err != nil {
			break
		}
	}
}

func (j *processJournal) read(cursor uint64, maxBytes int, status string, exitCode *int32, signal string) *ExecutionRead {
	j.mu.RLock()
	defer j.mu.RUnlock()

	var returnedChunks []JournalChunk
	bytesAccum := 0
	for _, chunk := range j.chunks {
		if chunk.Cursor > cursor {
			returnedChunks = append(returnedChunks, chunk)
			bytesAccum += len(chunk.Bytes)
			if maxBytes > 0 && bytesAccum >= maxBytes {
				break
			}
		}
	}

	return &ExecutionRead{
		Status:          status,
		Chunks:          returnedChunks,
		NextCursor:      j.cursor,
		TruncatedBefore: j.truncatedBefore,
		More:            status == "running" || j.cursor > cursor+uint64(bytesAccum),
		ExitCode:        exitCode,
		Signal:          signal,
	}
}
