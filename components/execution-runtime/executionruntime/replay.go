package executionruntime

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"

	storageStore "wit_component/ohrats_rc_storage_durable_store"
	storageTypes "wit_component/ohrats_rc_storage_types"
)

// The replay filter is a fixed-size, component-owned bitset in the durable
// store. Each execution id owns five independent bits. A claim fails closed: a
// false positive may reject an id, but a recorded id can never be forgotten.
const (
	replayBucket     = "execution-replay-v1"
	replayChunkBytes = 4_096
	replayChunks     = 256
	replayHashes     = 5
	replayRetries    = 12
)

// position is one (chunk, bit) cell of the replay filter.
type position struct {
	chunk int
	bit   int
}

// claimReplay records id in the durable replay filter. It commits
// optimistically against the observed store revision and retries a bounded
// number of conflicts. No execution content is stored.
func claimReplay(id string) error {
	if id == "" {
		return errors.New("execution id is empty")
	}
	cells := replayPositions(id)
	for attempt := 0; attempt < replayRetries; attempt++ {
		revision, err := replayRevision()
		if err != nil {
			return err
		}
		order, chunks, err := loadReplayChunks(cells)
		if err != nil {
			return err
		}
		claimed := true
		for _, cell := range cells {
			if chunks[cell.chunk][cell.bit/8]&(1<<(cell.bit%8)) == 0 {
				claimed = false
				break
			}
		}
		if claimed {
			return fmt.Errorf("execution id %q was already claimed", id)
		}
		for _, cell := range cells {
			chunks[cell.chunk][cell.bit/8] |= 1 << (cell.bit % 8)
		}
		changes := make([]storageTypes.Change, 0, len(order))
		for _, chunk := range order {
			changes = append(changes, storageTypes.MakeChangePut(storageTypes.Entry{
				Bucket: replayBucket,
				Key:    chunkKey(chunk),
				Value:  chunks[chunk],
			}))
		}
		committed := storageStore.Commit(revision, changes)
		if committed.IsOk() {
			return nil
		}
		failure := committed.Err()
		if failure.Tag() == storageTypes.CommitError_Conflict {
			continue
		}
		return errors.New(failure.Failure())
	}
	return errors.New("execution replay state changed too frequently")
}

func replayRevision() (uint64, error) {
	revision := storageStore.Revision()
	if revision.IsErr() {
		return 0, errors.New(revision.Err())
	}
	return revision.Ok(), nil
}

// loadReplayChunks reads every touched chunk in ascending key order and
// returns the chunk order together with the loaded values. A missing chunk is
// zero-filled; a chunk of the wrong size is corrupt state.
func loadReplayChunks(cells []position) ([]int, map[int][]uint8, error) {
	chunks := make(map[int][]uint8, len(cells))
	order := make([]int, 0, len(cells))
	for _, cell := range cells {
		if _, ok := chunks[cell.chunk]; ok {
			continue
		}
		stored := storageStore.Get(replayBucket, chunkKey(cell.chunk))
		if stored.IsErr() {
			return nil, nil, errors.New(stored.Err())
		}
		var value []uint8
		if stored.Ok().IsSome() {
			value = append([]uint8(nil), stored.Ok().Some()...)
		} else {
			value = make([]uint8, replayChunkBytes)
		}
		if len(value) != replayChunkBytes {
			return nil, nil, errors.New("execution replay state is corrupt")
		}
		chunks[cell.chunk] = value
		order = append(order, cell.chunk)
	}
	sort.Ints(order)
	return order, chunks, nil
}

// replayPositions maps an id to its five domain-separated bits.
func replayPositions(id string) []position {
	const totalBits = replayChunks * replayChunkBytes * 8
	cells := make([]position, 0, replayHashes)
	for domain := 0; domain < replayHashes; domain++ {
		digest := sha256.Sum256(append([]byte(id), byte(domain)))
		index := binary.BigEndian.Uint64(digest[:8]) % totalBits
		cells = append(cells, position{
			chunk: int(index / (replayChunkBytes * 8)),
			bit:   int(index % (replayChunkBytes * 8)),
		})
	}
	return cells
}

func chunkKey(chunk int) []uint8 {
	var key [2]byte
	binary.BigEndian.PutUint16(key[:], uint16(chunk))
	return key[:]
}
