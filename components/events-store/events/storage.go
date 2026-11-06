package events

import (
	"encoding/binary"
	"errors"

	witTypes "go.bytecodealliance.org/pkg/wit/types"
	"wit_component/ohrats_rc_storage_durable_store"
	storageTypes "wit_component/ohrats_rc_storage_types"
)

const (
	// EventsBucket stores encoded lifecycle events keyed by big-endian cursor.
	EventsBucket = "lifecycle-events"
	// IdempotencyBucket stores idempotency records keyed by request key digest.
	IdempotencyBucket = "event-idempotency"
	// MetaBucket stores the next cursor and the retention policy.
	MetaBucket = "event-meta"

	retries = 12
)

func get(bucket string, key []byte) ([]byte, bool, error) {
	result := ohrats_rc_storage_durable_store.Get(bucket, key)
	if result.IsErr() {
		return nil, false, errors.New(result.Err())
	}
	option := result.Ok()
	if option.IsNone() {
		return nil, false, nil
	}
	return option.Some(), true, nil
}

func scan(bucket string) ([]storageTypes.Entry, error) {
	for attempt := 0; attempt < retries; attempt++ {
		expected, err := revision()
		if err != nil {
			return nil, err
		}
		entries := []storageTypes.Entry{}
		var after []byte
		for {
			page := ohrats_rc_storage_durable_store.Scan(bucket, []byte{}, optionBytes(after), 1000)
			if page.IsErr() {
				return nil, errors.New(page.Err())
			}
			current := page.Ok()
			if current.Revision != expected {
				break
			}
			entries = append(entries, current.Entries...)
			if current.NextKey.IsSome() {
				after = current.NextKey.Some()
				continue
			}
			return entries, nil
		}
	}
	return nil, errors.New("event state changed too frequently")
}

func revision() (uint64, error) {
	result := ohrats_rc_storage_durable_store.Revision()
	if result.IsErr() {
		return 0, errors.New(result.Err())
	}
	return result.Ok(), nil
}

func transact(build func(uint64) ([]storageTypes.Change, error)) error {
	for attempt := 0; attempt < retries; attempt++ {
		expected, err := revision()
		if err != nil {
			return err
		}
		changes, err := build(expected)
		if err != nil {
			return err
		}
		result := ohrats_rc_storage_durable_store.Commit(expected, changes)
		if !result.IsErr() {
			return nil
		}
		failure := result.Err()
		if failure.Tag() == storageTypes.CommitError_Conflict {
			continue
		}
		return errors.New(failure.Failure())
	}
	return errors.New("event state changed too frequently")
}

func put(bucket string, key []byte, value []byte) storageTypes.Change {
	return storageTypes.MakeChangePut(storageTypes.Entry{Bucket: bucket, Key: key, Value: value})
}

func delete(bucket string, key []byte) storageTypes.Change {
	return storageTypes.MakeChangeDelete(storageTypes.Deletion{Bucket: bucket, Key: key})
}

func cursorKey(cursor uint64) []byte {
	key := make([]byte, 8)
	binary.BigEndian.PutUint64(key, cursor)
	return key
}

func readU64(value []byte, present bool) (uint64, error) {
	if !present {
		return 0, nil
	}
	if len(value) != 8 {
		return 0, errors.New("invalid event metadata")
	}
	return binary.BigEndian.Uint64(value), nil
}

func optionBytes(value []byte) witTypes.Option[[]byte] {
	if value == nil {
		return witTypes.None[[]byte]()
	}
	return witTypes.Some(value)
}
