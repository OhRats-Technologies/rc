package events

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"

	storageTypes "wit_component/ohrats_rc_storage_types"
)

// Policy reports the configured retention maximum.
func Policy() (uint32, error) {
	value, present, err := get(MetaBucket, []byte(policyKey))
	if err != nil {
		return 0, err
	}
	maximum, err := readU64(value, present)
	if err != nil {
		return 0, err
	}
	if maximum == 0 {
		return DefaultMax, nil
	}
	if maximum > math.MaxUint32 {
		return 0, errors.New("out of range integral type conversion attempted")
	}
	return uint32(maximum), nil
}

// Configure applies a retention maximum, pruning in bounded batches.
func Configure(maximum uint32) error {
	if maximum == 0 || maximum > MaxRetention {
		return errors.New("retention maximum is outside supported bounds")
	}
	for {
		complete := false
		err := transact(func(uint64) ([]storageTypes.Change, error) {
			entries, err := scan(EventsBucket)
			if err != nil {
				return nil, err
			}
			excess := len(entries) - int(maximum)
			if excess < 0 {
				excess = 0
			}
			prune := excess
			if prune > pruneBatch {
				prune = pruneBatch
			}
			pruned := make([][]byte, 0, prune)
			changes := make([]storageTypes.Change, 0, prune+1)
			for _, entry := range entries[:prune] {
				pruned = append(pruned, entry.Key)
				changes = append(changes, delete(EventsBucket, entry.Key))
			}
			extra, err := pruneIdempotency(pruned)
			if err != nil {
				return nil, err
			}
			changes = append(changes, extra...)
			complete = excess == 0
			if complete {
				changes = append(changes, put(MetaBucket, []byte(policyKey), be64(uint64(maximum))))
			}
			return changes, nil
		})
		if err != nil {
			return err
		}
		if complete {
			return nil
		}
	}
}

func pruneIdempotency(prunedCursors [][]byte) ([]storageTypes.Change, error) {
	if len(prunedCursors) == 0 {
		return nil, nil
	}
	cursors := make(map[uint64]struct{}, len(prunedCursors))
	for _, key := range prunedCursors {
		if len(key) != 8 {
			return nil, errors.New("invalid stored event cursor key")
		}
		cursors[binary.BigEndian.Uint64(key)] = struct{}{}
	}
	entries, err := scan(IdempotencyBucket)
	if err != nil {
		return nil, err
	}
	changes := make([]storageTypes.Change, 0, len(entries))
	for _, entry := range entries {
		stored, err := decodeIdempotency(entry.Value)
		if err != nil {
			return nil, fmt.Errorf("invalid stored event idempotency metadata: %v", err)
		}
		if _, prune := cursors[stored.Cursor]; prune {
			changes = append(changes, delete(IdempotencyBucket, entry.Key))
		}
	}
	return changes, nil
}
