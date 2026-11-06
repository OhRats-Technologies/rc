package schedule

import (
	"encoding/json"
	"errors"
	"sort"

	witTypes "go.bytecodealliance.org/pkg/wit/types"
	types "wit_component/ohrats_rc_scheduler_types"
	durableStore "wit_component/ohrats_rc_storage_durable_store"
	storageTypes "wit_component/ohrats_rc_storage_types"
)

const (
	scheduleBucket = "schedules"
	cursorBucket   = "schedule-cursors"
	storageRetries = 12
	storagePage    = 1000
)

// List returns every stored definition sorted by id, reading all pages at one
// store revision and retrying when the revision moves.
func List() ([]types.Definition, error) {
	for attempt := 0; attempt < storageRetries; attempt++ {
		revision, err := currentRevision()
		if err != nil {
			return nil, err
		}
		var after witTypes.Option[[]byte] = witTypes.None[[]byte]()
		result := make([]types.Definition, 0)
		for {
			pageResult := durableStore.Scan(scheduleBucket, []byte{}, after, storagePage)
			if pageResult.IsErr() {
				return nil, errors.New(pageResult.Err())
			}
			page := pageResult.Ok()
			if page.Revision != revision {
				break
			}
			for _, entry := range page.Entries {
				definition, err := decodeDefinition(entry.Value)
				if err != nil {
					return nil, err
				}
				result = append(result, definition)
			}
			if page.NextKey.IsNone() {
				sort.Slice(result, func(left, right int) bool {
					return result[left].Id < result[right].Id
				})
				return result, nil
			}
			after = page.NextKey
		}
	}
	return nil, errors.New("schedule state changed too frequently")
}

// Get returns one definition by id.
func Get(id string) (types.Definition, bool, error) {
	result := durableStore.Get(scheduleBucket, []byte(id))
	if result.IsErr() {
		return types.Definition{}, false, errors.New(result.Err())
	}
	stored := result.Ok()
	if stored.IsNone() {
		return types.Definition{}, false, nil
	}
	definition, err := decodeDefinition(stored.Some())
	if err != nil {
		return types.Definition{}, false, err
	}
	return definition, true, nil
}

// Put stores one definition.
func Put(value types.Definition) error {
	encoded, err := encodeDefinition(value)
	if err != nil {
		return err
	}
	key := []byte(value.Id)
	return commit(func(uint64) []storageTypes.Change {
		return []storageTypes.Change{putChange(scheduleBucket, key, encoded)}
	})
}

// Remove deletes a definition and its cursor. It reports whether a definition
// existed. Existence is checked on the raw stored bytes so corrupt values can
// still be removed.
func Remove(id string) (bool, error) {
	existing := durableStore.Get(scheduleBucket, []byte(id))
	if existing.IsErr() {
		return false, errors.New(existing.Err())
	}
	if existing.Ok().IsNone() {
		return false, nil
	}
	key := []byte(id)
	err := commit(func(uint64) []storageTypes.Change {
		return []storageTypes.Change{
			deleteChange(scheduleBucket, key),
			deleteChange(cursorBucket, key),
		}
	})
	if err != nil {
		return false, err
	}
	return true, nil
}

func cursor(id string) (*Cursor, error) {
	result := durableStore.Get(cursorBucket, []byte(id))
	if result.IsErr() {
		return nil, errors.New(result.Err())
	}
	stored := result.Ok()
	if stored.IsNone() {
		return nil, nil
	}
	var value storedCursor
	if err := json.Unmarshal(stored.Some(), &value); err != nil {
		return nil, err
	}
	return &Cursor{
		CheckedAtMs:      value.CheckedAtMs,
		LastOccurrenceID: value.LastOccurrenceID,
	}, nil
}

func putCursor(id string, value Cursor) error {
	encoded, err := json.Marshal(storedCursor{
		CheckedAtMs:      value.CheckedAtMs,
		LastOccurrenceID: value.LastOccurrenceID,
	})
	if err != nil {
		return err
	}
	key := []byte(id)
	return commit(func(uint64) []storageTypes.Change {
		return []storageTypes.Change{putChange(cursorBucket, key, encoded)}
	})
}

func decodeDefinition(data []byte) (types.Definition, error) {
	var stored storedDefinition
	if err := json.Unmarshal(data, &stored); err != nil {
		return types.Definition{}, err
	}
	return definitionFromStored(stored)
}

func encodeDefinition(value types.Definition) ([]byte, error) {
	return json.Marshal(definitionFromValue(value))
}

func currentRevision() (uint64, error) {
	result := durableStore.Revision()
	if result.IsErr() {
		return 0, errors.New(result.Err())
	}
	return result.Ok(), nil
}

// commit retries a read-modify-commit cycle while the store reports a revision
// conflict.
func commit(changes func(uint64) []storageTypes.Change) error {
	for attempt := 0; attempt < storageRetries; attempt++ {
		revision, err := currentRevision()
		if err != nil {
			return err
		}
		result := durableStore.Commit(revision, changes(revision))
		if result.IsErr() {
			failure := result.Err()
			if failure.Tag() == storageTypes.CommitError_Conflict {
				continue
			}
			return errors.New(failure.Failure())
		}
		return nil
	}
	return errors.New("schedule state changed too frequently")
}

func putChange(bucket string, key, value []byte) storageTypes.Change {
	return storageTypes.MakeChangePut(storageTypes.Entry{
		Bucket: bucket,
		Key:    key,
		Value:  value,
	})
}

func deleteChange(bucket string, key []byte) storageTypes.Change {
	return storageTypes.MakeChangeDelete(storageTypes.Deletion{
		Bucket: bucket,
		Key:    key,
	})
}
